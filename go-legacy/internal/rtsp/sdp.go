package rtsp

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/sdp/v3"
	"github.com/yourusername/cctv3/internal/core"
	"go.uber.org/zap"
)

// GenerateSDPFromStream은 Stream으로부터 SDP를 생성합니다
func GenerateSDPFromStream(stream *core.Stream, logger *zap.Logger) ([]byte, []*description.Media, error) {
	codec := stream.GetVideoCodec()
	if codec == "" {
		return nil, nil, fmt.Errorf("stream has no codec information")
	}

	logger.Info("Generating SDP for stream",
		zap.String("codec", codec),
	)

	var medias []*description.Media

	switch codec {
	case "H264":
		medias = generateH264Media()
	case "H265":
		medias = generateH265Media()
	default:
		return nil, nil, fmt.Errorf("unsupported codec: %s", codec)
	}

	// SDP 생성
	sdpData := generateSDPBytes(medias)

	logger.Info("SDP generated successfully",
		zap.String("codec", codec),
		zap.Int("sdp_length", len(sdpData)),
	)

	return sdpData, medias, nil
}

// generateH264Media는 H.264용 Media description을 생성합니다
func generateH264Media() []*description.Media {
	return []*description.Media{
		{
			Type: description.MediaTypeVideo,
			Formats: []format.Format{
				&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				},
			},
		},
	}
}

// generateH265Media는 H.265용 Media description을 생성합니다
func generateH265Media() []*description.Media {
	return []*description.Media{
		{
			Type: description.MediaTypeVideo,
			Formats: []format.Format{
				&format.H265{
					PayloadTyp: 96,
				},
			},
		},
	}
}

// generateSDPBytes는 Media description으로부터 SDP 바이트를 생성합니다
func generateSDPBytes(medias []*description.Media) []byte {
	// description.Session을 사용하여 SDP 생성
	sess := &description.Session{
		Medias: medias,
	}

	// Marshal to SDP (false = don't include control attributes)
	sdpData, err := sess.Marshal(false)
	if err != nil {
		// 에러 발생 시 기본 SDP 반환
		return []byte(generateBasicSDP())
	}

	return sdpData
}

// generateBasicSDP는 기본 SDP를 생성합니다 (fallback)
func generateBasicSDP() string {
	sd := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      0,
			SessionVersion: 0,
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: "127.0.0.1",
		},
		SessionName: "RTSP Server Stream",
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: "0.0.0.0"},
		},
		TimeDescriptions: []sdp.TimeDescription{
			{
				Timing: sdp.Timing{
					StartTime: 0,
					StopTime:  0,
				},
			},
		},
		MediaDescriptions: []*sdp.MediaDescription{
			{
				MediaName: sdp.MediaName{
					Media:   "video",
					Port:    sdp.RangedPort{Value: 0},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"96"},
				},
				Attributes: []sdp.Attribute{
					{Key: "rtpmap", Value: "96 H264/90000"},
					{Key: "fmtp", Value: "96 packetization-mode=1"},
					{Key: "control", Value: "trackID=0"},
				},
			},
		},
	}

	bytes, _ := sd.Marshal()
	return string(bytes)
}
