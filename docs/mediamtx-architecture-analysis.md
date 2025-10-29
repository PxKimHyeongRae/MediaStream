# mediaMTX 아키텍처 분석 리포트

> 우리 프로젝트(RTSP to WebRTC Media Server)에서 참조할 mediaMTX의 핵심 아키텍처 및 패턴 분석

생성일: 2025-10-29

---

## 1. 전체 아키텍처 개요

### 1.1 주요 컴포넌트 구조

mediaMTX는 Go 언어로 작성된 실시간 미디어 서버로, **모듈형 아키텍처**를 채택하고 있습니다.

```
mediamtx/
├── main.go (진입점)
└── internal/
    ├── core/           # 핵심 오케스트레이션
    │   ├── core.go           # 메인 Core 구조체
    │   ├── path_manager.go   # 스트리밍 경로 관리
    │   └── path.go           # 개별 경로 구현
    ├── stream/         # 스트림 관리
    ├── protocols/      # 프로토콜 구현체
    │   ├── rtsp/
    │   ├── webrtc/
    │   ├── rtmp/
    │   └── hls/
    ├── servers/        # 프로토콜별 서버
    │   ├── rtsp/
    │   └── webrtc/
    ├── staticsources/  # 정적 소스 (RTSP 클라이언트)
    ├── auth/           # 인증 관리
    ├── logger/         # 로깅 시스템
    └── conf/           # 설정 관리
```

#### 핵심 컴포넌트

**1. Core (internal/core/core.go)**
```go
type Core struct {
    ctx          context.Context
    ctxCancel    func()
    conf         *conf.Conf
    logger       *logger.Logger
    authManager  *auth.Manager
    metrics      *metrics.Metrics
    pathManager  *pathManager

    // 프로토콜 서버들
    rtspServer    *rtsp.Server
    rtspsServer   *rtsp.Server
    rtmpServer    *rtmp.Server
    hlsServer     *hls.Server
    webRTCServer  *webrtc.Server
    srtServer     *srt.Server
    api           *api.API
}
```

**핵심 설계 패턴:**
- **중앙 집중식 오케스트레이션**: Core가 모든 서버와 매니저를 초기화하고 관리
- **핫 리로드 지원**: 설정 변경 시 필요한 컴포넌트만 재생성
- **의존성 체인**: Logger → AuthManager → PathManager → Protocol Servers 순서로 초기화

**2. PathManager & Path**

```go
// Path 상태 머신
type pathOnDemandState int

const (
    pathOnDemandStateInitial pathOnDemandState = iota
    pathOnDemandStateWaitingReady
    pathOnDemandStateReady
    pathOnDemandStateClosing
)

type path struct {
    conf         *conf.Path
    stream       *stream.Stream
    source       source
    publisher    publisher
    readers      map[reader]struct{}

    // 요청 큐잉
    describeRequestsOnHold []describeRequest
    readerAddRequestsOnHold []readerAddRequest
}
```

**핵심 기능:**
- **온디맨드 활성화**: 첫 reader가 연결될 때 source 활성화
- **요청 큐잉**: 스트림이 준비되지 않았을 때 요청을 큐에 저장
- **Publisher/Reader 조정**: 하나의 publisher와 여러 reader 관리

**3. Stream (internal/stream/stream.go)**

```go
type Stream struct {
    bytesReceived atomic.Uint64
    bytesSent     atomic.Uint64

    readers map[reader]struct{}

    // 프로토콜별 스트림
    rtspStream  *gortsplib.ServerStream
    rtspsStream *gortsplib.ServerStream
}

// 데이터 입력 메소드
func (s *Stream) WriteUnit(medi *description.Media, u unit.Unit)
func (s *Stream) WriteRTPPacket(medi *description.Media, pkt *rtp.Packet)
```

### 1.2 데이터 플로우

```
[RTSP Source] ──┐
                ├──> [Path] ──> [Stream] ──┬──> [RTSP Reader]
[WebRTC Pub] ───┘                          ├──> [WebRTC Reader]
                                           ├──> [HLS Reader]
                                           └──> [Recording]
```

**데이터 플로우 상세:**

1. **입력 (Ingestion)**
   - RTSP 소스: `protocols/rtsp/to_stream.go`에서 RTP 패킷을 Stream으로 변환
   - WebRTC 발행: `protocols/webrtc/to_stream.go`에서 WebRTC 트랙을 Stream으로 변환

2. **중앙 배포 (Distribution)**
   - Stream이 등록된 모든 reader에게 비동기로 데이터 전달
   - Reader별 콜백 함수를 통해 format-specific 데이터 전달

3. **출력 (Egress)**
   - WebRTC: `protocols/webrtc/from_stream.go`에서 Stream을 WebRTC 트랙으로 변환
   - RTSP: gortsplib를 통해 직접 RTP 패킷 전송

### 1.3 동시성 처리 방식

#### 패턴 1: 요청 처리 루프

```go
// PathManager의 메인 루프
func (pm *pathManager) run() {
    for {
        select {
        case req := <-pm.chAddReader:
            pm.doAddReader(req)
        case req := <-pm.chAddPublisher:
            pm.doAddPublisher(req)
        case <-pm.chReloadConf:
            pm.doReloadConf()
        case <-pm.ctx.Done():
            return
        }
    }
}
```

**장점:**
- 모든 상태 변경이 단일 고루틴에서 처리되어 race condition 방지
- 명시적 mutex 없이 thread-safe 보장
- 요청-응답 패턴으로 동기화 구현

#### 패턴 2: Context 기반 생명주기 관리

```go
type Core struct {
    ctx       context.Context
    ctxCancel func()
}

func (c *Core) Close() {
    c.ctxCancel()  // 모든 하위 컴포넌트에 종료 신호
}
```

---

## 2. RTSP 처리 방식

### 2.1 RTSP 클라이언트 구현

**의존성:**
```go
github.com/bluenviron/gortsplib/v5
github.com/pion/rtp
github.com/pion/rtcp
```

**클라이언트 초기화:**

```go
func (s *Source) run(ctx context.Context) error {
    u, err := url.Parse(s.conf.Source)

    c := &gortsplib.Client{
        Transport:       s.conf.SourceProtocol.Transport,
        ReadTimeout:     s.conf.SourceReadTimeout,
        WriteTimeout:    s.conf.SourceWriteTimeout,
        ReadBufferCount: s.conf.SourceReadBufferCount,
    }

    err = c.Start(u.Scheme, u.Host)
    desc, _, err := c.Describe(u)
    err = c.SetupAll(desc.BaseURL, desc.Medias)

    s.parent.SetReady()

    rtspprotocol.ToStream(desc.Medias, c, stream, s.logger)

    _, err = c.Play(nil)

    return <-readErr
}
```

### 2.2 RTSP to Stream 변환

```go
// internal/protocols/rtsp/to_stream.go
func ToStream(
    medias []*description.Media,
    stream *stream.Stream,
    logger Logger,
) {
    for _, media := range medias {
        for _, forma := range media.Formats {
            forma.SetOnPacketRTP(func(pkt *rtp.Packet) {
                pts, ok := forma.PacketPTS(pkt)
                ntp := handleNTP(pkt)
                stream.WriteRTPPacket(media, pkt, ntp, pts)
            })
        }
    }
}
```

---

## 3. WebRTC 처리 방식

### 3.1 WebRTC 피어 연결

**의존성:**
```go
github.com/pion/webrtc/v4
github.com/pion/ice/v4
github.com/pion/dtls/v3
```

**PeerConnection 초기화:**

```go
func (co *PeerConnection) Start() error {
    se := webrtc.SettingEngine{}
    se.SetICETCPMux(co.iceTCPMux)
    se.SetICEUDPMux(co.iceUDPMux)

    me := &webrtc.MediaEngine{}
    registerVideoCodecs(me)
    registerAudioCodecs(me)

    i := &interceptor.Registry{}
    webrtc.RegisterDefaultInterceptors(me, i)

    api := webrtc.NewAPI(
        webrtc.WithSettingEngine(se),
        webrtc.WithMediaEngine(me),
        webrtc.WithInterceptorRegistry(i),
    )

    co.pc, err = api.NewPeerConnection(webrtc.Configuration{
        ICEServers: iceServers,
    })

    return nil
}
```

### 3.2 SDP 협상

**Offer 생성:**

```go
func (co *PeerConnection) CreatePartialOffer() (*webrtc.SessionDescription, error) {
    offer, err := co.pc.CreateOffer(nil)
    if err != nil {
        return nil, err
    }

    if err := co.waitGatheringDone(); err != nil {
        return nil, err
    }

    offer.SDP = co.filterSDP(offer.SDP)

    return offer, nil
}
```

---

## 4. RTSP to WebRTC 변환

### 4.1 미디어 파이프라인

```
[RTSP Source]
    ↓ gortsplib 수신
[RTP Packets]
    ↓ protocols/rtsp/to_stream.go
[Stream (내부 포맷)]
    ↓ stream.WriteRTPPacket()
[등록된 Readers]
    ↓ protocols/webrtc/from_stream.go
[WebRTC Tracks]
    ↓ pion/webrtc 전송
[WebRTC Peer]
```

### 4.2 H.264 비디오 트랙 생성

```go
func newVideoTrack(media *description.Media, writer Writer, logger Logger) (*OutgoingTrack, error) {
    var codecParams webrtc.RTPCodecParameters

    switch format := media.Formats[0].(type) {
    case *format.H264:
        if format.HasBFrames() {
            return nil, errors.New("WebRTC doesn't support H264 streams with B-frames")
        }

        codecParams = webrtc.RTPCodecParameters{
            RTPCodecCapability: webrtc.RTPCodecCapability{
                MimeType:  webrtc.MimeTypeH264,
                ClockRate: 90000,
                SDPFmtpLine: "level-asymmetry-allowed=1;" +
                    "packetization-mode=1;profile-level-id=42e01f",
            },
            PayloadType: 96,
        }

        format.OnData(func(au *unit.H264) {
            packets, err := rtpEnc.Encode(au.AU)
            ntp := ntpEpoch.Add(au.PTS)

            for _, pkt := range packets {
                track.WriteRTPWithNTP(pkt, ntp)
            }
        })
    }

    return &OutgoingTrack{codecParams, ...}, nil
}
```

---

## 5. 성능 최적화 기법

### 5.1 고루틴 활용

#### 패턴 1: 컴포넌트당 단일 고루틴

```go
type PathManager struct {
    chAddReader    chan addReaderRequest
    chAddPublisher chan addPublisherRequest
}

func (pm *PathManager) run() {
    for {
        select {
        case req := <-pm.chAddReader:
            pm.doAddReader(req)
        case req := <-pm.chAddPublisher:
            pm.doAddPublisher(req)
        }
    }
}
```

#### 패턴 2: Reader별 비동기 쓰기

```go
func (s *Stream) WriteUnit(media *description.Media, u unit.Unit) {
    s.mutex.RLock()
    readers := s.readers
    s.mutex.RUnlock()

    for reader := range readers {
        go reader.OnData(media, u)
    }
}
```

### 5.2 버퍼링 전략

```yaml
# mediamtx.yml
readBufferCount: 512      # RTP 수신 버퍼
writeQueueSize: 512       # 쓰기 큐 크기
```

---

## 우리 프로젝트 적용 패턴

### 1. 채널 기반 요청 처리

```go
type Manager struct {
    chAddConnection chan addConnectionReq
    chCloseConnection chan closeConnectionReq
}

func (m *Manager) run() {
    for {
        select {
        case req := <-m.chAddConnection:
            m.doAddConnection(req)
        case req := <-m.chCloseConnection:
            m.doCloseConnection(req)
        }
    }
}
```

### 2. Context 기반 생명주기

```go
type Component struct {
    ctx       context.Context
    ctxCancel func()
}

func (c *Component) Start(parentCtx context.Context) {
    c.ctx, c.ctxCancel = context.WithCancel(parentCtx)
    go c.run()
}

func (c *Component) Close() {
    c.ctxCancel()
}
```

### 3. RTSP 클라이언트 패턴

```go
func ConnectRTSP(url string) error {
    client := &gortsplib.Client{
        ReadTimeout: 10 * time.Second,
        OnPacketLost: func(err error) {
            log.Warn("packet lost: %v", err)
        },
    }

    defer client.Close()

    u, _ := url.Parse(url)
    client.Start(u.Scheme, u.Host)

    desc, _, _ := client.Describe(u)
    client.SetupAll(desc.BaseURL, desc.Medias)

    for _, media := range desc.Medias {
        media.Formats[0].SetOnPacketRTP(func(pkt *rtp.Packet) {
            // 처리
        })
    }

    client.Play(nil)

    return client.Wait()
}
```

### 4. WebRTC 피어 연결

```go
func CreateWebRTCPublisher() (*webrtc.PeerConnection, error) {
    config := webrtc.Configuration{
        ICEServers: []webrtc.ICEServer{
            {URLs: []string{"stun:stun.l.google.com:19302"}},
        },
    }

    pc, _ := webrtc.NewPeerConnection(config)

    track, _ := webrtc.NewTrackLocalStaticRTP(
        webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
        "video", "pion",
    )
    pc.AddTrack(track)

    offer, _ := pc.CreateOffer(nil)
    pc.SetLocalDescription(offer)

    return pc, nil
}
```

---

## 핵심 라이브러리

| 라이브러리 | 용도 |
|----------|------|
| **pion/webrtc** | WebRTC 구현 |
| **gortsplib** | RTSP 클라이언트/서버 |
| **bluenviron/mediacommon** | 코덱 및 미디어 처리 |

---

## 핵심 개념 요약

1. **고루틴 기반 동시성**: 각 컴포넌트가 독립 고루틴으로 실행
2. **채널 기반 통신**: mutex 대신 채널로 상태 동기화
3. **Context 생명주기**: 계층적 취소 전파
4. **인터페이스 추상화**: 프로토콜 무관 Stream/Source/Reader 인터페이스
5. **핫 리로드**: 설정 변경 시 무중단 업데이트
