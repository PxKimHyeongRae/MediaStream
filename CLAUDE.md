# CLAUDE.md - Living Document for Claude Code Collaboration

> **핵심 원칙**: 이 문서는 살아있는 문서(Living Document)입니다. 프로젝트가 진행되는 동안 지속적으로 CRUD(생성, 읽기, 수정, 삭제)하며 최신 상태를 유지해야 합니다.

## 📌 문서의 목적

이 CLAUDE.md는 Claude Code와의 협업에서 **단일 진실의 원천(Single Source of Truth, SSOT)**입니다:
- 대화 기록에 의존하지 않고 컨텍스트를 유지합니다
- 새로운 세션에서도 프로젝트를 원활하게 이어갈 수 있습니다
- 토큰 한계 문제와 컨텍스트 손실을 방지합니다
- 개발 과정의 모든 의사결정과 변경사항을 기록합니다

---

## 📋 프로젝트 개요

### 프로젝트 이름
**RTSP to WebRTC Media Server (cctv3)**

### 목적 및 목표
RTSP 프로토콜의 IP 카메라 스트림을 웹 브라우저에서 시청 가능하도록 WebRTC로 변환하는 고성능 미디어 서버 구축

**핵심 목표**:
- RTSP → WebRTC 실시간 변환 및 스트리밍
- H.265/H.264 코덱 자동 감지 및 선택
- 낮은 지연시간 (< 1초)
- 확장 가능한 아키텍처 (다중 스트림, 다중 클라이언트)
- mediaMTX와 유사한 성능 및 기능

### 주요 이해관계자
- 개발자: CCTV/IP 카메라 웹 스트리밍이 필요한 프로젝트
- 최종 사용자: 웹 브라우저에서 실시간 카메라 영상을 시청하는 사용자

---

## 🏗️ 아키텍처 설계

### 시스템 구조
```
[RTSP Camera (H.265/H.264)]
    ↓ TCP/RTSP
[RTSP Client (gortsplib v4)]
    ↓ RTP Packets (OnPacketRTPAny)
[Stream Manager (Pub/Sub)]
    ↓ Subscribe
[WebRTC Peer (pion v4)]
    ├─ H.265 지원 → H.265 트랙
    └─ H.264만 지원 → H.264 트랙
    ↓ WebRTC/SRTP
[Web Browser] ✅ 실시간 영상 재생
```

### 주요 컴포넌트

1. **RTSP Client** (`internal/rtsp/client.go`)
   - gortsplib v4 기반 RTSP 클라이언트
   - OnPacketRTPAny() 콜백을 통한 자동 RTP 패킷 수신
   - TCP 연결, 자동 재연결 지원

2. **Stream Manager** (`internal/core/stream_manager.go`)
   - Pub/Sub 패턴 구현
   - 다중 구독자 지원 (1:N 스트림 분배)
   - RTP 패킷 버퍼링 및 전달

3. **WebRTC Peer** (`internal/webrtc/peer.go`)
   - pion/webrtc v4 기반
   - 동적 코덱 선택 (Offer SDP 파싱)
   - ICE 연결 관리 (GatheringCompletePromise)
   - 구독자 정리 로직

4. **WebRTC Manager** (`internal/webrtc/manager.go`)
   - 피어 생성 및 관리
   - 피어 종료 시 자동 정리 (OnPeerClosed 콜백)

5. **Signaling Server** (`internal/signaling/server.go`)
   - WebSocket 기반 시그널링
   - Offer/Answer SDP 교환
   - ICE candidate 교환

6. **API Server** (`internal/api/server.go`)
   - Gin 프레임워크 기반 HTTP 서버
   - 정적 파일 서빙 (웹 UI)
   - WebSocket 엔드포인트, 헬스 체크 API

7. **Web Client** (`web/static/`)
   - HTML5 기반 웹 UI
   - WebRTC API 통합
   - 실시간 통계 표시

### 기술 스택

**언어/프레임워크**:
- Go 1.23+ (고성능, 뛰어난 동시성)
- HTML5/JavaScript (웹 클라이언트)

**주요 라이브러리**:
- [pion/webrtc v4](https://github.com/pion/webrtc) - Pure Go WebRTC 구현
- [bluenviron/gortsplib v4](https://github.com/bluenviron/gortsplib) - RTSP 클라이언트/서버
- [pion/rtp](https://github.com/pion/rtp) - RTP 패킷 처리
- [gin-gonic/gin](https://github.com/gin-gonic/gin) - HTTP 프레임워크
- [gorilla/websocket](https://github.com/gorilla/websocket) - WebSocket
- [uber-go/zap](https://github.com/uber-go/zap) - 구조화 로깅

**인프라**:
- YAML 기반 설정 파일
- 단일 바이너리 배포

### 디자인 패턴 및 원칙

1. **Pub/Sub 패턴**: 스트림 관리자가 RTP 패킷을 여러 구독자에게 분배
2. **의존성 주입**: 각 컴포넌트에 Logger, Config 주입
3. **콜백 패턴**: 피어 종료 시 자동 정리 (OnClose, OnPeerClosed)
4. **동시성 제어**: sync.RWMutex를 통한 안전한 동시 접근
5. **컨텍스트 관리**: context.Context를 통한 생명주기 관리

**코딩 컨벤션**:
- Go 표준 스타일 가이드 준수
- 구조화 로깅 (zap)
- 에러 처리: fmt.Errorf with %w
- 리소스 정리: defer 활용

---

## 🎯 현재 진행 상황

### 완료된 작업

#### Phase 1: 프로젝트 초기 설정 ✅
- ✅ Go 프로젝트 초기화 (go.mod, 디렉토리 구조)
- ✅ 기본 아키텍처 설계
- ✅ 설정 시스템 구축 (YAML)
- ✅ 로깅 시스템 구축 (zap)

#### Phase 2: RTSP 클라이언트 구현 ✅
- ✅ gortsplib v4 통합
- ✅ RTSP 스트림 연결 및 미디어 수신
- ✅ **RTP 패킷 콜백 구현 (OnPacketRTPAny)** ⭐
- ✅ H.265/H.264 코덱 지원
- ✅ 스트림 관리자 (Pub/Sub 패턴)

#### Phase 3: WebRTC 서버 구현 ✅
- ✅ pion/webrtc v4 통합
- ✅ WebRTC 피어 연결 관리
- ✅ **동적 코덱 선택 (Offer SDP 파싱)** ⭐ (가장 중요한 개선!)
- ✅ H.265/H.264 자동 협상
- ✅ **ICE 연결 처리 (GatheringCompletePromise)** ⭐
- ✅ 비디오/오디오 트랙 생성 및 관리

#### Phase 4: 시그널링 서버 ✅
- ✅ WebSocket 기반 시그널링
- ✅ Offer/Answer SDP 교환
- ✅ ICE candidate 교환
- ✅ 다중 클라이언트 연결 관리

#### Phase 5: 웹 클라이언트 ✅
- ✅ HTML5 기반 UI
- ✅ WebRTC API 통합
- ✅ 실시간 통계 표시 (비트레이트, 패킷 수 등)
- ✅ 연결 상태 모니터링

#### Phase 6: 테스트 및 검증 ✅
- ✅ E2E 자동화 테스트 (Go WebRTC 클라이언트)
- ✅ **구독자 정리 문제 해결** ⭐
- ✅ 실제 IP 카메라 스트리밍 성공
- ✅ 브라우저 호환성 검증 (Chrome, Edge, Firefox)

#### Phase 7: mediaMTX 스타일 다중 카메라 시스템 ✅
- ✅ **mediaMTX 스타일 paths 설정** ⭐
- ✅ **재사용 가능한 WebRTCEngine.js 라이브러리** ⭐
- ✅ 단일 스트림 뷰어 페이지 (viewer.html)
- ✅ 다중 카메라 대시보드 (dashboard.html)
- ✅ 온디맨드 스트림 관리 (sourceOnDemand)
- ✅ 스트림 관리 REST API
- ✅ 실제 4개 CCTV 카메라 통합
- ✅ RTSP 인증 URL 인코딩 처리
- ✅ 대시보드 자동 연결 기능

### 진행 중인 작업
- 없음 (Phase 7 완료)

### 다음 계획
1. **성능 최적화**: 지연시간 측정 및 튜닝, 버퍼 크기 조정
2. **다중 클라이언트 부하 테스트**: 수십~수백 동시 접속 테스트
3. **녹화 기능**: 스트림 녹화 및 재생
4. **HTTPS/WSS 지원**: 프로덕션 환경 준비
5. **인증/권한 관리**: JWT 기반 인증
6. **PTZ 카메라 제어**: 팬/틸트/줌 제어 기능

---

## 📝 핵심 기능 구현 상세

### 1. 동적 코덱 선택 (Dynamic Codec Selection)

**목적**: 브라우저가 지원하는 코덱(H.265/H.264)을 자동으로 감지하여 최적의 코덱 선택

**구현 위치**: `internal/webrtc/peer.go:127-151`

**기술적 의사결정**:
- **결정**: 클라이언트 Offer SDP를 파싱하여 지원 코덱 확인
- **이유**:
  - Chrome/Edge는 H.265 지원, Firefox는 H.264만 지원
  - 서버가 브라우저에 맞는 코덱을 선택해야 호환성 보장
  - 카메라가 H.265로 전송하더라도 브라우저 지원에 따라 트랙 생성
- **대안**:
  1. 서버에서 H.264로 트랜스코딩 (비용 높음, 지연 증가)
  2. 클라이언트가 코덱 선택 (복잡도 증가)

**핵심 코드**:
```go
func (p *Peer) selectVideoCodec(offerSDP string) string {
    offerUpper := strings.ToUpper(offerSDP)

    // H.265/HEVC 지원 여부 확인
    if strings.Contains(offerUpper, "H265") || strings.Contains(offerUpper, "HEVC") {
        return "H265"
    }
    // H.264/AVC 지원 여부 확인
    if strings.Contains(offerUpper, "H264") || strings.Contains(offerUpper, "AVC") {
        return "H264"
    }

    return "H265" // 기본값
}
```

**테스트 결과**:
- ✅ Chrome 107+: H.265 자동 선택
- ✅ Edge 107+: H.265 자동 선택
- ✅ Firefox: H.264 자동 선택

---

### 2. RTP 패킷 수신 (RTP Packet Reception)

**목적**: RTSP 스트림에서 RTP 패킷을 자동으로 수신하여 WebRTC로 전달

**구현 위치**: `internal/rtsp/client.go:249-258`

**기술적 의사결정**:
- **결정**: gortsplib v4의 `OnPacketRTPAny()` 콜백 사용
- **이유**:
  - v4부터 OnPacketRTP가 deprecated됨
  - OnPacketRTPAny는 모든 미디어 타입의 패킷을 자동으로 읽어줌
  - 수동으로 ReadPacketRTPOrRTCP 호출 불필요
- **이전 시도**:
  1. OnPacketRTP 사용 → deprecated 경고
  2. ReadPacketRTPOrRTCP 직접 호출 → 패킷 못 읽음
  3. OnPacketRTPAny 사용 → ✅ 성공

**핵심 코드**:
```go
// 각 미디어에 대해 패킷 콜백 등록
medi.OnPacketRTPAny(func(medi *media.Media, forma format.Format, pkt *rtp.Packet) {
    if c.onPacket != nil {
        c.onPacket(pkt)
    }
})
```

**변경 이력**:
- 2025-10-29: OnPacketRTPAny 사용으로 변경, 패킷 수신 성공

---

### 3. ICE 연결 처리 (ICE Connection Handling)

**목적**: WebRTC ICE 연결 실패 문제 해결

**구현 위치**: `internal/webrtc/peer.go:107-122`

**기술적 의사결정**:
- **결정**: `GatheringCompletePromise`를 사용하여 ICE candidate 수집 완료 대기
- **이유**:
  - Answer SDP를 너무 빨리 보내면 ICE candidates가 포함되지 않음
  - 클라이언트가 서버의 IP를 모르면 연결 실패
  - 모든 candidates 수집 후 Answer 전송 필요
- **이전 문제**:
  - Answer SDP에 ICE candidates 없음
  - 클라이언트: "ICE connection state: failed"

**핵심 코드**:
```go
// ICE candidate 수집 완료 대기
<-webrtc.GatheringCompletePromise(pc)

// Answer 생성 (모든 ICE candidates 포함)
answer, err := pc.CreateAnswer(nil)
pc.SetLocalDescription(answer)
```

**변경 이력**:
- 2025-10-29: GatheringCompletePromise 추가, ICE 연결 성공

---

### 4. 구독자 정리 (Subscriber Cleanup)

**목적**: 피어 연결 종료 시 스트림에서 자동으로 구독 해제

**구현 위치**:
- `internal/webrtc/manager.go:52-62` (OnPeerClosed 콜백)
- `cmd/server/main.go:352-393` (cleanupPeer 메서드)

**기술적 의사결정**:
- **결정**: 피어 종료 시 콜백 체인을 통해 자동 정리
- **이유**:
  - 이전에는 피어가 종료되어도 스트림에 남아있어 에러 발생
  - "Failed to send packet to subscriber" 에러 지속 발생
  - 메모리 누수 가능성
- **구현 방식**:
  1. Peer.Close() → OnClose 콜백
  2. Manager.OnPeerClosed → Application.cleanupPeer()
  3. Stream.Unsubscribe(peerID)

**핵심 코드**:
```go
// Manager에서 피어 생성 시
peer := NewPeer(PeerConfig{
    OnClose: func(peerID string) {
        // 외부 콜백 호출
        if m.onPeerClosed != nil {
            m.onPeerClosed(peerID)
        }
        // 매니저에서 제거
        m.RemovePeer(peerID)
    },
})

// Application의 cleanupPeer
func (app *Application) cleanupPeer(peerID string) {
    streamID := app.peerStreams[peerID]
    stream, _ := app.streamManager.GetStream(streamID)
    stream.Unsubscribe(peerID)
}
```

**변경 이력**:
- 2025-10-29: 구독자 자동 정리 구현, 에러 완전 해결

---

### 5. mediaMTX 스타일 Paths 설정 (mediaMTX-style Configuration)

**목적**: mediaMTX와 동일한 방식의 직관적인 설정 시스템 구현

**구현 위치**:
- `configs/config.yaml:11-28` (설정 파일)
- `internal/core/config.go:30-38` (PathConfig 구조체)
- `cmd/server/main.go:182-218` (loadStreamsFromConfig)

**기술적 의사결정**:
- **결정**: YAML의 paths 섹션에서 각 스트림을 개별 설정
- **이유**:
  - mediaMTX 사용자들에게 친숙한 구조
  - 스트림별 독립적인 설정 가능 (sourceOnDemand, rtspTransport)
  - 설정 파일만으로 스트림 추가/제거 가능
  - 코드 수정 없이 운영 가능
- **대안**:
  1. 데이터베이스 기반 설정 (복잡도 증가)
  2. REST API로만 관리 (재시작 시 설정 손실)

**핵심 코드**:
```yaml
# configs/config.yaml
paths:
  plx_cctv_01:
    source: rtsp://admin:live0416@192.168.4.121:554/Streaming/Channels/101
    sourceOnDemand: no  # 서버 시작 시 자동 연결
    rtspTransport: tcp
  plx_cctv_02:
    source: rtsp://admin:1q2w3e4r%21@192.168.4.54:554/profile2/media.smp
    sourceOnDemand: yes  # 클라이언트 요청 시 연결
    rtspTransport: tcp
```

```go
// internal/core/config.go
type PathConfig struct {
    Source         string `yaml:"source"`
    SourceOnDemand bool   `yaml:"sourceOnDemand"`
    RTSPTransport  string `yaml:"rtspTransport"`
}

type Config struct {
    Paths map[string]PathConfig `yaml:"paths"`
    // ... other fields
}
```

**변경 이력**:
- 2025-10-29: mediaMTX 스타일 paths 설정 구현, 4개 CCTV 통합

---

### 6. 재사용 가능한 WebRTC 엔진 (Reusable WebRTC Engine)

**목적**: 다중 스트림을 쉽게 통합할 수 있는 독립적인 JavaScript 라이브러리

**구현 위치**: `web/static/js/webrtc-engine.js`

**기술적 의사결정**:
- **결정**: 이벤트 기반 API를 가진 클래스 형태의 재사용 가능한 라이브러리
- **이유**:
  - 하나의 페이지에서 여러 스트림 동시 관리 가능
  - 깔끔한 API로 통합 간단화
  - 자동 재연결, 통계 수집 등 공통 기능 캡슐화
  - 다른 프로젝트에서도 재사용 가능
- **대안**:
  1. 각 페이지마다 WebRTC 코드 복사 (중복 코드)
  2. 전역 함수 기반 (충돌 가능성, 상태 관리 어려움)

**핵심 코드**:
```javascript
// web/static/js/webrtc-engine.js
class WebRTCEngine {
    constructor(config) {
        this.serverUrl = config.serverUrl || `ws://${window.location.host}/ws`;
        this.streamId = config.streamId;
        this.videoElement = config.videoElement;
        this.autoReconnect = config.autoReconnect !== undefined ? config.autoReconnect : true;

        this.eventHandlers = {
            'connected': [],
            'disconnected': [],
            'error': [],
            'stats': [],
            'statechange': []
        };
    }

    on(event, callback) {
        this.eventHandlers[event].push(callback);
        return this;
    }

    async connect() {
        await this.connectWebSocket();
        await this.createPeerConnection();
        await this.createOffer();
    }
}
```

**사용 예시**:
```javascript
// 단일 스트림
const engine = new WebRTCEngine({
    streamId: 'plx_cctv_01',
    videoElement: document.getElementById('video1')
});

engine.on('connected', () => console.log('Connected!'));
engine.on('stats', (stats) => console.log('Bitrate:', stats.bitrate));
await engine.connect();

// 다중 스트림 (대시보드)
const engines = {};
for (const streamId of streamIds) {
    engines[streamId] = new WebRTCEngine({
        streamId: streamId,
        videoElement: document.getElementById(`video-${streamId}`)
    });
    await engines[streamId].connect();
}
```

**변경 이력**:
- 2025-10-29: WebRTCEngine.js 라이브러리 구현, 대시보드 통합

---

### 7. 온디맨드 스트림 관리 (On-Demand Stream Management)

**목적**: 필요할 때만 RTSP 연결하여 리소스 절약

**구현 위치**:
- `cmd/server/main.go:220-269` (startOnDemandStream)
- `internal/api/server.go:161-176` (API 엔드포인트)

**기술적 의사결정**:
- **결정**: Stream 객체는 서버 시작 시 생성, RTSP 클라이언트는 필요 시 생성
- **이유**:
  - 메모리 효율: 사용하지 않는 카메라는 연결 안 함
  - 네트워크 효율: 불필요한 RTSP 트래픽 방지
  - 카메라 부하 감소: 시청 중인 카메라만 스트리밍
  - 유연성: API로 수동 시작/정지 가능
- **구현 방식**:
  1. 서버 시작 시: 모든 paths에 대해 Stream 객체 생성
  2. sourceOnDemand=no: RTSP 클라이언트 즉시 생성
  3. sourceOnDemand=yes: Stream만 생성, RTSP 클라이언트는 미생성
  4. 클라이언트 요청 시: API로 RTSP 클라이언트 생성 및 시작

**핵심 코드**:
```go
// cmd/server/main.go
func (app *Application) loadStreamsFromConfig(config *core.Config) error {
    for streamID, pathConfig := range config.Paths {
        // 모든 스트림에 대해 Stream 객체 생성
        if _, err := app.streamManager.CreateStream(streamID, streamID); err != nil {
            return err
        }

        if !pathConfig.SourceOnDemand {
            // always-on: RTSP 클라이언트 즉시 시작
            if err := app.startRTSPClient(streamID, pathConfig); err != nil {
                logger.Error("Failed to start always-on stream", zap.Error(err))
            }
        }
        // on-demand: RTSP 클라이언트는 나중에 생성
    }
    return nil
}

func (app *Application) startOnDemandStream(streamID string) error {
    // 이미 실행 중인지 확인
    if _, exists := app.rtspClients[streamID]; exists {
        return nil
    }

    // Stream 객체는 이미 존재 (서버 시작 시 생성됨)
    stream, err := app.streamManager.GetStream(streamID)
    if err != nil {
        return fmt.Errorf("stream not found: %w", err)
    }

    // RTSP 클라이언트만 생성
    pathConfig := app.config.Paths[streamID]
    return app.startRTSPClient(streamID, pathConfig)
}
```

**API 엔드포인트**:
- `POST /api/v1/streams/:id/start` - 온디맨드 스트림 시작
- `DELETE /api/v1/streams/:id` - 스트림 정지
- `GET /api/v1/streams` - 모든 스트림 목록 및 상태
- `GET /api/v1/streams/:id` - 특정 스트림 정보 (코덱, 상태 등)

**변경 이력**:
- 2025-10-29: 온디맨드 스트림 구현, 스트림 생성 중복 버그 수정

---

### 8. 다중 카메라 대시보드 (Multi-Camera Dashboard)

**목적**: 여러 CCTV를 한 화면에서 동시에 모니터링

**구현 위치**: `web/static/dashboard.html`

**기술적 의사결정**:
- **결정**: CSS Grid 기반 반응형 레이아웃 + 자동 연결
- **이유**:
  - Grid: 카메라 개수에 따라 자동으로 배치
  - 반응형: 화면 크기에 맞춰 그리드 크기 조절
  - 자동 연결: 페이지 로드 1초 후 모든 카메라 자동 연결
  - 개별 제어: 각 카메라별 연결/해제 가능
- **대안**:
  1. Flexbox (Grid보다 유연성 낮음)
  2. 수동 연결 (사용자 불편)

**핵심 코드**:
```javascript
// web/static/dashboard.html
async function init() {
    await loadStreams();
    setupEventListeners();

    // 자동 연결
    if (streams.length > 0) {
        console.log('Auto-connecting all cameras...');
        setTimeout(() => {
            connectAll();
        }, 1000); // 1초 후 자동 연결 시작
    }
}

async function connectCamera(streamId) {
    const streamInfo = streams.find(s => s.id === streamId);

    // 온디맨드 스트림이면 먼저 시작
    if (streamInfo && streamInfo.onDemand && streamInfo.status === 'stopped') {
        await fetch(`/api/v1/streams/${streamId}/start`, { method: 'POST' });
        await new Promise(resolve => setTimeout(resolve, 1500));
    }

    // WebRTC 엔진 생성
    const engine = new WebRTCEngine({
        streamId: streamId,
        videoElement: document.getElementById(`video-${streamId}`),
        autoReconnect: true
    });

    engines[streamId] = engine;
    await engine.connect();
}
```

**CSS Grid 레이아웃**:
```css
.grid-container {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
    gap: 20px;
}
```

**기능**:
- ✅ 페이지 로드 시 자동 연결
- ✅ 각 카메라별 상태 표시 (연결 중/연결됨/오류)
- ✅ 개별 재연결 버튼
- ✅ 전체 연결/해제 버튼
- ✅ 실시간 비트레이트 및 패킷 통계
- ✅ 풀스크린 지원

**변경 이력**:
- 2025-10-29: 다중 카메라 대시보드 구현, 자동 연결 추가

---

## 🐛 알려진 이슈 및 제약사항

### 해결된 이슈 (과거)

1. **RTP 패킷 미수신** ✅ 해결됨
   - 원인: gortsplib v4에서 OnPacketRTP deprecated
   - 해결: OnPacketRTPAny 사용

2. **ICE 연결 실패** ✅ 해결됨
   - 원인: Answer SDP에 ICE candidates 미포함
   - 해결: GatheringCompletePromise 사용

3. **코덱 불일치로 영상 안 나옴** ✅ 해결됨
   - 원인: 카메라 H.265, 서버 H.264만 전송
   - 해결: 동적 코덱 선택 구현

4. **구독자 정리 문제** ✅ 해결됨
   - 원인: 피어 종료 후 스트림에 남아있음
   - 해결: OnPeerClosed 콜백 체인 구현

5. **클라이언트 ID 중복 (심각한 버그)** ✅ 해결됨 (2025-10-29)
   - 원인: randomString() 함수가 매번 동일한 문자열 "abcdefgh" 생성
   - 증상:
     - 모든 클라이언트가 동일한 ID "client-abcdefgh"
     - WebSocket 메시지 라우팅 실패
     - 테스트에서 Answer 메시지 미수신
     - 클라이언트가 ICE state "new"에서 멈춤
   - 해결:
     - math/rand 패키지 import
     - randomString()을 진짜 랜덤하게 수정: `rng.Intn(len(letters))`
     - 클라이언트 생성 시 ID를 한 번만 생성하도록 수정 (logger 중복 생성 버그 수정)
   - 파일: `internal/signaling/server.go:3-12, 74-81, 281-290`
   - 검증: E2E 테스트에서 서로 다른 클라이언트 ID 확인 (client-6hara0rp, client-mh3iitjg 등)

6. **뮤텍스 데드락으로 연속 테스트 실패** ✅ 해결됨 (2025-10-29)
   - 원인 1: `webrtc.Manager.CreatePeer()`에서 Lock 획득 후 OnClose 콜백이 `RemovePeer()`를 동기 호출하면 데드락
   - 원인 2: `peer.Close()`가 여러 곳에서 호출되어 onClose 콜백이 중복 실행
   - 증상:
     - 첫 번째 E2E 테스트 성공
     - 두 번째 테스트가 "Handling WebRTC offer" 로그 후 25초 타임아웃
     - handleWebRTCOffer()가 CreatePeer()에서 블로킹
   - 해결:
     - **고루틴 비동기 실행**: OnClose 콜백을 `go func()` 으로 감싸서 데드락 방지
     - **sync.Once 보호**: `peer.Close()`에 `closeOnce.Do()` 추가하여 중복 실행 방지
   - 파일:
     - `internal/webrtc/manager.go:55-65` (고루틴 추가)
     - `internal/webrtc/peer.go:27, 389-407` (sync.Once 추가)
   - 검증:
     - `TestVideoStreaming`: PASS (10.56s, 101 packets)
     - `TestMultipleClients`: PASS (9.56s, 3 clients × 102 packets)
     - 연속 실행 총 20.2초 소요, 데드락 없음

7. **온디맨드 스트림 생성 중복 버그** ✅ 해결됨 (2025-10-29)
   - 원인: `startOnDemandStream()`에서 `addStream()`을 호출하여 이미 존재하는 Stream 객체를 다시 생성 시도
   - 증상:
     - `POST /api/v1/streams/:id/start` 호출 시 "stream already exists" 에러
     - 온디맨드 스트림 시작 불가능
   - 해결:
     - Stream 객체는 서버 시작 시 한 번만 생성 (loadStreamsFromConfig)
     - `startOnDemandStream()`에서는 RTSP 클라이언트만 생성
     - 기존 Stream 객체를 GetStream()으로 가져와서 재사용
   - 파일: `cmd/server/main.go:220-269`
   - 검증: 온디맨드 스트림 정상 시작, 여러 번 호출해도 에러 없음

8. **RTSP 인증 URL 인코딩 문제** ✅ 해결됨 (2025-10-29)
   - 원인: 비밀번호에 특수문자(!@#)가 포함되어 RTSP URL이 잘못 파싱됨
   - 증상:
     - plx_cctv_03: 401 Unauthorized 에러
     - 서버 로그: "bad status code: 401 (Unauthorized)"
     - 프론트엔드: 코덱 "알 수 없음" 표시
   - 해결:
     - config.yaml에서 특수문자 URL 인코딩
     - `!` → `%21`, `@` → `%40`, `#` → `%23`
     - plx_cctv_02, plx_cctv_03 비밀번호 모두 수정
   - 파일: `configs/config.yaml:18, 22`
   - 검증: 모든 카메라 정상 인증, 코덱 정보 표시됨

9. **대시보드 수동 연결 문제** ✅ 해결됨 (2025-10-29)
   - 원인: 대시보드 페이지 로드 후 "모두 연결" 버튼을 수동으로 클릭해야 함
   - 증상:
     - 사용자가 매번 버튼 클릭 필요
     - 자동 모니터링 시스템으로 사용 불편
   - 해결:
     - `init()` 함수에서 페이지 로드 1초 후 자동으로 `connectAll()` 호출
     - setTimeout으로 DOM 렌더링 완료 대기
   - 파일: `web/static/dashboard.html:566-575`
   - 검증: 페이지 열면 자동으로 모든 카메라 연결

### 현재 이슈

없음 - 모든 알려진 이슈가 해결되었습니다! ✅

### 기술적 부채

1. **에러 핸들링 부족**: 일부 에러가 로그만 남기고 처리 안 됨
   - 해결 계획: 에러 타입별 재시도 로직, 알림 시스템 추가

2. **테스트 커버리지 낮음**: E2E 테스트만 존재, 유닛 테스트 없음
   - 해결 계획: 주요 컴포넌트별 유닛 테스트 추가

3. **WebRTCEngine.js 테스트 없음**: 프론트엔드 라이브러리에 대한 자동화 테스트 부재
   - 해결 계획: Jest 또는 Playwright 기반 E2E 테스트 추가

### 제약사항

1. **브라우저 H.265 지원**: Firefox는 H.265를 지원하지 않음
   - 해결: 서버가 자동으로 H.264를 선택하도록 구현 완료

2. **네트워크 환경**: 복잡한 NAT 환경에서 STUN/TURN 서버 필요
   - 현재: 로컬 네트워크에서만 테스트됨
   - 향후: TURN 서버 설정 추가 필요

3. **카메라 코덱**: 카메라가 H.265/H.264를 지원해야 함
   - 현재: 테스트 카메라는 H.265 지원 확인

---

## 📚 참조 문서

### 내부 문서
- [README.md](./README.md) - 프로젝트 소개 및 사용 가이드
- [configs/config.yaml](./configs/config.yaml) - 설정 파일 예시
- [CLAUDE.md](./CLAUDE.md) - 프로젝트 살아있는 문서 (현재 파일)

### Skills (재사용 가능한 지식)
- [.claude/skills/rtsp-webrtc-streaming.md](./.claude/skills/rtsp-webrtc-streaming.md) - RTSP to WebRTC 스트리밍 시스템 종합 가이드
- [.claude/skills/README.md](./.claude/skills/README.md) - Skill 관리 가이드

**Skills vs CLAUDE.md**:
- **CLAUDE.md**: 현재 프로젝트(cctv3)의 구체적인 상태, 의사결정, 진행 상황
- **Skills**: 일반화된 패턴과 재사용 가능한 지식, 다른 프로젝트에도 적용 가능

**Skills 활용**:
```
"RTSP to WebRTC 시스템을 새로 만들어야 해.
.claude/skills/rtsp-webrtc-streaming.md를 참고해서 설계해줘."
```

### 외부 리소스

**핵심 라이브러리**:
- [pion/webrtc](https://github.com/pion/webrtc) - WebRTC 구현 가이드
- [bluenviron/gortsplib](https://github.com/bluenviron/gortsplib) - RTSP 사용법
- [mediaMTX](https://github.com/bluenviron/mediamtx) - 참조 아키텍처

**표준 및 프로토콜**:
- [WebRTC 표준](https://webrtc.org/)
- [RTSP RFC 2326](https://tools.ietf.org/html/rfc2326)
- [RTP RFC 3550](https://tools.ietf.org/html/rfc3550)

**학습 리소스**:
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Concurrency Patterns](https://go.dev/blog/pipelines)

---

## 💬 Claude Code 사용 가이드

### 이 프로젝트에서 효과적인 프롬프팅

1. **기능 추가 시**:
   ```
   "다중 카메라 지원 기능을 추가하고 싶어. 현재 코드 구조를 유지하면서
   어떻게 구현하면 좋을까?"
   ```

2. **문제 해결 시**:
   ```
   "E2E 테스트가 연속 실행하면 실패해. 로그를 보면 [로그 내용].
   원인과 해결 방법을 제시해줘."
   ```

3. **코드 리뷰 시**:
   ```
   "@internal/webrtc/peer.go 파일의 코드를 리뷰하고
   개선 사항을 제안해줘."
   ```

### 작업 프로세스

1. **새 기능 개발**:
   - CLAUDE.md에서 현재 상태 확인
   - 기능 요구사항 설명
   - 설계 제안 받기
   - 구현 후 CLAUDE.md 업데이트

2. **버그 수정**:
   - 로그/에러 내용 제공
   - 관련 코드 파일 참조
   - 원인 분석 및 수정
   - "알려진 이슈" 섹션 업데이트

3. **테스트**:
   - `go test -v ./test/e2e/stream_test.go -run [테스트명]`
   - 실패 시 로그 분석
   - 수정 후 재테스트

### 서브 에이전트 활용

- **Explore Agent**: 코드베이스 탐색 시 사용
- **Plan Agent**: 복잡한 기능 설계 시 사용

### Skills 관리 (재사용 가능한 지식)

**Skills는 CLAUDE.md처럼 Living Document입니다.**

#### Skills 생성 (Create)
```
"새로운 패턴이나 해결책을 발견했어.
.claude/skills/에 [skill-name].md로 저장하고
README.md에도 추가해줘."
```

#### Skills 읽기 (Read)
```
"RTSP to WebRTC 관련 패턴을 참고하고 싶어.
.claude/skills/rtsp-webrtc-streaming.md를 보여줘."
```

#### Skills 업데이트 (Update)
```
"ICE 연결 처리 패턴이 개선되었어.
rtsp-webrtc-streaming.md의 ICE 섹션을 업데이트하고
Maintenance Log에도 기록해줘."
```

#### Skills 삭제 (Delete)
```
"[skill-name].md가 더 이상 유용하지 않아.
삭제하고 README.md에서도 제거해줘."
```

**Best Practice**:
- 프로젝트에서 중요한 패턴 발견 시 즉시 Skill로 저장
- 버그 해결 시 트러블슈팅 섹션에 추가
- 다른 프로젝트에도 적용 가능하도록 일반화
- CLAUDE.md는 현재 프로젝트, Skills는 재사용 가능한 지식

---

## 📊 성공 지표

### 프로젝트 성공 기준
- ✅ 실시간 RTSP → WebRTC 스트리밍 성공
- ✅ 브라우저 호환성 (Chrome, Edge, Firefox)
- ✅ 지연시간 < 1초
- ✅ 자동화된 테스트 (E2E)
- ✅ **다중 스트림 지원 (4개 CCTV 통합)** ⭐
- ✅ **mediaMTX 스타일 설정 시스템** ⭐
- ✅ **재사용 가능한 WebRTC 라이브러리** ⭐
- ✅ **다중 카메라 대시보드** ⭐
- ✅ **온디맨드 스트림 관리** ⭐
- 🔶 다중 클라이언트 부하 테스트 (계획 중)
- 🔶 녹화 기능 (계획 중)

### 코드 품질 지표
- 테스트 커버리지: 현재 ~10% (E2E만) / 목표 60%+
- 알려진 버그: 0개 (치명적 버그)
- 기술 부채: 낮음 (주요 구조 완성)
- 프론트엔드 라이브러리: 재사용 가능한 WebRTCEngine.js
- 실제 배포: 4개 실제 CCTV 카메라 연동 성공

---

## 🚀 배포 및 운영

### 빌드 프로세스
```bash
# 개발 빌드
go build -o bin/media-server.exe cmd/server/main.go

# 프로덕션 빌드 (최적화)
go build -ldflags="-s -w" -o bin/media-server cmd/server/main.go
```

### 실행
```bash
# 기본 실행
./bin/media-server.exe

# 설정 파일 지정
./bin/media-server.exe -config=configs/config.yaml

# 버전 확인
./bin/media-server.exe -version
```

### 모니터링
- **로그 위치**: stdout (콘솔)
- **로그 레벨**: configs/config.yaml에서 설정
- **주요 메트릭**:
  - 활성 스트림 수
  - 연결된 피어 수
  - RTP 패킷 수신률
  - ICE 연결 상태

---

## 📌 중요 알림

### ⚠️ 개발 시 주의사항

1. **의존성 버전**: pion/webrtc v4, gortsplib v4 반드시 사용
   - v3와 v4 API가 크게 다름 (OnPacketRTPAny 등)

2. **동시성 제어**:
   - Stream, Peer 접근 시 mutex 사용
   - 고루틴에서 안전한 로거 사용

3. **리소스 정리**:
   - 피어 종료 시 반드시 Unsubscribe 호출
   - 컨텍스트 취소 시 고루틴 정리 확인

4. **테스트**:
   - E2E 테스트 실행 전 서버 재시작
   - 테스트 간 충분한 정리 시간 확보

### 💡 Best Practices

1. **에러 처리**: 항상 fmt.Errorf with %w 사용
2. **로깅**: 구조화 로깅 (zap.String, zap.Int 등) 사용
3. **설정**: 하드코딩 금지, config.yaml 사용
4. **문서화**: 주요 의사결정은 CLAUDE.md에 기록

---

## 🔄 버전 히스토리

### v0.1.0 (2025-10-29)
- ✅ RTSP to WebRTC 기본 기능 완성
- ✅ 동적 코덱 선택 구현
- ✅ ICE 연결 문제 해결
- ✅ 구독자 정리 로직 구현
- ✅ E2E 자동화 테스트 추가
- ✅ 실제 IP 카메라 스트리밍 성공

### v0.2.0 (2025-10-29) - mediaMTX Edition
- ✅ **mediaMTX 스타일 paths 설정 시스템** ⭐
- ✅ **재사용 가능한 WebRTCEngine.js 라이브러리** ⭐
- ✅ 단일 스트림 뷰어 페이지 (viewer.html)
- ✅ 다중 카메라 대시보드 (dashboard.html)
- ✅ 온디맨드 스트림 관리 (sourceOnDemand)
- ✅ 스트림 관리 REST API
- ✅ 4개 실제 CCTV 카메라 통합
- ✅ RTSP 인증 URL 인코딩 처리
- ✅ 대시보드 자동 연결 기능
- ✅ 버그 수정: 스트림 생성 중복, URL 인코딩, 자동 연결

**주요 개선사항**:
- 23개 파일 수정, 4,958 줄 추가
- 완전한 다중 카메라 지원
- 사용자 친화적인 프론트엔드
- 프로덕션 레벨 기능

### 다음 버전 (v0.3.0) 계획
- 성능 최적화 및 지연시간 측정
- 다중 클라이언트 부하 테스트
- 유닛 테스트 추가
- 녹화 기능
- PTZ 카메라 제어

---

## 📝 메모 및 임시 노트

### 개발 중 발견한 팁

1. **gortsplib v4 사용 시**: 반드시 OnPacketRTPAny 사용
2. **ICE 문제 디버깅**: 브라우저 콘솔에서 "ICE connection state" 로그 확인
3. **코덱 문제**: 서버 로그에서 "Media format detected" 확인
4. **구독자 정리**: "Subscriber removed" 로그로 정상 동작 확인
5. **RTSP 비밀번호 특수문자**: URL 인코딩 필수 (!→%21, @→%40, #→%23)
6. **온디맨드 스트림**: Stream 객체와 RTSP 클라이언트 생명주기 분리
7. **대시보드 자동 연결**: setTimeout 1초로 DOM 렌더링 완료 대기
8. **WebRTCEngine.js**: 다중 인스턴스 생성 가능, videoElement는 각각 독립적

### 웹 페이지 접속 URL

**프로덕션 사용**:
- 대시보드 (권장): http://localhost:8080/static/dashboard.html
- 단일 뷰어: http://localhost:8080/static/viewer.html
- 원본 데모: http://localhost:8080/

**API 엔드포인트**:
- GET /api/v1/streams - 스트림 목록
- GET /api/v1/streams/:id - 스트림 정보
- POST /api/v1/streams/:id/start - 온디맨드 시작
- DELETE /api/v1/streams/:id - 스트림 정지
- GET /api/v1/health - 헬스 체크

### 실제 배포 카메라 정보

| 카메라 ID | 위치 | 코덱 | 타입 | 상태 |
|----------|-----|------|-----|------|
| plx_cctv_01 | 192.168.4.121 | H265 | Always-on | ✅ 작동 |
| plx_cctv_02 | 192.168.4.54 | H264 | On-demand | ✅ 작동 |
| plx_cctv_03 | 192.168.4.46 | H265 | On-demand | ✅ 작동 |
| park_cctv_01 | 121.190.36.211 | - | On-demand | ⚠️ 외부망 |

### 다음 세션 시작 시

1. README.md와 CLAUDE.md 먼저 읽기
2. 서버 실행 상태 확인 (포트 8080)
3. 최근 변경사항 git log 확인
4. 대시보드 접속하여 정상 동작 확인: http://localhost:8080/static/dashboard.html
5. 모든 카메라 자동 연결 확인

---

**마지막 업데이트**: 2025-10-29
**현재 버전**: v0.2.0 (mediaMTX Edition)
**프로젝트 상태**: Phase 7 완료 - 다중 카메라 시스템 완성 ✅
**다음 마일스톤**: Phase 8 - 성능 최적화 및 부하 테스트
