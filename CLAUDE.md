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
**Media Server - Kotlin Migration**

### 목적 및 목표
기존 Go 기반 RTSP to WebRTC 미디어 서버를 **Kotlin + Spring Boot + Virtual Threads** 기반으로 마이그레이션합니다.

**핵심 목표**:
- ✅ Go와 동등한 성능 (OpenJDK 21 + ZGC)
- ✅ 향상된 개발 생산성 (Kotlin DSL, 타입 안전성)
- ✅ 풍부한 생태계 (Java/Kotlin 라이브러리)
- ✅ 프로덕션 레벨 안정성
- RTSP → WebRTC 실시간 변환 및 스트리밍
- H.265/H.264 코덱 자동 감지 및 선택
- 낮은 지연시간 (< 1초)
- 확장 가능한 아키텍처 (다중 스트림, 다중 클라이언트)

### 마이그레이션 전략
**3단계 점진적 최적화 전략** (MIGRATION_STRATEGY.md 참조):
1. **Phase 1**: Spring Boot + Tomcat (안정성 우선, 80% 성공 확률)
2. **Phase 2**: Selective Netty (병목 부분만 최적화, 15% 필요)
3. **Phase 3**: Full Ktor (최후의 수단, 5% 필요)

**현재 전략**: Phase 1 - Spring Boot + Tomcat + Virtual Threads

### 주요 이해관계자
- 개발팀: Go를 모르지만 Kotlin은 학습 가능한 팀
- 운영팀: 안정성과 유지보수성 중시
- 최종 사용자: 웹 브라우저에서 실시간 카메라 영상 시청

---

## 🏗️ 아키텍처 설계

### 시스템 구조 (Kotlin 버전)
```
[RTSP Camera (H.265/H.264)]
    ↓ TCP/RTSP
[RTSP Client (JavaCV + Virtual Threads)]
    ↓ RTP Packets
[StreamManager (Kotlin Flow)]
    ↓ collect/emit
[WebRTC Peer (Coroutines)]
    ├─ H.265 지원 → H.265 트랙
    └─ H.264만 지원 → H.264 트랙
    ↓ WebRTC/SRTP
[Web Browser] ✅ 실시간 영상 재생
```

### 레이어 아키텍처

```
┌─────────────────────────────────────────┐
│         Presentation Layer              │
│  - REST API (@RestController)           │
│  - WebSocket (Spring WebSocket)         │
│  - Static Files (ResourceHandler)       │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│         Application Layer               │
│  - StreamService                        │
│  - RTSPService (Virtual Threads)        │
│  - WebRTCService (Coroutines)           │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│         Domain Layer                    │
│  - StreamManager (Flow)                 │
│  - RTSPClient (Virtual Threads)         │
│  - WebRTCPeer (Coroutines)              │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│         Infrastructure Layer            │
│  - JavaCV 1.5.9 (FFmpeg 래퍼)           │
│  - Netty ByteBuf (Off-heap)             │
│  - WebRTC Library (TBD)                 │
└─────────────────────────────────────────┘
```

### 주요 컴포넌트

#### 1. Common Infrastructure (완료 ✅)
**위치**: `src/main/kotlin/com/pluxity/mediaserver/common/`

- **LoggingExtensions.kt**: 구조화 로깅 유틸리티
  - `errorWithContext()`, `infoWithContext()`, `measureTime()`
  - `logStreamEvent()`, `logPeerEvent()`, `logRTPPacket()`

- **Exceptions.kt**: 예외 계층 구조
  - `MediaServerException` (sealed class)
  - `StreamException`, `RTSPException`, `WebRTCException`
  - `CodecException`, `ConfigurationException`, `ResourceLimitException`
  - `TimeoutException`, `RTPPacketException`

- **MetricsCollector.kt**: Micrometer 기반 메트릭
  - 활성 스트림/피어 Gauge
  - RTP 패킷 송수신 Counter/DistributionSummary
  - 에러 카운팅, 작업 시간 측정

- **ByteBufExtensions.kt**: Netty ByteBuf 유틸리티
  - `withDirectBuffer()` - 자동 release
  - `writeRTPHeader()`, `readRTPHeader()`
  - `RTPHeader` data class
  - Off-heap 메모리 관리 헬퍼

#### 2. Configuration
**위치**: `src/main/kotlin/com/pluxity/mediaserver/config/`

- **MediaServerProperties.kt**: 설정 클래스
  - RTSP, WebRTC, HLS, Performance 설정
  - `@ConfigurationProperties` 바인딩

#### 3. Controllers (기본만 완료)
**위치**: `src/main/kotlin/com/pluxity/mediaserver/controller/`

- **HealthController.kt**: 헬스 체크
- **VirtualThreadTestController.kt**: Virtual Threads 검증용

#### 4. Domain (미구현)
**위치**: `src/main/kotlin/com/pluxity/mediaserver/domain/`

- **StreamManager** (예정): Kotlin Flow 기반 Pub/Sub
- **RTPPacket** (예정): RTP 패킷 데이터 클래스
- **Codec** (예정): 코덱 정보

#### 5. Service (미구현)
**위치**: `src/main/kotlin/com/pluxity/mediaserver/service/`

- **RTSPService** (예정): JavaCV + Virtual Threads
- **WebRTCService** (예정): Coroutines 기반
- **StreamService** (예정): 스트림 생명주기 관리

### 기술 스택

**언어/프레임워크**:
- **Kotlin 1.9.21**: 주 언어
- **Spring Boot 3.2.0**: 웹 프레임워크 (내장 Tomcat)
- **Java 21**: Virtual Threads, ZGC 지원
- **Kotlin Coroutines 1.8.0**: 비동기 처리

**미디어 처리**:
- **JavaCV 1.5.9**: FFmpeg 래퍼 (RTSP 클라이언트)
- **Netty 4.1.104**: ByteBuf (Off-heap 메모리)

**모니터링**:
- **Micrometer + Prometheus**: 메트릭 수집
- **Spring Actuator**: 헬스 체크
- **kotlin-logging 5.1.0**: 구조화 로깅

**빌드/런타임**:
- **Gradle 8.5** (Kotlin DSL)
- **OpenJDK 21** + ZGC (Generational)

### 디자인 패턴 및 원칙

1. **Kotlin Flow**: RTP 패킷 스트리밍 (Go 채널 → Kotlin Flow)
2. **Virtual Threads**: Blocking I/O 처리 (RTSP 연결)
3. **Coroutines**: 비동기 작업 (WebRTC 피어 관리)
4. **의존성 주입**: Spring @Component, @Service, @Autowired
5. **Sealed Classes**: 예외 계층, 상태 타입 안전성
6. **Extension Functions**: 코드 재사용 및 가독성
7. **Data Classes**: 불변 데이터 모델

**코딩 컨벤션**:
- Kotlin 공식 스타일 가이드 준수
- 클래스명: PascalCase, 함수명: camelCase, 상수: UPPER_SNAKE_CASE
- 구조화 로깅 (kotlin-logging)
- 예외 처리: sealed class MediaServerException
- 리소스 정리: use {} 블록 (AutoCloseable)

---

## 🎯 현재 진행 상황

### 완료된 작업

#### Phase 1 Week 1: 프로젝트 초기화 ✅ (2025-11-24)
- ✅ Go 파일 go-legacy/ 디렉토리로 이동
- ✅ Kotlin + Spring Boot 프로젝트 구조 생성
- ✅ build.gradle.kts 설정
  - Java 21, Kotlin 1.9.21, Spring Boot 3.2.0
  - JavaCV, Netty, Coroutines 의존성
  - ZGC JVM 옵션 (runWithZGC 태스크)
- ✅ application.yaml 기본 설정
  - **Virtual Threads 활성화**: `spring.threads.virtual.enabled: true`
  - Tomcat 설정, Actuator, 로깅
- ✅ MediaServerApplication.kt
  - ZGC 감지 및 로깅
  - JVM 정보 출력
- ✅ HealthController.kt (기본 헬스 체크)
- ✅ Gradle wrapper 생성

#### Phase 1 Week 2: 공통 인프라 구현 ✅ (2025-11-24)
- ✅ **LoggingExtensions.kt**: 구조화 로깅 유틸리티
  - `errorWithContext()`, `infoWithContext()`, `measureTime()`
  - 스트림/피어/RTP 전용 로깅 함수
- ✅ **Exceptions.kt**: 예외 계층 구조 (10개 예외 클래스)
- ✅ **MetricsCollector.kt**: Micrometer 메트릭 수집기
  - Gauge (활성 스트림/피어), Counter (패킷/에러)
  - DistributionSummary (바이트), Timer (작업 시간)
- ✅ **ByteBufExtensions.kt**: Netty ByteBuf 유틸리티
  - `withDirectBuffer()`, RTP 헤더 읽기/쓰기
  - Off-heap 메모리 안전 관리
- ✅ **단위 테스트 작성**: ByteBufExtensionsTest, ExceptionsTest
- ✅ **테스트 통과**: 모든 common 모듈 테스트 PASSED

#### Java 21 + Virtual Threads 검증 ✅ (2025-11-24)
- ✅ Java 21.0.8 설치 확인 (`C:\Program Files\Java\jdk-21`)
- ✅ Virtual Threads 활성화 설정
- ✅ **VirtualThreadTestController.kt** 작성 및 테스트
  - `GET /api/v1/test/thread-info`: Virtual Thread 확인
  - `GET /api/v1/test/blocking-test`: Blocking 작업 테스트
- ✅ **검증 결과**: `isVirtual: true`, `threadClass: VirtualThread`
- ✅ Spring Boot + 내장 Tomcat + Virtual Threads 정상 작동

#### Phase 2 Week 3-4: Stream Domain 구현 ✅ (2025-11-24)
- ✅ **RTPPacket.kt**: RTP 패킷 데이터 모델
  - Netty ByteBuf 기반 메모리 관리
  - `fromByteArray()`, `fromByteBuf()`, `create()` 팩토리 메서드
  - `copy()`, `release()` 메모리 안전성
  - **테스트**: 11개 테스트 모두 통과 ✅
- ✅ **StreamFlow.kt**: Kotlin Flow 기반 Pub/Sub
  - `MutableSharedFlow` (1:N 브로드캐스트)
  - `subscribe()`, `publish()` 메서드
  - BufferOverflow.DROP_OLDEST 전략
  - 통계 수집 (패킷 발행/전달, 비트레이트)
- ✅ **StreamManager.kt**: 스트림 생명주기 관리
  - `ConcurrentHashMap` 기반 thread-safe 관리
  - CRUD 작업 (생성, 조회, 삭제)
  - 스트림별 구독자 관리
- ✅ **Netty ByteBuf 분석**: Tomcat 환경에서 사용 문제 없음 확인
  - Off-heap 메모리 관리로 GC 압력 최소화
  - Virtual Threads와 호환

**테스트 상태**:
- RTPPacket: 11개 테스트 통과 ✅
- StreamFlow/StreamManager: 구현 완료, 단위 테스트는 통합 테스트로 이동 예정
  - 이슈: `runTest` TestDispatcher와 `Flow.collect` 무한 루프 간의 타이밍 문제
  - 해결: Phase 2 Week 5-6에서 실제 환경 통합 테스트로 검증

### 진행 중인 작업
- E2E 통합 테스트 준비

### 완료된 마일스톤 ✅

#### Phase 3-4: WebRTC 완전 구현 (2025-11-25)
- ✅ **Jitsi 라이브러리 직접 분석**: `javap`로 API 확인
- ✅ **ICEAgent (ice4j 3.2-9)**: Pure Java ICE 구현
- ✅ **SRTPTransformer (jitsi-srtp 1.1-21)**: Pure Java SRTP 암호화
- ✅ **WebRTCPeer 통합**: ICE + SRTP + RTPRepacketizer
- ✅ **Virtual Threads 완벽 호환**: JNI 없음!
- ✅ **BUILD SUCCESSFUL**

#### Phase 5: RTSP Client 구현 (완료 ✅)
- ✅ JavaCV + FFmpegFrameGrabber 사용
- ✅ Virtual Threads로 blocking I/O 처리
- ✅ 자동 재연결 로직
- ✅ H.264/H.265 코덱 자동 감지
- ✅ StreamManager 통합

### 다음 계획

#### Phase 6: E2E 테스트 및 검증 (다음 단계)
- [ ] 실제 RTSP 카메라 연결 테스트
- [ ] 브라우저 WebRTC 연결 테스트
- [ ] 성능 벤치마크 (처리량, 레이턴시)
- [ ] 메모리 프로파일링 (ByteBuf 누수 체크)

#### Phase 7: 프로덕션 강화 (예정)
- [ ] TURN 서버 지원
- [ ] 에러 복구 로직 강화
- [ ] 모니터링 및 알림

---

## 📝 핵심 기능 구현 상세

### 1. Virtual Threads 활성화 (Completed ✅)

**목적**: Spring Boot에서 모든 blocking 작업을 Virtual Threads로 처리

**구현 위치**: `src/main/resources/application.yaml:4-6`

**기술적 의사결정**:
- **결정**: `spring.threads.virtual.enabled: true` 설정 추가
- **이유**:
  - Java 21의 Virtual Threads (Project Loom) 활용
  - Blocking I/O (RTSP 연결) 시 OS 스레드 점유 최소화
  - Go의 goroutine과 유사한 경량 동시성
  - Tomcat의 모든 요청 처리가 Virtual Thread로 실행
- **대안**:
  1. Reactive Stack (WebFlux) - 복잡도 증가, 학습 곡선 높음
  2. 일반 Platform Threads - 컨텍스트 스위칭 비용 높음

**핵심 코드**:
```yaml
spring:
  threads:
    virtual:
      enabled: true  # Enable Virtual Threads for all blocking operations

server:
  tomcat:
    threads:
      max: 200  # Virtual threads are lightweight, can handle more
```

**검증 결과**:
```json
{
  "threadName": "tomcat-handler-0",
  "isVirtual": true,
  "threadClass": "VirtualThread",
  "message": "✅ Virtual Threads ENABLED"
}
```

**테스트**:
- `GET /api/v1/test/thread-info`: Virtual Thread 확인
- `GET /api/v1/test/blocking-test`: Thread.sleep(100) 테스트
- 결과: 모든 요청이 Virtual Thread에서 처리됨

---

### 2. Off-heap 메모리 관리 (ByteBuf Extensions)

**목적**: RTP 패킷 처리 시 GC 압력 최소화, Zero-Copy I/O

**구현 위치**: `src/main/kotlin/com/pluxity/mediaserver/common/ByteBufExtensions.kt`

**기술적 의사결정**:
- **결정**: Netty PooledByteBufAllocator를 사용한 Direct ByteBuf
- **이유**:
  - Off-heap 메모리 사용으로 GC 압력 제거
  - Pooling으로 할당/해제 비용 최소화
  - Zero-Copy network I/O (Socket → ByteBuf 직접 전송)
  - RTP 패킷(평균 1500바이트)을 매번 할당하면 GC 부하 큼
- **Go 코드와의 비교**:
  - Go: `[]byte` 슬라이스, 자동 GC
  - Kotlin: Netty ByteBuf, 수동 release 필요 → `use {}` 패턴으로 안전 보장

**핵심 코드**:
```kotlin
// 자동 release 패턴
withDirectBuffer(1500) { buffer ->
    buffer.writeRTPHeader(
        payloadType = 96,
        sequenceNumber = 12345,
        timestamp = 987654321L,
        ssrc = 0x12345678
    )
    buffer.writeBytes(payload)
    // use 블록 종료 시 자동 release
}

// RTP 헤더 파싱
val header = buffer.readRTPHeader()
println("Seq: ${header.sequenceNumber}, TS: ${header.timestamp}")
```

**메모리 안전성**:
- `use {}` 블록으로 자동 release
- Reference Counting으로 메모리 누수 방지
- PooledByteBufAllocator로 재사용

**변경 이력**:
- 2025-11-24: ByteBuf 확장 함수 구현, RTP 헤더 읽기/쓰기

---

### 3. 구조화 로깅 (Logging Extensions)

**목적**: 일관된 로그 형식, 컨텍스트 정보 포함

**구현 위치**: `src/main/kotlin/com/pluxity/mediaserver/common/LoggingExtensions.kt`

**기술적 의사결정**:
- **결정**: kotlin-logging + Extension Functions
- **이유**:
  - kotlin-logging은 lazy evaluation (람다)
  - Extension Functions로 도메인별 로깅 함수 제공
  - `measureTime()` inline 함수로 성능 측정
  - Go의 zap 로거와 유사한 구조화 로깅
- **패턴**:
  ```kotlin
  logger.logStreamEvent("stream123", "started", "codec: H265")
  logger.logRTPPacket("stream123", seq=100, ts=1234567, size=1400)
  logger.measureTime("RTSP connection") { connectToRTSP() }
  ```

**사용 예시**:
```kotlin
private val logger = KotlinLogging.logger {}

logger.infoWithContext("Stream connected",
    "streamId" to streamId,
    "codec" to "H265",
    "resolution" to "1920x1080"
)

logger.measureTime("RTP packet processing") {
    processRTPPacket(packet)
}
```

**변경 이력**:
- 2025-11-24: 로깅 확장 함수 구현

---

### 4. 메트릭 수집 (MetricsCollector)

**목적**: Prometheus 메트릭 수집, 성능 모니터링

**구현 위치**: `src/main/kotlin/com/pluxity/mediaserver/common/MetricsCollector.kt`

**기술적 의사결정**:
- **결정**: Micrometer + Prometheus
- **이유**:
  - Spring Boot Actuator와 통합
  - Prometheus + Grafana 표준 스택
  - Counter, Gauge, DistributionSummary, Timer 지원
- **메트릭 종류**:
  - `mediaserver.streams.active`: 활성 스트림 수
  - `mediaserver.peers.active`: 연결된 피어 수
  - `mediaserver.stream.packets.received`: RTP 패킷 수신
  - `mediaserver.peer.bytes.sent`: 피어 전송 바이트
  - `mediaserver.rtsp.errors`: RTSP 에러
  - `mediaserver.operation.duration`: 작업 실행 시간

**사용 예시**:
```kotlin
@Service
class StreamService(
    private val metrics: MetricsCollector
) {
    fun startStream(streamId: String) {
        metrics.streamStarted(streamId)
        // ...
    }

    fun onRTPPacket(streamId: String, packet: ByteBuf) {
        metrics.rtpPacketReceived(streamId, packet.readableBytes())
        // ...
    }
}
```

**Prometheus 엔드포인트**:
- `http://localhost:8080/actuator/prometheus`

**변경 이력**:
- 2025-11-24: MetricsCollector 구현

---

## 🐛 알려진 이슈 및 제약사항

### 현재 이슈

1. **StreamFlow/StreamManager 단위 테스트 타이밍 이슈**:
   - 문제: `runTest` TestDispatcher와 `Flow.collect` 무한 루프 간의 동기화 문제
   - 원인: `collect`는 무한 루프이므로 `advanceUntilIdle()`이 작동하지 않음
   - 현재 상태: RTPPacket 테스트는 통과, Stream 테스트는 보류
   - 해결 계획: Phase 2 Week 5-6에서 실제 환경 통합 테스트로 검증

### 기술적 부채

1. **WebRTC 라이브러리 미선택**:
   - 후보: Kurento, webrtc-java, 직접 구현
   - 해결 계획: Phase 4에서 평가 후 결정

2. **HLS 지원 미구현**:
   - 현재: WebRTC만 지원
   - 해결 계획: Phase 5에서 HLS Muxer 추가

3. **테스트 커버리지 낮음**:
   - 현재: Common 모듈만 테스트
   - 해결 계획: 각 Phase에서 통합 테스트 추가

4. **ZGC 설정**:
   - `bootRun`에는 ZGC 미적용
   - 해결: `./gradlew runWithZGC` 또는 JAR 직접 실행

### 제약사항

1. **Java 21 필수**:
   - Virtual Threads 사용
   - Generational ZGC 사용
   - 개발 환경: `C:\Program Files\Java\jdk-21`

2. **브라우저 H.265 지원**:
   - Chrome/Edge: H.265 지원
   - Firefox: H.264만 지원
   - 해결: 동적 코덱 선택 (Go 코드와 동일)

3. **네트워크 환경**:
   - STUN/TURN 서버 필요 (NAT 환경)
   - 현재: Google STUN 서버 사용

---

## 📚 참조 문서

### 내부 문서 (이 프로젝트)
- [README_KOTLIN.md](./README_KOTLIN.md) - Kotlin 프로젝트 소개 및 빠른 시작
- [docs/IMPLEMENTATION_PLAN.md](./docs/IMPLEMENTATION_PLAN.md) - 22주 상세 로드맵
- [docs/MIGRATION_STRATEGY.md](./docs/MIGRATION_STRATEGY.md) - 3단계 최적화 전략
- [docs/KOTLIN_MIGRATION_PLAN.md](./docs/KOTLIN_MIGRATION_PLAN.md) - Kotlin 마이그레이션 계획
- [docs/KOTLIN_PRODUCTION_GUIDE.md](./docs/KOTLIN_PRODUCTION_GUIDE.md) - ZGC, Panama, Off-heap 가이드
- [docs/KTOR_VS_SPRING_ANALYSIS.md](./docs/KTOR_VS_SPRING_ANALYSIS.md) - 프레임워크 선택 분석
- [docs/DEPENDENCIES.md](./docs/DEPENDENCIES.md) - Go 의존성 분석
- [CLAUDE.md](./CLAUDE.md) - 현재 문서 (프로젝트 SSOT)

### Go 레거시 참조
- [go-legacy/README.md](./go-legacy/README.md) - 기존 Go 프로젝트 문서
- Go 소스 코드는 `go-legacy/` 디렉토리에 참조용으로 보관

### 외부 리소스

**Kotlin/Spring**:
- [Kotlin 공식 문서](https://kotlinlang.org/docs/home.html)
- [Spring Boot 3.2 문서](https://docs.spring.io/spring-boot/docs/3.2.0/reference/html/)
- [Kotlin Coroutines 가이드](https://kotlinlang.org/docs/coroutines-guide.html)

**Java 21**:
- [Virtual Threads (JEP 444)](https://openjdk.org/jeps/444)
- [Generational ZGC (JEP 439)](https://openjdk.org/jeps/439)

**미디어 처리**:
- [JavaCV](https://github.com/bytedeco/javacv)
- [Netty](https://netty.io/)
- [FFmpeg](https://ffmpeg.org/)

**Go 참조 (레거시)**:
- [pion/webrtc](https://github.com/pion/webrtc)
- [bluenviron/gortsplib](https://github.com/bluenviron/gortsplib)
- [mediaMTX](https://github.com/bluenviron/mediamtx)

---

## 💬 Claude Code 사용 가이드

### 이 프로젝트에서 효과적인 프롬프팅

1. **기능 추가 시**:
   ```
   "StreamManager를 Kotlin Flow 기반으로 구현하고 싶어.
   IMPLEMENTATION_PLAN.md의 Phase 2 Week 3-4를 참고해서
   Go 코드(go-legacy/internal/core/stream_manager.go)를 마이그레이션해줘."
   ```

2. **문제 해결 시**:
   ```
   "ByteBuf가 release되지 않아서 메모리 누수가 발생해.
   로그: [로그 내용]. 원인과 해결 방법을 제시해줘."
   ```

3. **코드 리뷰 시**:
   ```
   "@src/main/kotlin/com/pluxity/mediaserver/common/ 디렉토리의
   코드를 리뷰하고 Kotlin best practices 관점에서 개선사항을 제안해줘."
   ```

### 작업 프로세스

1. **새 기능 개발**:
   - CLAUDE.md에서 현재 상태 확인
   - IMPLEMENTATION_PLAN.md에서 해당 Phase 확인
   - 기능 요구사항 설명
   - 설계 제안 받기
   - 구현 후 CLAUDE.md 업데이트

2. **버그 수정**:
   - 로그/에러 내용 제공
   - 관련 코드 파일 참조
   - 원인 분석 및 수정
   - "알려진 이슈" 섹션 업데이트

3. **테스트**:
   - `./gradlew test` (단위 테스트)
   - `./gradlew integrationTest` (통합 테스트)
   - 실패 시 로그 분석 및 수정

### 서브 에이전트 활용

- **Explore Agent**: 코드베이스 탐색, Go 레거시 코드 분석
- **Plan Agent**: 복잡한 기능 설계 시 사용

---

## 📊 성공 지표

### 프로젝트 성공 기준
- ✅ Kotlin + Spring Boot 프로젝트 구조 생성
- ✅ Java 21 + Virtual Threads 환경 구축
- ✅ 공통 인프라 모듈 완성 (Logging, Exceptions, Metrics, ByteBuf)
- ✅ StreamManager 구현 (Flow 기반 Pub/Sub) - RTPPacket, StreamFlow, StreamManager
- ✅ Netty ByteBuf 사용 검증 (Tomcat 환경 호환성 확인)
- 🔶 RTSP Client 구현 (JavaCV + Virtual Threads)
- 🔶 WebRTC Peer 구현 (Coroutines)
- 🔶 E2E 테스트 (실제 CCTV 카메라)
- 🔶 브라우저 호환성 (Chrome, Edge, Firefox)
- 🔶 지연시간 < 1초
- 🔶 Go 대비 성능: 처리량 ≥ 100%, P99 레이턴시 < 3ms (ZGC)

### 코드 품질 지표
- 테스트 커버리지: 현재 Common 모듈만 / 목표 60%+
- 알려진 버그: 0개 (치명적 버그)
- 기술 부채: 낮음 (주요 인프라 완성)
- 코드 스타일: Kotlin 공식 가이드 준수

---

## 🚀 배포 및 운영

### 빌드 프로세스
```bash
# 개발 빌드
./gradlew build

# 프로덕션 빌드 (테스트 포함)
./gradlew clean build

# 테스트 제외 빌드
./gradlew build -x test

# JAR 생성
./gradlew bootJar
# 결과: build/libs/media-server-0.1.0-SNAPSHOT.jar
```

### 실행
```bash
# 기본 실행
./gradlew bootRun

# ZGC 활성화 실행 (권장)
./gradlew runWithZGC

# JAR 직접 실행 (프로덕션)
java -XX:+UseZGC -XX:+ZGenerational \
     -Xms2g -Xmx4g \
     -XX:MaxDirectMemorySize=2g \
     -XX:+AlwaysPreTouch \
     -jar build/libs/media-server-0.1.0-SNAPSHOT.jar
```

### 모니터링
- **헬스 체크**: http://localhost:8080/api/v1/health
- **Actuator**: http://localhost:8080/actuator/health
- **Prometheus 메트릭**: http://localhost:8080/actuator/prometheus
- **Virtual Threads 확인**: http://localhost:8080/api/v1/test/thread-info

**로그 위치**:
- 콘솔 출력 (stdout)
- 파일 로그: `logs/media-server.log` (최대 500MB, 15일 보관)

**주요 메트릭**:
- `mediaserver.streams.active`: 활성 스트림 수
- `mediaserver.peers.active`: 연결된 피어 수
- `mediaserver.stream.packets.received`: RTP 패킷 수신률
- `mediaserver.peer.bytes.sent`: 피어 전송 바이트
- JVM 메트릭: Heap, GC, Thread

---

## 📌 중요 알림

### ⚠️ 개발 시 주의사항

1. **의존성 버전**:
   - Java 21 필수 (Virtual Threads)
   - Spring Boot 3.2.0 (Virtual Threads 지원)
   - Kotlin 1.9.21, Coroutines 1.8.0

2. **메모리 관리**:
   - ByteBuf는 반드시 release (use {} 패턴 사용)
   - Off-heap Direct Memory 누수 주의
   - `-XX:MaxDirectMemorySize=2g` 설정

3. **동시성 처리**:
   - Blocking I/O: Virtual Threads 사용
   - 비동기 작업: Coroutines 사용
   - Flow: 스트림 데이터 처리

4. **테스트**:
   - 단위 테스트: `@Test`, MockK
   - 통합 테스트: `@SpringBootTest`
   - Java 21 환경 필수

### 💡 Best Practices

1. **예외 처리**: sealed class MediaServerException 사용
2. **로깅**: kotlin-logging extension functions 사용
3. **설정**: application.yaml, @ConfigurationProperties
4. **리소스 정리**: use {} 블록 활용
5. **메트릭**: MetricsCollector 주입 후 사용
6. **타입 안전성**: data class, sealed class 활용

---

## 🔄 버전 히스토리

### v0.1.0-SNAPSHOT (2025-11-24) - Initial Migration
- ✅ **프로젝트 초기화**: Go → Kotlin 마이그레이션 시작
- ✅ **Phase 1 Week 1 완료**: 프로젝트 구조 생성
  - Spring Boot 3.2.0 + Kotlin 1.9.21
  - Java 21 + Virtual Threads 설정
  - Gradle 8.5 (Kotlin DSL)
- ✅ **Phase 1 Week 2 완료**: 공통 인프라
  - LoggingExtensions, Exceptions, MetricsCollector, ByteBufExtensions
  - 단위 테스트 작성 및 통과
- ✅ **Virtual Threads 검증**:
  - VirtualThreadTestController 작성
  - `isVirtual: true` 확인
  - Tomcat + Virtual Threads 정상 작동
- ✅ **Phase 2 Week 3-4 완료**: Stream Domain 구현
  - RTPPacket.kt (Netty ByteBuf 기반, 11개 테스트 통과)
  - StreamFlow.kt (Kotlin Flow Pub/Sub)
  - StreamManager.kt (ConcurrentHashMap 기반)
  - Netty ByteBuf 분석: Tomcat 환경 호환성 확인

**다음 버전 (v0.2.0) 계획**:
- Phase 2 Week 5-6: 통합 테스트 (StreamFlow/StreamManager 실제 환경 검증)
- Phase 3: RTSP Client 구현 (JavaCV + Virtual Threads)

---

## 📝 메모 및 임시 노트

### 개발 중 발견한 팁

1. **Virtual Threads 활성화**: `spring.threads.virtual.enabled: true`
2. **ByteBuf 메모리 누수 방지**: `use {}` 블록 필수
3. **Kotlin Map<String, Any> 타입 이슈**: 명시적 타입 파라미터 `mapOf<String, Any>()`
4. **Java 21 환경변수**: `export JAVA_HOME="C:\Program Files\Java\jdk-21"`
5. **Gradle Daemon 재시작**: Java 버전 변경 시 `./gradlew --stop`
6. **ZGC 적용**: `bootRun`이 아닌 `runWithZGC` 또는 JAR 직접 실행
7. **Netty 의존성**: `netty-all` 대신 개별 모듈 사용 (native 라이브러리 이슈 방지)

### 웹 페이지 접속 URL (미구현)

**프로덕션 사용** (예정):
- 대시보드: http://localhost:8080/static/dashboard.html
- 단일 뷰어: http://localhost:8080/static/viewer.html

**API 엔드포인트** (현재):
- GET /api/v1/health - 헬스 체크
- GET /actuator/health - Actuator 헬스
- GET /actuator/prometheus - 메트릭
- GET /api/v1/test/thread-info - Virtual Threads 확인
- GET /api/v1/test/blocking-test - Blocking 테스트

### Go 레거시 참조 경로

중요한 Go 파일들:
- `go-legacy/internal/core/stream_manager.go` - StreamManager 참조
- `go-legacy/internal/rtsp/client.go` - RTSP Client 참조
- `go-legacy/internal/webrtc/peer.go` - WebRTC Peer 참조
- `go-legacy/cmd/server/main.go` - 메인 로직 참조

### 다음 세션 시작 시

1. CLAUDE.md와 README_KOTLIN.md 먼저 읽기
2. IMPLEMENTATION_PLAN.md에서 현재 Phase 확인
3. `./gradlew clean build` 실행하여 빌드 상태 확인
4. `./gradlew test` 실행하여 테스트 통과 확인
5. Java 21 환경변수 설정 확인: `java -version`
6. Phase 2 Week 3-4 시작: StreamManager 구현

---

**마지막 업데이트**: 2025-11-24
**현재 버전**: v0.1.0-SNAPSHOT
**프로젝트 상태**: Phase 1 완료 (Week 1-2) ✅, Phase 2 준비 중
**다음 마일스톤**: Phase 2 Week 3-4 - StreamManager 구현 (Kotlin Flow)
