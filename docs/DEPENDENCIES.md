# MediaStream 프로젝트 의존성 분석

> **작성일**: 2025-11-24
> **프로젝트**: RTSP to WebRTC Media Server (cctv3)
> **Go 버전**: 1.24.0

---

## 📋 목차

1. [핵심 의존성 (Direct Dependencies)](#핵심-의존성-direct-dependencies)
2. [카테고리별 라이브러리 분류](#카테고리별-라이브러리-분류)
3. [간접 의존성 (Indirect Dependencies)](#간접-의존성-indirect-dependencies)
4. [의존성 트리 및 사용처](#의존성-트리-및-사용처)
5. [라이선스 정보](#라이선스-정보)
6. [보안 및 유지보수](#보안-및-유지보수)
7. [업데이트 정책](#업데이트-정책)

---

## 핵심 의존성 (Direct Dependencies)

프로젝트는 총 **21개의 직접 의존성**을 가지고 있습니다.

### 미디어 스트리밍 라이브러리

| 라이브러리 | 버전 | 목적 | 중요도 |
|-----------|------|------|--------|
| `github.com/pion/webrtc/v4` | v4.1.6 | WebRTC 구현 (Pure Go) | ⭐⭐⭐ 필수 |
| `github.com/bluenviron/gortsplib/v4` | v4.16.2 | RTSP 클라이언트/서버 | ⭐⭐⭐ 필수 |
| `github.com/bluenviron/gohlslib/v2` | v2.2.3 | HLS 스트리밍 | ⭐⭐⭐ 필수 |
| `github.com/pion/rtp` | v1.8.23 | RTP 패킷 처리 | ⭐⭐⭐ 필수 |
| `github.com/pion/sdp/v3` | v3.0.16 | SDP 파싱 및 생성 | ⭐⭐⭐ 필수 |
| `github.com/pion/interceptor` | v0.1.41 | WebRTC interceptor | ⭐⭐ 중요 |

### 웹 프레임워크 및 서버

| 라이브러리 | 버전 | 목적 | 중요도 |
|-----------|------|------|--------|
| `github.com/gin-gonic/gin` | v1.10.0 | HTTP 웹 프레임워크 | ⭐⭐⭐ 필수 |
| `github.com/gorilla/websocket` | v1.5.1 | WebSocket 시그널링 | ⭐⭐⭐ 필수 |

### 미디어 처리

| 라이브러리 | 버전 | 목적 | 중요도 |
|-----------|------|------|--------|
| `github.com/asticode/go-astits` | v1.14.0 | MPEG-TS 패킷 처리 | ⭐⭐ 중요 |
| `github.com/grafov/m3u8` | v0.12.1 | M3U8 플레이리스트 파싱 | ⭐⭐ 중요 |

### 로깅 및 설정

| 라이브러리 | 버전 | 목적 | 중요도 |
|-----------|------|------|--------|
| `go.uber.org/zap` | v1.27.0 | 구조화 로깅 | ⭐⭐⭐ 필수 |
| `gopkg.in/natefinch/lumberjack.v2` | v2.2.1 | 로그 로테이션 | ⭐⭐ 중요 |
| `gopkg.in/yaml.v3` | v3.0.1 | YAML 설정 파싱 | ⭐⭐⭐ 필수 |

### 유틸리티

| 라이브러리 | 버전 | 목적 | 중요도 |
|-----------|------|------|--------|
| `github.com/google/uuid` | v1.6.0 | UUID 생성 | ⭐⭐ 중요 |
| `github.com/stretchr/testify` | v1.11.1 | 테스트 프레임워크 | ⭐⭐ 중요 |

---

## 카테고리별 라이브러리 분류

### 1. WebRTC 관련 (Pion 생태계)

**Pion**은 Pure Go로 작성된 WebRTC 구현입니다. 프로젝트의 핵심 의존성입니다.

```
github.com/pion/webrtc/v4         - WebRTC 메인 라이브러리
github.com/pion/rtp               - RTP 패킷 처리
github.com/pion/sdp/v3            - SDP 파싱/생성
github.com/pion/interceptor       - RTP/RTCP interceptor
github.com/pion/rtcp              - RTCP 프로토콜 (간접)
github.com/pion/ice/v4            - ICE 연결 관리 (간접)
github.com/pion/dtls/v3           - DTLS 암호화 (간접)
github.com/pion/srtp/v3           - SRTP 암호화 (간접)
github.com/pion/sctp              - SCTP 프로토콜 (간접)
github.com/pion/datachannel       - DataChannel (간접)
github.com/pion/turn/v4           - TURN 서버 (간접)
github.com/pion/stun/v3           - STUN 프로토콜 (간접)
github.com/pion/mdns/v2           - mDNS (간접)
github.com/pion/transport/v3      - 네트워크 전송 (간접)
github.com/pion/logging           - Pion 로깅 (간접)
github.com/pion/randutil          - 랜덤 유틸리티 (간접)
```

**사용처**:
- `internal/webrtc/peer.go` - WebRTC 피어 연결 관리
- `internal/webrtc/manager.go` - WebRTC 매니저
- `internal/rtsp/client.go` - RTP 패킷 수신
- `internal/hls/muxer.go` - RTP 패킷을 HLS로 변환

**특징**:
- ✅ Pure Go 구현 (C 의존성 없음)
- ✅ 크로스 플랫폼 지원
- ✅ v4부터 대규모 API 개선
- ⚠️ API 변경이 자주 발생 (메이저 버전 업데이트 주의)

---

### 2. RTSP/미디어 스트리밍 (Bluenviron 생태계)

**Bluenviron**은 mediaMTX 개발팀에서 만든 미디어 스트리밍 라이브러리입니다.

```
github.com/bluenviron/gortsplib/v4    - RTSP 클라이언트/서버
github.com/bluenviron/gohlslib/v2     - HLS 스트리밍
github.com/bluenviron/mediacommon/v2  - 공통 미디어 유틸리티 (간접)
```

**사용처**:
- `internal/rtsp/client.go` - RTSP 클라이언트 (카메라 연결)
- `internal/rtsp/server_rtsp.go` - RTSP 서버
- `internal/rtsp/publisher.go` - RTSP 퍼블리셔
- `internal/rtsp/subscriber.go` - RTSP 구독자
- `internal/rtsp/path_manager.go` - RTSP 경로 관리
- `internal/hls/muxer_gohlslib.go` - HLS muxer

**특징**:
- ✅ mediaMTX와 동일한 코드베이스 (검증된 안정성)
- ✅ H.265/H.264 코덱 자동 감지
- ✅ TCP/UDP 전송 지원
- ✅ RTSP 인증 지원

---

### 3. HLS 관련

```
github.com/grafov/m3u8            - M3U8 플레이리스트
github.com/asticode/go-astits     - MPEG-TS 패킷 처리
```

**사용처**:
- `internal/hls/types.go` - HLS 타입 정의
- `internal/hls/muxer.go` - HLS muxer
- `internal/hls/manager.go` - HLS 관리자

**특징**:
- ✅ HTTP Live Streaming 지원
- ✅ MPEG-TS 컨테이너 생성
- ✅ 다양한 플레이리스트 타입 지원

---

### 4. 웹 프레임워크

```
github.com/gin-gonic/gin          - HTTP 웹 프레임워크
github.com/gorilla/websocket      - WebSocket 프로토콜
```

**사용처**:
- `internal/api/server.go` - HTTP API 서버
- `internal/api/paths_handler.go` - REST API 핸들러
- `internal/signaling/server.go` - WebSocket 시그널링 서버

**특징**:
- ✅ Gin: 고성능 HTTP 라우터 (Martini API 호환)
- ✅ Gorilla WebSocket: 업계 표준 WebSocket 라이브러리
- ✅ 미들웨어 지원 (CORS, 로깅, 인증 등)

---

### 5. 로깅

```
go.uber.org/zap                   - 구조화 로깅
gopkg.in/natefinch/lumberjack.v2  - 로그 로테이션
```

**사용처**:
- `pkg/logger/logger.go` - 로거 초기화 및 설정
- 모든 패키지에서 사용

**특징**:
- ✅ Zap: 초고성능 구조화 로깅 (JSON 지원)
- ✅ Lumberjack: 날짜별/크기별 로그 로테이션
- ✅ 자동 압축 및 백업

**설정 예시**:
```yaml
# configs/config.yaml
logging:
  level: info
  output: both  # console, file, both
  file_path: logs/media-server.log
  max_size: 500      # MB
  max_backups: 15    # 파일 개수
  max_age: 15        # 일
  compress: true     # gzip 압축
```

---

### 6. 설정 관리

```
gopkg.in/yaml.v3                  - YAML 파싱
```

**사용처**:
- `internal/core/config.go` - 설정 로드
- `configs/config.yaml` - 설정 파일

**특징**:
- ✅ YAML 1.2 지원
- ✅ 구조체 태그 기반 파싱
- ✅ 주석 유지 가능

---

### 7. 데이터베이스 (SQLite)

```
modernc.org/sqlite                - Pure Go SQLite
```

**사용처**:
- `internal/database/database.go` - 데이터베이스 연결
- `internal/database/stream_repository.go` - 스트림 저장소

**특징**:
- ✅ Pure Go 구현 (CGO 불필요)
- ✅ SQLite3 호환
- ✅ 크로스 플랫폼

---

### 8. 유틸리티

```
github.com/google/uuid            - UUID 생성
github.com/stretchr/testify       - 테스트 프레임워크
```

**사용처**:
- `internal/webrtc/peer.go` - 피어 ID 생성
- `test/e2e/*_test.go` - 테스트 코드

---

## 간접 의존성 (Indirect Dependencies)

총 **55개의 간접 의존성**이 있습니다.

### 주요 간접 의존성

#### Pion 생태계 (WebRTC)
```
github.com/pion/datachannel      v1.5.10   - WebRTC DataChannel
github.com/pion/dtls/v3          v3.0.7    - DTLS 암호화
github.com/pion/ice/v4           v4.0.10   - ICE 연결 관리
github.com/pion/rtcp             v1.2.15   - RTCP 프로토콜
github.com/pion/sctp             v1.8.40   - SCTP 프로토콜
github.com/pion/srtp/v3          v3.0.8    - SRTP 암호화
github.com/pion/stun/v3          v3.0.0    - STUN 프로토콜
github.com/pion/turn/v4          v4.1.1    - TURN 서버
github.com/pion/mdns/v2          v2.0.7    - mDNS
github.com/pion/transport/v3     v3.0.8    - 네트워크 전송
github.com/pion/logging          v0.2.4    - Pion 로깅
github.com/pion/randutil         v0.1.0    - 랜덤 유틸리티
```

#### Bluenviron 생태계
```
github.com/bluenviron/mediacommon/v2  v2.4.2  - 미디어 공통 유틸리티
```

#### Gin 프레임워크
```
github.com/gin-contrib/sse                v0.1.0   - Server-Sent Events
github.com/go-playground/validator/v10    v10.20.0 - 입력 검증
github.com/bytedance/sonic                v1.11.6  - 고성능 JSON 인코딩
github.com/ugorji/go/codec                v1.2.12  - MessagePack 코덱
```

#### 미디어 처리
```
github.com/abema/go-mp4          v1.4.1    - MP4 파일 처리
github.com/asticode/go-astikit   v0.30.0   - Astits 유틸리티
```

#### 시스템 라이브러리
```
golang.org/x/crypto              v0.41.0   - 암호화
golang.org/x/net                 v0.43.0   - 네트워크
golang.org/x/sys                 v0.36.0   - 시스템 호출
golang.org/x/text                v0.28.0   - 텍스트 처리
golang.org/x/exp                 v0.0.0-20250620022241-b7579e27df2b - 실험적 기능
```

#### 기타
```
go.uber.org/multierr             v1.10.0   - 다중 에러 처리
github.com/wlynxg/anet           v0.0.5    - 네트워크 유틸리티
github.com/dustin/go-humanize    v1.0.1    - 인간 친화적 포맷팅
```

---

## 의존성 트리 및 사용처

### 주요 파일별 의존성

#### `cmd/server/main.go` (메인 애플리케이션)
```go
github.com/pion/rtp                      // RTP 패킷 처리
github.com/yourusername/cctv3/internal/* // 내부 패키지
go.uber.org/zap                          // 로깅
```

#### `internal/webrtc/peer.go` (WebRTC 피어)
```go
github.com/google/uuid              // 피어 ID 생성
github.com/pion/rtp                 // RTP 패킷
github.com/pion/webrtc/v4           // WebRTC API
```

#### `internal/rtsp/client.go` (RTSP 클라이언트)
```go
github.com/bluenviron/gortsplib/v4                  // RTSP 클라이언트
github.com/bluenviron/gortsplib/v4/pkg/base         // RTSP 베이스
github.com/bluenviron/gortsplib/v4/pkg/description  // SDP 설명
github.com/bluenviron/gortsplib/v4/pkg/format       // 미디어 포맷
github.com/pion/rtp                                 // RTP 패킷
```

#### `internal/hls/muxer_gohlslib.go` (HLS Muxer)
```go
github.com/bluenviron/gohlslib/v2              // HLS 라이브러리
github.com/bluenviron/gohlslib/v2/pkg/codecs   // 코덱 정의
github.com/pion/rtp                            // RTP 패킷
```

#### `internal/api/server.go` (API 서버)
```go
github.com/gin-gonic/gin                  // HTTP 프레임워크
github.com/yourusername/cctv3/internal/*  // 내부 패키지
```

#### `internal/signaling/server.go` (시그널링 서버)
```go
github.com/gorilla/websocket  // WebSocket
```

---

## 라이선스 정보

### 오픈소스 라이선스 요약

| 라이브러리 | 라이선스 | 상업적 사용 | 주의사항 |
|-----------|---------|-----------|----------|
| Pion (webrtc, rtp 등) | MIT | ✅ 가능 | 없음 |
| Bluenviron (gortsplib, gohlslib) | MIT | ✅ 가능 | 없음 |
| Gin | MIT | ✅ 가능 | 없음 |
| Gorilla WebSocket | BSD-2-Clause | ✅ 가능 | 없음 |
| Zap | MIT | ✅ 가능 | 없음 |
| UUID | BSD-3-Clause | ✅ 가능 | 없음 |
| Testify | MIT | ✅ 가능 | 없음 |
| go-astits | MIT | ✅ 가능 | 없음 |
| m3u8 | BSD-3-Clause | ✅ 가능 | 없음 |
| YAML v3 | MIT | ✅ 가능 | 없음 |
| Lumberjack | MIT | ✅ 가능 | 없음 |
| modernc.org/sqlite | BSD-3-Clause | ✅ 가능 | 없음 |

**결론**: 모든 의존성이 **MIT 또는 BSD 라이선스**로, 상업적 사용에 제약이 없습니다. ✅

---

## 보안 및 유지보수

### 취약점 스캔

```bash
# Go 의존성 취약점 확인
go list -json -m all | nancy sleuth

# 또는 govulncheck 사용
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

### 의존성 업데이트

```bash
# 모든 의존성 최신 마이너 버전으로 업데이트
go get -u ./...

# 특정 패키지만 업데이트
go get -u github.com/pion/webrtc/v4@latest

# go.mod 정리
go mod tidy
```

### 주의해야 할 패키지

| 패키지 | 이유 | 권장사항 |
|--------|------|----------|
| `pion/webrtc/v4` | API 변경 빈번 | 메이저 버전 업데이트 전 릴리스 노트 확인 |
| `bluenviron/gortsplib/v4` | API 변경 빈번 | 메이저 버전 업데이트 전 테스트 필수 |
| `gin-gonic/gin` | 미들웨어 호환성 | 업데이트 후 API 엔드포인트 테스트 |

---

## 업데이트 정책

### 정기 업데이트 주기

1. **보안 패치**: 즉시 적용
2. **마이너 버전**: 월 1회 검토
3. **메이저 버전**: 분기 1회 검토

### 업데이트 체크리스트

- [ ] `go.mod`에서 의존성 버전 확인
- [ ] 릴리스 노트 확인 (Breaking Changes)
- [ ] 로컬 테스트 실행 (`go test ./...`)
- [ ] E2E 테스트 실행 (`go test -v ./test/e2e/...`)
- [ ] 실제 카메라로 스트리밍 테스트
- [ ] 변경사항 문서화 (CLAUDE.md, CHANGELOG.md)
- [ ] Git 커밋 및 태그

### 현재 버전 정책

```yaml
# 주요 의존성 버전 고정
pion/webrtc/v4: ~v4.1.6       # 메이저 버전 고정
gortsplib/v4: ~v4.16.2        # 메이저 버전 고정
gin: ~v1.10.0                 # 안정 버전 사용
```

---

## 의존성 최소화 전략

### 불필요한 의존성 제거

현재 프로젝트는 **필수 의존성만** 포함하고 있습니다.

```bash
# 사용하지 않는 의존성 자동 제거
go mod tidy
```

### 대체 가능한 패키지

일부 의존성은 표준 라이브러리로 대체 가능합니다:

| 현재 | 대체 가능 | 추천 |
|------|----------|------|
| `google/uuid` | `crypto/rand` + 수동 생성 | ❌ 유지 (편의성) |
| `yaml.v3` | `encoding/json` | ❌ 유지 (YAML 필요) |
| `testify` | `testing` 표준 라이브러리 | ❌ 유지 (가독성) |

---

## 의존성 다이어그램

```
MediaStream 프로젝트
│
├─ WebRTC 스택 (Pion)
│  ├─ pion/webrtc/v4 ────────┐
│  ├─ pion/rtp                │
│  ├─ pion/sdp/v3             │
│  ├─ pion/interceptor        │
│  └─ pion/* (간접)           │
│                             ↓
├─ RTSP/미디어 (Bluenviron)   WebRTC 피어 연결
│  ├─ gortsplib/v4 ──────────┐
│  ├─ gohlslib/v2             │
│  └─ mediacommon/v2 (간접)   │
│                             ↓
├─ HLS                       RTSP 카메라 스트림
│  ├─ grafov/m3u8            │
│  └─ asticode/go-astits     │
│                             ↓
├─ 웹 서버                   HLS 스트리밍
│  ├─ gin-gonic/gin
│  └─ gorilla/websocket
│
├─ 로깅
│  ├─ uber-go/zap
│  └─ lumberjack.v2
│
├─ 설정
│  └─ yaml.v3
│
├─ 데이터베이스
│  └─ modernc.org/sqlite
│
└─ 유틸리티
   ├─ google/uuid
   └─ stretchr/testify
```

---

## 요약

### 통계

- **총 의존성**: 76개 (직접 21개 + 간접 55개)
- **평균 업데이트**: 월 1~2회
- **라이선스**: 100% MIT/BSD (상업적 사용 가능)
- **보안 취약점**: 0개 (2025-11-24 기준)

### 핵심 의존성 Top 5

1. **pion/webrtc/v4** - WebRTC 구현 (가장 중요)
2. **bluenviron/gortsplib/v4** - RTSP 클라이언트 (가장 중요)
3. **gin-gonic/gin** - HTTP 프레임워크
4. **gorilla/websocket** - WebSocket 시그널링
5. **uber-go/zap** - 구조화 로깅

### 유지보수 포인트

- ✅ 모든 의존성이 활발히 유지보수됨
- ✅ 커뮤니티 지원 우수
- ✅ 프로덕션 레벨 안정성
- ⚠️ Pion v4, gortsplib v4는 API 변경 주의

---

**마지막 업데이트**: 2025-11-24
**문서 버전**: 1.0
