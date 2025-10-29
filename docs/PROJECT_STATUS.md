# 프로젝트 현재 상태

**생성일**: 2025-10-29
**버전**: 0.1.0 (Phase 1)
**상태**: 기본 구조 완성, 실제 RTSP/WebRTC 연동 대기

---

## ✅ 완료된 작업

### 1. 프로젝트 초기 설정
- [x] Go 모듈 설정 (go.mod)
- [x] 디렉토리 구조 생성
- [x] 설정 파일 및 로더
- [x] 로거 시스템
- [x] 개발 도구 (Makefile, Docker)
- [x] 빌드 성공 ✅

### 2. 핵심 컴포넌트 구현
- [x] **RTSP 클라이언트** (`internal/rtsp/client.go`)
  - 재연결 로직
  - RTP 패킷 콜백
  - 통계 수집

- [x] **스트림 관리자** (`internal/core/stream_manager.go`)
  - 스트림 생성/제거
  - 구독자 관리
  - 패킷 배포 (pub/sub 패턴)

- [x] **WebRTC 피어** (`internal/webrtc/peer.go`, `manager.go`)
  - 피어 연결 관리
  - 통계 수집
  - 피어 풀 관리

- [x] **시그널링 서버** (`internal/signaling/server.go`)
  - WebSocket 기반 시그널링
  - Offer/Answer 교환
  - ICE candidate 처리

- [x] **HTTP API 서버** (`internal/api/server.go`)
  - REST API 엔드포인트
  - 헬스 체크
  - 스트림 정보 조회
  - WebSocket 엔드포인트

- [x] **웹 클라이언트** (`web/static/`)
  - HTML5 비디오 플레이어
  - WebRTC 연결 로직
  - 실시간 통계 표시
  - 로그 뷰어

### 3. 통합 및 빌드
- [x] main.go 통합
- [x] 모든 컴포넌트 초기화
- [x] 빌드 성공
- [x] 설정 파일 완성

### 4. 문서화
- [x] README.md (프로젝트 개요)
- [x] mediaMTX 아키텍처 분석 문서
- [x] 설정 파일 (config.yaml)

---

## 🚧 현재 제한사항 (Phase 1)

### 1. RTSP 클라이언트 (internal/rtsp/client.go)
**현재 상태**: 기본 구조만 구현
**미구현 기능**:
- gortsplib를 사용한 실제 RTSP 연결
- RTP 패킷 파싱
- 코덱 협상

**필요한 작업**:
```go
// TODO in client.go:
func (c *Client) run() error {
    // gortsplib.Client 생성
    // RTSP DESCRIBE, SETUP, PLAY
    // RTP 패킷 수신 및 처리
}
```

### 2. WebRTC 피어 (internal/webrtc/peer.go)
**현재 상태**: 인터페이스만 구현
**미구현 기능**:
- pion/webrtc PeerConnection 생성
- SDP 협상
- RTP 패킷을 WebRTC 트랙으로 전송

**필요한 작업**:
```go
// TODO in peer.go:
// - pion/webrtc API 통합
// - createPeerConnection()
// - createOffer()/createAnswer()
// - addTrack() 및 RTP 전송
```

### 3. 시그널링 서버 (internal/signaling/server.go)
**현재 상태**: WebSocket 통신 구현
**미구현 기능**:
- 실제 Offer 처리 및 Answer 생성

**필요한 작업**:
```go
// TODO in main.go OnOffer callback:
// - WebRTC PeerConnection 생성
// - Remote Offer 설정
// - Local Answer 생성
// - 스트림 구독 연결
```

---

## 📋 다음 단계 (Phase 2)

### 우선순위 1: RTSP to WebRTC 파이프라인 완성
1. **RTSP 클라이언트 완성**
   ```bash
   # 필요한 라이브러리 추가
   go get github.com/bluenviron/gortsplib/v4
   go get github.com/bluenviron/mediacommon
   ```

2. **WebRTC 피어 완성**
   ```bash
   go get github.com/pion/webrtc/v4
   go get github.com/pion/interceptor
   ```

3. **전체 파이프라인 연결**
   ```
   RTSP → RTP Packets → Stream → WebRTC Peers → Web Client
   ```

### 우선순위 2: 테스트 및 검증
1. 단일 스트림 재생 테스트
2. 다중 클라이언트 연결 테스트
3. 재연결 테스트
4. 지연시간 측정

### 우선순위 3: 성능 최적화
1. 버퍼 크기 튜닝
2. 고루틴 수 최적화
3. 메모리 프로파일링

---

## 🏗️ 현재 아키텍처

### 데이터 흐름
```
[RTSP Camera]
    ↓
[RTSP Client] ──(RTP Packets)──> [Stream Manager]
                                        ↓
                                  [Subscribers]
                                        ↓
                                  [WebRTC Peers] ──(WebRTC)──> [Web Clients]
                                        ↑
                                        │
                                 [Signaling Server]
                                     (WebSocket)
```

### 파일 구조
```
cctv3/
├── cmd/server/main.go              # 메인 엔트리포인트 ✅
├── internal/
│   ├── core/
│   │   ├── config.go               # 설정 로더 ✅
│   │   └── stream_manager.go       # 스트림 관리 ✅
│   ├── rtsp/
│   │   └── client.go               # RTSP 클라이언트 🚧
│   ├── webrtc/
│   │   ├── peer.go                 # WebRTC 피어 🚧
│   │   └── manager.go              # 피어 관리자 ✅
│   ├── signaling/
│   │   └── server.go               # 시그널링 서버 ✅
│   └── api/
│       └── server.go               # HTTP API ✅
├── web/static/
│   ├── index.html                  # 웹 클라이언트 ✅
│   ├── app.js                      # WebRTC 클라이언트 로직 ✅
│   └── style.css                   # 스타일 ✅
├── configs/
│   └── config.yaml                 # 설정 파일 ✅
└── docs/
    ├── mediamtx-architecture-analysis.md  # mediaMTX 분석 ✅
    └── PROJECT_STATUS.md           # 현재 문서 ✅
```

---

## 🚀 실행 방법 (Phase 2 완성 후)

### 1. 설정 파일 수정
```yaml
# configs/config.yaml
rtsp:
  test_stream:
    url: "rtsp://admin:live0416@192.168.4.121:554/Streaming/Channels/101"
    name: "test-camera-1"
```

### 2. 서버 실행
```bash
go run cmd/server/main.go
# 또는
./bin/media-server.exe
```

### 3. 웹 브라우저 접속
```
http://localhost:8080
```

### 4. Connect 버튼 클릭
웹 페이지에서 "Connect" 버튼을 클릭하여 스트림 시청

---

## 📊 목표 성능 (Phase 1 완성 시)

| 메트릭 | 목표 |
|--------|------|
| 동시 스트림 | 1-10개 |
| 동시 클라이언트 | 10-50개 |
| 지연시간 | < 1초 |
| CPU 사용률 | < 30% |
| 메모리 | < 500MB |

---

## 🔧 개발 도구

### 빌드
```bash
make build
```

### 개발 모드 (hot reload)
```bash
make dev
```

### 테스트
```bash
make test
```

### Docker
```bash
make docker-build
make docker-run
```

---

## 📝 참고 자료

- [mediaMTX](https://github.com/bluenviron/mediamtx) - 참조 아키텍처
- [pion/webrtc](https://github.com/pion/webrtc) - Go WebRTC 구현
- [gortsplib](https://github.com/bluenviron/gortsplib) - Go RTSP 라이브러리
- [WebRTC 명세](https://webrtc.org/) - WebRTC 표준

---

## ✅ 체크리스트

### Phase 1 (현재)
- [x] 프로젝트 구조 설정
- [x] 기본 컴포넌트 구현
- [x] 빌드 성공
- [x] 웹 클라이언트 UI
- [ ] RTSP 연결 (실제 구현 필요)
- [ ] WebRTC 연결 (실제 구현 필요)
- [ ] 전체 파이프라인 테스트

### Phase 2 (다음)
- [ ] gortsplib 통합
- [ ] pion/webrtc 통합
- [ ] 단일 스트림 재생 성공
- [ ] 다중 클라이언트 지원
- [ ] 성능 측정 및 최적화

### Phase 3 (향후)
- [ ] 다중 스트림 지원
- [ ] 메트릭 수집 (Prometheus)
- [ ] 로드 밸런싱
- [ ] 보안 강화
- [ ] 프로덕션 배포

---

**현재 상태**: Phase 1 기본 구조 완성 ✅
**다음 작업**: RTSP 및 WebRTC 실제 연동
**예상 작업 시간**: 4-8시간 (Phase 2 완성)
