package webrtc

import (
	"fmt"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"
	"go.uber.org/zap"
)

// Manager는 WebRTC 피어들을 관리합니다
type Manager struct {
	logger *zap.Logger

	peers map[string]*Peer
	mutex sync.RWMutex

	maxPeers     int
	onPeerClosed func(peerID string)

	// 공유 WebRTC API (싱글톤 패턴 - 메모리 절약)
	sharedAPI *webrtc.API
	apiOnce   sync.Once
}

// ManagerConfig는 WebRTC 관리자 설정
type ManagerConfig struct {
	Logger       *zap.Logger
	MaxPeers     int
	OnPeerClosed func(peerID string) // 피어가 닫힐 때 호출되는 콜백
}

// NewManager는 새로운 WebRTC 관리자를 생성합니다
func NewManager(config ManagerConfig) *Manager {
	if config.MaxPeers == 0 {
		config.MaxPeers = 1000
	}

	m := &Manager{
		logger:       config.Logger,
		peers:        make(map[string]*Peer),
		maxPeers:     config.MaxPeers,
		onPeerClosed: config.OnPeerClosed,
	}

	// 공유 API 초기화 (한 번만 생성)
	m.getOrCreateAPI()

	return m
}

// getOrCreateAPI는 공유 WebRTC API를 반환합니다 (싱글톤 패턴)
func (m *Manager) getOrCreateAPI() *webrtc.API {
	m.apiOnce.Do(func() {
		// MediaEngine 설정
		mediaEngine := &webrtc.MediaEngine{}

		// H.264 비디오 코덱 등록
		_ = mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				Channels:    0,
				SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
			},
			PayloadType: 96,
		}, webrtc.RTPCodecTypeVideo)

		// H.265 비디오 코덱 등록
		_ = mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH265,
				ClockRate:   90000,
				Channels:    0,
				SDPFmtpLine: "level-id=180;profile-id=1;tier-flag=0;tx-mode=SRST",
			},
			PayloadType: 49,
		}, webrtc.RTPCodecTypeVideo)

		// Opus 오디오 코덱 등록
		_ = mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeOpus,
				ClockRate:   48000,
				Channels:    2,
				SDPFmtpLine: "minptime=10;useinbandfec=1",
			},
			PayloadType: 111,
		}, webrtc.RTPCodecTypeAudio)

		// Interceptor 설정
		registry := &interceptor.Registry{}
		_ = webrtc.RegisterDefaultInterceptors(mediaEngine, registry)

		// SettingEngine 설정 (Phase 3 최적화)
		settingEngine := webrtc.SettingEngine{}
		settingEngine.SetSRTPReplayProtectionWindow(512) // 기본값 64 → 512
		// settingEngine.SetICEMulticastDNSMode(webrtc.ICEMulticastDNSModeDisabled)  // mDNS 비활성화 (버전 미지원)
		settingEngine.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4}) // IPv4 UDP만 사용

		// API 생성 (모든 피어가 공유)
		m.sharedAPI = webrtc.NewAPI(
			webrtc.WithMediaEngine(mediaEngine),
			webrtc.WithInterceptorRegistry(registry),
			webrtc.WithSettingEngine(settingEngine),
		)

		m.logger.Info("Shared WebRTC API initialized (singleton pattern)")
	})

	return m.sharedAPI
}

// CreatePeer는 새로운 피어를 생성합니다
func (m *Manager) CreatePeer(streamID string) (*Peer, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 최대 피어 수 체크
	if len(m.peers) >= m.maxPeers {
		return nil, fmt.Errorf("max peers reached: %d", m.maxPeers)
	}

	peer := NewPeer(PeerConfig{
		StreamID:  streamID,
		Logger:    m.logger,
		SharedAPI: m.sharedAPI, // 공유 API 전달
		OnClose: func(peerID string) {
			// 고루틴으로 실행하여 데드락 방지
			go func() {
				// 먼저 외부 콜백 호출 (스트림 구독 해제 등)
				if m.onPeerClosed != nil {
					m.onPeerClosed(peerID)
				}
				// 그 다음 매니저에서 피어 제거
				m.RemovePeer(peerID)
			}()
		},
	})

	m.peers[peer.GetID()] = peer

	m.logger.Info("Peer created",
		zap.String("peer_id", peer.GetID()),
		zap.Int("total_peers", len(m.peers)),
	)

	return peer, nil
}

// GetPeer는 피어를 조회합니다
func (m *Manager) GetPeer(peerID string) (*Peer, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	peer, exists := m.peers[peerID]
	if !exists {
		return nil, fmt.Errorf("peer %s not found", peerID)
	}

	return peer, nil
}

// RemovePeer는 피어를 제거합니다
func (m *Manager) RemovePeer(peerID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	peer, exists := m.peers[peerID]
	if !exists {
		return fmt.Errorf("peer %s not found", peerID)
	}

	peer.Close()
	delete(m.peers, peerID)

	m.logger.Info("Peer removed",
		zap.String("peer_id", peerID),
		zap.Int("total_peers", len(m.peers)),
	)

	return nil
}

// GetPeerCount는 피어 수를 반환합니다
func (m *Manager) GetPeerCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.peers)
}

// Close는 모든 피어를 종료합니다
func (m *Manager) Close() {
	m.logger.Info("Closing WebRTC manager")

	m.mutex.Lock()
	defer m.mutex.Unlock()

	for id, peer := range m.peers {
		peer.Close()
		delete(m.peers, id)
	}
}
