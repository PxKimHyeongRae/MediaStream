# Phase 3: RTP 패킷 수신 수정 완료

**완료일**: 2025-10-29
**작업 시간**: 1시간
**상태**: ✅ RTP 패킷 수신 구현 완료

---

## 🎯 문제 정의

Phase 2 완료 후, `internal/rtsp/client.go`의 `readPackets()` 함수가 placeholder로 남아있었습니다:

```go
// 이전 코드 (문제)
func (c *Client) readPackets() {
    for {
        select {
        case <-c.ctx.Done():
            return
        default:
            // Note: gortsplib v4는 OnDecodeError 콜백을 통해 패킷을 받습니다
            // 현재는 클라이언트의 Wait()가 패킷을 자동으로 처리하므로
            // 이 고루틴은 placeholder입니다
            time.Sleep(time.Millisecond)
        }
    }
}
```

이 코드는 실제로 RTP 패킷을 읽지 않고 있었으며, gortsplib v4 API 변경으로 인해 `OnPacketRTP` 콜백이 제거된 상황이었습니다.

---

## 🔍 조사 결과

### gortsplib v4 API 분석

gortsplib v4.16.2에서는 다음과 같은 콜백 메서드를 제공합니다:

1. **`OnPacketRTPAny(cb OnPacketRTPAnyFunc)`**
   - 모든 미디어 트랙의 RTP 패킷을 하나의 콜백으로 수신
   - 시그니처: `func(*description.Media, format.Format, *rtp.Packet)`
   - 추천: 간단한 구현에 적합

2. **`OnPacketRTP(medi *description.Media, forma format.Format, cb OnPacketRTPFunc)`**
   - 특정 미디어/포맷에 대한 개별 콜백 등록
   - 시그니처: `func(*rtp.Packet)`
   - 추천: 미디어별 다른 처리가 필요한 경우

### 핵심 발견사항

- **콜백 등록 시점**: `SetupAll()` 이후, `Play()` 이전에 등록해야 함
- **자동 호출**: gortsplib가 내부적으로 RTP 패킷을 읽고 콜백을 자동 호출
- **별도 고루틴 불필요**: `readPackets()` 같은 별도 고루틴이 필요 없음

---

## ✅ 구현 내용

### 1. 필요한 import 추가

```go
import (
    // ... 기존 imports
    "github.com/bluenviron/gortsplib/v4/pkg/description"
    "github.com/bluenviron/gortsplib/v4/pkg/format"
)
```

### 2. `run()` 함수 수정

**변경 위치**: `internal/rtsp/client.go:241-286`

```go
// 미디어 정보 로깅 추가
for i, media := range desc.Medias {
    for j, forma := range media.Formats {
        c.logger.Info("Media format detected",
            zap.Int("media_index", i),
            zap.Int("format_index", j),
            zap.String("codec", forma.Codec()),
            zap.Uint8("payload_type", forma.PayloadType()),
        )
    }
}

// SETUP: 모든 미디어 트랙 설정
err = c.client.SetupAll(baseURL, desc.Medias)
if err != nil {
    return fmt.Errorf("failed to setup: %w", err)
}

c.logger.Info("All media tracks setup completed")

// ✅ RTP 패킷 콜백 등록 (핵심 수정!)
c.client.OnPacketRTPAny(func(medi *description.Media, forma format.Format, pkt *rtp.Packet) {
    // RTP 패킷 수신 시 호출됨
    c.handleRTPPacket(pkt)
})

c.logger.Info("RTP packet callback registered")

// PLAY: 재생 시작
_, err = c.client.Play(nil)
if err != nil {
    return fmt.Errorf("failed to play: %w", err)
}

c.logger.Info("RTSP playback started")

c.setConnected(true)

if c.onConnect != nil {
    c.onConnect()
}

// 연결 유지 및 에러 대기
// OnPacketRTPAny 콜백이 자동으로 RTP 패킷을 수신하므로
// 별도의 readPackets 고루틴은 불필요
return c.client.Wait()
```

### 3. `readPackets()` 함수 제거

불필요한 placeholder 함수를 완전히 제거했습니다.

---

## 📊 데이터 흐름

### 완성된 전체 파이프라인

```
┌─────────────────────────┐
│   RTSP Camera           │
│   192.168.4.121:554     │
└──────────┬──────────────┘
           │ RTSP Protocol
           ▼
┌─────────────────────────┐
│   gortsplib Client      │
│   - Start()             │
│   - Describe()          │
│   - SetupAll()          │
│   - OnPacketRTPAny() ✅ │  ◄─── 여기서 콜백 등록!
│   - Play()              │
│   - Wait()              │
└──────────┬──────────────┘
           │ RTP Packets (자동 수신)
           │ ┌──────────────────────┐
           └─▶ OnPacketRTPAny()    │
             │ Callback 자동 호출   │
             └──────────┬───────────┘
                        │
                        ▼
           ┌────────────────────────┐
           │  handleRTPPacket()     │
           │  - 통계 업데이트        │
           │  - onPacket() 호출     │
           └──────────┬─────────────┘
                      │
                      ▼
           ┌────────────────────────┐
           │  Stream.WritePacket()  │
           │  - 버퍼에 패킷 저장     │
           │  - 구독자에게 배포      │
           └──────────┬─────────────┘
                      │
                      ▼
           ┌────────────────────────┐
           │  WebRTC Peer           │
           │  - OnPacket() 수신     │
           │  - videoTrack.WriteRTP()│
           └──────────┬─────────────┘
                      │ WebRTC/SRTP
                      ▼
           ┌────────────────────────┐
           │  Web Browser           │
           │  - Video 재생          │
           └────────────────────────┘
```

---

## 🧪 빌드 검증

### 빌드 명령
```bash
go build -o bin/media-server.exe cmd/server/main.go
```

### 결과
```bash
$ ls -lh bin/media-server.exe
-rwxr-xr-x 1 user group 18M Oct 29 10:57 bin/media-server.exe
```

✅ **빌드 성공**: 18MB 바이너리 생성 완료

---

## 📝 코드 변경 요약

| 파일 | 변경 내용 | 라인 |
|------|----------|------|
| `internal/rtsp/client.go` | import 추가 (description, format) | 12-13 |
| `internal/rtsp/client.go` | 미디어 정보 로깅 추가 | 241-251 |
| `internal/rtsp/client.go` | OnPacketRTPAny 콜백 등록 | 261-266 |
| `internal/rtsp/client.go` | readPackets() 함수 제거 | (삭제됨) |
| `internal/rtsp/client.go` | go c.readPackets() 호출 제거 | (삭제됨) |

---

## 🔧 핵심 개선사항

### 이전 (Phase 2)
- ❌ placeholder `readPackets()` 함수로 패킷을 실제로 읽지 않음
- ❌ 불필요한 고루틴으로 CPU 낭비
- ❌ 주석에만 "gortsplib가 처리한다"고 적혀있음

### 현재 (Phase 3)
- ✅ `OnPacketRTPAny()` 콜백으로 실제 패킷 수신
- ✅ gortsplib 내부 고루틴 활용으로 효율적
- ✅ 미디어 포맷 정보 로깅으로 디버깅 용이
- ✅ 올바른 API 사용으로 향후 안정성 보장

---

## 🚀 다음 단계

### 우선순위 1: 실제 카메라 테스트 ⚡
```bash
# 서버 실행
./bin/media-server.exe

# 웹 브라우저 접속
http://localhost:8080

# "Connect" 버튼 클릭
```

**예상 동작**:
1. RTSP 연결 성공 메시지
2. "Media format detected" 로그 출력 (H.264 비디오, 오디오 등)
3. "RTP packet callback registered" 로그
4. "RTSP playback started" 로그
5. 웹 브라우저에서 비디오 재생 시작

### 우선순위 2: 로그 확인
다음 로그가 나타나는지 확인:
```
[INFO] Connected to RTSP server
[INFO] Stream description received (media_count=2)
[INFO] Media format detected (codec="H264", payload_type=96)
[INFO] Media format detected (codec="MPEG4-GENERIC", payload_type=97)
[INFO] All media tracks setup completed
[INFO] RTP packet callback registered
[INFO] RTSP playback started
```

### 우선순위 3: 통계 확인
`handleRTPPacket()` 함수에서 업데이트되는 통계 확인:
- `packetsReceived` 증가
- `bytesReceived` 증가
- WebRTC peer의 `packetsSent` 증가

### 우선순위 4: 성능 측정
- ⏱️ 지연시간 (RTSP 카메라 → 웹 브라우저)
- 🎯 목표: < 1초
- 📊 CPU 사용률
- 💾 메모리 사용량

---

## 📚 참고 자료

### gortsplib v4 API 문서
- [gortsplib GitHub](https://github.com/bluenviron/gortsplib)
- 콜백 메서드: `OnPacketRTPAny()`, `OnPacketRTP()`
- 사용 순서: Start → Describe → SetupAll → **OnPacket*** → Play → Wait

### mediaMTX 참조 코드
- `internal/protocols/rtsp/to_stream.go`
- Format별 `SetOnPacketRTP()` 사용 패턴

### 프로젝트 문서
- `docs/PHASE2_COMPLETE.md` - Phase 2 완료 보고서
- `docs/mediamtx-architecture-analysis.md` - mediaMTX 아키텍처 분석

---

## ✅ 완료 체크리스트

- [x] gortsplib v4 API 조사
- [x] OnPacketRTPAny 콜백 구현
- [x] 미디어 정보 로깅 추가
- [x] readPackets() 함수 제거
- [x] 빌드 성공 확인
- [ ] 실제 RTSP 카메라 테스트
- [ ] 웹 브라우저 재생 확인
- [ ] 지연시간 측정
- [ ] 다중 클라이언트 테스트

---

**현재 상태**: 코드 구현 완료 ✅
**다음 작업**: 실제 RTSP 스트림 테스트 🚀
**예상 소요 시간**: 30분 (테스트 및 검증)
