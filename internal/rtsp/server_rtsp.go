package rtsp

import (
	"fmt"
	"sync"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/yourusername/cctv3/internal/core"
	"go.uber.org/zap"
)

// ServerRTSP는 RTSP 서버를 나타냅니다
// ffmpeg 등 외부 프로그램이 publish/subscribe할 수 있도록 합니다
type ServerRTSP struct {
	address       string
	server        *gortsplib.Server
	pathManager   *PathManager
	streamManager *core.StreamManager
	logger        *zap.Logger
	wg            sync.WaitGroup
	ctx           *ServerContext
}

// ServerContext는 서버 컨텍스트를 저장합니다
type ServerContext struct {
	server *ServerRTSP
}

// ServerRTSPConfig는 RTSP 서버 설정입니다
type ServerRTSPConfig struct {
	Address       string              // ":8554"
	StreamManager *core.StreamManager
	Logger        *zap.Logger
}

// NewServerRTSP는 새로운 RTSP 서버를 생성합니다
func NewServerRTSP(config ServerRTSPConfig) *ServerRTSP {
	s := &ServerRTSP{
		address:       config.Address,
		streamManager: config.StreamManager,
		logger:        config.Logger,
		pathManager:   NewPathManager(config.StreamManager, config.Logger),
	}

	s.ctx = &ServerContext{
		server: s,
	}

	// gortsplib Server 생성
	s.server = &gortsplib.Server{
		Handler:     s,
		RTSPAddress: s.address,
	}

	return s
}

// Start는 RTSP 서버를 시작합니다
func (s *ServerRTSP) Start() error {
	s.logger.Info("Starting RTSP server", zap.String("address", s.address))

	if err := s.server.Start(); err != nil {
		return fmt.Errorf("failed to start RTSP server: %w", err)
	}

	s.logger.Info("RTSP server started successfully", zap.String("address", s.address))
	return nil
}

// Stop은 RTSP 서버를 중지합니다
func (s *ServerRTSP) Stop() {
	s.logger.Info("Stopping RTSP server")

	if s.server != nil {
		s.server.Close()
	}

	s.wg.Wait()
	s.logger.Info("RTSP server stopped")
}

// OnConnOpen는 클라이언트 연결 시 호출됩니다 (gortsplib.ServerHandlerOnConnOpen)
func (s *ServerRTSP) OnConnOpen(ctx *gortsplib.ServerHandlerOnConnOpenCtx) {
	s.logger.Info("RTSP client connected",
		zap.String("remote_addr", ctx.Conn.NetConn().RemoteAddr().String()),
	)
}

// OnConnClose는 클라이언트 종료 시 호출됩니다 (gortsplib.ServerHandlerOnConnClose)
func (s *ServerRTSP) OnConnClose(ctx *gortsplib.ServerHandlerOnConnCloseCtx) {
	s.logger.Info("RTSP client disconnected",
		zap.String("remote_addr", ctx.Conn.NetConn().RemoteAddr().String()),
	)
}

// OnSessionOpen은 세션 생성 시 호출됩니다 (gortsplib.ServerHandlerOnSessionOpen)
func (s *ServerRTSP) OnSessionOpen(ctx *gortsplib.ServerHandlerOnSessionOpenCtx) {
	s.logger.Info("RTSP session opened")
}

// OnSessionClose는 세션 종료 시 호출됩니다 (gortsplib.ServerHandlerOnSessionClose)
func (s *ServerRTSP) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	s.logger.Info("RTSP session closed")
}

// OnDescribe는 DESCRIBE 요청 시 호출됩니다 (gortsplib.ServerHandlerOnDescribe)
// 클라이언트(ffmpeg)가 스트림 정보를 요청할 때 (subscribe 시작)
func (s *ServerRTSP) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	pathName := ctx.Path
	s.logger.Info("DESCRIBE request received",
		zap.String("path", pathName),
		zap.String("remote_addr", ctx.Conn.NetConn().RemoteAddr().String()),
	)

	// PathManager를 통해 SDP 생성
	stream, sdp, err := s.pathManager.GetStreamSDP(pathName)
	if err != nil {
		s.logger.Error("Failed to get stream SDP",
			zap.String("path", pathName),
			zap.Error(err),
		)
		return &base.Response{
			StatusCode: base.StatusNotFound,
		}, nil, nil
	}

	s.logger.Info("DESCRIBE response prepared",
		zap.String("path", pathName),
		zap.Int("sdp_length", len(sdp)),
	)

	return &base.Response{
		StatusCode: base.StatusOK,
	}, stream, nil
}

// OnAnnounce는 ANNOUNCE 요청 시 호출됩니다 (gortsplib.ServerHandlerOnAnnounce)
// 클라이언트(ffmpeg)가 스트림을 publish하려고 할 때
func (s *ServerRTSP) OnAnnounce(ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	pathName := ctx.Path
	s.logger.Info("ANNOUNCE request received",
		zap.String("path", pathName),
		zap.String("remote_addr", ctx.Conn.NetConn().RemoteAddr().String()),
	)

	// PathManager를 통해 Publisher 등록
	if err := s.pathManager.RegisterPublisher(pathName, ctx); err != nil {
		s.logger.Error("Failed to register publisher",
			zap.String("path", pathName),
			zap.Error(err),
		)
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	s.logger.Info("Publisher registered",
		zap.String("path", pathName),
	)

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnSetup은 SETUP 요청 시 호출됩니다 (gortsplib.ServerHandlerOnSetup)
func (s *ServerRTSP) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	pathName := ctx.Path
	s.logger.Info("SETUP request received",
		zap.String("path", pathName),
		
	)

	// PathManager에서 stream 가져오기
	stream, err := s.pathManager.GetServerStream(pathName)
	if err != nil {
		s.logger.Error("Failed to get server stream",
			zap.String("path", pathName),
			zap.Error(err),
		)
		return &base.Response{
			StatusCode: base.StatusNotFound,
		}, nil, err
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, stream, nil
}

// OnPlay는 PLAY 요청 시 호출됩니다 (gortsplib.ServerHandlerOnPlay)
// 클라이언트(ffmpeg)가 실제로 스트림을 받기 시작할 때
func (s *ServerRTSP) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	pathName := ctx.Path
	s.logger.Info("PLAY request received",
		zap.String("path", pathName),
		
	)

	// PathManager를 통해 Subscriber 등록
	if err := s.pathManager.RegisterSubscriber(pathName, ctx); err != nil {
		s.logger.Error("Failed to register subscriber",
			zap.String("path", pathName),
			zap.Error(err),
		)
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	s.logger.Info("Subscriber registered",
		zap.String("path", pathName),
	)

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnRecord는 RECORD 요청 시 호출됩니다 (gortsplib.ServerHandlerOnRecord)
// ANNOUNCE 후 실제로 publish를 시작할 때
func (s *ServerRTSP) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	pathName := ctx.Path
	s.logger.Info("RECORD request received",
		zap.String("path", pathName),
		
	)

	// Publisher 활성화
	if err := s.pathManager.ActivatePublisher(pathName, ctx); err != nil {
		s.logger.Error("Failed to activate publisher",
			zap.String("path", pathName),
			zap.Error(err),
		)
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	s.logger.Info("Publisher activated",
		zap.String("path", pathName),
	)

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}
