# RTSP Server Implementation Plan

## 목표
mediaMTX를 완전히 대체하는 RTSP 서버 구현 - ffmpeg 트랜스코딩 파이프라인 지원

## 아키텍처

### 전체 데이터 플로우
```
┌─────────────┐
│ RTSP Camera │ (H.265 원본)
└──────┬──────┘
       │ RTSP/RTP
       ↓
┌──────────────────────┐
│ RTSP Client (기존)    │
│  - gortsplib v4      │
│  - TCP transport     │
└──────┬───────────────┘
       │ RTP Packets
       ↓
┌──────────────────────┐
│ Stream Manager       │ plx_cctv_01 (H.265)
│  - Pub/Sub Hub       │
│  - 1:N 분배          │
└──────┬───────────────┘
       │ Subscribe
       ↓
┌──────────────────────┐
│ RTSP Server (NEW!)   │ :8554/plx_cctv_01
│  - gortsplib.Server  │ ← ffmpeg 읽기
│  - Path 관리         │
└──────────────────────┘
       │ RTSP/RTP
       ↓
┌──────────────────────┐
│     ffmpeg           │ H.265 → H.264 트랜스코딩
│  runOnDemand         │
└──────┬───────────────┘
       │ RTSP/RTP
       ↓
┌──────────────────────┐
│ RTSP Server (NEW!)   │ :8554/plx_cctv_01_h264
│  - Publish 받기      │ ← ffmpeg publish
└──────┬───────────────┘
       │ RTP Packets
       ↓
┌──────────────────────┐
│ Stream Manager       │ plx_cctv_01_h264 (H.264)
└──────┬───────────────┘
       │ Subscribe
       ↓
┌──────────────────────┐
│  WebRTC Peer         │
│  Browser Client      │
└──────────────────────┘
```

## 구현 단계

### Phase 1: RTSP Server 기본 구조 ✅
**파일**: `internal/rtsp/server.go`

**책임**:
- gortsplib.Server 인스턴스 생성 및 관리
- 포트 8554에서 RTSP 요청 리스닝
- Connection 생명주기 관리
- Path별 라우팅

**주요 구현**:
```go
type Server struct {
    address        string              // ":8554"
    server         *gortsplib.Server
    pathManager    *PathManager        // Path별 세션 관리
    streamManager  *core.StreamManager // 기존 Stream과 연동
    logger         *zap.Logger
}

// OnConnOpen: 클라이언트 연결 시
// OnConnClose: 클라이언트 종료 시
// OnRequest: DESCRIBE, SETUP, PLAY, ANNOUNCE 처리
```

**예상 코드량**: 250-300줄

---

### Phase 2: Path Manager ✅
**파일**: `internal/rtsp/path_manager.go`

**책임**:
- Path별 Publisher/Subscriber 관리
- 예: `/plx_cctv_01`, `/plx_cctv_01_h264`
- Session 라우팅

**주요 구현**:
```go
type PathManager struct {
    paths map[string]*Path  // path name -> Path
    mu    sync.RWMutex
}

type Path struct {
    name        string
    publisher   *Publisher   // 1개만 (ffmpeg)
    subscribers []*Subscriber // N개 가능
    stream      *core.Stream // Stream Manager 연동
}
```

**예상 코드량**: 200-250줄

---

### Phase 3: Publisher/Subscriber Session ✅
**파일**: `internal/rtsp/publisher.go`, `internal/rtsp/subscriber.go`

**책임**:

**Publisher**:
- ANNOUNCE 요청 처리
- SDP 파싱 (ffmpeg가 보내는 SDP)
- RTP 패킷 수신 → Stream Manager에 전달
- 예: ffmpeg → `/plx_cctv_01_h264` publish

**Subscriber**:
- DESCRIBE 요청 처리 → SDP 생성
- SETUP, PLAY 요청 처리
- Stream Manager에서 RTP 패킷 구독 → 클라이언트에 전송
- 예: ffmpeg ← `/plx_cctv_01` subscribe

**주요 구현**:
```go
type Publisher struct {
    conn      *gortsplib.ServerConn
    session   *gortsplib.ServerSession
    stream    *core.Stream
    transport string // "tcp" or "udp"
}

type Subscriber struct {
    conn      *gortsplib.ServerConn
    session   *gortsplib.ServerSession
    stream    *core.Stream
    tracks    []*gortsplib.ServerSessionTrack
}
```

**예상 코드량**: 각 150-200줄

---

### Phase 4: SDP 생성 ✅
**파일**: `internal/rtsp/sdp.go`

**책임**:
- Stream의 코덱 정보 기반으로 SDP 생성
- H.264: SPS/PPS 포함
- H.265: VPS/SPS/PPS 포함

**주요 구현**:
```go
func GenerateSDP(stream *core.Stream) (string, error) {
    codec := stream.GetVideoCodec()

    switch codec {
    case "H264":
        return generateH264SDP(stream)
    case "H265":
        return generateH265SDP(stream)
    }
}

// SDP 예시 (H.265):
// v=0
// o=- 0 0 IN IP4 127.0.0.1
// s=Stream
// c=IN IP4 0.0.0.0
// t=0 0
// m=video 0 RTP/AVP 96
// a=rtpmap:96 H265/90000
// a=fmtp:96 sprop-vps=<base64>; sprop-sps=<base64>; sprop-pps=<base64>
```

**도전 과제**:
- Stream에서 VPS/SPS/PPS 추출 필요
- 현재 Stream은 RTP 패킷만 저장, 코덱 파라미터 미저장
- → Stream 구조 확장 필요

**예상 코드량**: 200-250줄

---

### Phase 5: Stream 구조 확장 ✅
**파일**: `internal/core/stream_manager.go` 수정

**현재 문제**:
- Stream은 videoCodec만 저장 (문자열)
- SDP 생성에 필요한 코덱 파라미터 없음

**필요한 추가**:
```go
type Stream struct {
    // 기존
    videoCodec string

    // 추가
    codecParams *CodecParameters
}

type CodecParameters struct {
    // H.264
    SPS []byte
    PPS []byte

    // H.265
    VPS []byte
    SPS []byte
    PPS []byte
}

// RTSP Client에서 SPS/PPS 추출 시 저장
func (s *Stream) SetCodecParameters(params *CodecParameters)
```

**예상 코드량**: 100줄

---

### Phase 6: Config 확장 ✅
**파일**: `internal/core/config.go` 수정

**추가 설정**:
```yaml
rtspServer:
  enabled: true
  port: 8554
  protocols:
    - tcp
    - udp
  readTimeout: 10s
  writeTimeout: 10s
```

```go
type RTSPServerConfig struct {
    Enabled      bool     `yaml:"enabled"`
    Port         int      `yaml:"port"`
    Protocols    []string `yaml:"protocols"`
    ReadTimeout  int      `yaml:"readTimeout"`
    WriteTimeout int      `yaml:"writeTimeout"`
}
```

**예상 코드량**: 50줄

---

### Phase 7: main.go 통합 ✅
**파일**: `cmd/server/main.go` 수정

**통합 작업**:
```go
type Application struct {
    // 기존
    config          *core.Config
    streamManager   *core.StreamManager
    rtspClients     map[string]*rtsp.Client

    // 추가
    rtspServer      *rtsp.Server  // NEW!
}

func initializeApplication(config *core.Config) (*Application, error) {
    // ...기존 초기화

    // RTSP Server 초기화
    app.rtspServer = rtsp.NewServer(rtsp.ServerConfig{
        Address:       fmt.Sprintf(":%d", config.RTSPServer.Port),
        StreamManager: app.streamManager,
        Logger:        logger.Log,
    })

    if err := app.rtspServer.Start(); err != nil {
        return nil, err
    }

    return app, nil
}
```

**예상 코드량**: 50줄

---

## 기술적 도전 과제

### 1. SDP 생성 (가장 어려움) ⭐⭐⭐⭐⭐
**문제**:
- H.264/H.265 SDP는 SPS/PPS/VPS 파라미터 필요
- 이 정보는 RTSP DESCRIBE 응답에서 얻을 수 있음
- 현재 RTSP Client는 이 정보를 저장하지 않음

**해결**:
- RTSP Client에서 media.Format 정보 추출
- gortsplib의 `format.H264`, `format.H265` 사용
- SafeParams(), SafeSPS(), SafePPS() 메서드 활용

### 2. RTP 패킷 라우팅 ⭐⭐⭐
**Publisher → Stream**:
```go
// Publisher.OnPacketRTP 콜백
func (p *Publisher) handlePacket(pkt *rtp.Packet) {
    p.stream.WritePacket(pkt)  // 기존 메서드 재사용
}
```

**Stream → Subscriber**:
```go
// Subscriber가 Stream 구독
stream.Subscribe(subscriber)

// Subscriber.WritePacket() 구현
func (s *Subscriber) WritePacket(pkt *rtp.Packet) error {
    // gortsplib.ServerSession.WritePacketRTP() 사용
    return s.session.WritePacketRTP(s.tracks[0], pkt)
}
```

### 3. 동시성 제어 ⭐⭐⭐
- Path별 Publisher/Subscriber 관리
- sync.RWMutex 사용
- 기존 Stream Manager 패턴 재사용

---

## 예상 일정

| Phase | 작업 | 예상 시간 |
|-------|------|----------|
| 1 | RTSP Server 기본 구조 | 40분 |
| 2 | Path Manager | 30분 |
| 3 | Publisher/Subscriber | 60분 |
| 4 | SDP 생성 | 60분 (가장 어려움) |
| 5 | Stream 구조 확장 | 30분 |
| 6 | Config 확장 | 20분 |
| 7 | main.go 통합 | 20분 |
| 8 | 디버깅 & 테스트 | 90분 |
| **합계** | | **5-6시간** |

---

## 테스트 계획

### 1. 기본 RTSP 서버 테스트
```bash
# ffprobe로 RTSP 서버 확인
ffprobe -rtsp_transport tcp rtsp://127.0.0.1:8554/plx_cctv_01
```

### 2. ffmpeg Subscribe 테스트
```bash
# 원본 스트림 읽기 (H.265)
ffmpeg -rtsp_transport tcp -i rtsp://127.0.0.1:8554/plx_cctv_01 -f null -
```

### 3. ffmpeg Publish 테스트
```bash
# 트랜스코딩 후 publish (H.264)
ffmpeg -rtsp_transport tcp -i rtsp://127.0.0.1:8554/plx_cctv_01 \
       -c:v libx264 -preset veryfast \
       -f rtsp rtsp://127.0.0.1:8554/plx_cctv_01_h264
```

### 4. 완전한 파이프라인 테스트
```bash
# runOnDemand로 자동 시작
curl -X POST http://localhost:8080/api/v1/paths -d '{
  "plx_cctv_01_h264": {
    "runOnDemand": "ffmpeg -rtsp_transport tcp -i rtsp://127.0.0.1:8554/plx_cctv_01 -c:v libx264 -f rtsp rtsp://127.0.0.1:8554/plx_cctv_01_h264",
    "runOnDemandRestart": true,
    "runOnDemandCloseAfter": "15s"
  }
}'

# 브라우저에서 http://localhost:8080/static/viewer.html?stream=plx_cctv_01_h264
```

---

## 참고 자료

- gortsplib v4 Server 예시: https://github.com/bluenviron/gortsplib/tree/main/examples
- mediaMTX 소스: https://github.com/bluenviron/mediamtx
- SDP RFC: https://tools.ietf.org/html/rfc4566
- H.264 RTP: https://tools.ietf.org/html/rfc6184
- H.265 RTP: https://tools.ietf.org/html/rfc7798

---

**작성일**: 2025-10-29
**상태**: 설계 완료, 구현 시작 예정
