package core

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

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

	// Phase 2.3: config 기반 버퍼 크기
	videoBufferSize int
}

// subscriberWorker는 구독자와 전용 워커를 관리합니다
type subscriberWorker struct {
	sub        StreamSubscriber
	packetChan chan *rtp.Packet
	ctx        context.Context
	cancel     context.CancelFunc
	logger     *zap.Logger
}

// Stream은 단일 미디어 스트림을 나타냅니다
type Stream struct {
	id     string
	name   string
	logger *zap.Logger

	// 미디어 정보
	videoCodec string // H264 또는 H265
	codecMutex sync.RWMutex

	// 구독자 관리 (워커 풀 패턴)
	subscribers map[string]*subscriberWorker
	subMutex    sync.RWMutex

	// 통계 (atomic으로 lock-free)
	packetsReceived atomic.Uint64
	packetsSent     atomic.Uint64
	bytesReceived   atomic.Uint64
	bytesSent       atomic.Uint64

	// 버퍼링
	packetBuffer chan *rtp.Packet

	// 스트림 종료 상태
	closed     bool
	closeMutex sync.RWMutex

	// 컨텍스트
	ctx context.Context

	// 워커 슬라이스 풀 (Phase 2.1 최적화 - 메모리 재사용)
	workerSlicePool sync.Pool
}

// StreamSubscriber는 스트림 구독자 인터페이스
type StreamSubscriber interface {
	OnPacket(packet *rtp.Packet) error
	GetID() string
}

// NewStreamManager는 새로운 스트림 관리자를 생성합니다
func NewStreamManager(logger *zap.Logger, videoBufferSize int) *StreamManager {
	ctx, cancel := context.WithCancel(context.Background())

	// Phase 2.3: 기본값 설정
	if videoBufferSize <= 0 {
		videoBufferSize = 500 // 기본값
	}

	return &StreamManager{
		ctx:             ctx,
		ctxCancel:       cancel,
		logger:          logger,
		streams:         make(map[string]*Stream),
		videoBufferSize: videoBufferSize,
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
		subscribers:  make(map[string]*subscriberWorker),
		packetBuffer: make(chan *rtp.Packet, sm.videoBufferSize), // Phase 2.3: config 기반 버퍼 크기
		ctx:          sm.ctx,
		workerSlicePool: sync.Pool{
			New: func() interface{} {
				// 초기 capacity 20 (예상 구독자 수)
				return make([]*subscriberWorker, 0, 20)
			},
		},
	}

	sm.streams[id] = stream

	// 패킷 배포 고루틴 시작 (워커 풀 패턴)
	go stream.distributePackets()

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

	// 스트림을 닫힌 상태로 표시 (WritePacket 차단)
	stream.closeMutex.Lock()
	stream.closed = true
	stream.closeMutex.Unlock()

	// 패킷 버퍼 닫기
	close(stream.packetBuffer)

	delete(sm.streams, id)

	sm.logger.Info("Stream removed",
		zap.String("stream_id", id),
	)

	return nil
}

// ListStreams는 모든 스트림 목록을 반환합니다
func (sm *StreamManager) ListStreams() map[string]*Stream {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	// 복사본 반환 (외부에서 맵을 수정하지 못하도록)
	streams := make(map[string]*Stream, len(sm.streams))
	for id, stream := range sm.streams {
		streams[id] = stream
	}

	return streams
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
	// 스트림이 닫혔는지 확인
	s.closeMutex.RLock()
	if s.closed {
		s.closeMutex.RUnlock()
		return fmt.Errorf("stream %s is closed", s.id)
	}
	s.closeMutex.RUnlock()

	select {
	case s.packetBuffer <- pkt:
		// atomic으로 lock-free 통계 업데이트
		s.packetsReceived.Add(1)
		s.bytesReceived.Add(uint64(len(pkt.Payload)))
		return nil
	default:
		// 버퍼 가득 참 - 가장 오래된 패킷 드롭
		s.logger.Warn("Packet buffer full, dropping oldest packet")
		select {
		case <-s.packetBuffer:
		default:
		}

		// 다시 닫혔는지 확인 (경쟁 조건 방지)
		s.closeMutex.RLock()
		if s.closed {
			s.closeMutex.RUnlock()
			return fmt.Errorf("stream %s is closed", s.id)
		}
		s.closeMutex.RUnlock()

		s.packetBuffer <- pkt
		return nil
	}
}

// Subscribe는 스트림 구독을 추가합니다 (워커 풀 패턴)
func (s *Stream) Subscribe(subscriber StreamSubscriber) error {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	id := subscriber.GetID()
	if _, exists := s.subscribers[id]; exists {
		return fmt.Errorf("subscriber %s already exists", id)
	}

	// 구독자별 전용 워커 생성
	ctx, cancel := context.WithCancel(s.ctx)
	worker := &subscriberWorker{
		sub:        subscriber,
		packetChan: make(chan *rtp.Packet, 100), // 구독자별 버퍼
		ctx:        ctx,
		cancel:     cancel,
		logger:     s.logger.With(zap.String("subscriber_id", id)),
	}

	s.subscribers[id] = worker

	// 워커 고루틴 시작 (구독자당 1개만 유지)
	go worker.run(s)

	s.logger.Info("Subscriber added with dedicated worker",
		zap.String("subscriber_id", id),
		zap.Int("total_subscribers", len(s.subscribers)),
	)

	return nil
}

// Unsubscribe는 스트림 구독을 제거합니다
func (s *Stream) Unsubscribe(subscriberID string) error {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	worker, exists := s.subscribers[subscriberID]
	if !exists {
		return fmt.Errorf("subscriber %s not found", subscriberID)
	}

	// 워커 종료
	worker.cancel()
	close(worker.packetChan)

	delete(s.subscribers, subscriberID)

	s.logger.Info("Subscriber removed",
		zap.String("subscriber_id", subscriberID),
		zap.Int("total_subscribers", len(s.subscribers)),
	)

	return nil
}

// distributePackets는 패킷을 모든 구독자 워커에게 배포합니다 (워커 풀 패턴 + sync.Pool)
func (s *Stream) distributePackets() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case pkt, ok := <-s.packetBuffer:
			if !ok {
				return
			}

			// sync.Pool에서 워커 슬라이스 가져오기 (Phase 2.1 최적화)
			workers := s.workerSlicePool.Get().([]*subscriberWorker)
			workers = workers[:0] // 길이 0으로 리셋 (capacity 유지)

			// 구독자 워커 목록 복사 (읽기 락 최소화)
			s.subMutex.RLock()
			for _, worker := range s.subscribers {
				workers = append(workers, worker)
			}
			s.subMutex.RUnlock()

			// 각 워커의 채널에 패킷 전송 (고루틴 생성 없음!)
			for _, worker := range workers {
				select {
				case worker.packetChan <- pkt:
					// 성공적으로 전송
				default:
					// 워커 버퍼 가득 참, 패킷 드롭 (블로킹 방지)
					worker.logger.Debug("Worker buffer full, dropping packet")
				}
			}

			// sync.Pool에 슬라이스 반환 (재사용)
			s.workerSlicePool.Put(workers)
		}
	}
}

// run은 구독자 워커의 메인 루프 (구독자당 1개의 고루틴만 유지)
func (w *subscriberWorker) run(s *Stream) {
	for {
		select {
		case <-w.ctx.Done():
			w.logger.Debug("Worker stopped")
			return
		case pkt, ok := <-w.packetChan:
			if !ok {
				w.logger.Debug("Worker channel closed")
				return
			}

			// 패킷 전송
			if err := w.sub.OnPacket(pkt); err != nil {
				// "peer not connected" 에러는 일시적이므로 DEBUG 레벨로 로깅
				if err.Error() == "peer not connected or track not ready" {
					w.logger.Debug("Peer not ready yet, skipping packet")
				} else {
					w.logger.Error("Failed to send packet to subscriber",
						zap.Error(err),
					)
				}
			} else {
				// atomic으로 통계 업데이트 (lock-free)
				s.packetsSent.Add(1)
				s.bytesSent.Add(uint64(len(pkt.Payload)))
			}
		}
	}
}

// GetStats는 스트림 통계를 반환합니다 (atomic, lock-free)
func (s *Stream) GetStats() (packetsReceived, packetsSent, bytesReceived, bytesSent uint64) {
	return s.packetsReceived.Load(), s.packetsSent.Load(), s.bytesReceived.Load(), s.bytesSent.Load()
}

// GetSubscriberCount는 구독자 수를 반환합니다
func (s *Stream) GetSubscriberCount() int {
	s.subMutex.RLock()
	defer s.subMutex.RUnlock()
	return len(s.subscribers)
}

// SetVideoCodec는 스트림의 비디오 코덱을 설정합니다
func (s *Stream) SetVideoCodec(codec string) {
	s.codecMutex.Lock()
	defer s.codecMutex.Unlock()
	s.videoCodec = codec
	s.logger.Info("Video codec set", zap.String("codec", codec))
}

// GetVideoCodec는 스트림의 비디오 코덱을 반환합니다
func (s *Stream) GetVideoCodec() string {
	s.codecMutex.RLock()
	defer s.codecMutex.RUnlock()
	return s.videoCodec
}

// GetID는 스트림 ID를 반환합니다
func (s *Stream) GetID() string {
	return s.id
}

// GetName은 스트림 이름을 반환합니다
func (s *Stream) GetName() string {
	return s.name
}
