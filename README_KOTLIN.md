# Media Server - Kotlin Migration

> **프로젝트 전환**: Go → Kotlin + Spring Boot + Virtual Threads
> **마이그레이션 시작일**: 2025-11-24

---

## 📌 프로젝트 개요

기존 Go 기반 RTSP to WebRTC 미디어 서버를 **Kotlin + Spring Boot + Virtual Threads** 기반으로 마이그레이션합니다.

### 주요 목표
- ✅ Go와 동등한 성능 (OpenJDK 21 + ZGC)
- ✅ 향상된 개발 생산성 (Kotlin DSL, 타입 안전성)
- ✅ 풍부한 생태계 (Java/Kotlin 라이브러리)
- ✅ 프로덕션 레벨 안정성

---

## 🏗️ 프로젝트 구조

```
MediaStream/
├── go-legacy/              # 기존 Go 코드 (참조용)
│   ├── cmd/
│   ├── internal/
│   ├── pkg/
│   └── ...
│
├── src/
│   ├── main/
│   │   ├── kotlin/
│   │   │   └── com/pluxity/mediaserver/
│   │   │       ├── MediaServerApplication.kt
│   │   │       ├── config/
│   │   │       ├── controller/
│   │   │       ├── service/
│   │   │       └── domain/
│   │   └── resources/
│   │       └── application.yaml
│   └── test/
│       └── kotlin/
│
├── docs/                   # 마이그레이션 문서
│   ├── DEPENDENCIES.md
│   ├── LANGUAGE_MIGRATION_ANALYSIS.md
│   ├── KOTLIN_MIGRATION_PLAN.md
│   └── KOTLIN_PRODUCTION_GUIDE.md
│
├── build.gradle.kts        # Gradle 빌드 설정
├── settings.gradle.kts
└── README_KOTLIN.md        # 이 파일
```

---

## 🚀 빠른 시작

### 1. 요구사항

- **Java**: OpenJDK 21 이상
- **Gradle**: 8.5+ (자동 다운로드됨)
- **IDE**: IntelliJ IDEA (권장)

### 2. 프로젝트 빌드

```bash
# Windows
.\gradlew build

# Linux/macOS
./gradlew build
```

### 3. 애플리케이션 실행

#### 일반 실행
```bash
.\gradlew bootRun
```

#### ZGC 활성화 실행 (권장)
```bash
.\gradlew runWithZGC
```

#### 수동 실행
```bash
java -XX:+UseZGC -XX:+ZGenerational -Xms2g -Xmx4g -jar build/libs/media-server-0.1.0-SNAPSHOT.jar
```

### 4. 헬스 체크

```bash
# Health endpoint
curl http://localhost:8080/api/v1/health

# Actuator health
curl http://localhost:8080/actuator/health

# Prometheus metrics
curl http://localhost:8080/actuator/prometheus
```

### 5. 웹 클라이언트 접속

브라우저에서 다음 URL로 접속:
```
http://localhost:8080/
```

**기능**:
- RTSP 스트림 시작/중지
- WebRTC 비디오 스트리밍
- 실시간 로그 확인

---

## 🎬 사용 방법

### REST API 사용

#### 스트림 목록 조회
```bash
curl http://localhost:8080/api/v1/streams
```

#### RTSP 스트림 시작
```bash
curl -X POST http://localhost:8080/api/v1/streams/plx_cctv_01/start \
  -H "Content-Type: application/json" \
  -d '{"url": "rtsp://admin:password@192.168.1.100:554/stream"}'
```

#### RTSP 스트림 중지
```bash
curl -X POST http://localhost:8080/api/v1/streams/plx_cctv_01/stop
```

#### 스트림 통계 조회
```bash
curl http://localhost:8080/api/v1/streams/plx_cctv_01/stats
```

### WebSocket 시그널링

WebSocket 엔드포인트: `ws://localhost:8080/ws/signaling`

**프로토콜**:
```javascript
// 연결
const ws = new WebSocket('ws://localhost:8080/ws/signaling');

// SDP Offer 전송
ws.send(JSON.stringify({
  type: 'offer',
  streamId: 'plx_cctv_01',
  sdp: '<SDP offer>'
}));

// SDP Answer 수신
ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === 'answer') {
    // SDP answer 처리
  }
};
```

### 웹 클라이언트 사용

1. 브라우저에서 `http://localhost:8080/` 접속
2. **스트림 ID**와 **RTSP URL** 입력
3. **"RTSP 스트림 시작"** 클릭
4. 스트림 목록에서 시작된 스트림 선택
5. **"WebRTC 연결 시작"** 클릭
6. 비디오 스트림 재생 확인

---

## 📚 기술 스택

### 핵심 프레임워크
- **Spring Boot 3.2.0**: 웹 프레임워크
- **Kotlin 1.9.21**: 주 언어
- **Kotlin Coroutines**: 비동기 처리

### 미디어 처리
- **JavaCV 1.5.9**: FFmpeg 래퍼 (RTSP 클라이언트)
- **Netty 4.1.104**: 고성능 네트워킹 (ByteBuf, Zero-Copy)

### 런타임 최적화
- **OpenJDK 21**: JVM 런타임
- **ZGC (Generational)**: 초저지연 가비지 컬렉터

### 모니터링
- **Micrometer + Prometheus**: 메트릭 수집
- **Spring Actuator**: 헬스 체크

---

## ⚙️ 설정

### application.yaml

```yaml
server:
  port: 8080

media:
  rtsp:
    pool:
      max-streams: 100
    transport: tcp

  webrtc:
    settings:
      max-peers: 1000

streams:
  plx_cctv_01:
    source: "rtsp://..."
    source-on-demand: false
```

전체 설정은 `src/main/resources/application.yaml` 참조

---

## 🎯 마이그레이션 로드맵

### Phase 1: 초기 설정 ✅ (완료)
- [x] Go 파일 go-legacy로 이동
- [x] Kotlin + Spring Boot 프로젝트 구조 생성
- [x] build.gradle.kts 설정
- [x] application.yaml 기본 설정
- [x] Virtual Threads 활성화 및 검증

### Phase 2: 핵심 모듈 ✅ (완료)
- [x] StreamManager (Flow 기반 Pub/Sub)
- [x] RTPPacket (Netty ByteBuf 기반)
- [x] StreamFlow (Kotlin SharedFlow)
- [x] 공통 인프라 (Logging, Exceptions, Metrics, ByteBuf Extensions)

### Phase 3: RTSP 클라이언트 ✅ (완료)
- [x] RTSPClient (JavaCV + Virtual Threads)
- [x] RTSPManager (클라이언트 생명주기 관리)
- [x] 자동 재연결 로직
- [x] Frame to RTP 패킷 변환

### Phase 4: WebRTC 및 API ✅ (완료)
- [x] REST API (Stream 관리)
- [x] WebSocket Signaling (SDP/ICE 교환)
- [x] WebRTCPeer (기본 구조)
- [x] WebRTCManager (피어 관리)
- [x] 웹 클라이언트 (HTML/JavaScript)

### Phase 5: 테스트 및 최적화 (진행 중)
- [x] 통합 테스트 작성
- [ ] 실제 RTSP 스트림 테스트
- [ ] WebRTC 라이브러리 통합 (Kurento, webrtc-java 등)
- [ ] 성능 테스트 및 벤치마크
- [ ] ZGC 튜닝

### Phase 6: HLS 지원 (예정)
- [ ] HLS Muxer
- [ ] Playlist 생성
- [ ] Segment 관리

---

## 📖 참조 문서

### 마이그레이션 가이드
- [의존성 분석](docs/DEPENDENCIES.md) - Go 프로젝트 라이브러리 분석
- [언어 비교](docs/LANGUAGE_MIGRATION_ANALYSIS.md) - 5개 언어 마이그레이션 비교
- [Kotlin 마이그레이션 계획](docs/KOTLIN_MIGRATION_PLAN.md) - 22주 로드맵
- [프로덕션 가이드](docs/KOTLIN_PRODUCTION_GUIDE.md) - ZGC, Panama, Off-heap 전략

### Go 레거시
- [Go README](go-legacy/README.md) - 기존 Go 프로젝트 문서
- [CLAUDE.md](CLAUDE.md) - Go 프로젝트 개발 히스토리

---

## 🔧 개발 가이드

### IDE 설정 (IntelliJ IDEA)

1. **Project Import**
   - File → Open → build.gradle.kts 선택
   - "Open as Project" 클릭

2. **Kotlin 플러그인** (자동 설치됨)

3. **JVM 설정**
   - Run → Edit Configurations
   - VM options: `-XX:+UseZGC -XX:+ZGenerational -Xms2g -Xmx4g`

### 코드 스타일

**Kotlin 공식 스타일 가이드** 준수:
```kotlin
// 클래스명: PascalCase
class StreamManager

// 함수명: camelCase
fun createStream(id: String)

// 상수: UPPER_SNAKE_CASE
const val MAX_RETRY_COUNT = 5

// 프로퍼티: camelCase
val streamId: String
```

### 로깅

```kotlin
import io.github.oshai.kotlinlogging.KotlinLogging

private val logger = KotlinLogging.logger {}

fun example() {
    logger.info { "Stream started: $streamId" }
    logger.error(e) { "Failed to connect" }
}
```

---

## 🧪 테스트

### 단위 테스트 실행
```bash
.\gradlew test
```

### 통합 테스트 실행
```bash
.\gradlew integrationTest
```

### 테스트 커버리지
```bash
.\gradlew jacocoTestReport
# 리포트: build/reports/jacoco/test/html/index.html
```

---

## 📦 빌드 및 배포

### JAR 빌드
```bash
.\gradlew bootJar
# 결과: build/libs/media-server-0.1.0-SNAPSHOT.jar
```

### Docker 이미지 빌드
```bash
docker build -t media-server:latest .
```

### Docker 실행
```bash
docker run -p 8080:8080 \
  -e JAVA_OPTS="-XX:+UseZGC -XX:+ZGenerational -Xms2g -Xmx4g" \
  media-server:latest
```

---

## 🐛 트러블슈팅

### 문제: ZGC가 활성화되지 않음

**증상**:
```
⚠️ ZGC not enabled!
Current GC: G1 Young Generation, G1 Old Generation
```

**해결**:
```bash
# JVM 옵션에 추가
-XX:+UseZGC -XX:+ZGenerational
```

### 문제: OutOfMemoryError: Direct buffer memory

**원인**: Netty ByteBuf release 누락

**해결**:
```kotlin
allocator.directBuffer(1500).use { buffer ->
    // 작업
}  // 자동 release
```

### 문제: Port 8080 already in use

**해결**:
```bash
# 포트 변경
.\gradlew bootRun --args='--server.port=8081'
```

---

## 📊 성능 비교 (예상)

| 지표 | Go (레거시) | Kotlin (목표) |
|------|------------|--------------|
| 시작 시간 | 0.1초 | 2초 |
| 처리량 | 10K pkt/s | 12K pkt/s |
| P99 레이턴시 | 5ms | < 3ms (ZGC) |
| 메모리 (idle) | 50MB | 100MB |

---

## 🤝 기여 가이드

### 브랜치 전략
- `main`: 프로덕션 준비 코드
- `develop`: 개발 브랜치
- `feature/*`: 기능 개발
- `go-legacy`: Go 코드 (읽기 전용)

### 커밋 메시지
```
feat: 새로운 기능 추가
fix: 버그 수정
refactor: 리팩토링
docs: 문서 수정
test: 테스트 추가
perf: 성능 개선
```

---

## 📞 문의

**프로젝트 관리자**: Lay (kmr1993@pluxity.com)

**이슈 트래킹**: GitHub Issues

---

**Last Updated**: 2025-11-24
**Version**: 0.1.0-SNAPSHOT
**Status**: 🚧 마이그레이션 진행 중 (Phase 1 완료)
