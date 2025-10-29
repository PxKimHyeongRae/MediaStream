package webrtc

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"go.uber.org/zap"
)

// Peer는 WebRTC 피어 연결을 나타냅니다
type Peer struct {
	id        string
	streamID  string
	logger    *zap.Logger

	// 상태
	ctx       context.Context
	ctxCancel context.CancelFunc
	connected bool
	mutex     sync.RWMutex
	closeOnce sync.Once // Close가 여러 번 호출되는 것을 방지

	// pion/webrtc
	pc           *webrtc.PeerConnection
	videoTrack   *webrtc.TrackLocalStaticRTP
	audioTrack   *webrtc.TrackLocalStaticRTP

	// 콜백
	onClose func(peerID string)

	// 통계
	packetsSent uint64
	bytesSent   uint64
	statsMutex  sync.RWMutex
}

// PeerConfig는 WebRTC 피어 설정
type PeerConfig struct {
	StreamID string
	Logger   *zap.Logger
	OnClose  func(peerID string)
}

// NewPeer는 새로운 WebRTC 피어를 생성합니다
func NewPeer(config PeerConfig) *Peer {
	id := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())

	peer := &Peer{
		id:        id,
		streamID:  config.StreamID,
		logger:    config.Logger.With(zap.String("peer_id", id)),
		ctx:       ctx,
		ctxCancel: cancel,
		onClose:   config.OnClose,
	}

	peer.logger.Info("WebRTC peer created",
		zap.String("stream_id", config.StreamID),
	)

	return peer
}

// CreateOffer는 WebRTC Offer를 생성합니다
func (p *Peer) CreateOffer(offer string) (answer string, err error) {
	// Offer SDP에서 클라이언트가 지원하는 비디오 코덱 확인
	selectedCodec := p.selectVideoCodec(offer)
	p.logger.Info("Video codec selected based on client support",
		zap.String("codec", selectedCodec),
	)

	// PeerConnection 생성 (선택된 코덱 사용)
	if err := p.createPeerConnection(selectedCodec); err != nil {
		return "", fmt.Errorf("failed to create peer connection: %w", err)
	}

	// Remote Offer 설정
	if err := p.pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer,
	}); err != nil {
		return "", fmt.Errorf("failed to set remote description: %w", err)
	}

	p.logger.Info("Remote offer set")

	// Local Answer 생성
	answerSDP, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("failed to create answer: %w", err)
	}

	// ICE gathering 완료 채널 생성
	gatherComplete := webrtc.GatheringCompletePromise(p.pc)

	// Local Description 설정 (ICE gathering 시작)
	if err := p.pc.SetLocalDescription(answerSDP); err != nil {
		return "", fmt.Errorf("failed to set local description: %w", err)
	}

	p.logger.Info("Waiting for ICE gathering to complete...")

	// ICE gathering 완료 대기
	<-gatherComplete

	p.logger.Info("ICE gathering complete")

	// 완전한 SDP 반환 (ICE candidates 포함)
	finalAnswer := p.pc.LocalDescription()
	if finalAnswer == nil {
		return "", fmt.Errorf("local description is nil after ICE gathering")
	}

	p.logger.Info("Local answer created with ICE candidates",
		zap.Int("sdp_length", len(finalAnswer.SDP)),
	)

	return finalAnswer.SDP, nil
}

// selectVideoCodec는 클라이언트 Offer SDP에서 지원하는 비디오 코덱을 선택합니다
func (p *Peer) selectVideoCodec(offerSDP string) string {
	// SDP 내용을 대소문자 구분 없이 검색
	offerUpper := strings.ToUpper(offerSDP)

	// H.265/HEVC 지원 여부 확인
	supportsH265 := strings.Contains(offerUpper, "H265") ||
					strings.Contains(offerUpper, "HEVC")

	// H.264/AVC 지원 여부 확인
	supportsH264 := strings.Contains(offerUpper, "H264") ||
					strings.Contains(offerUpper, "AVC")

	// 우선순위: H.265 > H.264 (H.265가 더 효율적)
	if supportsH265 {
		p.logger.Info("Client supports H.265 (HEVC)")
		return "H265"
	} else if supportsH264 {
		p.logger.Info("Client supports H.264 only")
		return "H264"
	} else {
		// 기본값: H.265
		p.logger.Warn("Client codec support unclear, defaulting to H.265")
		return "H265"
	}
}

// createPeerConnection은 PeerConnection을 생성합니다
func (p *Peer) createPeerConnection(selectedCodec string) error {
	// MediaEngine 설정
	m := &webrtc.MediaEngine{}

	// H.264 비디오 코덱 등록
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeH264,
			ClockRate:    90000,
			Channels:     0,
			SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
		PayloadType: 96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return fmt.Errorf("failed to register H264 codec: %w", err)
	}

	// H.265 비디오 코덱 등록
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH265,
			ClockRate:   90000,
			Channels:    0,
			SDPFmtpLine: "level-id=180;profile-id=1;tier-flag=0;tx-mode=SRST",
		},
		PayloadType: 49,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return fmt.Errorf("failed to register H265 codec: %w", err)
	}

	// Opus 오디오 코덱 등록
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return fmt.Errorf("failed to register Opus codec: %w", err)
	}

	// Interceptor 설정
	i := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		return fmt.Errorf("failed to register interceptors: %w", err)
	}

	// SettingEngine 설정
	s := webrtc.SettingEngine{}

	// API 생성
	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(m),
		webrtc.WithInterceptorRegistry(i),
		webrtc.WithSettingEngine(s),
	)

	// PeerConnection 설정
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
					"stun:stun1.l.google.com:19302",
				},
			},
		},
	}

	// PeerConnection 생성
	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("failed to create peer connection: %w", err)
	}

	p.pc = pc

	// 선택된 코덱으로 비디오 트랙 생성
	var videoTrack *webrtc.TrackLocalStaticRTP
	if selectedCodec == "H265" {
		videoTrack, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeH265,
				ClockRate: 90000,
			},
			"video",
			"pion",
		)
	} else {
		// H.264
		videoTrack, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeH264,
				ClockRate: 90000,
			},
			"video",
			"pion",
		)
	}
	if err != nil {
		return fmt.Errorf("failed to create video track: %w", err)
	}

	p.videoTrack = videoTrack

	// 비디오 트랙 추가
	if _, err = pc.AddTrack(videoTrack); err != nil {
		return fmt.Errorf("failed to add video track: %w", err)
	}

	p.logger.Info("Video track added",
		zap.String("codec", selectedCodec),
	)

	// 오디오 트랙 생성 (선택적)
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: 48000,
		},
		"audio",
		"pion",
	)
	if err != nil {
		return fmt.Errorf("failed to create audio track: %w", err)
	}

	p.audioTrack = audioTrack

	// 오디오 트랙 추가
	if _, err = pc.AddTrack(audioTrack); err != nil {
		return fmt.Errorf("failed to add audio track: %w", err)
	}

	p.logger.Info("Audio track added")

	// 이벤트 핸들러 등록
	p.setupEventHandlers()

	return nil
}

// setupEventHandlers는 PeerConnection 이벤트 핸들러를 설정합니다
func (p *Peer) setupEventHandlers() {
	// ICE Connection State 변경
	p.pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		p.logger.Info("ICE connection state changed",
			zap.String("state", state.String()),
		)

		if state == webrtc.ICEConnectionStateConnected {
			p.SetConnected(true)
		} else if state == webrtc.ICEConnectionStateFailed ||
			state == webrtc.ICEConnectionStateClosed {
			p.SetConnected(false)
			p.Close()
		}
	})

	// Peer Connection State 변경
	p.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		p.logger.Info("Peer connection state changed",
			zap.String("state", state.String()),
		)

		if state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateClosed {
			p.Close()
		}
	})

	// ICE Candidate
	p.pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			p.logger.Debug("ICE candidate",
				zap.String("candidate", candidate.String()),
			)
		}
	})
}

// GetID는 피어 ID를 반환합니다
func (p *Peer) GetID() string {
	return p.id
}

// OnPacket은 StreamSubscriber 인터페이스 구현
func (p *Peer) OnPacket(packet *rtp.Packet) error {
	p.mutex.RLock()
	connected := p.connected
	p.mutex.RUnlock()

	if !connected || p.videoTrack == nil {
		return fmt.Errorf("peer not connected or track not ready")
	}

	// RTP 패킷을 WebRTC 트랙으로 전송
	if err := p.videoTrack.WriteRTP(packet); err != nil {
		return fmt.Errorf("failed to write RTP packet: %w", err)
	}

	p.statsMutex.Lock()
	p.packetsSent++
	p.bytesSent += uint64(len(packet.Payload))
	p.statsMutex.Unlock()

	return nil
}

// SetConnected는 연결 상태를 설정합니다
func (p *Peer) SetConnected(connected bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.connected = connected

	if connected {
		p.logger.Info("Peer connected")
	} else {
		p.logger.Info("Peer disconnected")
	}
}

// IsConnected는 연결 상태를 반환합니다
func (p *Peer) IsConnected() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.connected
}

// Close는 피어 연결을 종료합니다 (여러 번 호출되어도 한 번만 실행됨)
func (p *Peer) Close() {
	p.closeOnce.Do(func() {
		p.logger.Info("Closing peer")

		p.SetConnected(false)

		if p.pc != nil {
			if err := p.pc.Close(); err != nil {
				p.logger.Error("Failed to close peer connection", zap.Error(err))
			}
		}

		p.ctxCancel()

		if p.onClose != nil {
			p.onClose(p.id)
		}
	})
}

// GetStats는 통계를 반환합니다
func (p *Peer) GetStats() (packetsSent, bytesSent uint64) {
	p.statsMutex.RLock()
	defer p.statsMutex.RUnlock()
	return p.packetsSent, p.bytesSent
}
