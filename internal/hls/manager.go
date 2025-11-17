package hls

import (
	"fmt"
	"sync"

	"github.com/pion/rtp"
	"go.uber.org/zap"
)

// Manager는 HLS 스트림 관리자
type Manager struct {
	logger *zap.Logger
	config Config

	// Muxer 관리 (gohlslib 기반)
	muxers map[string]*MuxerGoHLS
	mutex  sync.RWMutex
}

// NewManager는 새로운 HLS Manager를 생성
func NewManager(config Config, logger *zap.Logger) *Manager {
	return &Manager{
		logger: logger,
		config: config,
		muxers: make(map[string]*MuxerGoHLS),
	}
}

// CreateMuxer는 새로운 HLS Muxer를 생성 (gohlslib 기반)
func (m *Manager) CreateMuxer(streamID string, codec string, sps, pps, vps []byte) (*MuxerGoHLS, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 이미 존재하는지 확인
	if _, exists := m.muxers[streamID]; exists {
		return nil, fmt.Errorf("muxer for stream %s already exists", streamID)
	}

	m.logger.Info("Creating gohlslib HLS muxer",
		zap.String("stream_id", streamID),
		zap.String("codec", codec),
	)

	// Muxer 생성
	muxer, err := NewMuxerGoHLS(streamID, m.logger, &m.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create muxer: %w", err)
	}

	// 코덱 설정
	if err := muxer.SetCodec(codec, sps, pps, vps); err != nil {
		return nil, fmt.Errorf("failed to set codec: %w", err)
	}

	// Muxer 시작
	if err := muxer.Start(); err != nil {
		return nil, fmt.Errorf("failed to start muxer: %w", err)
	}

	m.muxers[streamID] = muxer

	m.logger.Info("gohlslib HLS muxer created and started",
		zap.String("stream_id", streamID),
	)

	return muxer, nil
}

// GetMuxer는 Muxer를 반환
func (m *Manager) GetMuxer(streamID string) (*MuxerGoHLS, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	muxer, exists := m.muxers[streamID]
	return muxer, exists
}

// RemoveMuxer는 Muxer를 중지하고 제거
func (m *Manager) RemoveMuxer(streamID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	muxer, exists := m.muxers[streamID]
	if !exists {
		return fmt.Errorf("muxer for stream %s not found", streamID)
	}

	m.logger.Info("Removing HLS muxer",
		zap.String("stream_id", streamID),
	)

	// Muxer 중지
	muxer.Stop()

	// 맵에서 제거
	delete(m.muxers, streamID)

	m.logger.Info("HLS muxer removed",
		zap.String("stream_id", streamID),
	)

	return nil
}

// WritePacket은 특정 스트림의 Muxer에 RTP 패킷 전달
func (m *Manager) WritePacket(streamID string, pkt *rtp.Packet) error {
	m.mutex.RLock()
	muxer, exists := m.muxers[streamID]
	m.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("muxer for stream %s not found", streamID)
	}

	return muxer.WriteRTPPacket(pkt)
}

// GetStreamInfo는 스트림 정보를 반환
func (m *Manager) GetStreamInfo(streamID string) (*StreamInfo, error) {
	m.mutex.RLock()
	muxer, exists := m.muxers[streamID]
	m.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("muxer for stream %s not found", streamID)
	}

	info := muxer.GetStreamInfo()
	return &info, nil
}

// GetAllStreams는 모든 스트림 정보를 반환
func (m *Manager) GetAllStreams() map[string]StreamInfo {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	streams := make(map[string]StreamInfo)
	for streamID, muxer := range m.muxers {
		streams[streamID] = muxer.GetStreamInfo()
	}

	return streams
}

// GetStats는 스트림의 통계를 반환
func (m *Manager) GetStats(streamID string) (*Stats, error) {
	m.mutex.RLock()
	muxer, exists := m.muxers[streamID]
	m.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("muxer for stream %s not found", streamID)
	}

	stats := muxer.GetStats()
	return &stats, nil
}

// GetAllStats는 모든 스트림의 통계를 반환
func (m *Manager) GetAllStats() map[string]Stats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	allStats := make(map[string]Stats)
	for streamID, muxer := range m.muxers {
		allStats[streamID] = muxer.GetStats()
	}

	return allStats
}

// StopAll은 모든 Muxer를 중지
func (m *Manager) StopAll() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.logger.Info("Stopping all HLS muxers",
		zap.Int("count", len(m.muxers)),
	)

	for streamID, muxer := range m.muxers {
		m.logger.Debug("Stopping muxer",
			zap.String("stream_id", streamID),
		)
		muxer.Stop()
	}

	m.muxers = make(map[string]*MuxerGoHLS)

	m.logger.Info("All HLS muxers stopped")
}

// Count는 현재 실행 중인 Muxer 수를 반환
func (m *Manager) Count() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return len(m.muxers)
}

// IsEnabled는 HLS가 활성화되어 있는지 반환
func (m *Manager) IsEnabled() bool {
	return m.config.Enabled
}
