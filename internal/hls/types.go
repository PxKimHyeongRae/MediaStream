package hls

import (
	"os"
	"time"

	"github.com/asticode/go-astits"
	"github.com/grafov/m3u8"
)

// Config는 HLS 설정
type Config struct {
	Enabled           bool   `yaml:"enabled"`
	SegmentDuration   int    `yaml:"segment_duration"`   // 세그먼트 길이 (초)
	SegmentCount      int    `yaml:"segment_count"`      // 플레이리스트에 유지할 세그먼트 수
	OutputDir         string `yaml:"output_dir"`         // 세그먼트 저장 디렉토리
	CleanupThreshold  int    `yaml:"cleanup_threshold"`  // 디스크에 유지할 최대 세그먼트 수
	EnableCompression bool   `yaml:"enable_compression"` // 세그먼트 압축 활성화
}

// Segment는 HLS 세그먼트 정보
type Segment struct {
	Index     int           // 세그먼트 인덱스
	Filename  string        // 파일명 (예: segment_0.ts)
	FilePath  string        // 파일 전체 경로
	File      *os.File      // 파일 핸들
	Duration  float64       // 세그먼트 길이 (초)
	StartTime time.Time     // 시작 시간
	EndTime   time.Time     // 종료 시간
	Muxer     *astits.Muxer // TS Muxer
	Size      int64         // 파일 크기 (bytes)
	Packets   int           // RTP 패킷 수
}

// StreamInfo는 HLS 스트림 정보
type StreamInfo struct {
	StreamID       string              // 스트림 ID
	PlaylistURL    string              // M3U8 플레이리스트 URL
	SegmentCount   int                 // 현재 세그먼트 수
	TotalDuration  float64             // 총 재생 시간 (초)
	CurrentSegment int                 // 현재 세그먼트 인덱스
	Bandwidth      int64               // 예상 대역폭 (bps)
	Playlist       *m3u8.MediaPlaylist // M3U8 플레이리스트
	StartTime      time.Time           // 스트림 시작 시간
	LastUpdated    time.Time           // 마지막 업데이트 시간
}

// Stats는 HLS 통계 정보
type Stats struct {
	StreamID          string    `json:"stream_id"`
	SegmentsGenerated int       `json:"segments_generated"`
	TotalBytes        int64     `json:"total_bytes"`
	TotalPackets      int       `json:"total_packets"`
	CurrentBitrate    int64     `json:"current_bitrate"` // bps
	AverageBitrate    int64     `json:"average_bitrate"` // bps
	Uptime            int64     `json:"uptime"`          // seconds
	LastSegmentTime   time.Time `json:"last_segment_time"`
}
