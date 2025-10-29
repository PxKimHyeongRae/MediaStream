package core

import (
	"context"
	"fmt"
	"sync"

	"github.com/pion/rtp"
	"go.uber.org/zap"
)

// StreamManager는 스트림을 관리합니다
type StreamManager struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	logger    *zap.Logger

	streams map[string]*Stream
	mutex   sync.RWMutex
}

// Stream은 단일 미디어 스트림을 나타냅니다
type Stream struct {
	id       string
	name     string
	logger   *zap.Logger

	// 구독자 관리
	subscribers map[string]StreamSubscriber
	subMutex    sync.RWMutex

	// 통계
	packetsReceived uint64
	packetsSent     uint64
	bytesReceived   uint64
	bytesSent       uint64
	statsMutex      sync.RWMutex

	// 버퍼링
	packetBuffer chan *rtp.Packet
}

// StreamSubscriber는 스트림 구독자 인터페이스
type StreamSubscriber interface {
	OnPacket(packet *rtp.Packet) error
	GetID() string
}

// NewStreamManager는 새로운 스트림 관리자를 생성합니다
func NewStreamManager(logger *zap.Logger) *StreamManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &StreamManager{
		ctx:       ctx,
		ctxCancel: cancel,
		logger:    logger,
		streams:   make(map[string]*Stream),
	}
}

// CreateStream은 새로운 스트림을 생성합니다
func (sm *StreamManager) CreateStream(id, name string) (*Stream, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if _, exists := sm.streams[id]; exists {
		return nil, fmt.Errorf("stream %s already exists", id)
	}

	stream := &Stream{
		id:           id,
		name:         name,
		logger:       sm.logger.With(zap.String("stream_id", id)),
		subscribers:  make(map[string]StreamSubscriber),
		packetBuffer: make(chan *rtp.Packet, 500),
	}

	sm.streams[id] = stream

	// 패킷 배포 고루틴 시작
	go stream.distributePackets(sm.ctx)

	sm.logger.Info("Stream created",
		zap.String("stream_id", id),
		zap.String("stream_name", name),
	)

	return stream, nil
}

// GetStream은 스트림을 조회합니다
func (sm *StreamManager) GetStream(id string) (*Stream, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	stream, exists := sm.streams[id]
	if !exists {
		return nil, fmt.Errorf("stream %s not found", id)
	}

	return stream, nil
}

// RemoveStream은 스트림을 제거합니다
func (sm *StreamManager) RemoveStream(id string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	stream, exists := sm.streams[id]
	if !exists {
		return fmt.Errorf("stream %s not found", id)
	}

	// 패킷 버퍼 닫기
	close(stream.packetBuffer)

	delete(sm.streams, id)

	sm.logger.Info("Stream removed",
		zap.String("stream_id", id),
	)

	return nil
}

// Close는 스트림 관리자를 종료합니다
func (sm *StreamManager) Close() {
	sm.logger.Info("Closing stream manager")
	sm.ctxCancel()

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	for id, stream := range sm.streams {
		close(stream.packetBuffer)
		delete(sm.streams, id)
	}
}

// WritePacket은 스트림에 RTP 패킷을 씁니다
func (s *Stream) WritePacket(pkt *rtp.Packet) error {
	select {
	case s.packetBuffer <- pkt:
		s.statsMutex.Lock()
		s.packetsReceived++
		s.bytesReceived += uint64(len(pkt.Payload))
		s.statsMutex.Unlock()
		return nil
	default:
		// 버퍼 가득 참 - 가장 오래된 패킷 드롭
		s.logger.Warn("Packet buffer full, dropping oldest packet")
		select {
		case <-s.packetBuffer:
		default:
		}
		s.packetBuffer <- pkt
		return nil
	}
}

// Subscribe는 스트림 구독을 추가합니다
func (s *Stream) Subscribe(subscriber StreamSubscriber) error {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	id := subscriber.GetID()
	if _, exists := s.subscribers[id]; exists {
		return fmt.Errorf("subscriber %s already exists", id)
	}

	s.subscribers[id] = subscriber

	s.logger.Info("Subscriber added",
		zap.String("subscriber_id", id),
		zap.Int("total_subscribers", len(s.subscribers)),
	)

	return nil
}

// Unsubscribe는 스트림 구독을 제거합니다
func (s *Stream) Unsubscribe(subscriberID string) error {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	if _, exists := s.subscribers[subscriberID]; !exists {
		return fmt.Errorf("subscriber %s not found", subscriberID)
	}

	delete(s.subscribers, subscriberID)

	s.logger.Info("Subscriber removed",
		zap.String("subscriber_id", subscriberID),
		zap.Int("total_subscribers", len(s.subscribers)),
	)

	return nil
}

// distributePackets는 패킷을 모든 구독자에게 배포합니다
func (s *Stream) distributePackets(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case pkt, ok := <-s.packetBuffer:
			if !ok {
				return
			}

			s.subMutex.RLock()
			subscribers := make([]StreamSubscriber, 0, len(s.subscribers))
			for _, sub := range s.subscribers {
				subscribers = append(subscribers, sub)
			}
			s.subMutex.RUnlock()

			// 각 구독자에게 비동기 전달
			for _, sub := range subscribers {
				go func(subscriber StreamSubscriber) {
					if err := subscriber.OnPacket(pkt); err != nil {
						s.logger.Error("Failed to send packet to subscriber",
							zap.String("subscriber_id", subscriber.GetID()),
							zap.Error(err),
						)
					} else {
						s.statsMutex.Lock()
						s.packetsSent++
						s.bytesSent += uint64(len(pkt.Payload))
						s.statsMutex.Unlock()
					}
				}(sub)
			}
		}
	}
}

// GetStats는 스트림 통계를 반환합니다
func (s *Stream) GetStats() (packetsReceived, packetsSent, bytesReceived, bytesSent uint64) {
	s.statsMutex.RLock()
	defer s.statsMutex.RUnlock()
	return s.packetsReceived, s.packetsSent, s.bytesReceived, s.bytesSent
}

// GetSubscriberCount는 구독자 수를 반환합니다
func (s *Stream) GetSubscriberCount() int {
	s.subMutex.RLock()
	defer s.subMutex.RUnlock()
	return len(s.subscribers)
}
