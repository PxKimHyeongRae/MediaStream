package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/pion/rtp"
	"github.com/yourusername/cctv3/internal/api"
	"github.com/yourusername/cctv3/internal/core"
	"github.com/yourusername/cctv3/internal/rtsp"
	"github.com/yourusername/cctv3/internal/signaling"
	"github.com/yourusername/cctv3/internal/webrtc"
	"github.com/yourusername/cctv3/pkg/logger"
	"go.uber.org/zap"
)

const (
	defaultConfigPath = "configs/config.yaml"
	version           = "0.1.0"
)

func main() {
	// 커맨드라인 플래그 파싱
	configPath := flag.String("config", defaultConfigPath, "설정 파일 경로")
	showVersion := flag.Bool("version", false, "버전 정보 출력")
	flag.Parse()

	// 버전 정보 출력
	if *showVersion {
		fmt.Printf("RTSP to WebRTC Media Server v%s\n", version)
		fmt.Printf("Go version: %s\n", runtime.Version())
		fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	// 설정 로드
	config, err := core.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 로거 초기화
	if err := logger.InitLogger(logger.LogConfig{
		Level:      config.Logging.Level,
		Output:     config.Logging.Output,
		FilePath:   config.Logging.FilePath,
		MaxSize:    config.Logging.MaxSize,
		MaxBackups: config.Logging.MaxBackups,
		MaxAge:     config.Logging.MaxAge,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// 시작 로그
	logger.Info("Starting RTSP to WebRTC Media Server",
		zap.String("version", version),
		zap.String("go_version", runtime.Version()),
		zap.Int("num_cpu", runtime.NumCPU()),
		zap.Int("gomaxprocs", runtime.GOMAXPROCS(0)),
	)

	// GC 설정 조정
	if config.Performance.GCPercent > 0 {
		oldPercent := debug.SetGCPercent(config.Performance.GCPercent)
		logger.Info("GC percent adjusted",
			zap.Int("old", oldPercent),
			zap.Int("new", config.Performance.GCPercent),
		)
	}

	// 설정 정보 출력
	logger.Info("Server configuration",
		zap.Int("http_port", config.Server.HTTPPort),
		zap.Int("ws_port", config.Server.WSPort),
		zap.Bool("production", config.Server.Production),
		zap.Int("max_streams", config.RTSP.Pool.MaxStreams),
		zap.Int("max_peers", config.WebRTC.Settings.MaxPeers),
	)

	// 서버 컴포넌트 초기화
	app, err := initializeApplication(config)
	if err != nil {
		logger.Fatal("Failed to initialize application", zap.Error(err))
	}
	defer app.cleanup()

	logger.Info("All components initialized successfully")

	// Paths 설정에서 스트림 로드
	if err := app.loadStreamsFromConfig(config); err != nil {
		logger.Error("Failed to load streams from config", zap.Error(err))
	}

	// 종료 시그널 대기
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	logger.Info("Server is running. Press Ctrl+C to stop.")

	// 시그널 대기
	sig := <-sigChan
	logger.Info("Received shutdown signal",
		zap.String("signal", sig.String()),
	)

	// TODO: 그레이스풀 셧다운
	// 1. 새로운 연결 거부
	// 2. 기존 연결 정리
	// 3. 리소스 해제

	logger.Info("Server stopped gracefully")
}

// maskRTSPURL은 RTSP URL의 비밀번호를 마스킹합니다
func maskRTSPURL(url string) string {
	// rtsp://user:pass@host:port/path 형식에서 pass 부분 마스킹
	// 간단한 구현으로 전체 credential 마스킹
	return "rtsp://***:***@<masked>"
}

// Application은 애플리케이션 컴포넌트들을 관리합니다
type Application struct {
	config          *core.Config
	streamManager   *core.StreamManager
	webrtcManager   *webrtc.Manager
	signalingServer *signaling.Server
	apiServer       *api.Server
	rtspClients     map[string]*rtsp.Client // streamID -> RTSP client

	// 피어와 스트림 매핑
	peerStreams map[string]string // peerID -> streamID
	peerMutex   sync.RWMutex
}

// initializeApplication은 애플리케이션을 초기화합니다
func initializeApplication(config *core.Config) (*Application, error) {
	app := &Application{
		config:      config,
		rtspClients: make(map[string]*rtsp.Client),
		peerStreams: make(map[string]string),
	}

	// 1. 스트림 관리자 초기화
	app.streamManager = core.NewStreamManager(logger.Log)
	logger.Info("Stream manager initialized")

	// 2. WebRTC 관리자 초기화
	app.webrtcManager = webrtc.NewManager(webrtc.ManagerConfig{
		Logger:   logger.Log,
		MaxPeers: config.WebRTC.Settings.MaxPeers,
		OnPeerClosed: func(peerID string) {
			app.cleanupPeer(peerID)
		},
	})
	logger.Info("WebRTC manager initialized")

	// 3. 시그널링 서버 초기화
	app.signalingServer = signaling.NewServer(signaling.ServerConfig{
		Logger: logger.Log,
		OnOffer: func(offer string, streamID string, client *signaling.Client) (string, error) {
			return app.handleWebRTCOffer(offer, streamID, client)
		},
		OnClose: func(clientID string) {
			logger.Info("Client disconnected",
				zap.String("client_id", clientID),
			)
			// 클라이언트와 연결된 피어들을 정리
			app.cleanupClientPeers(clientID)
		},
	})
	logger.Info("Signaling server initialized")

	// 4. API 서버 초기화
	app.apiServer = api.NewServer(api.ServerConfig{
		Port:       config.Server.HTTPPort,
		Production: config.Server.Production,
		Logger:     logger.Log,
		HealthHandler: func() map[string]interface{} {
			return map[string]interface{}{
				"status":    "ok",
				"version":   version,
				"streams":   len(app.rtspClients),
				"clients":   app.signalingServer.GetClientCount(),
				"peers":     app.webrtcManager.GetPeerCount(),
			}
		},
		StreamsHandler: func() []map[string]interface{} {
			streams := []map[string]interface{}{}

			// config에서 모든 paths를 순회
			for streamID, pathConfig := range app.config.Paths {
				stream, err := app.streamManager.GetStream(streamID)

				streamInfo := map[string]interface{}{
					"id":        streamID,
					"source":    maskRTSPURL(pathConfig.Source),
					"onDemand":  pathConfig.SourceOnDemand,
				}

				// RTSP 클라이언트가 실행 중인지 확인
				if _, isRunning := app.rtspClients[streamID]; isRunning {
					streamInfo["status"] = "running"
					if err == nil {
						streamInfo["codec"] = stream.GetVideoCodec()
						streamInfo["subscribers"] = stream.GetSubscriberCount()
					}
				} else {
					streamInfo["status"] = "stopped"
					streamInfo["codec"] = nil
					streamInfo["subscribers"] = 0
				}

				streams = append(streams, streamInfo)
			}
			return streams
		},
		StreamInfoHandler: func(streamID string) (map[string]interface{}, error) {
			// 설정에서 path 찾기
			pathConfig, exists := app.config.Paths[streamID]
			if !exists {
				return nil, fmt.Errorf("stream %s not found", streamID)
			}

			stream, err := app.streamManager.GetStream(streamID)
			if err != nil {
				return nil, err
			}

			info := map[string]interface{}{
				"id":        streamID,
				"source":    maskRTSPURL(pathConfig.Source),
				"onDemand":  pathConfig.SourceOnDemand,
			}

			// RTSP 클라이언트 상태 확인
			if _, isRunning := app.rtspClients[streamID]; isRunning {
				info["status"] = "running"
				info["codec"] = stream.GetVideoCodec()
				info["subscribers"] = stream.GetSubscriberCount()
			} else {
				info["status"] = "stopped"
				info["codec"] = nil
				info["subscribers"] = 0
			}

			return info, nil
		},
		StartStreamHandler: func(streamID string) error {
			return app.startOnDemandStream(streamID)
		},
		StopStreamHandler: func(streamID string) error {
			return app.stopStream(streamID)
		},
		WebSocketHandler: app.signalingServer.HandleWebSocket,
	})

	// API 서버 시작
	if err := app.apiServer.Start(); err != nil {
		return nil, fmt.Errorf("failed to start API server: %w", err)
	}
	logger.Info("API server started")

	return app, nil
}

// addStream은 새로운 RTSP 스트림을 추가합니다
func (app *Application) addStream(streamID, rtspURL string) error {
	// 스트림 생성
	stream, err := app.streamManager.CreateStream(streamID, streamID)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	// RTSP 클라이언트 생성
	client, err := rtsp.NewClient(rtsp.ClientConfig{
		URL:          rtspURL,
		Transport:    "tcp",
		Timeout:      time.Duration(app.config.RTSP.Client.Timeout) * time.Second,
		RetryCount:   app.config.RTSP.Client.RetryCount,
		RetryDelay:   time.Duration(app.config.RTSP.Client.RetryDelay) * time.Second,
		Logger:       logger.Log,
		Stream:       stream, // Stream 참조 전달 (코덱 설정용)
		OnPacket: func(pkt *rtp.Packet) {
			// RTP 패킷을 스트림에 전달
			if err := stream.WritePacket(pkt); err != nil {
				logger.Error("Failed to write packet to stream",
					zap.String("stream_id", streamID),
					zap.Error(err),
				)
			}
		},
		OnConnect: func() {
			logger.Info("RTSP client connected", zap.String("stream_id", streamID))
		},
		OnDisconnect: func(err error) {
			logger.Warn("RTSP client disconnected",
				zap.String("stream_id", streamID),
				zap.Error(err),
			)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create RTSP client: %w", err)
	}

	// RTSP 클라이언트 시작
	if err := client.Start(); err != nil {
		return fmt.Errorf("failed to start RTSP client: %w", err)
	}

	// map에 저장
	app.rtspClients[streamID] = client

	return nil
}

// cleanup은 애플리케이션 리소스를 정리합니다
func (app *Application) cleanup() {
	logger.Info("Cleaning up application resources")

	for streamID, client := range app.rtspClients {
		logger.Info("Stopping RTSP client", zap.String("stream_id", streamID))
		client.Stop()
	}

	if app.apiServer != nil {
		app.apiServer.Stop()
	}

	if app.signalingServer != nil {
		app.signalingServer.Close()
	}

	if app.webrtcManager != nil {
		app.webrtcManager.Close()
	}

	if app.streamManager != nil {
		app.streamManager.Close()
	}

	logger.Info("Cleanup completed")
}

// handleWebRTCOffer는 클라이언트의 WebRTC Offer를 처리합니다
func (app *Application) handleWebRTCOffer(offer string, streamID string, client *signaling.Client) (string, error) {
	logger.Info("Handling WebRTC offer",
		zap.String("client_id", client.GetID()),
		zap.String("stream_id", streamID),
	)

	// 요청한 스트림 가져오기
	stream, err := app.streamManager.GetStream(streamID)
	if err != nil {
		logger.Error("Failed to get stream",
			zap.String("stream_id", streamID),
			zap.Error(err),
		)
		return "", fmt.Errorf("stream not found: %w", err)
	}

	// WebRTC 피어 생성
	peer, err := app.webrtcManager.CreatePeer(streamID)
	if err != nil {
		logger.Error("Failed to create peer", zap.Error(err))
		return "", fmt.Errorf("failed to create peer: %w", err)
	}

	logger.Info("WebRTC peer created",
		zap.String("peer_id", peer.GetID()),
		zap.String("stream_id", streamID),
	)

	// Offer 처리 및 Answer 생성
	// Stream의 실제 비디오 코덱을 확인하여 전달
	streamCodec := stream.GetVideoCodec()
	answer, err := peer.CreateOffer(offer, streamCodec)
	if err != nil {
		logger.Error("Failed to create answer", zap.Error(err))
		app.webrtcManager.RemovePeer(peer.GetID())
		return "", fmt.Errorf("failed to create answer: %w", err)
	}

	// 피어를 스트림 구독자로 등록
	if err := stream.Subscribe(peer); err != nil {
		logger.Error("Failed to subscribe peer to stream", zap.Error(err))
		app.webrtcManager.RemovePeer(peer.GetID())
		return "", fmt.Errorf("failed to subscribe: %w", err)
	}

	// 피어-스트림 매핑 저장
	app.peerMutex.Lock()
	app.peerStreams[peer.GetID()] = streamID
	app.peerMutex.Unlock()

	logger.Info("Peer subscribed to stream",
		zap.String("peer_id", peer.GetID()),
		zap.String("stream_id", streamID),
		zap.Int("total_subscribers", stream.GetSubscriberCount()),
	)

	return answer, nil
}

// cleanupPeer는 피어와 관련된 리소스를 정리합니다
func (app *Application) cleanupPeer(peerID string) {
	app.peerMutex.Lock()
	streamID, exists := app.peerStreams[peerID]
	if exists {
		delete(app.peerStreams, peerID)
	}
	app.peerMutex.Unlock()

	if !exists {
		return
	}

	// 스트림에서 구독 해제
	stream, err := app.streamManager.GetStream(streamID)
	if err != nil {
		logger.Error("Failed to get stream for cleanup",
			zap.String("peer_id", peerID),
			zap.String("stream_id", streamID),
			zap.Error(err),
		)
		return
	}

	if err := stream.Unsubscribe(peerID); err != nil {
		logger.Error("Failed to unsubscribe peer from stream",
			zap.String("peer_id", peerID),
			zap.String("stream_id", streamID),
			zap.Error(err),
		)
	} else {
		logger.Info("Peer unsubscribed from stream",
			zap.String("peer_id", peerID),
			zap.String("stream_id", streamID),
		)
	}
}

// cleanupClientPeers는 클라이언트와 관련된 피어들을 정리합니다 (현재 사용되지 않음)
func (app *Application) cleanupClientPeers(clientID string) {
	// 현재는 WebRTC 피어 연결 상태 변화에서 자동으로 정리됨
}

// loadStreamsFromConfig는 설정 파일에서 paths를 읽어 스트림을 로드합니다
func (app *Application) loadStreamsFromConfig(config *core.Config) error {
	logger.Info("Loading streams from config", zap.Int("path_count", len(config.Paths)))

	for streamID, pathConfig := range config.Paths {
		logger.Info("Processing path",
			zap.String("stream_id", streamID),
			zap.String("source", maskRTSPURL(pathConfig.Source)),
			zap.Bool("on_demand", pathConfig.SourceOnDemand),
		)

		// sourceOnDemand가 false인 경우에만 즉시 연결
		if !pathConfig.SourceOnDemand {
			if err := app.addStream(streamID, pathConfig.Source); err != nil {
				logger.Error("Failed to add stream",
					zap.String("stream_id", streamID),
					zap.Error(err),
				)
				continue
			}
			logger.Info("Stream started (always-on)",
				zap.String("stream_id", streamID),
			)
		} else {
			// 온디맨드 스트림은 스트림만 생성하고 RTSP 연결은 하지 않음
			if _, err := app.streamManager.CreateStream(streamID, streamID); err != nil {
				logger.Error("Failed to create on-demand stream",
					zap.String("stream_id", streamID),
					zap.Error(err),
				)
				continue
			}
			logger.Info("Stream created (on-demand)",
				zap.String("stream_id", streamID),
			)
		}
	}

	return nil
}

// startOnDemandStream은 온디맨드 스트림을 시작합니다
func (app *Application) startOnDemandStream(streamID string) error {
	// 설정에서 path 찾기
	pathConfig, exists := app.config.Paths[streamID]
	if !exists {
		return fmt.Errorf("stream %s not found in config", streamID)
	}

	// 이미 RTSP 클라이언트가 있는지 확인
	if _, exists := app.rtspClients[streamID]; exists {
		logger.Info("Stream already running", zap.String("stream_id", streamID))
		return nil
	}

	// 스트림이 이미 존재하는지 확인
	stream, err := app.streamManager.GetStream(streamID)
	if err != nil {
		return fmt.Errorf("stream not found: %w", err)
	}

	// RTSP 클라이언트만 생성하고 시작
	client, err := rtsp.NewClient(rtsp.ClientConfig{
		URL:          pathConfig.Source,
		Transport:    pathConfig.RTSPTransport,
		Timeout:      time.Duration(app.config.RTSP.Client.Timeout) * time.Second,
		RetryCount:   app.config.RTSP.Client.RetryCount,
		RetryDelay:   time.Duration(app.config.RTSP.Client.RetryDelay) * time.Second,
		Logger:       logger.Log,
		Stream:       stream,
		OnPacket: func(pkt *rtp.Packet) {
			if err := stream.WritePacket(pkt); err != nil {
				logger.Error("Failed to write packet to stream",
					zap.String("stream_id", streamID),
					zap.Error(err),
				)
			}
		},
		OnConnect: func() {
			logger.Info("RTSP client connected", zap.String("stream_id", streamID))
		},
		OnDisconnect: func(err error) {
			logger.Warn("RTSP client disconnected",
				zap.String("stream_id", streamID),
				zap.Error(err),
			)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create RTSP client: %w", err)
	}

	// RTSP 클라이언트 시작
	if err := client.Start(); err != nil {
		return fmt.Errorf("failed to start RTSP client: %w", err)
	}

	// map에 저장
	app.rtspClients[streamID] = client

	logger.Info("On-demand stream started", zap.String("stream_id", streamID))
	return nil
}

// stopStream은 스트림을 중지합니다
func (app *Application) stopStream(streamID string) error {
	client, exists := app.rtspClients[streamID]
	if !exists {
		return fmt.Errorf("stream %s not running", streamID)
	}

	logger.Info("Stopping stream", zap.String("stream_id", streamID))
	client.Stop()
	delete(app.rtspClients, streamID)

	return nil
}
