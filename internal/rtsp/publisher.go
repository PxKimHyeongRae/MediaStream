package rtsp

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"
	"github.com/yourusername/cctv3/internal/core"
	"go.uber.org/zap"
)

// Publisher는 RTSP를 통해 스트림을 publish하는 클라이언트입니다
// 예: ffmpeg가 트랜스코딩 결과를 publish
type Publisher struct {
	path    string
	session *gortsplib.ServerSession
	stream  *core.Stream
	medias  []*description.Media
	logger  *zap.Logger
	active  bool
}

// PublisherConfig는 Publisher 설정입니다
type PublisherConfig struct {
	Path    string
	Session *gortsplib.ServerSession
	Stream  *core.Stream
	Medias  []*description.Media
	Logger  *zap.Logger
}

// NewPublisher는 새로운 Publisher를 생성합니다
func NewPublisher(config PublisherConfig) (*Publisher, error) {
	p := &Publisher{
		path:    config.Path,
		session: config.Session,
		stream:  config.Stream,
		medias:  config.Medias,
		logger:  config.Logger,
		active:  false,
	}

	// SDP에서 코덱 정보 추출 및 Stream에 설정
	if err := p.parseCodecInfo(); err != nil {
		return nil, fmt.Errorf("failed to parse codec info: %w", err)
	}

	return p, nil
}

// parseCodecInfo는 SDP에서 코덱 정보를 추출합니다
func (p *Publisher) parseCodecInfo() error {
	for _, media := range p.medias {
		for _, forma := range media.Formats {
			switch f := forma.(type) {
			case *format.H264:
				// H.264 코덱 설정
				p.stream.SetVideoCodec("H264")
				p.logger.Info("Publisher codec detected",
					zap.String("path", p.path),
					zap.String("codec", "H264"),
				)
				return nil

			case *format.H265:
				// H.265 코덱 설정
				p.stream.SetVideoCodec("H265")
				p.logger.Info("Publisher codec detected",
					zap.String("path", p.path),
					zap.String("codec", "H265"),
				)
				return nil

			default:
				p.logger.Debug("Unsupported format",
					zap.String("path", p.path),
					zap.String("format", fmt.Sprintf("%T", f)),
				)
			}
		}
	}

	return fmt.Errorf("no supported video codec found in SDP")
}

// Activate는 publisher를 활성화합니다 (RECORD 시작)
func (p *Publisher) Activate(ctx *gortsplib.ServerHandlerOnRecordCtx) error {
	if p.active {
		return fmt.Errorf("publisher already active")
	}

	// OnPacketRTPAny 콜백 등록
	p.session.OnPacketRTPAny(func(medi *description.Media, forma format.Format, pkt *rtp.Packet) {
		// Stream Manager에 패킷 전달
		if err := p.stream.WritePacket(pkt); err != nil {
			p.logger.Error("Failed to write packet to stream",
				zap.String("path", p.path),
				zap.Error(err),
			)
		}
	})

	p.active = true
	p.logger.Info("Publisher activated",
		zap.String("path", p.path),
	)

	return nil
}

// handlePacket은 publisher로부터 받은 RTP 패킷을 처리합니다
func (p *Publisher) handlePacket(mediaIndex int, pkt *rtp.Packet) {
	// Stream Manager에 패킷 전달
	if err := p.stream.WritePacket(pkt); err != nil {
		p.logger.Error("Failed to write packet to stream",
			zap.String("path", p.path),
			zap.Error(err),
		)
	}
}

// Close는 publisher를 닫습니다
func (p *Publisher) Close() {
	p.logger.Info("Closing publisher", zap.String("path", p.path))
	p.active = false
}
