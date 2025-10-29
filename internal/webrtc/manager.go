package webrtc

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// Manager는 WebRTC 피어들을 관리합니다
type Manager struct {
	logger *zap.Logger

	peers map[string]*Peer
	mutex sync.RWMutex

	maxPeers     int
	onPeerClosed func(peerID string)
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

	return &Manager{
		logger:       config.Logger,
		peers:        make(map[string]*Peer),
		maxPeers:     config.MaxPeers,
		onPeerClosed: config.OnPeerClosed,
	}
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
		StreamID: streamID,
		Logger:   m.logger,
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
