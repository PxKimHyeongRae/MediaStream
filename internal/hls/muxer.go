package hls

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/asticode/go-astits"
	"github.com/grafov/m3u8"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"go.uber.org/zap"
)

// Muxer는 RTP 패킷을 HLS 세그먼트로 변환
type Muxer struct {
	streamID string
	logger   *zap.Logger
	config   Config

	// 출력 설정
	outputDir    string
	playlistPath string

	// 세그먼트 관리
	currentSegment   *Segment
	segments         []*Segment
	segmentIndex     int
	segmentStartTime time.Time

	// M3U8 플레이리스트
	playlist      *m3u8.MediaPlaylist
	playlistMutex sync.RWMutex

	// 통계
	stats      Stats
	statsMutex sync.RWMutex

	// RTP 디패킷화
	h264Depacketizer *codecs.H264Packet
	h265Depacketizer *codecs.H265Packet
	customH264Depkt  *H264Depacketizer // Custom depacketizer
	customH265Depkt  *H265Depacketizer // Custom depacketizer

	// 코덱 정보
	videoCodec  string // "H264" or "H265"
	videoPID    uint16 // Video PID for TS
	patWritten  bool   // PAT/PMT written for current segment
	firstPacket bool   // First packet flag
	pts         uint64 // Presentation timestamp
	dts         uint64 // Decode timestamp

	// SPS/PPS 저장 (H.264)
	sps []byte
	pps []byte
	// VPS/SPS/PPS 저장 (H.265)
	vps []byte

	// 컨텍스트
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 상태
	running bool
	mutex   sync.RWMutex
}

// NewMuxer는 새로운 HLS Muxer를 생성
func NewMuxer(streamID string, config Config, logger *zap.Logger) (*Muxer, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// 출력 디렉토리 생성
	outputDir := filepath.Join(config.OutputDir, streamID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// M3U8 플레이리스트 생성
	playlist, err := m3u8.NewMediaPlaylist(uint(config.SegmentCount), uint(config.CleanupThreshold))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create playlist: %w", err)
	}

	m := &Muxer{
		streamID:         streamID,
		logger:           logger,
		config:           config,
		outputDir:        outputDir,
		playlistPath:     filepath.Join(outputDir, "index.m3u8"),
		playlist:         playlist,
		segments:         make([]*Segment, 0),
		h264Depacketizer: &codecs.H264Packet{},
		h265Depacketizer: &codecs.H265Packet{},
		customH264Depkt:  NewH264Depacketizer(), // Custom depacketizer
		customH265Depkt:  NewH265Depacketizer(), // Custom depacketizer
		videoPID:         256,                   // Standard video PID
		firstPacket:      true,
		ctx:              ctx,
		cancel:           cancel,
		stats: Stats{
			StreamID: streamID,
		},
	}

	return m, nil
}

// Start는 Muxer를 시작
func (m *Muxer) Start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return fmt.Errorf("muxer already running")
	}

	m.logger.Info("Starting HLS muxer",
		zap.String("stream_id", m.streamID),
		zap.String("output_dir", m.outputDir),
	)

	// 첫 번째 세그먼트 생성
	if err := m.createNewSegment(); err != nil {
		return fmt.Errorf("failed to create initial segment: %w", err)
	}

	m.running = true
	m.stats.LastSegmentTime = time.Now()

	// 세그먼트 로테이션 고루틴 시작
	m.wg.Add(1)
	go m.segmentRotationLoop()

	return nil
}

// Stop은 Muxer를 중지
func (m *Muxer) Stop() {
	m.mutex.Lock()
	if !m.running {
		m.mutex.Unlock()
		return
	}
	m.running = false
	m.mutex.Unlock()

	m.logger.Info("Stopping HLS muxer", zap.String("stream_id", m.streamID))

	// 컨텍스트 취소
	m.cancel()

	// 고루틴 종료 대기
	m.wg.Wait()

	// 현재 세그먼트 종료
	if m.currentSegment != nil {
		m.finalizeSegment(m.currentSegment)
	}

	// 최종 플레이리스트 저장
	m.savePlaylist()

	m.logger.Info("HLS muxer stopped",
		zap.String("stream_id", m.streamID),
		zap.Int("total_segments", m.stats.SegmentsGenerated),
	)
}

// WriteRTPPacket은 RTP 패킷을 TS 세그먼트로 변환
func (m *Muxer) WriteRTPPacket(pkt *rtp.Packet) error {
	m.mutex.RLock()
	if !m.running || m.currentSegment == nil {
		m.mutex.RUnlock()
		return fmt.Errorf("muxer not running or no current segment")
	}
	currentSeg := m.currentSegment
	m.mutex.RUnlock()

	// 통계 업데이트
	m.statsMutex.Lock()
	m.stats.TotalPackets++
	m.stats.TotalBytes += int64(len(pkt.Payload))
	m.statsMutex.Unlock()

	// 첫 패킷에서 코덱 감지 (payload type 기반)
	if m.firstPacket {
		// PayloadType 96-127은 동적 할당 (일반적으로 H264/H265)
		// 실제로는 SDP에서 정보를 가져와야 하지만, 일단 H264로 가정
		m.videoCodec = "H264"
		m.firstPacket = false

		m.logger.Info("Video codec detected",
			zap.String("stream_id", m.streamID),
			zap.String("codec", m.videoCodec),
			zap.Uint8("payload_type", pkt.PayloadType),
		)
	}

	// PAT/PMT 작성 (세그먼트당 한 번)
	// TODO: PAT/PMT 작성은 일단 스킵하고 PES만 테스트
	// if !m.patWritten {
	// 	if err := m.writePATandPMT(currentSeg); err != nil {
	// 		m.logger.Error("Failed to write PAT/PMT",
	// 			zap.String("stream_id", m.streamID),
	// 			zap.Error(err),
	// 		)
	// 		return fmt.Errorf("failed to write PAT/PMT: %w", err)
	// 	}
	// 	m.patWritten = true
	// }

	// RTP 디패킷화 (custom depacketizer 사용)
	var nalUnits [][]byte
	var err error

	if m.videoCodec == "H265" {
		nalUnits, err = m.customH265Depkt.Depacketize(pkt)
		if err != nil {
			// 에러는 로깅만 하고 계속 진행 (부분 패킷일 수 있음)
			m.logger.Debug("H265 depacketization skipped",
				zap.String("stream_id", m.streamID),
				zap.Error(err),
			)
			currentSeg.Packets++
			return nil
		}
	} else {
		nalUnits, err = m.customH264Depkt.Depacketize(pkt)
		if err != nil {
			// 에러는 로깅만 하고 계속 진행 (부분 패킷일 수 있음)
			m.logger.Debug("H264 depacketization skipped",
				zap.String("stream_id", m.streamID),
				zap.Error(err),
			)
			currentSeg.Packets++
			return nil
		}
	}

	// NAL units가 없으면 스킵 (FU-A 중간 패킷일 수 있음)
	if len(nalUnits) == 0 {
		currentSeg.Packets++
		return nil
	}

	// PTS/DTS 계산 (RTP timestamp 기반)
	// 90kHz clock (MPEG-TS standard)
	m.pts = uint64(pkt.Timestamp)
	m.dts = m.pts

	// NAL units 처리 및 SPS/PPS 저장
	var annexBData []byte
	var hasIDR bool

	for _, nalUnit := range nalUnits {
		if len(nalUnit) == 0 {
			continue
		}

		nalType := nalUnit[0] & 0x1F // H.264 NAL type

		// SPS/PPS 저장
		if nalType == 7 { // SPS
			m.sps = make([]byte, len(nalUnit))
			copy(m.sps, nalUnit)
			m.logger.Info("SPS detected and saved",
				zap.String("stream_id", m.streamID),
				zap.Int("size", len(nalUnit)),
			)
		} else if nalType == 8 { // PPS
			m.pps = make([]byte, len(nalUnit))
			copy(m.pps, nalUnit)
			m.logger.Info("PPS detected and saved",
				zap.String("stream_id", m.streamID),
				zap.Int("size", len(nalUnit)),
			)
		} else if nalType == 5 { // IDR (keyframe)
			hasIDR = true
			m.logger.Info("IDR (keyframe) detected",
				zap.String("stream_id", m.streamID),
				zap.Bool("has_sps", len(m.sps) > 0),
				zap.Bool("has_pps", len(m.pps) > 0),
			)
		}
	}

	// IDR 프레임이면 SPS/PPS를 앞에 추가
	if hasIDR && len(m.sps) > 0 && len(m.pps) > 0 {
		// SPS 추가
		annexBData = append(annexBData, 0x00, 0x00, 0x00, 0x01)
		annexBData = append(annexBData, m.sps...)
		// PPS 추가
		annexBData = append(annexBData, 0x00, 0x00, 0x00, 0x01)
		annexBData = append(annexBData, m.pps...)
	}

	// 모든 NAL units를 Annex B 형식으로 추가
	// 단, SPS/PPS는 스킵 (IDR 앞에만 추가되어야 함)
	for _, nalUnit := range nalUnits {
		if len(nalUnit) == 0 {
			continue
		}

		nalType := nalUnit[0] & 0x1F // H.264 NAL type

		// SPS(7)와 PPS(8)는 스킵 - IDR 앞에만 prepend됨
		if nalType == 7 || nalType == 8 {
			continue
		}

		// Annex B start code 추가
		annexBData = append(annexBData, 0x00, 0x00, 0x00, 0x01)
		annexBData = append(annexBData, nalUnit...)
	}

	if len(annexBData) == 0 {
		currentSeg.Packets++
		return nil
	}

	// PES 패킷 생성
	pesData := &astits.PESData{
		Header: &astits.PESHeader{
			OptionalHeader: &astits.PESOptionalHeader{
				MarkerBits:      2,
				PTSDTSIndicator: astits.PTSDTSIndicatorBothPresent,
				DTS:             &astits.ClockReference{Base: int64(m.dts)},
				PTS:             &astits.ClockReference{Base: int64(m.pts)},
			},
			StreamID: 224, // Video stream (0xE0)
		},
		Data: annexBData,
	}

	// TS에 PES 쓰기
	muxerData := &astits.MuxerData{
		PID: m.videoPID,
		PES: pesData,
	}

	if _, err := currentSeg.Muxer.WriteData(muxerData); err != nil {
		m.logger.Error("Failed to write PES data",
			zap.String("stream_id", m.streamID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to write PES: %w", err)
	}

	currentSeg.Packets++
	return nil
}

// writePATandPMT는 PAT와 PMT 테이블을 작성
// TODO: astits API 확인 후 재구현 필요
/*
func (m *Muxer) writePATandPMT(segment *Segment) error {
	// PAT (Program Association Table)
	pat := &astits.PATData{
		Programs: []*astits.PATProgram{
			{
				ProgramMapID:  4096, // PMT PID
				ProgramNumber: 1,
			},
		},
		TransportStreamID: 1,
	}

	patMuxerData := &astits.MuxerData{
		PID: astits.PIDPAT,
		PAT: pat,
	}

	if _, err := segment.Muxer.WriteData(patMuxerData); err != nil {
		return fmt.Errorf("failed to write PAT: %w", err)
	}

	// PMT (Program Map Table)
	streamType := astits.StreamTypeH264Video
	if m.videoCodec == "H265" {
		streamType = astits.StreamTypeH265Video
	}

	pmt := &astits.PMTData{
		ProgramNumber: 1,
		PCRPID:        m.videoPID,
		ElementaryStreams: []*astits.PMTElementaryStream{
			{
				ElementaryPID: m.videoPID,
				StreamType:    streamType,
			},
		},
	}

	pmtMuxerData := &astits.MuxerData{
		PID: 4096, // PMT PID
		PMT: pmt,
	}

	if _, err := segment.Muxer.WriteData(pmtMuxerData); err != nil {
		return fmt.Errorf("failed to write PMT: %w", err)
	}

	m.logger.Debug("PAT/PMT written",
		zap.String("stream_id", m.streamID),
		zap.String("codec", m.videoCodec),
	)

	return nil
}
*/

// createNewSegment는 새로운 세그먼트를 생성
func (m *Muxer) createNewSegment() error {
	filename := fmt.Sprintf("segment_%d.ts", m.segmentIndex)
	filePath := filepath.Join(m.outputDir, filename)

	m.logger.Debug("Creating new segment",
		zap.String("stream_id", m.streamID),
		zap.String("filename", filename),
		zap.Int("index", m.segmentIndex),
	)

	// 파일 생성 (파일을 열린 상태로 유지)
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create segment file: %w", err)
	}

	// TS Muxer 생성
	muxer := astits.NewMuxer(context.Background(), file)

	// Video PID를 muxer에 등록 (PAT/PMT 대신)
	streamType := astits.StreamTypeH264Video
	if m.videoCodec == "H265" {
		streamType = astits.StreamTypeH265Video
	}
	muxer.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: m.videoPID,
		StreamType:    streamType,
	})

	// PAT/PMT 자동 설정
	muxer.SetPCRPID(m.videoPID)

	segment := &Segment{
		Index:     m.segmentIndex,
		Filename:  filename,
		FilePath:  filePath,
		File:      file, // 파일 핸들 저장
		StartTime: time.Now(),
		Muxer:     muxer,
		Packets:   0,
	}

	m.currentSegment = segment
	m.segmentStartTime = time.Now()
	m.segmentIndex++
	m.patWritten = false // 새 세그먼트마다 PAT/PMT 재작성 필요

	// 세그먼트 시작 시 SPS/PPS 작성 (H.264)
	if m.videoCodec == "H264" && len(m.sps) > 0 && len(m.pps) > 0 {
		if err := m.writeSPSPPSToSegment(segment); err != nil {
			m.logger.Warn("Failed to write SPS/PPS to segment start",
				zap.String("stream_id", m.streamID),
				zap.Error(err),
			)
		} else {
			m.logger.Info("Wrote SPS/PPS to segment start",
				zap.String("stream_id", m.streamID),
				zap.String("segment", segment.Filename),
			)
		}
	}

	return nil
}

// writeSPSPPSToSegment는 세그먼트 시작 시 SPS/PPS를 작성
func (m *Muxer) writeSPSPPSToSegment(segment *Segment) error {
	// Annex B 형식으로 SPS/PPS 작성
	var annexBData []byte

	// SPS 추가
	annexBData = append(annexBData, 0x00, 0x00, 0x00, 0x01)
	annexBData = append(annexBData, m.sps...)

	// PPS 추가
	annexBData = append(annexBData, 0x00, 0x00, 0x00, 0x01)
	annexBData = append(annexBData, m.pps...)

	// PES 패킷 생성
	pesData := &astits.PESData{
		Header: &astits.PESHeader{
			OptionalHeader: &astits.PESOptionalHeader{
				MarkerBits:      2,
				PTSDTSIndicator: astits.PTSDTSIndicatorBothPresent,
				DTS:             &astits.ClockReference{Base: int64(m.dts)},
				PTS:             &astits.ClockReference{Base: int64(m.pts)},
			},
			StreamID: 224, // Video stream (0xE0)
		},
		Data: annexBData,
	}

	// TS에 PES 쓰기
	muxerData := &astits.MuxerData{
		PID: m.videoPID,
		PES: pesData,
	}

	if _, err := segment.Muxer.WriteData(muxerData); err != nil {
		return fmt.Errorf("failed to write SPS/PPS: %w", err)
	}

	return nil
}

// finalizeSegment는 세그먼트를 종료하고 플레이리스트에 추가
func (m *Muxer) finalizeSegment(segment *Segment) {
	segment.EndTime = time.Now()
	segment.Duration = segment.EndTime.Sub(segment.StartTime).Seconds()

	// 파일 닫기
	if segment.File != nil {
		segment.File.Close()
		segment.File = nil
	}

	// 파일 크기 확인
	if info, err := os.Stat(segment.FilePath); err == nil {
		segment.Size = info.Size()
	}

	m.logger.Debug("Finalizing segment",
		zap.String("stream_id", m.streamID),
		zap.String("filename", segment.Filename),
		zap.Float64("duration", segment.Duration),
		zap.Int64("size", segment.Size),
		zap.Int("packets", segment.Packets),
	)

	// 플레이리스트에 추가
	m.playlistMutex.Lock()
	m.playlist.Append(segment.Filename, segment.Duration, "")
	m.playlistMutex.Unlock()

	// 세그먼트 목록에 추가
	m.segments = append(m.segments, segment)

	// 통계 업데이트
	m.statsMutex.Lock()
	m.stats.SegmentsGenerated++
	m.stats.LastSegmentTime = time.Now()
	m.statsMutex.Unlock()

	// 플레이리스트 저장
	m.savePlaylist()

	// 오래된 세그먼트 정리
	m.cleanupOldSegments()
}

// savePlaylist는 M3U8 플레이리스트를 파일로 저장
func (m *Muxer) savePlaylist() {
	m.playlistMutex.RLock()
	defer m.playlistMutex.RUnlock()

	file, err := os.Create(m.playlistPath)
	if err != nil {
		m.logger.Error("Failed to create playlist file",
			zap.String("stream_id", m.streamID),
			zap.Error(err),
		)
		return
	}
	defer file.Close()

	if _, err := m.playlist.Encode().WriteTo(file); err != nil {
		m.logger.Error("Failed to write playlist",
			zap.String("stream_id", m.streamID),
			zap.Error(err),
		)
		return
	}

	m.logger.Debug("Playlist saved",
		zap.String("stream_id", m.streamID),
		zap.String("path", m.playlistPath),
	)
}

// segmentRotationLoop는 세그먼트 로테이션을 처리
func (m *Muxer) segmentRotationLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(time.Duration(m.config.SegmentDuration) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.rotateSegment()
		case <-m.ctx.Done():
			return
		}
	}
}

// rotateSegment는 현재 세그먼트를 종료하고 새 세그먼트를 시작
func (m *Muxer) rotateSegment() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.currentSegment == nil {
		return
	}

	m.logger.Debug("Rotating segment",
		zap.String("stream_id", m.streamID),
		zap.Int("current_index", m.currentSegment.Index),
	)

	// 현재 세그먼트 종료
	m.finalizeSegment(m.currentSegment)

	// 새 세그먼트 생성
	if err := m.createNewSegment(); err != nil {
		m.logger.Error("Failed to create new segment after rotation",
			zap.String("stream_id", m.streamID),
			zap.Error(err),
		)
	}
}

// cleanupOldSegments는 오래된 세그먼트를 삭제
func (m *Muxer) cleanupOldSegments() {
	if len(m.segments) <= m.config.CleanupThreshold {
		return
	}

	// 삭제할 세그먼트 수
	deleteCount := len(m.segments) - m.config.CleanupThreshold

	m.logger.Debug("Cleaning up old segments",
		zap.String("stream_id", m.streamID),
		zap.Int("delete_count", deleteCount),
	)

	for i := 0; i < deleteCount; i++ {
		segment := m.segments[i]
		filePath := filepath.Join(m.outputDir, segment.Filename)

		if err := os.Remove(filePath); err != nil {
			m.logger.Warn("Failed to remove old segment",
				zap.String("stream_id", m.streamID),
				zap.String("filename", segment.Filename),
				zap.Error(err),
			)
		}
	}

	// 세그먼트 목록 업데이트
	m.segments = m.segments[deleteCount:]
}

// GetStats는 현재 통계를 반환
func (m *Muxer) GetStats() Stats {
	m.statsMutex.RLock()
	defer m.statsMutex.RUnlock()

	stats := m.stats
	stats.Uptime = int64(time.Since(m.stats.LastSegmentTime).Seconds())

	// 평균 비트레이트 계산
	if stats.Uptime > 0 {
		stats.AverageBitrate = (stats.TotalBytes * 8) / stats.Uptime
	}

	return stats
}

// GetPlaylistPath는 플레이리스트 파일 경로를 반환
func (m *Muxer) GetPlaylistPath() string {
	return m.playlistPath
}

// GetStreamInfo는 스트림 정보를 반환
func (m *Muxer) GetStreamInfo() StreamInfo {
	m.playlistMutex.RLock()
	m.statsMutex.RLock()
	defer m.playlistMutex.RUnlock()
	defer m.statsMutex.RUnlock()

	return StreamInfo{
		StreamID:       m.streamID,
		PlaylistURL:    fmt.Sprintf("/hls/%s/index.m3u8", m.streamID),
		SegmentCount:   len(m.segments),
		TotalDuration:  float64(m.stats.SegmentsGenerated * m.config.SegmentDuration),
		CurrentSegment: m.segmentIndex - 1,
		Bandwidth:      m.stats.AverageBitrate,
		Playlist:       m.playlist,
		StartTime:      m.stats.LastSegmentTime,
		LastUpdated:    time.Now(),
	}
}
