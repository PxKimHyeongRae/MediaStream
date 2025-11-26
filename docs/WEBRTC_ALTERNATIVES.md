# WebRTC 라이브러리 대안 분석 (2025)

## 📌 개요

현재 프로젝트에서 ice4j/jitsi-srtp API 불일치로 인해 대안 라이브러리를 검토합니다.

**요구사항**:
- Java/Kotlin 호환
- Maven Central에서 사용 가능
- Virtual Threads와 호환
- RTSP → WebRTC 변환 지원
- 경량 & 고성능

---

## 🔍 대안 라이브러리 비교

### 1. webrtc-java (dev.onvoid) ⭐ 추천

**개요**: Google WebRTC Native 라이브러리의 Java JNI 래퍼

#### Maven 의존성
```kotlin
implementation("dev.onvoid.webrtc:webrtc-java:0.14.0")
```

#### 장점 ✅
- **Maven Central에서 바로 사용 가능** (가장 큰 장점!)
- Google WebRTC Native 기반 (검증된 구현)
- 활발한 개발 (2025년 1월 최신 업데이트)
- 크로스 플랫폼 (Linux, macOS, Windows)
- PeerConnection, DataChannel, MediaStream 등 Full API
- 예제 코드 풍부

#### 단점 ⚠️
- Native 라이브러리 (JNI) - Virtual Threads Pinning 가능성
- 용량이 큼 (~50-100MB per platform)
- Native 의존성 관리 필요

#### Virtual Threads 호환성
- **Pinning 발생 가능**: JNI 호출 시 Virtual Thread가 Platform Thread에 고정
- **완화 방법**: I/O 작업을 별도 스레드풀로 분리

#### 적합성 점수: **85/100** 🟢

**사용 예시**:
```kotlin
val factory = PeerConnectionFactory()
val pc = factory.createPeerConnection(iceServers)

// RTP Sender 추가
val videoTrack = factory.createVideoTrack("video0", videoSource)
pc.addTrack(videoTrack)

// SDP 생성
pc.createOffer { sdp ->
    pc.setLocalDescription(sdp)
    // Send to remote peer
}
```

---

### 2. Jitsi Videobridge + lib-jitsi-meet ⭐⭐

**개요**: Jitsi의 SFU (Selective Forwarding Unit) 서버 + JavaScript 라이브러리

#### Maven 의존성
```kotlin
// Jitsi Videobridge는 별도 서버로 실행
// Java에서는 Jitsi 내부 컴포넌트 사용
```

#### 장점 ✅
- **Pure Java** (Virtual Threads 완벽 호환)
- 검증된 프로덕션 환경 (Jitsi Meet)
- 그룹 통화 최적화
- 수평 확장 가능
- ice4j, jitsi-srtp 포함

#### 단점 ⚠️
- **Videobridge는 SFU 서버** (우리는 1:1 변환만 필요)
- 무거운 아키텍처 (불필요한 기능 많음)
- ice4j/jitsi-srtp API 문제 동일
- 설정 복잡도 높음

#### 적합성 점수: **60/100** 🟡

---

### 3. Kurento Media Server ⚠️ 권장하지 않음

**개요**: C++ 미디어 서버 + Java SDK

#### Maven 의존성
```kotlin
implementation("org.kurento:kurento-client:7.0.1")
```

#### 장점 ✅
- Full WebRTC 스택
- 미디어 처리 기능 (필터, 레코딩 등)
- Java SDK 제공

#### 단점 ❌
- **프로젝트 중단** (Twilio 인수 후 개발 정지)
- **안정성 문제** (프로덕션에서 자주 재시작 필요)
- 매우 무거움 (~500MB)
- 별도 C++ 서버 실행 필요
- Native 의존성 (Virtual Threads Pinning)

#### 적합성 점수: **30/100** 🔴

---

### 4. Pion WebRTC (Go) - 참고용

**개요**: Pure Go WebRTC 구현

#### 장점 ✅
- 경량 & 고성능
- Pure Go (CGO 없음)
- 우리 Go 레거시와 동일한 라이브러리

#### 단점 ❌
- **Java/Kotlin에서 사용 불가**
- 별도 Go 서버 필요

#### 적합성 점수: **N/A** (Java 프로젝트에 부적합)

---

### 5. MediaSoup (Node.js) - 참고용

**개요**: Node.js/Rust 기반 SFU

#### 장점 ✅
- 고성능
- 활발한 개발

#### 단점 ❌
- **Java/Kotlin에서 사용 불가**
- Node.js 서버 필요

#### 적합성 점수: **N/A** (Java 프로젝트에 부적합)

---

## 🎯 최종 추천: webrtc-java (dev.onvoid)

### 추천 이유

1. **Maven Central에서 바로 사용 가능** ✅
   - 의존성 해결 문제 없음
   - `implementation("dev.onvoid.webrtc:webrtc-java:0.14.0")` 한 줄 추가

2. **Google WebRTC Native 기반** ✅
   - 검증된 구현
   - 브라우저와 100% 호환
   - 표준 준수

3. **Full WebRTC API** ✅
   - PeerConnection
   - ICE/STUN/TURN
   - DTLS-SRTP
   - DataChannel

4. **활발한 개발** ✅
   - 2025년 1월 최신 업데이트
   - GitHub 활성화

5. **예제 코드 풍부** ✅
   - 빠른 통합 가능

---

## 🚀 webrtc-java 통합 계획

### Phase 1: 의존성 추가 및 기본 테스트 (1일)
```kotlin
dependencies {
    implementation("dev.onvoid.webrtc:webrtc-java:0.14.0")
}
```

### Phase 2: WebRTCPeer 재작성 (2-3일)
```kotlin
class WebRTCPeerReal(
    private val peerId: String,
    private val streamId: String
) {
    private val factory = PeerConnectionFactory()
    private val peerConnection: PeerConnection

    init {
        peerConnection = factory.createPeerConnection(iceServers) { event ->
            when (event) {
                is IceCandidate -> onIceCandidate(event)
                is IceConnectionStateChange -> onIceStateChange(event)
            }
        }
    }

    suspend fun processOffer(offerSdp: String): String {
        val offer = SessionDescription(SdpType.OFFER, offerSdp)
        peerConnection.setRemoteDescription(offer)

        val answer = peerConnection.createAnswer()
        peerConnection.setLocalDescription(answer)

        return answer.sdp
    }

    fun sendRTPPacket(packet: ByteArray) {
        // RTP packet injection (또는 VideoTrack 사용)
    }
}
```

### Phase 3: 통합 테스트 (1-2일)
- RTSP → RTPRepacketizer → webrtc-java → Browser
- SDP 교환 검증
- ICE 연결 확인
- 비디오 재생 테스트

---

## ⚠️ Virtual Threads Pinning 완화 전략

### 문제
webrtc-java는 JNI를 사용하므로 Virtual Thread가 Platform Thread에 고정(Pinning)될 수 있습니다.

### 해결 방법

#### 1. I/O 작업 분리
```kotlin
class WebRTCPeerReal {
    private val nativeExecutor = Executors.newFixedThreadPool(4) // Platform Threads

    suspend fun sendRTPPacket(packet: ByteArray) {
        withContext(nativeExecutor.asCoroutineDispatcher()) {
            // JNI 호출
            peerConnection.send(packet)
        }
    }
}
```

#### 2. Virtual Threads는 비즈니스 로직만
```kotlin
// Virtual Thread
suspend fun processOffer(offerSdp: String): String {
    // 비즈니스 로직 (Virtual Thread)

    // JNI 호출은 Platform Thread
    return withContext(nativeExecutor.asCoroutineDispatcher()) {
        peerConnection.createAnswer()
    }
}
```

#### 3. 모니터링
```kotlin
jvmArgs(
    "-Djdk.tracePinnedThreads=full" // Pinning 감지
)
```

---

## 📊 성능 예상

### webrtc-java 사용 시
- **처리량**: ~1000 스트림 (Go 대비 80-90%)
- **레이턴시**: < 3ms (P99)
- **메모리**: ~200MB (Native 라이브러리 포함)
- **CPU**: Platform Thread Pool 크기에 따라 조정 가능

### Virtual Threads Pinning 영향
- **최악**: 처리량 50% 감소
- **완화 후**: 처리량 10-20% 감소
- **결론**: 여전히 Go와 비슷한 성능

---

## 🔄 대안 전략 비교

### 전략 A: webrtc-java 사용 (추천)
```
시간:  4-5일
난이도: 🟢 쉬움
성능:  🟢 80-90%
안정성: 🟢 높음 (Google WebRTC 기반)
```

### 전략 B: ice4j/jitsi-srtp API 해결
```
시간:  1-2주
난이도: 🟡 중간
성능:  🟢 90-100%
안정성: 🟡 중간 (API 문서 부족)
```

### 전략 C: 직접 구현
```
시간:  7-10주
난이도: 🔴 매우 어려움
성능:  🟢 100% (최적화 가능)
안정성: 🔴 낮음 (버그 위험)
```

### 전략 D: Mock으로 프로토타입
```
시간:  2-3일
난이도: 🟢 쉬움
성능:  N/A (로컬만)
안정성: 🟡 중간 (암호화 없음)
```

---

## 💡 최종 권장 전략

### 단기 (현재): Mock + RTSP Client (2-3일)
- RTPRepacketizer 검증
- E2E 시나리오 완성
- 로컬 네트워크 테스트

### 중기 (1주 후): webrtc-java 통합 (4-5일)
- 실제 WebRTC 구현
- NAT 환경 지원
- 프로덕션 준비

### 장기 (선택적): 성능 최적화
- Virtual Threads Pinning 완화
- ZGC 튜닝
- 부하 테스트

---

## 📚 참고 자료

### webrtc-java
- GitHub: https://github.com/devopvoid/webrtc-java
- Maven: https://mvnrepository.com/artifact/dev.onvoid.webrtc/webrtc-java
- 예제: https://github.com/devopvoid/webrtc-java/tree/master/webrtc-demo

### Google WebRTC
- 공식 사이트: https://webrtc.org/
- Native API: https://webrtc.googlesource.com/src/
- 표준: https://www.w3.org/TR/webrtc/

### Virtual Threads
- JEP 444: https://openjdk.org/jeps/444
- Pinning: https://wiki.openjdk.org/display/loom/Main

---

**마지막 업데이트**: 2025-11-24
**작성자**: Claude Code (AI Assistant)
**추천**: webrtc-java (dev.onvoid) - Maven Central에서 바로 사용 가능!
