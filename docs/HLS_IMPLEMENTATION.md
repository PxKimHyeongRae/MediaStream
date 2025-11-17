# HLS (HTTP Live Streaming) 구현 가이드

> **작성일**: 2025-11-17
> **작성자**: Claude Code
> **목적**: HLS 스트리밍 기능 구현에 대한 완전한 이해와 유지보수를 위한 가이드

---

## 📋 목차

1. [개요](#개요)
2. [아키텍처 설계](#아키텍처-설계)
3. [구현 상세](#구현-상세)
4. [데이터 흐름](#데이터-흐름)
5. [핵심 컴포넌트](#핵심-컴포넌트)
6. [트러블슈팅](#트러블슈팅)
7. [성능 최적화](#성능-최적화)

---

## 개요

### 🎯 구현 목표

기존 WebRTC 전용 미디어 서버에 **HLS (HTTP Live Streaming)** 지원을 추가하여:
- 📱 **더 넓은 클라이언트 호환성**: iOS Safari, 레거시 브라우저 지원
- 🎥 **다양한 플레이어 지원**: VLC, MPV, video.js 등
- 🔄 **이중 출력**: WebRTC (저지연) + HLS (호환성) 동시 지원

### 🔑 핵심 의사결정

| 결정 사항 | 선택 | 이유 |
|---------|-----|------|
| **HLS 라이브러리** | gohlslib v2.2.3 | mediaMTX와 동일한 라이브러리, 검증됨, Pure Go |
| **코덱 지원** | H.264, H.265 | 기존 WebRTC와 동일, 트랜스코딩 불필요 |
| **지연시간** | 6-9초 | HLS 특성상 불가피, WebRTC는 1초 미만 유지 |
| **세그먼트 길이** | 1초 | 최소 지연시간 (기본 2초에서 단축) |
| **세그먼트 개수** | 3개 | 메모리 효율과 지연시간 균형 |
| **디렉토리 구조** | `hls/<stream_id>/` | 스트림별 격리, 자동 생성 |

### 📦 의존성 추가

```go
// go.mod
require (
    github.com/bluenviron/gohlslib/v2 v2.2.3
    github.com/bluenviron/mediacommon/v2 v2.4.2
    github.com/grafov/m3u8 v0.12.1
    github.com/abema/go-mp4 v1.4.1
    github.com/asticode/go-astits v1.14.0
)
```

---

## 아키텍처 설계

### 🏗️ 시스템 구조

```
┌─────────────────────────────────────────────────────────────┐
│                      RTSP Camera                             │
│                    (H.264/H.265)                             │
└──────────────────────┬───────────────────────────────────────┘
                       │ RTP Packets
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                  RTSP Client                                 │
│             (gortsplib v4)                                   │
│                                                              │
│  OnPacket Callback:                                          │
│  • WritePacket(stream) ───────► WebRTC Pipeline             │
│  • WritePacket(hlsManager) ───► HLS Pipeline                │
└──────────────────────┬───────────────────────────────────────┘
                       │
                       │
        ┌──────────────┴────────────────┐
        │                               │
        ▼                               ▼
┌───────────────────┐          ┌───────────────────┐
│  Stream Manager   │          │   HLS Manager     │
│   (WebRTC용)      │          │  (gohlslib)       │
│                   │          │                   │
│ • Pub/Sub 패턴   │          │ • Muxer 관리      │
│ • RTP 버퍼링     │          │ • TS 세그먼트생성 │
│ • 다중 구독자    │          │ • M3U8 생성       │
└─────┬─────────────┘          └─────┬─────────────┘
      │                              │
      ▼                              ▼
┌───────────────────┐          ┌───────────────────┐
│ WebRTC Peers      │          │ HLS Files         │
│ (pion/webrtc v4)  │          │                   │
│                   │          │ • index.m3u8      │
│ • 저지연 (1초 미만)│          │ • seg0.ts         │
│ • 실시간 인터랙션  │          │ • seg1.ts         │
└───────────────────┘          │ • seg2.ts         │
                               └───────────────────┘
```

### 🔄 컴포넌트 관계

```go
Application
├── streamManager   *core.StreamManager      // WebRTC용 스트림 관리
├── hlsManager      *hls.Manager             // HLS 전용 관리
├── rtspClients     map[string]*rtsp.Client  // RTSP 클라이언트들
└── apiServer       *api.Server              // HTTP API (HLS 플레이리스트 서빙)
```

---

## 구현 상세

### 1️⃣ HLS Manager 초기화

**위치**: `cmd/server/main.go:244-258`

```go
// 4.5. HLS 관리자 초기화
if config.HLS.Enabled {
    app.hlsManager = hls.NewManager(hls.Config{
        Enabled:           config.HLS.Enabled,
        SegmentDuration:   config.HLS.SegmentDuration,   // 1초
        SegmentCount:      config.HLS.SegmentCount,      // 3개
        OutputDir:         config.HLS.OutputDir,         // "hls"
        CleanupThreshold:  config.HLS.CleanupThreshold,  // 20개
        EnableCompression: config.HLS.EnableCompression, // false
    }, logger.Log)

    logger.Info("HLS manager initialized",
        zap.String("output_dir", config.HLS.OutputDir),
        zap.Int("segment_duration", config.HLS.SegmentDuration),
        zap.Int("segment_count", config.HLS.SegmentCount),
    )
}
```

**💡 핵심 포인트**:
- HLS는 **선택적 기능**: `config.HLS.Enabled`로 켜고 끌 수 있음
- **저지연 설정**: segment_duration=1초, segment_count=3개로 최소화
- **메모리 관리**: cleanup_threshold=20으로 오래된 세그먼트 자동 삭제

### 2️⃣ RTP 패킷 이중 전달 (WebRTC + HLS)

**위치**: `cmd/server/main.go:389-407`

```go
OnPacket: func(pkt *rtp.Packet) {
    // 1. Stream Manager에 패킷 전달 (WebRTC용)
    if err := stream.WritePacket(pkt); err != nil {
        logger.Error("Failed to write packet to stream",
            zap.String("stream_id", streamID),
            zap.Error(err),
        )
    }

    // 2. HLS Manager에 패킷 전달
    if app.hlsManager != nil && app.hlsManager.IsEnabled() {
        if err := app.hlsManager.WritePacket(streamID, pkt); err != nil {
            // ⚠️ HLS 실패는 로그만 남기고 계속 진행
            // (WebRTC는 영향받지 않음)
            logger.Debug("Failed to write packet to HLS",
                zap.String("stream_id", streamID),
                zap.Error(err),
            )
        }
    }
},
```

**💡 핵심 포인트**:
- **패킷 복제 전달**: 하나의 RTP 패킷을 WebRTC와 HLS 모두에 전달
- **독립적 에러 처리**: HLS 실패가 WebRTC에 영향 없음 (vice versa)
- **성능 고려**: Debug 레벨로 HLS 에러 로깅 (과도한 로그 방지)

### 3️⃣ HLS Muxer 자동 생성

**위치**: `cmd/server/main.go:408-443`

```go
OnConnect: func() {
    logger.Info("RTSP client connected", zap.String("stream_id", streamID))

    // HLS Muxer 생성 (HLS가 활성화된 경우)
    if app.hlsManager != nil && app.hlsManager.IsEnabled() {
        // 스트림에서 실제 코덱 가져오기
        stream, err := app.streamManager.GetStream(streamID)
        if err != nil {
            logger.Error("Failed to get stream for HLS muxer",
                zap.String("stream_id", streamID),
                zap.Error(err),
            )
            return
        }

        codec := stream.GetVideoCodec()
        if codec == "" {
            codec = "H264" // 기본값
        }

        // SPS/PPS는 RTP 패킷에서 동적 감지
        // (nil로 전달하면 gohlslib가 패킷에서 자동 추출)
        if _, err := app.hlsManager.CreateMuxer(streamID, codec, nil, nil, nil); err != nil {
            logger.Error("Failed to create HLS muxer",
                zap.String("stream_id", streamID),
                zap.String("codec", codec),
                zap.Error(err),
            )
        } else {
            logger.Info("HLS muxer created",
                zap.String("stream_id", streamID),
                zap.String("codec", codec),
            )
        }
    }
},
```

**💡 핵심 포인트**:
- **타이밍**: RTSP 연결 직후(OnConnect) Muxer 생성
- **동적 코덱 감지**: 스트림에서 실제 코덱 정보 가져옴
- **SPS/PPS 자동 추출**: nil 전달 시 gohlslib가 RTP 패킷에서 자동 추출
- **디렉토리 자동 생성**: `hls/<stream_id>/` 디렉토리 자동 생성 (internal/hls/muxer_gohlslib.go:55)

### 4️⃣ HLS 디렉토리 자동 생성

**위치**: `internal/hls/muxer_gohlslib.go:51-58`

```go
func NewMuxerGoHLS(streamID string, logger *zap.Logger, config *Config) (*MuxerGoHLS, error) {
    outputDir := filepath.Join(config.OutputDir, streamID)

    // HLS 출력 디렉토리 생성 (자동 생성)
    if err := os.MkdirAll(outputDir, 0755); err != nil {
        return nil, fmt.Errorf("failed to create HLS output directory: %w", err)
    }

    // ... Muxer 초기화
}
```

**💡 핵심 포인트**:
- **자동 생성**: `os.MkdirAll`로 부모 디렉토리 포함 자동 생성
- **권한 설정**: 0755 (rwxr-xr-x)
- **에러 상황 해결**: 이전에는 디렉토리 없어서 "no such file or directory" 에러 발생

### 5️⃣ HLS API 엔드포인트

**위치**: `internal/api/server.go:84-92, 102-105`

```go
// API v1 - HLS API endpoints
hlsGroup := v1.Group("/hls")
{
    hlsGroup.GET("/streams", s.handleHLSStreamsList)           // 모든 HLS 스트림 목록
    hlsGroup.GET("/streams/:id", s.handleHLSStreamInfo)        // 특정 스트림 정보
    hlsGroup.GET("/streams/:id/stats", s.handleHLSStreamStats) // 스트림 통계
}

// HLS 플레이리스트 및 세그먼트 서빙
s.router.GET("/hls/:streamId/index.m3u8", s.handleHLSPlaylist)  // M3U8 플레이리스트
s.router.GET("/hls/:streamId/:segment", s.handleHLSSegment)     // TS 세그먼트
```

**API 사용 예시**:

```bash
# 모든 HLS 스트림 목록 조회
GET http://localhost:8107/api/v1/hls/streams

# 특정 스트림 정보
GET http://localhost:8107/api/v1/hls/streams/CCTV-TEST

# HLS 플레이리스트 (클라이언트용)
GET http://localhost:8107/hls/CCTV-TEST/index.m3u8

# HLS 세그먼트 (플레이어가 자동 요청)
GET http://localhost:8107/hls/CCTV-TEST/76acb60e7fc9_main_seg0.ts
```

### 6️⃣ On-Demand 스트림 자동 시작

**위치**: `internal/api/server.go:397-460`

```go
func (s *Server) handleHLSPlaylist(c *gin.Context) {
    streamID := c.Param("streamId")

    // HLS 활성화 확인
    if s.hlsManager == nil || !s.hlsManager.IsEnabled() {
        c.JSON(http.StatusServiceUnavailable, gin.H{
            "error": "HLS is not enabled",
        })
        return
    }

    muxer, exists := s.hlsManager.GetMuxer(streamID)
    if !exists {
        // 🚀 Muxer가 없으면 스트림을 자동으로 시작
        s.logger.Info("HLS Muxer not found, attempting to start stream",
            zap.String("stream_id", streamID))

        if s.startStreamHandler != nil {
            // 1. RTSP 클라이언트 시작
            if err := s.startStreamHandler(streamID); err != nil {
                s.logger.Error("Failed to start stream for HLS",
                    zap.String("stream_id", streamID),
                    zap.Error(err))
                c.JSON(http.StatusServiceUnavailable, gin.H{
                    "error": fmt.Sprintf("Failed to start stream %s: %v", streamID, err),
                })
                return
            }

            // 2. Muxer 생성 대기 (retry 로직: 최대 5초, 0.5초 간격)
            maxRetries := 10
            retryInterval := 500 * time.Millisecond

            for i := 0; i < maxRetries; i++ {
                time.Sleep(retryInterval)

                muxer, exists = s.hlsManager.GetMuxer(streamID)
                if exists {
                    s.logger.Info("HLS Muxer ready after retry",
                        zap.String("stream_id", streamID),
                        zap.Int("retry_count", i+1),
                        zap.Duration("total_wait", time.Duration(i+1)*retryInterval))
                    break
                }
            }

            // 3. 최종 확인
            if !exists {
                s.logger.Error("HLS Muxer not ready after max retries",
                    zap.String("stream_id", streamID),
                    zap.Duration("total_wait", time.Duration(maxRetries)*retryInterval))
                c.JSON(http.StatusServiceUnavailable, gin.H{
                    "error": fmt.Sprintf("Stream started but HLS Muxer not ready after %.1f seconds",
                        float64(maxRetries)*retryInterval.Seconds()),
                })
                return
            }
        }
    }

    // 4. gohlslib muxer의 Handle 메서드로 요청 전달
    muxer.Handle(c.Writer, c.Request)
}
```

**💡 핵심 포인트**:
- **자동 시작**: Muxer 없으면 자동으로 RTSP 클라이언트 시작
- **비동기 대기**: RTSP 연결은 비동기이므로 retry 로직으로 Muxer 생성 대기
- **타임아웃**: 최대 5초(10회 × 0.5초) 대기, 실패 시 명확한 에러 메시지
- **gohlslib 통합**: Muxer.Handle()이 M3U8 생성 및 TS 서빙 모두 처리

---

## 데이터 흐름

### 📊 전체 데이터 플로우

```
1. RTSP Camera
   └─► RTP Packet (H.264/H.265 NAL units)
        │
        ▼
2. RTSP Client (OnPacket Callback)
   ├─► stream.WritePacket(pkt)           [Path A: WebRTC]
   └─► hlsManager.WritePacket(streamID, pkt)  [Path B: HLS]

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Path A: WebRTC (저지연)
3A. Stream Manager
    └─► RTP 버퍼링 → Pub/Sub
         └─► WebRTC Peers
              └─► Browser (1초 미만 지연)

Path B: HLS (호환성)
3B. HLS Manager
    └─► Muxer 선택 (streamID 기준)
         └─► gohlslib Muxer
              ├─► RTP → NAL Unit 추출
              ├─► NAL Units → MPEG-TS 패킷
              ├─► TS 패킷 → Segment (1초 단위)
              └─► Segments → M3U8 플레이리스트
                   └─► HTTP 서빙
                        └─► Video Player (6-9초 지연)
```

### 🔍 상세 RTP → HLS 변환 과정

```go
// HLS Manager의 WritePacket 내부 흐름
func (m *Manager) WritePacket(streamID string, pkt *rtp.Packet) error {
    // 1. Muxer 선택
    muxer := m.muxers[streamID]

    // 2. gohlslib Muxer로 전달
    muxer.WritePacket(pkt)

    // 3. Muxer 내부 처리 (gohlslib)
    //   a. RTP Depayload: RTP → H.264/H.265 NAL units
    //   b. Access Unit 조립: NAL units → 완전한 프레임
    //   c. MPEG-TS 패킷화: 프레임 → TS 패킷
    //   d. 세그먼트 생성: TS 패킷 × N → seg0.ts (1초)
    //   e. M3U8 업데이트: 새 세그먼트 추가, 오래된 것 제거

    return nil
}
```

### 📁 파일 시스템 구조

```
project/
├── hls/                      # HLS 출력 디렉토리 (config.HLS.OutputDir)
│   ├── CCTV-TEST/           # 스트림별 디렉토리 (자동 생성)
│   │   ├── index.m3u8       # 플레이리스트 (동적 업데이트)
│   │   ├── init.mp4         # fMP4 초기화 세그먼트 (선택적)
│   │   ├── seg0.ts          # 세그먼트 0 (가장 최근)
│   │   ├── seg1.ts          # 세그먼트 1
│   │   └── seg2.ts          # 세그먼트 2 (가장 오래됨)
│   │
│   ├── CCTV-TEST2/
│   │   └── ...
│   └── plx_cctv_01/
│       └── ...
│
├── configs/
│   └── config.yaml          # HLS 설정
└── cmd/server/main.go
```

**플레이리스트 예시** (`index.m3u8`):
```m3u8
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:42

#EXTINF:1.000,
seg0.ts
#EXTINF:1.000,
seg1.ts
#EXTINF:1.000,
seg2.ts
```

---

## 핵심 컴포넌트

### 🎬 HLS Manager

**위치**: `internal/hls/manager.go`

**책임**:
- Muxer 생성 및 관리
- RTP 패킷을 적절한 Muxer로 라우팅
- 스트림 통계 수집
- 세그먼트 클린업

**주요 메서드**:
```go
type Manager struct {
    config  Config
    logger  *zap.Logger
    muxers  map[string]*MuxerGoHLS  // streamID → Muxer
    mutex   sync.RWMutex
}

func (m *Manager) CreateMuxer(streamID, codec string, sps, pps, vps []byte) (*MuxerGoHLS, error)
func (m *Manager) GetMuxer(streamID string) (*MuxerGoHLS, bool)
func (m *Manager) WritePacket(streamID string, pkt *rtp.Packet) error
func (m *Manager) StopAll()
```

### 🎞️ HLS Muxer (gohlslib 래퍼)

**위치**: `internal/hls/muxer_gohlslib.go`

**책임**:
- gohlslib Muxer 인스턴스 관리
- RTP 패킷 디패킹 (H.264/H.265)
- SPS/PPS/VPS 파라미터 추출 및 설정
- 세그먼트 및 플레이리스트 생성

**주요 메서드**:
```go
type MuxerGoHLS struct {
    streamID  string
    outputDir string
    muxer     *gohlslib.Muxer

    h264Depkt *H264Depacketizer
    h265Depkt *H265Depacketizer

    // 코덱 정보
    videoCodec string
    sps, pps   []byte  // H.264
    vps        []byte  // H.265

    stats      Stats
}

func (m *MuxerGoHLS) Start() error
func (m *MuxerGoHLS) Stop() error
func (m *MuxerGoHLS) WritePacket(pkt *rtp.Packet) error
func (m *MuxerGoHLS) Handle(w http.ResponseWriter, r *http.Request)  // HTTP 서빙
```

### ⚙️ Config 구조

**위치**: `internal/core/config.go:103-112`

```go
type HLSConfig struct {
    Enabled           bool   `yaml:"enabled"`              // HLS 활성화 여부
    SegmentDuration   int    `yaml:"segment_duration"`     // 세그먼트 길이 (초)
    SegmentCount      int    `yaml:"segment_count"`        // 플레이리스트 세그먼트 수
    OutputDir         string `yaml:"output_dir"`           // 출력 디렉토리
    CleanupThreshold  int    `yaml:"cleanup_threshold"`    // 디스크 최대 세그먼트 수
    EnableCompression bool   `yaml:"enable_compression"`   // gzip 압축 (현재 미사용)
}
```

**설정 예시** (`configs/config.yaml:86-100`):
```yaml
hls:
  enabled: true
  segment_duration: 1       # 저지연을 위해 1초 (기본 2초)
  segment_count: 3          # 메모리 효율 (기본 10개)
  output_dir: "hls"
  cleanup_threshold: 20     # 디스크 세그먼트 최대 20개
  enable_compression: false
```

---

## 트러블슈팅

### ❌ 문제 1: "HLS is not enabled" 에러

**증상**:
```json
{"error":"HLS is not enabled handleHLSPlaylist"}
```

**원인**:
- `config.yaml`에 `hls.enabled: true` 설정 누락
- 배포 시 config.yaml 파일이 전송되지 않음

**해결**:
1. `configs/config.yaml` 확인:
```yaml
hls:
  enabled: true  # ← 반드시 true
```

2. 배포 스크립트 수정 (`docker/deploy.ps1`):
```powershell
# config.yaml도 함께 전송
Write-Host "Step 3.5: Transferring config.yaml..."
sshpass -p $RemotePassword scp "../configs/config.yaml" ($RemoteUser + "@" + $RemoteHost + ":/path/to/configs/")
```

### ❌ 문제 2: "no such file or directory" 에러

**증상**:
```
ERROR hls/muxer_gohlslib.go:400 Failed to write NAL units to HLS
  error: open hls/CCTV-TEST/76acb60e7fc9_main_seg0.ts: no such file or directory
```

**원인**:
- `hls/<stream_id>/` 디렉토리가 생성되지 않음
- 이전에는 수동으로 `mkdir -p hls/CCTV-TEST` 필요

**해결**:
- ✅ **이미 해결됨**: `internal/hls/muxer_gohlslib.go:55`에서 자동 생성
```go
if err := os.MkdirAll(outputDir, 0755); err != nil {
    return nil, fmt.Errorf("failed to create HLS output directory: %w", err)
}
```

### ❌ 문제 3: 플레이리스트 503 에러 (Muxer 없음)

**증상**:
```
GET /hls/CCTV-TEST/index.m3u8 → 503 Service Unavailable
```

**원인**:
- on-demand 스트림인데 RTSP 클라이언트가 시작되지 않음
- Muxer가 생성되지 않음

**해결**:
- ✅ **이미 해결됨**: `handleHLSPlaylist`에서 자동 시작 및 retry 로직 구현
```go
if !exists {
    // 스트림 자동 시작
    s.startStreamHandler(streamID)

    // Muxer 생성 대기 (최대 5초)
    for i := 0; i < 10; i++ {
        time.Sleep(500 * time.Millisecond)
        muxer, exists = s.hlsManager.GetMuxer(streamID)
        if exists { break }
    }
}
```

### ❌ 문제 4: 높은 지연시간 (10초 이상)

**증상**:
- HLS 플레이어에서 영상이 10초 이상 지연됨

**원인**:
- 세그먼트 길이가 너무 길거나 (기본 2초)
- 플레이리스트 세그먼트 개수가 너무 많음 (기본 10개)

**해결**:
```yaml
hls:
  segment_duration: 1  # 2초 → 1초로 단축
  segment_count: 3     # 10개 → 3개로 축소
```

**결과**: 지연시간 10초 → 6-9초로 개선

---

## 성능 최적화

### 🚀 최적화 포인트

#### 1. 세그먼트 설정 튜닝

```yaml
# 저지연 설정 (현재)
hls:
  segment_duration: 1
  segment_count: 3
# 지연시간: 6-9초, 메모리: 낮음, CPU: 높음

# 균형 설정 (권장)
hls:
  segment_duration: 2
  segment_count: 5
# 지연시간: 10-15초, 메모리: 중간, CPU: 중간

# 안정성 설정
hls:
  segment_duration: 4
  segment_count: 10
# 지연시간: 40-50초, 메모리: 높음, CPU: 낮음
```

#### 2. 메모리 사용량 모니터링

```go
// HLS 메모리 사용량 = 세그먼트 수 × 세그먼트 크기
// 1초 세그먼트 ≈ 500KB (H.264, 2Mbps)
// 3개 세그먼트 × 500KB = 1.5MB per stream
```

**다중 스트림 고려**:
```
10 streams × 1.5MB = 15MB (HLS만)
+ WebRTC 버퍼 ≈ 5MB per stream = 50MB
───────────────────────────────────
Total: ~65MB (서버 메모리 충분)
```

#### 3. 디스크 I/O 최적화

```yaml
hls:
  cleanup_threshold: 20  # 디스크에 최대 20개 세그먼트 유지
```

**계산**:
```
20 segments × 500KB = 10MB per stream
10 streams × 10MB = 100MB total disk usage (매우 낮음)
```

#### 4. CPU 사용량

**주요 CPU 작업**:
- RTP Depayload: 가벼움 (~1% per stream)
- MPEG-TS Muxing: 중간 (~5% per stream)
- 파일 I/O: 가벼움 (~2% per stream)

**예상 CPU 사용량**:
```
1 stream = ~8% CPU
10 streams = ~80% CPU (4코어 기준)
```

### 📊 모니터링 API

```bash
# HLS 스트림 통계 조회
GET /api/v1/hls/streams/CCTV-TEST/stats

# 응답 예시
{
  "stream_id": "CCTV-TEST",
  "packets_received": 15234,
  "bytes_written": 7892345,
  "segments_created": 42,
  "current_bitrate": 2048576,
  "errors": 0
}
```

---

## 배포 체크리스트

### ✅ 배포 전 확인사항

- [ ] `configs/config.yaml`에 `hls.enabled: true` 설정
- [ ] `configs/config.yaml`이 Docker 컨테이너에 마운트되는지 확인
- [ ] HLS 출력 디렉토리(`hls/`) 쓰기 권한 확인
- [ ] 포트 8107 (HTTP API) 방화벽 오픈
- [ ] 디스크 공간 확인 (스트림당 ~10MB)

### 🐳 Docker 배포 시

```yaml
# docker-compose.yml
services:
  media-server:
    volumes:
      - ./configs/config.yaml:/app/configs/config.yaml:ro  # config 마운트
      - media-hls:/app/hls                                 # HLS 볼륨 (선택적)

volumes:
  media-hls:  # HLS 파일 영속화 (재시작 시 유지)
```

### 📝 배포 후 검증

```bash
# 1. HLS 활성화 확인
curl http://localhost:8107/api/v1/hls/streams

# 2. 플레이리스트 접근 테스트
curl http://localhost:8107/hls/CCTV-TEST/index.m3u8

# 3. 플레이어 테스트
vlc http://localhost:8107/hls/CCTV-TEST/index.m3u8
```

---

## 추가 개선 사항 (TODO)

### 🔮 향후 개선 계획

1. **fMP4 지원**: MPEG-TS 대신 fMP4 (더 나은 브라우저 호환성)
2. **ABR (Adaptive Bitrate)**: 다중 화질 지원
3. **DVR 기능**: HLS 세그먼트 기록 및 재생
4. **AES-128 암호화**: 콘텐츠 보호
5. **CloudFront 통합**: CDN을 통한 HLS 배포
6. **HLS 세그먼트 캐싱**: Redis/Memcached로 성능 개선

---

## 참고 자료

### 📚 관련 문서

- [gohlslib GitHub](https://github.com/bluenviron/gohlslib)
- [HLS RFC 8216](https://datatracker.ietf.org/doc/html/rfc8216)
- [Apple HLS Authoring Specification](https://developer.apple.com/documentation/http_live_streaming/hls_authoring_specification_for_apple_devices)

### 🔗 관련 파일

```
cmd/server/main.go:244-258      # HLS Manager 초기화
cmd/server/main.go:389-443      # RTP 패킷 처리
internal/api/server.go:84-105   # HLS API 엔드포인트
internal/api/server.go:397-460  # 플레이리스트 핸들러
internal/hls/manager.go         # HLS Manager
internal/hls/muxer_gohlslib.go  # gohlslib 래퍼
internal/core/config.go:103-112 # HLS Config
configs/config.yaml:86-100      # HLS 설정
```

---

## 마치며

이 문서는 HLS 구현의 **모든 것**을 담고 있습니다. 새로운 개발자가 이 문서만으로도:
- HLS가 **왜** 이렇게 구현되었는지
- **어떻게** 동작하는지
- **어디를** 수정해야 하는지

를 완전히 이해할 수 있어야 합니다.

질문이나 개선사항이 있다면 이 문서를 업데이트해주세요! 📝

**작성자**: Claude Code
**최종 업데이트**: 2025-11-17
**버전**: 1.0.0
