package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pion/rtp"
	"github.com/yourusername/cctv3/internal/api"
	"github.com/yourusername/cctv3/internal/cctv"
	"github.com/yourusername/cctv3/internal/core"
	"github.com/yourusername/cctv3/internal/process"
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
	defer logger.Close()

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
		zap.Bool("api_enabled", config.API.Enabled),
		zap.String("api_url", config.API.BaseURL),
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

	// API 기반 CCTV 스트림 로드
	if config.API.Enabled {
		if err := app.cctvManager.Start(); err != nil {
			logger.Error("Failed to start CCTV manager", zap.Error(err))
		} else {
			// CCTV 데이터로 스트림 로드
			if err := app.loadStreamsFromCCTV(); err != nil {
				logger.Error("Failed to load streams from CCTV", zap.Error(err))
			}
		}
	} else {
		logger.Warn("API is disabled, no streams will be loaded")
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
func maskRTSPURL(urlStr string) string {
	// rtsp://user:pass@host:port/path 형식에서 pass 부분만 마스킹
	// URL 파싱을 시도하여 credential만 마스킹
	if len(urlStr) < 8 {
		return "***"
	}

	// rtsp:// 또는 http:// 프로토콜 찾기
	protocolEnd := 0
	if idx := strings.Index(urlStr, "://"); idx != -1 {
		protocolEnd = idx + 3
	} else {
		return "***"
	}

	// @ 기호 찾기 (credential이 있는 경우)
	atIdx := strings.Index(urlStr[protocolEnd:], "@")
	if atIdx == -1 {
		// credential이 없는 경우 그대로 반환
		return urlStr
	}

	// credential 부분 파싱
	credentials := urlStr[protocolEnd : protocolEnd+atIdx]
	colonIdx := strings.Index(credentials, ":")
	if colonIdx == -1 {
		// 비밀번호가 없는 경우
		return urlStr
	}

	// 비밀번호 부분만 마스킹
	username := credentials[:colonIdx]
	restOfURL := urlStr[protocolEnd+atIdx:]

	return urlStr[:protocolEnd] + username + ":***" + restOfURL
}

// Application은 애플리케이션 컴포넌트들을 관리합니다
type Application struct {
	config          *core.Config
	streamManager   *core.StreamManager
	webrtcManager   *webrtc.Manager
	signalingServer *signaling.Server
	apiServer       *api.Server
	processManager  *process.Manager
	cctvManager     *cctv.CCTVManager
	rtspClients     map[string]*rtsp.Client // streamID -> RTSP client
	rtspServer      *rtsp.ServerRTSP        // RTSP 서버 (ffmpeg publish/subscribe용)

	// 피어와 스트림 매핑
	peerStreams map[string]string // peerID -> streamID
	peerMutex   sync.RWMutex

	// Context for cancellation
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// initializeApplication은 애플리케이션을 초기화합니다
func initializeApplication(config *core.Config) (*Application, error) {
	// Context 생성 (프로세스 관리용)
	ctx, cancel := context.WithCancel(context.Background())

	app := &Application{
		config:      config,
		rtspClients: make(map[string]*rtsp.Client),
		peerStreams: make(map[string]string),
		ctx:         ctx,
		cancelFunc:  cancel,
	}

	// 1. 스트림 관리자 초기화 (먼저 초기화해야 함)
	app.streamManager = core.NewStreamManager(logger.Log)
	logger.Info("Stream manager initialized")

	// 2. ProcessManager 초기화 (runOnDemand 프로세스 관리)
	app.processManager = process.NewManager(logger.Log)
	logger.Info("ProcessManager initialized")

	// 3. CCTV Manager 초기화 (외부 API 연동) - StreamManager가 필요
	if config.API.Enabled {
		app.cctvManager = cctv.NewCCTVManager(cctv.Config{
			APIURL:            config.API.BaseURL,
			Username:          config.API.Username,
			Password:          config.API.Password,
			StreamManager:     app.streamManager,
			Logger:            logger.Log,
			RequestTimeoutSec: config.API.RequestTimeoutSec,
		})
		logger.Info("CCTV manager initialized")
	}

	// 4. WebRTC 관리자 초기화
	app.webrtcManager = webrtc.NewManager(webrtc.ManagerConfig{
		Logger:   logger.Log,
		MaxPeers: config.WebRTC.Settings.MaxPeers,
		OnPeerClosed: func(peerID string) {
			app.cleanupPeer(peerID)
		},
	})
	logger.Info("WebRTC manager initialized")

	// 5. 시그널링 서버 초기화
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

	// 6. Skip PathsHandler (API 기반으로 변경)

	// 7. API 서버 초기화
	app.apiServer = api.NewServer(api.ServerConfig{
		Port:       config.Server.HTTPPort,
		Production: config.Server.Production,
		Logger:     logger.Log,
		HealthHandler: func() map[string]interface{} {
			return map[string]interface{}{
				"status":  "ok",
				"version": version,
				"streams": len(app.rtspClients),
				"clients": app.signalingServer.GetClientCount(),
				"peers":   app.webrtcManager.GetPeerCount(),
			}
		},
		StatsHandler: func() map[string]interface{} {
			stats := map[string]interface{}{
				"uptime":      "0h 0m 0s", // TODO: 실제 uptime 계산
				"streams":     len(app.rtspClients),
				"clients":     app.signalingServer.GetClientCount(),
				"peers":       app.webrtcManager.GetPeerCount(),
				"api_enabled": config.API.Enabled,
			}

			// API가 활성화된 경우 CCTV 정보 추가
			if config.API.Enabled && app.cctvManager != nil {
				cctvs := app.cctvManager.GetCCTVs()
				stats["cctvs"] = len(cctvs)

				// CCTV 목록 추가
				cctvList := make([]map[string]interface{}, 0, len(cctvs))
				for streamID, cctv := range cctvs {
					cctvInfo := map[string]interface{}{
						"id":             streamID,
						"name":           cctv.Name,
						"sourceOnDemand": cctv.SourceOnDemand,
					}

					// RTSP 클라이언트 상태 확인
					if _, isRunning := app.rtspClients[streamID]; isRunning {
						cctvInfo["status"] = "running"
						if stream, err := app.streamManager.GetStream(streamID); err == nil {
							cctvInfo["codec"] = stream.GetVideoCodec()
							cctvInfo["subscribers"] = stream.GetSubscriberCount()
						}
					} else {
						cctvInfo["status"] = "stopped"
						cctvInfo["codec"] = nil
						cctvInfo["subscribers"] = 0
					}

					cctvList = append(cctvList, cctvInfo)
				}
				stats["cctv_list"] = cctvList
			}

			return stats
		},
		WebSocketHandler: app.signalingServer.HandleWebSocket,
		CCTVManager:      app.cctvManager,
	})

	// API 서버 시작
	if err := app.apiServer.Start(); err != nil {
		return nil, fmt.Errorf("failed to start API server: %w", err)
	}
	logger.Info("API server started")

	// 8. RTSP 서버 초기화 (ffmpeg publish/subscribe용)
	if config.RTSP.Server.Enabled {
		app.rtspServer = rtsp.NewServerRTSP(rtsp.ServerRTSPConfig{
			Address:       fmt.Sprintf(":%d", config.RTSP.Server.Port),
			StreamManager: app.streamManager,
			Logger:        logger.Log,
		})

		if err := app.rtspServer.Start(); err != nil {
			return nil, fmt.Errorf("failed to start RTSP server: %w", err)
		}
		logger.Info("RTSP server started", zap.Int("port", config.RTSP.Server.Port))
	} else {
		logger.Info("RTSP server disabled in configuration")
	}

	// 9. ProcessManager 비활동 모니터 시작
	go app.processManager.StartInactivityMonitor(ctx)
	logger.Info("ProcessManager inactivity monitor started")

	return app, nil
}

// createRTSPClient은 RTSP 클라이언트를 생성하고 시작하는 헬퍼 메서드입니다
func (app *Application) createRTSPClient(streamID, rtspURL, transport string, stream *core.Stream) (*rtsp.Client, error) {
	// Transport 기본값 설정
	if transport == "" {
		transport = "tcp"
	}

	// RTSP 클라이언트 생성
	client, err := rtsp.NewClient(rtsp.ClientConfig{
		URL:        rtspURL,
		Transport:  transport,
		Timeout:    time.Duration(app.config.RTSP.Client.Timeout) * time.Second,
		RetryCount: app.config.RTSP.Client.RetryCount,
		RetryDelay: time.Duration(app.config.RTSP.Client.RetryDelay) * time.Second,
		Logger:     logger.Log,
		Stream:     stream,
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
		return nil, fmt.Errorf("failed to create RTSP client: %w", err)
	}

	// RTSP 클라이언트 시작
	if err := client.Start(); err != nil {
		return nil, fmt.Errorf("failed to start RTSP client: %w", err)
	}

	return client, nil
}

// addStream은 새로운 RTSP 스트림을 추가합니다
func (app *Application) addStream(streamID, rtspURL string) error {
	// 스트림 생성
	stream, err := app.streamManager.CreateStream(streamID, streamID)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	// RTSP 클라이언트 생성 및 시작
	client, err := app.createRTSPClient(streamID, rtspURL, "tcp", stream)
	if err != nil {
		return err
	}

	// map에 저장
	app.rtspClients[streamID] = client

	return nil
}

// cleanup은 애플리케이션 리소스를 정리합니다
func (app *Application) cleanup() {
	logger.Info("Cleaning up application resources")

	// 1. Context 취소 (ProcessManager 모니터 종료)
	if app.cancelFunc != nil {
		app.cancelFunc()
		logger.Info("Context cancelled")
	}

	// 2. CCTV Manager 중지
	if app.cctvManager != nil {
		app.cctvManager.Stop()
		logger.Info("CCTV manager stopped")
	}

	// 3. 모든 외부 프로세스 중지
	if app.processManager != nil {
		app.processManager.StopAll()
		logger.Info("All external processes stopped")
	}

	// 3. RTSP 클라이언트 중지
	for streamID, client := range app.rtspClients {
		logger.Info("Stopping RTSP client", zap.String("stream_id", streamID))
		client.Stop()
	}

	// 4. RTSP 서버 종료
	if app.rtspServer != nil {
		app.rtspServer.Stop()
		logger.Info("RTSP server stopped")
	}

	// 5. API 서버 종료
	if app.apiServer != nil {
		app.apiServer.Stop()
	}

	// 6. 시그널링 서버 종료
	if app.signalingServer != nil {
		app.signalingServer.Close()
	}

	// 7. WebRTC 관리자 종료
	if app.webrtcManager != nil {
		app.webrtcManager.Close()
	}

	// 8. 스트림 관리자 종료
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

	// 스트림이 실행 중이 아니면 온디맨드로 시작 시도
	if _, isRunning := app.rtspClients[streamID]; !isRunning {
		logger.Info("Stream not running, attempting to start on-demand",
			zap.String("stream_id", streamID),
		)

		if err := app.startOnDemandStream(streamID); err != nil {
			logger.Error("Failed to start on-demand stream",
				zap.String("stream_id", streamID),
				zap.Error(err),
			)
			return "", fmt.Errorf("failed to start stream: %w", err)
		}

		// 잠시 대기하여 RTSP 연결이 안정화되도록 함
		waitTime := time.Duration(app.config.API.OnDemandWaitSec) * time.Second
		time.Sleep(waitTime)
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

// loadStreamsFromCCTV는 CCTV 매니저에서 스트림을 로드합니다
func (app *Application) loadStreamsFromCCTV() error {
	if app.cctvManager == nil {
		return fmt.Errorf("CCTV manager not initialized")
	}

	cctvs := app.cctvManager.GetCCTVs()
	logger.Info("Loading streams from CCTV manager", zap.Int("cctv_count", len(cctvs)))

	for streamID, cctv := range cctvs {
		logger.Info("Processing CCTV stream",
			zap.String("stream_id", streamID),
			zap.String("name", cctv.Name),
			zap.String("url", maskRTSPURL(cctv.URL)),
			zap.Bool("on_demand", cctv.SourceOnDemand),
		)

		// 모든 CCTV 스트림은 온디맨드로 처리 (API 기반이므로)
		if _, err := app.streamManager.CreateStream(streamID, streamID); err != nil {
			logger.Error("Failed to create CCTV stream",
				zap.String("stream_id", streamID),
				zap.Error(err),
			)
			continue
		}

		logger.Info("CCTV stream created (on-demand)",
			zap.String("stream_id", streamID),
			zap.String("name", cctv.Name),
		)
	}

	return nil
}

// startOnDemandStream은 온디맨드 스트림을 시작합니다
func (app *Application) startOnDemandStream(streamID string) error {
	// CCTV Manager에서 stream config 찾기
	if app.cctvManager == nil {
		return fmt.Errorf("CCTV manager not initialized")
	}

	pathConfig, err := app.cctvManager.GetStreamConfig(streamID)
	if err != nil {
		return fmt.Errorf("stream %s not found in CCTV manager: %w", streamID, err)
	}

	// 스트림이 이미 존재하는지 확인
	stream, err := app.streamManager.GetStream(streamID)
	if err != nil {
		return fmt.Errorf("stream not found: %w", err)
	}

	// 이미 RTSP 클라이언트가 있는지 확인
	if _, exists := app.rtspClients[streamID]; exists {
		logger.Info("RTSP stream already running", zap.String("stream_id", streamID))
		return nil
	}

	// RTSP 클라이언트 생성 및 시작
	client, err := app.createRTSPClient(streamID, pathConfig.Source, pathConfig.RTSPTransport, stream)
	if err != nil {
		return err
	}

	// map에 저장
	app.rtspClients[streamID] = client

	logger.Info("On-demand RTSP stream started",
		zap.String("stream_id", streamID),
		zap.String("url", maskRTSPURL(pathConfig.Source)),
	)

	return nil
}

// parseDuration은 문자열을 time.Duration으로 파싱합니다
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 10 * time.Second, nil // 기본값
	}
	return time.ParseDuration(s)
}

// stopStream은 스트림을 중지합니다
func (app *Application) stopStream(streamID string) error {
	// runOnDemand 프로세스 중지 시도
	if app.processManager.IsRunning(streamID) {
		logger.Info("Stopping runOnDemand process", zap.String("stream_id", streamID))
		if err := app.processManager.Stop(streamID); err != nil {
			return fmt.Errorf("failed to stop runOnDemand process: %w", err)
		}
		return nil
	}

	// RTSP 클라이언트 중지 시도
	client, exists := app.rtspClients[streamID]
	if exists {
		logger.Info("Stopping RTSP stream", zap.String("stream_id", streamID))
		client.Stop()
		delete(app.rtspClients, streamID)
		return nil
	}

	return fmt.Errorf("stream %s not running", streamID)
}
