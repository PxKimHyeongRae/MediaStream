package rtsp

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/pion/rtp"
	"github.com/yourusername/cctv3/internal/core"
	"go.uber.org/zap"
)

// Subscriber는 RTSP를 통해 스트림을 subscribe하는 클라이언트입니다
// 예: ffmpeg가 원본 스트림을 읽어감
type Subscriber struct {
	path         string
	session      *gortsplib.ServerSession
	stream       *core.Stream
	serverStream *gortsplib.ServerStream
	media        *description.Media // RTP 패킷을 전송할 미디어
	subscriberID string
	logger       *zap.Logger
	active       bool
}

// SubscriberConfig는 Subscriber 설정입니다
type SubscriberConfig struct {
	Path         string
	Session      *gortsplib.ServerSession
	Stream       *core.Stream
	ServerStream *gortsplib.ServerStream
	Media        *description.Media
	Logger       *zap.Logger
}

// NewSubscriber는 새로운 Subscriber를 생성합니다
func NewSubscriber(config SubscriberConfig) (*Subscriber, error) {
	// Session 포인터를 고유 ID로 사용
	subscriberID := fmt.Sprintf("%p", config.Session)

	s := &Subscriber{
		path:         config.Path,
		session:      config.Session,
		stream:       config.Stream,
		serverStream: config.ServerStream,
		media:        config.Media,
		subscriberID: subscriberID,
		logger:       config.Logger,
		active:       false,
	}

	return s, nil
}

// Activate는 subscriber를 활성화합니다 (PLAY 시작)
func (s *Subscriber) Activate() error {
	if s.active {
		return fmt.Errorf("subscriber already active")
	}

	// Stream의 Subscriber 인터페이스를 구현하여 등록
	if err := s.stream.Subscribe(s); err != nil {
		return fmt.Errorf("failed to subscribe to stream: %w", err)
	}

	s.active = true
	s.logger.Info("Subscriber activated",
		zap.String("path", s.path),
		
		zap.String("subscriber_id", s.subscriberID),
	)

	return nil
}

// GetID는 subscriber ID를 반환합니다 (core.StreamSubscriber 인터페이스)
func (s *Subscriber) GetID() string {
	return s.subscriberID
}

// OnPacket은 Stream으로부터 RTP 패킷을 받습니다 (core.StreamSubscriber 인터페이스)
func (s *Subscriber) OnPacket(pkt *rtp.Packet) error {
	return s.WritePacket(pkt)
}

// WritePacket은 RTP 패킷을 RTSP 클라이언트에 전송합니다
func (s *Subscriber) WritePacket(pkt *rtp.Packet) error {
	if !s.active {
		return fmt.Errorf("subscriber not active")
	}

	// gortsplib v4: ServerSession.WritePacketRTP 사용
	err := s.session.WritePacketRTP(s.media, pkt)
	if err != nil {
		return fmt.Errorf("failed to write RTP packet: %w", err)
	}

	return nil
}

// Close는 subscriber를 닫습니다
func (s *Subscriber) Close() {
	if !s.active {
		return
	}

	// Stream에서 구독 해제
	if err := s.stream.Unsubscribe(s.subscriberID); err != nil {
		s.logger.Error("Failed to unsubscribe from stream",
			zap.String("path", s.path),
			zap.String("subscriber_id", s.subscriberID),
			zap.Error(err),
		)
	}

	s.active = false
	s.logger.Info("Subscriber closed",
		zap.String("path", s.path),
		zap.String("subscriber_id", s.subscriberID),
	)
}
