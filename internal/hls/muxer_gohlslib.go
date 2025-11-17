package hls

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/pion/rtp"
	"go.uber.org/zap"
)

// MuxerGoHLS는 gohlslib 기반 HLS Muxer
type MuxerGoHLS struct {
	streamID  string
	outputDir string
	logger    *zap.Logger
	config    *Config

	// gohlslib muxer
	muxer *gohlslib.Muxer
	track *gohlslib.Track

	// 코덱 정보
	videoCodec string // "H264" or "H265"
	sps        []byte
	pps        []byte
	vps        []byte // H265용

	// RTP depacketizer
	h264Depkt *H264Depacketizer
	h265Depkt *H265Depacketizer

	// 상태
	running     bool
	started     bool // muxer.Start() 호출 여부
	startFailed bool // muxer.Start() 실패 여부 (재시도 방지)
	ctx         context.Context
	cancel      context.CancelFunc
	mutex       sync.RWMutex
	wg          sync.WaitGroup

	// Timestamp 관리 (RTP timestamp 오버플로우 처리)
	lastTimestamp   uint32 // 마지막 RTP timestamp
	timestampOffset uint64 // 오버플로우 발생 시 누적 offset

	// 통계
	stats Stats
}

// NewMuxerGoHLS는 새로운 gohlslib 기반 Muxer 생성
func NewMuxerGoHLS(streamID string, logger *zap.Logger, config *Config) (*MuxerGoHLS, error) {
	outputDir := filepath.Join(config.OutputDir, streamID)

	// HLS 출력 디렉토리 생성 (자동 생성)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create HLS output directory: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &MuxerGoHLS{
		streamID:  streamID,
		outputDir: outputDir,
		logger:    logger,
		config:    config,
		ctx:       ctx,
		cancel:    cancel,
		h264Depkt: NewH264Depacketizer(),
		h265Depkt: NewH265Depacketizer(),
		stats: Stats{
			StreamID: streamID,
		},
	}

	return m, nil
}

// SetCodec은 비디오 코덱 설정
func (m *MuxerGoHLS) SetCodec(codec string, sps, pps, vps []byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.videoCodec = codec
	m.sps = sps
	m.pps = pps
	m.vps = vps

	m.logger.Info("Codec set for HLS muxer",
		zap.String("stream_id", m.streamID),
		zap.String("codec", codec),
		zap.Int("sps_size", len(sps)),
		zap.Int("pps_size", len(pps)),
	)

	return nil
}

// Start는 Muxer 시작
func (m *MuxerGoHLS) Start() error {
	m.mutex.Lock()
	if m.running {
		m.mutex.Unlock()
		return fmt.Errorf("muxer already running")
	}
	m.running = true
	m.mutex.Unlock()

	m.logger.Info("Starting gohlslib HLS muxer",
		zap.String("stream_id", m.streamID),
		zap.String("output_dir", m.outputDir),
	)

	// gohlslib Muxer 생성
	m.muxer = &gohlslib.Muxer{
		Variant:            gohlslib.MuxerVariantMPEGTS, // 표준 TS 세그먼트
		SegmentCount:       m.config.SegmentCount,
		SegmentMinDuration: time.Duration(m.config.SegmentDuration) * time.Second,
		Directory:          m.outputDir,
		OnEncodeError: func(err error) {
			m.logger.Error("HLS encode error",
				zap.String("stream_id", m.streamID),
				zap.Error(err),
			)
		},
	}

	// SPS/PPS가 있으면 바로 트랙 생성 및 시작
	if len(m.sps) > 0 && len(m.pps) > 0 {
		if err := m.createVideoTrack(); err != nil {
			return fmt.Errorf("failed to create video track: %w", err)
		}

		if err := m.muxer.Start(); err != nil {
			return fmt.Errorf("failed to start muxer: %w", err)
		}

		m.started = true

		m.logger.Info("gohlslib HLS muxer started successfully",
			zap.String("stream_id", m.streamID),
		)
	} else {
		m.logger.Info("gohlslib HLS muxer created, waiting for SPS/PPS",
			zap.String("stream_id", m.streamID),
		)
	}

	return nil
}

// createVideoTrack는 비디오 트랙 생성
func (m *MuxerGoHLS) createVideoTrack() error {
	// SPS/PPS가 없으면 스킵 (나중에 동적 생성)
	if len(m.sps) == 0 || len(m.pps) == 0 {
		return fmt.Errorf("SPS/PPS not available yet")
	}

	if m.videoCodec == "H264" {
		// H.264 트랙
		m.track = &gohlslib.Track{
			Codec: &codecs.H264{
				SPS: m.sps,
				PPS: m.pps,
			},
			ClockRate: 90000, // H.264 표준 clock rate
		}
		m.muxer.Tracks = append(m.muxer.Tracks, m.track)

		m.logger.Info("Created H.264 track for HLS",
			zap.String("stream_id", m.streamID),
		)
	} else if m.videoCodec == "H265" {
		// H.265 트랙
		m.track = &gohlslib.Track{
			Codec: &codecs.H265{
				VPS: m.vps,
				SPS: m.sps,
				PPS: m.pps,
			},
			ClockRate: 90000, // H.265 표준 clock rate
		}
		m.muxer.Tracks = append(m.muxer.Tracks, m.track)

		m.logger.Info("Created H.265 track for HLS",
			zap.String("stream_id", m.streamID),
		)
	} else {
		return fmt.Errorf("unsupported codec: %s", m.videoCodec)
	}

	return nil
}

// WriteRTPPacket은 RTP 패킷을 HLS로 변환
func (m *MuxerGoHLS) WriteRTPPacket(pkt *rtp.Packet) error {
	m.mutex.RLock()
	if !m.running {
		m.mutex.RUnlock()
		return nil
	}
	m.mutex.RUnlock()

	// RTP 디패킷화
	var nalUnits [][]byte
	var err error

	if m.videoCodec == "H265" {
		nalUnits, err = m.h265Depkt.Depacketize(pkt)
		if err != nil {
			return nil // 부분 패킷일 수 있으므로 무시
		}
	} else {
		nalUnits, err = m.h264Depkt.Depacketize(pkt)
		if err != nil {
			return nil // 부분 패킷일 수 있으므로 무시
		}
	}

	if len(nalUnits) == 0 {
		return nil
	}

	// SPS/PPS 동적 감지 (H.264)
	// startFailed가 true면 이미 시작 실패했으므로 재시도하지 않음
	if m.videoCodec == "H264" && m.track == nil && !m.startFailed {
		m.logger.Debug("Checking NAL units for SPS/PPS",
			zap.String("stream_id", m.streamID),
			zap.Int("nal_count", len(nalUnits)),
		)

		for _, nalUnit := range nalUnits {
			if len(nalUnit) == 0 {
				continue
			}
			nalType := nalUnit[0] & 0x1F

			m.logger.Debug("NAL unit detected",
				zap.String("stream_id", m.streamID),
				zap.Uint8("nal_type", nalType),
				zap.Int("size", len(nalUnit)),
			)

			if nalType == 7 { // SPS
				m.sps = make([]byte, len(nalUnit))
				copy(m.sps, nalUnit)
				m.logger.Info("Dynamically detected SPS",
					zap.String("stream_id", m.streamID),
					zap.Int("size", len(nalUnit)),
				)
			} else if nalType == 8 { // PPS
				m.pps = make([]byte, len(nalUnit))
				copy(m.pps, nalUnit)
				m.logger.Info("Dynamically detected PPS",
					zap.String("stream_id", m.streamID),
					zap.Int("size", len(nalUnit)),
				)
			}
		}

		// SPS와 PPS를 모두 감지했으면 트랙 생성
		if len(m.sps) > 0 && len(m.pps) > 0 {
			if err := m.createVideoTrack(); err != nil {
				m.logger.Error("Failed to create video track dynamically",
					zap.String("stream_id", m.streamID),
					zap.Error(err),
				)
				return err
			}

			// 트랙 생성 후 muxer 시작
			if err := m.muxer.Start(); err != nil {
				m.startFailed = true // 시작 실패 표시, 재시도 방지
				m.logger.Warn("HLS muxer start failed (expected for H.264-only limitations)",
					zap.String("stream_id", m.streamID),
					zap.String("codec", m.videoCodec),
					zap.Error(err),
				)
				return fmt.Errorf("failed to start muxer: %w", err)
			}

			m.started = true

			m.logger.Info("gohlslib HLS muxer started after dynamic track creation",
				zap.String("stream_id", m.streamID),
			)
		} else {
			// 아직 SPS/PPS를 못 찾았으면 스킵
			return nil
		}
	}

	// VPS/SPS/PPS 동적 감지 (H.265)
	// startFailed가 true면 이미 시작 실패했으므로 재시도하지 않음
	if m.videoCodec == "H265" && m.track == nil && !m.startFailed {
		m.logger.Debug("Checking NAL units for VPS/SPS/PPS",
			zap.String("stream_id", m.streamID),
			zap.Int("nal_count", len(nalUnits)),
		)

		for _, nalUnit := range nalUnits {
			if len(nalUnit) < 2 {
				continue
			}
			// H.265 NAL type is in bits 1-6 of the first byte
			nalType := (nalUnit[0] >> 1) & 0x3F

			m.logger.Debug("H.265 NAL unit detected",
				zap.String("stream_id", m.streamID),
				zap.Uint8("nal_type", nalType),
				zap.Int("size", len(nalUnit)),
			)

			if nalType == 32 { // VPS
				m.vps = make([]byte, len(nalUnit))
				copy(m.vps, nalUnit)
				m.logger.Info("Dynamically detected VPS",
					zap.String("stream_id", m.streamID),
					zap.Int("size", len(nalUnit)),
				)
			} else if nalType == 33 { // SPS
				m.sps = make([]byte, len(nalUnit))
				copy(m.sps, nalUnit)
				m.logger.Info("Dynamically detected SPS",
					zap.String("stream_id", m.streamID),
					zap.Int("size", len(nalUnit)),
				)
			} else if nalType == 34 { // PPS
				m.pps = make([]byte, len(nalUnit))
				copy(m.pps, nalUnit)
				m.logger.Info("Dynamically detected PPS",
					zap.String("stream_id", m.streamID),
					zap.Int("size", len(nalUnit)),
				)
			}
		}

		// VPS, SPS, PPS를 모두 감지했으면 트랙 생성
		if len(m.vps) > 0 && len(m.sps) > 0 && len(m.pps) > 0 {
			if err := m.createVideoTrack(); err != nil {
				m.logger.Error("Failed to create H.265 video track dynamically",
					zap.String("stream_id", m.streamID),
					zap.Error(err),
				)
				return err
			}

			// 트랙 생성 후 muxer 시작
			if err := m.muxer.Start(); err != nil {
				m.startFailed = true // 시작 실패 표시, 재시도 방지
				m.logger.Warn("HLS muxer start failed (H.265 not supported in MPEG-TS variant)",
					zap.String("stream_id", m.streamID),
					zap.String("codec", m.videoCodec),
					zap.Error(err),
				)
				return fmt.Errorf("failed to start muxer: %w", err)
			}

			m.started = true

			m.logger.Info("gohlslib HLS muxer started after H.265 dynamic track creation",
				zap.String("stream_id", m.streamID),
			)
		} else {
			// 아직 VPS/SPS/PPS를 못 찾았으면 스킵
			return nil
		}
	}

	// 트랙이 없거나 muxer가 시작되지 않았으면 스킵
	// (H.265 MPEG-TS 미지원으로 Start 실패한 경우 등)
	if m.track == nil || !m.started {
		return nil
	}

	// RTP timestamp 오버플로우 처리
	// RTP timestamp는 uint32이므로 2^32에서 오버플로우됨
	currentTS := pkt.Timestamp

	// 오버플로우 감지: 이전 timestamp가 크고 현재가 작으면 오버플로우
	// 차이가 2^31 (2147483648) 이상이면 오버플로우로 판단
	if m.lastTimestamp > 0 && currentTS < m.lastTimestamp {
		diff := m.lastTimestamp - currentTS
		if diff > (1 << 31) { // 2^31
			m.timestampOffset += (1 << 32) // 2^32 추가
			m.logger.Info("RTP timestamp overflow detected",
				zap.String("stream_id", m.streamID),
				zap.Uint32("last_ts", m.lastTimestamp),
				zap.Uint32("current_ts", currentTS),
				zap.Uint64("new_offset", m.timestampOffset),
			)
		}
	}
	m.lastTimestamp = currentTS

	// PTS 계산: RTP timestamp + offset (오버플로우 처리 포함)
	// Track.ClockRate = 90000으로 설정했으므로 변환 불필요 (mediaMTX 방식)
	pts := int64(m.timestampOffset + uint64(currentTS))
	ntp := time.Now()

	// H.264: IDR 프레임이 있으면 SPS/PPS를 앞에 추가
	if m.videoCodec == "H264" {
		hasIDR := false
		for _, nalUnit := range nalUnits {
			if len(nalUnit) > 0 && (nalUnit[0]&0x1F) == 5 { // IDR frame
				hasIDR = true
				break
			}
		}

		if hasIDR && len(m.sps) > 0 && len(m.pps) > 0 {
			// SPS/PPS를 NAL units 앞에 추가
			nalUnitsWithParams := make([][]byte, 0, len(nalUnits)+2)
			nalUnitsWithParams = append(nalUnitsWithParams, m.sps)
			nalUnitsWithParams = append(nalUnitsWithParams, m.pps)
			nalUnitsWithParams = append(nalUnitsWithParams, nalUnits...)
			nalUnits = nalUnitsWithParams

			m.logger.Debug("Prepended SPS/PPS to IDR frame",
				zap.String("stream_id", m.streamID),
			)
		}
	}

	// gohlslib로 NAL units 전달
	if m.videoCodec == "H264" {
		err = m.muxer.WriteH264(m.track, ntp, pts, nalUnits)
	} else if m.videoCodec == "H265" {
		err = m.muxer.WriteH265(m.track, ntp, pts, nalUnits)
	}

	if err != nil {
		m.logger.Error("Failed to write NAL units to HLS",
			zap.String("stream_id", m.streamID),
			zap.Error(err),
		)
		return err
	}

	// 통계 업데이트
	m.stats.TotalPackets++

	return nil
}

// Stop은 Muxer 중지
func (m *MuxerGoHLS) Stop() {
	m.mutex.Lock()
	if !m.running {
		m.mutex.Unlock()
		return
	}
	m.running = false
	m.mutex.Unlock()

	m.logger.Info("Stopping gohlslib HLS muxer",
		zap.String("stream_id", m.streamID),
	)

	// gohlslib Muxer 정리
	if m.muxer != nil {
		m.muxer.Close()
	}

	m.cancel()
	m.wg.Wait()

	m.logger.Info("gohlslib HLS muxer stopped",
		zap.String("stream_id", m.streamID),
	)
}

// GetStats는 통계 반환
func (m *MuxerGoHLS) GetStats() Stats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.stats
}

// GetStreamInfo는 스트림 정보 반환
func (m *MuxerGoHLS) GetStreamInfo() StreamInfo {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return StreamInfo{
		StreamID:       m.streamID,
		PlaylistURL:    fmt.Sprintf("/hls/%s/index.m3u8", m.streamID),
		CurrentSegment: 0,          // gohlslib가 관리
		StartTime:      time.Now(), // 임시
		LastUpdated:    time.Now(),
	}
}

// GetPlaylistPath는 플레이리스트 파일 경로 반환
func (m *MuxerGoHLS) GetPlaylistPath() string {
	return filepath.Join(m.outputDir, "index.m3u8")
}

// Handle는 HTTP 요청을 gohlslib muxer로 전달하여 플레이리스트 및 세그먼트 제공
func (m *MuxerGoHLS) Handle(w http.ResponseWriter, r *http.Request) {
	m.mutex.RLock()
	muxer := m.muxer
	started := m.started
	m.mutex.RUnlock()

	if muxer != nil && started {
		muxer.Handle(w, r)
	} else {
		http.Error(w, "Muxer not ready (waiting for SPS/PPS)", http.StatusServiceUnavailable)
	}
}
