# WebRTC 라이브러리 설치 가이드

## 📌 개요

현재 프로젝트는 **RTP Repacketizer** (RTSP → WebRTC 변환)가 완전히 구현되어 있습니다.
하지만 실제 브라우저와 통신하기 위해서는 다음 라이브러리가 필요합니다:

- **ice4j**: ICE/STUN 프로토콜 (NAT 통과)
- **jitsi-srtp**: DTLS/SRTP 암호화

이 라이브러리들은 Maven Central에 없으므로 수동 설치가 필요합니다.

---

## 🔧 방법 1: Jitsi GitHub 저장소에서 직접 다운로드

### 1단계: 저장소 클론

```bash
cd /tmp
git clone https://github.com/jitsi/jitsi-maven-repository.git
cd jitsi-maven-repository/releases
```

### 2단계: JAR 파일 찾기

```bash
# ice4j 찾기
find . -name "ice4j*.jar" -type f

# jitsi-srtp 찾기
find . -name "jitsi-srtp*.jar" -type f
```

### 3단계: 프로젝트 libs 디렉토리로 복사

```bash
# MediaStream 프로젝트 디렉토리로 이동
cd /path/to/MediaStream
mkdir -p libs

# JAR 파일 복사
cp /tmp/jitsi-maven-repository/releases/org/ice4j/ice4j/VERSION/ice4j-VERSION.jar libs/
cp /tmp/jitsi-maven-repository/releases/org/jitsi/jitsi-srtp/VERSION/jitsi-srtp-VERSION.jar libs/
```

### 4단계: build.gradle.kts 수정

```kotlin
dependencies {
    // ... 기존 의존성 ...

    // Local JARs
    implementation(fileTree(mapOf("dir" to "libs", "include" to listOf("*.jar"))))
}
```

### 5단계: 빌드

```bash
./gradlew clean build
```

---

## 🔧 방법 2: 대안 라이브러리 사용

### Option A: WebRTC 전체 라이브러리
```kotlin
// Kurento (무겁지만 완전함)
implementation("org.kurento:kurento-client:7.0.0")

// 또는 webrtc-java
implementation("dev.onvoid.webrtc:webrtc-java:0.8.0")
```

### Option B: 직접 구현
- Bouncy Castle로 DTLS/SRTP 직접 구현
- Java NIO DatagramChannel로 ICE 직접 구현
- 복잡하지만 완전한 제어 가능

---

## 📊 현재 구현 상태

### ✅ 완전 구현 (75%)
1. **RTPRepacketizer** - RTSP → WebRTC RTP 변환 ✅
   - SSRC 변경
   - Sequence Number 재할당
   - Timestamp 조정
   - Payload Type 매핑

2. **DTLSHandler** - 인증서 관리 ✅
   - 자체 서명 인증서 생성
   - Fingerprint 계산 (SHA-256)
   - SDP에 포함될 정보 생성

3. **WebRTCPeer** - 피어 관리 ✅
   - StreamManager 구독
   - RTP 패킷 변환
   - 통계 추적

### ⏳ Mock 구현 (25%)
4. **ICE/STUN** - NAT 통과 (Mock)
   - SDP candidate 생성 (랜덤)
   - 실제 UDP 통신 없음

5. **SRTP** - 암호화 (Mock)
   - 키 생성 (랜덤)
   - 실제 암호화/복호화 없음

---

## 🚀 다음 단계

### 옵션 1: ice4j + jitsi-srtp 통합 (권장)
```
예상 시간: 2-3시간
난이도: 중간
성능: 최고
```

**장점**:
- Jitsi에서 검증된 라이브러리
- 경량 (~5MB)
- Virtual Threads와 호환

**단점**:
- 수동 설치 필요
- 문서가 부족

### 옵션 2: Kurento 사용
```
예상 시간: 1시간
난이도: 쉬움
성능: 중간
```

**장점**:
- Maven Central에서 바로 설치
- 완전한 WebRTC 스택

**단점**:
- 무거움 (~500MB)
- Native 라이브러리 (JNI Pinning)
- Virtual Threads 비효율

### 옵션 3: 직접 구현
```
예상 시간: 1-2주
난이도: 높음
성능: 최고
```

**장점**:
- 완전한 제어
- 최적화 가능
- 의존성 없음

**단점**:
- 개발 시간 많이 소요
- 버그 위험

---

## 💡 추천 전략

**현재 프로젝트 상태를 고려한 최선의 선택**:

1. **ice4j + jitsi-srtp 수동 설치** (방법 1)
   - RTPRepacketizer가 이미 완성되어 있음
   - 아키텍처가 Jitsi 라이브러리와 완벽히 호환
   - 경량 & 고성능

2. **설치 순서**:
   ```
   1. Jitsi 저장소 클론
   2. JAR 파일 복사 (libs/)
   3. build.gradle.kts 수정
   4. ICEAgent.kt, SRTPTransformer.kt 복구
   5. WebRTCPeerReal.kt 사용
   6. 빌드 및 테스트
   ```

3. **예상 결과**:
   - 빌드 성공 ✅
   - RTSP → WebRTC 완전 동작 ✅
   - 브라우저 비디오 재생 ✅

---

## 📚 참고 자료

- [Jitsi GitHub](https://github.com/jitsi)
- [ice4j Wiki](https://github.com/jitsi/ice4j/wiki)
- [jitsi-srtp](https://github.com/jitsi/jitsi-srtp)
- [WebRTC 표준](https://www.w3.org/TR/webrtc/)

---

**마지막 업데이트**: 2025-11-24
**작성자**: Claude Code (AI Assistant)
