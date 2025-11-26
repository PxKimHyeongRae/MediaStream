package rtsp

import (
	"fmt"
	"sync"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/yourusername/cctv3/internal/core"
	"go.uber.org/zap"
)

// PathManager는 RTSP path별 세션을 관리합니다
type PathManager struct {
	paths         map[string]*Path
	streamManager *core.StreamManager
	logger        *zap.Logger
	mu            sync.RWMutex
}

// Path는 단일 RTSP 경로를 나타냅니다
// 예: /plx_cctv_01, /plx_cctv_01_h264
type Path struct {
	name          string
	publisher     *Publisher
	subscribers   map[string]*Subscriber // session ID -> Subscriber
	stream        *core.Stream           // Stream Manager의 스트림
	serverStream  *gortsplib.ServerStream
	medias        []*description.Media   // SDP medias
	logger        *zap.Logger
	mu            sync.RWMutex
}

// NewPathManager는 새로운 PathManager를 생성합니다
func NewPathManager(streamManager *core.StreamManager, logger *zap.Logger) *PathManager {
	return &PathManager{
		paths:         make(map[string]*Path),
		streamManager: streamManager,
		logger:        logger,
	}
}

// GetOrCreatePath는 path를 가져오거나 생성합니다
func (pm *PathManager) GetOrCreatePath(pathName string) (*Path, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// RTSP 경로는 /로 시작하므로 제거 (예: /plx_cctv_01 → plx_cctv_01)
	streamID := pathName
	if len(streamID) > 0 && streamID[0] == '/' {
		streamID = streamID[1:]
	}

	// 이미 존재하면 반환
	if path, exists := pm.paths[streamID]; exists {
		return path, nil
	}

	// 새로 생성
	// Stream Manager에서 스트림 가져오기 (슬래시 제거된 ID로)
	stream, err := pm.streamManager.GetStream(streamID)
	if err != nil {
		// 스트림이 없으면 생성
		stream, err = pm.streamManager.CreateStream(streamID, streamID)
		if err != nil {
			return nil, fmt.Errorf("failed to create stream: %w", err)
		}
		pm.logger.Info("Stream created for RTSP path",
			zap.String("path", pathName),
			zap.String("stream_id", streamID),
		)
	} else {
		pm.logger.Info("Using existing stream for RTSP path",
			zap.String("path", pathName),
			zap.String("stream_id", streamID),
		)
	}

	path := &Path{
		name:        streamID,
		stream:      stream,
		subscribers: make(map[string]*Subscriber),
		logger:      pm.logger,
	}

	pm.paths[streamID] = path
	pm.logger.Info("Path created", zap.String("path", streamID))

	return path, nil
}

// GetPath는 path를 가져옵니다 (없으면 에러)
func (pm *PathManager) GetPath(pathName string) (*Path, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// RTSP 경로는 /로 시작하므로 제거
	streamID := pathName
	if len(streamID) > 0 && streamID[0] == '/' {
		streamID = streamID[1:]
	}

	path, exists := pm.paths[streamID]
	if !exists {
		return nil, fmt.Errorf("path not found: %s", pathName)
	}

	return path, nil
}

// RegisterPublisher는 publisher를 등록합니다
func (pm *PathManager) RegisterPublisher(pathName string, ctx *gortsplib.ServerHandlerOnAnnounceCtx) error {
	path, err := pm.GetOrCreatePath(pathName)
	if err != nil {
		return err
	}

	return path.RegisterPublisher(ctx)
}

// ActivatePublisher는 publisher를 활성화합니다 (RECORD 시)
func (pm *PathManager) ActivatePublisher(pathName string, ctx *gortsplib.ServerHandlerOnRecordCtx) error {
	path, err := pm.GetPath(pathName)
	if err != nil {
		return err
	}

	return path.ActivatePublisher(ctx)
}

// RegisterSubscriber는 subscriber를 등록합니다
func (pm *PathManager) RegisterSubscriber(pathName string, ctx *gortsplib.ServerHandlerOnPlayCtx) error {
	path, err := pm.GetPath(pathName)
	if err != nil {
		return err
	}

	return path.RegisterSubscriber(ctx)
}

// GetStreamSDP는 path의 SDP를 생성합니다
func (pm *PathManager) GetStreamSDP(pathName string) (*gortsplib.ServerStream, []byte, error) {
	path, err := pm.GetOrCreatePath(pathName)
	if err != nil {
		return nil, nil, err
	}

	return path.GetSDP()
}

// GetServerStream은 gortsplib.ServerStream을 반환합니다
func (pm *PathManager) GetServerStream(pathName string) (*gortsplib.ServerStream, error) {
	path, err := pm.GetPath(pathName)
	if err != nil {
		return nil, err
	}

	return path.GetServerStream()
}

// RegisterPublisher는 publisher를 path에 등록합니다
func (p *Path) RegisterPublisher(ctx *gortsplib.ServerHandlerOnAnnounceCtx) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 이미 publisher가 있으면 에러
	if p.publisher != nil {
		return fmt.Errorf("path already has a publisher")
	}

	// Publisher 생성
	publisher, err := NewPublisher(PublisherConfig{
		Path:    p.name,
		Session: ctx.Session,
		Stream:  p.stream,
		Medias:  ctx.Description.Medias,
		Logger:  p.logger,
	})
	if err != nil {
		return err
	}

	p.publisher = publisher
	p.logger.Info("Publisher registered",
		zap.String("path", p.name),
	)

	return nil
}

// ActivatePublisher는 publisher를 활성화합니다
func (p *Path) ActivatePublisher(ctx *gortsplib.ServerHandlerOnRecordCtx) error {
	p.mu.RLock()
	publisher := p.publisher
	p.mu.RUnlock()

	if publisher == nil {
		return fmt.Errorf("no publisher registered")
	}

	return publisher.Activate(ctx)
}

// RegisterSubscriber는 subscriber를 path에 등록합니다
func (p *Path) RegisterSubscriber(ctx *gortsplib.ServerHandlerOnPlayCtx) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Session 포인터를 고유 ID로 사용
	sessionID := fmt.Sprintf("%p", ctx.Session)

	// 이미 등록되어 있으면 에러
	if _, exists := p.subscribers[sessionID]; exists {
		return fmt.Errorf("session already subscribed")
	}

	// ServerStream 필요 (SDP 생성되어 있어야 함)
	if p.serverStream == nil {
		return fmt.Errorf("path has no server stream (no SDP)")
	}

	// 첫 번째 media 가져오기 (비디오)
	var media *description.Media
	if len(p.medias) > 0 {
		media = p.medias[0]
	}

	// Subscriber 생성
	subscriber, err := NewSubscriber(SubscriberConfig{
		Path:         p.name,
		Session:      ctx.Session,
		Stream:       p.stream,
		ServerStream: p.serverStream,
		Media:        media,
		Logger:       p.logger,
	})
	if err != nil {
		return err
	}

	p.subscribers[sessionID] = subscriber
	p.logger.Info("Subscriber registered",
		zap.String("path", p.name),
		zap.String("session_id", sessionID),
		zap.Int("total_subscribers", len(p.subscribers)),
	)

	// Subscriber 활성화 (Stream으로부터 패킷 받기 시작)
	if err := subscriber.Activate(); err != nil {
		delete(p.subscribers, sessionID)
		return err
	}

	return nil
}

// GetSDP는 path의 SDP를 생성합니다
func (p *Path) GetSDP() (*gortsplib.ServerStream, []byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 이미 ServerStream이 있으면 재사용
	if p.serverStream != nil {
		return p.serverStream, nil, nil
	}

	// Stream에서 SDP 생성
	sdpBytes, medias, err := GenerateSDPFromStream(p.stream, p.logger)
	if err != nil {
		return nil, nil, err
	}

	// description.Session 생성
	desc := &description.Session{
		Medias: medias,
	}

	// gortsplib.ServerStream 생성
	// v4에서는 Server 인스턴스와 description.Session이 필요
	// 임시로 nil server 사용 (나중에 수정 필요)
	p.serverStream = gortsplib.NewServerStream(nil, desc)

	// Medias 저장 (Subscriber에서 사용)
	p.medias = medias

	p.logger.Info("SDP generated for path",
		zap.String("path", p.name),
		zap.Int("sdp_length", len(sdpBytes)),
	)

	return p.serverStream, sdpBytes, nil
}

// GetServerStream은 ServerStream을 반환합니다
func (p *Path) GetServerStream() (*gortsplib.ServerStream, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.serverStream == nil {
		return nil, fmt.Errorf("server stream not initialized (call DESCRIBE first)")
	}

	return p.serverStream, nil
}

// RemoveSubscriber는 subscriber를 제거합니다
func (p *Path) RemoveSubscriber(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if subscriber, exists := p.subscribers[sessionID]; exists {
		subscriber.Close()
		delete(p.subscribers, sessionID)
		p.logger.Info("Subscriber removed",
			zap.String("path", p.name),
			zap.String("session_id", sessionID),
			zap.Int("remaining_subscribers", len(p.subscribers)),
		)
	}
}

// RemovePublisher는 publisher를 제거합니다
func (p *Path) RemovePublisher() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.publisher != nil {
		p.publisher.Close()
		p.publisher = nil
		p.logger.Info("Publisher removed", zap.String("path", p.name))
	}
}
