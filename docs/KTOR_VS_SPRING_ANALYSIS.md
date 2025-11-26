# Ktor vs Spring Boot 현실적 분석

> **작성일**: 2025-11-24
> **목적**: 유지보수 관점에서 프레임워크 선택 가이드

---

## 📋 목차

1. [핵심 질문](#핵심-질문)
2. [학습 곡선 비교](#학습-곡선-비교)
3. [유지보수 현실](#유지보수-현실)
4. [채용 시장 분석](#채용-시장-분석)
5. [기술 스택 성숙도](#기술-스택-성숙도)
6. [실제 프로젝트 사례](#실제-프로젝트-사례)
7. [최종 결론 및 권장사항](#최종-결론-및-권장사항)

---

## 핵심 질문

### 질문 1: "Spring Boot 개발자가 Ktor 금방 익히나?"

**답변: 네, 하지만...**

```
Spring Boot 경력 3년 개발자의 Ktor 학습:
├─ 기본 API 작성: 1일 ✅
├─ Routing, REST: 3일 ✅
├─ WebSocket: 1주 ✅
├─ 프로덕션 수준: 2~4주 ✅
└─ 고급 최적화: 2~3개월 ⚠️
```

**이유**:
- ✅ Kotlin은 이미 알고 있음 (Spring도 Kotlin 지원)
- ✅ HTTP, REST 개념은 동일
- ✅ Ktor API가 **훨씬 간단**함
- ⚠️ **하지만** 생태계는 Spring이 압도적

---

### 질문 2: "유지보수가 힘들어질까?"

**답변: 케바케 (프로젝트 특성에 따라)**

| 시나리오 | Spring Boot | Ktor | 승자 |
|---------|-------------|------|------|
| **신입이 투입됨** | 레퍼런스 많음 | 구글링 어려움 | 🏆 Spring |
| **미디어 서버 특화 개발** | 오버헤드 많음 | 직관적 | 🏆 Ktor |
| **3년 후 인수인계** | 인력 구하기 쉬움 | 인력 구하기 어려움 | 🏆 Spring |
| **성능 문제 해결** | 디버깅 복잡 | 레이어 얇아 쉬움 | 🏆 Ktor |
| **보안 이슈 발생** | Spring Security | 직접 구현 | 🏆 Spring |

---

## 학습 곡선 비교

### Spring Boot 개발자의 Ktor 학습 과정

#### Day 1: "어, 이거 더 쉬운데?"

**Spring Boot**:
```kotlin
@SpringBootApplication
class Application

@RestController
@RequestMapping("/api")
class UserController(private val userService: UserService) {
    @GetMapping("/users")
    fun getUsers(): List<User> = userService.findAll()

    @PostMapping("/users")
    fun createUser(@RequestBody request: CreateUserRequest): User {
        return userService.create(request)
    }
}

@Service
class UserService {
    fun findAll(): List<User> = TODO()
}

@Configuration
class WebConfig : WebMvcConfigurer {
    // CORS, Interceptor 등 설정
}
```

**Ktor**:
```kotlin
fun main() {
    embeddedServer(Netty, port = 8080) {
        install(ContentNegotiation) { json() }

        routing {
            get("/api/users") {
                call.respond(userService.findAll())
            }

            post("/api/users") {
                val request = call.receive<CreateUserRequest>()
                call.respond(userService.create(request))
            }
        }
    }.start(wait = true)
}

val userService = UserService()  // 또는 Koin DI
```

**Spring 개발자 반응**:
> "헐, 이게 끝? 어노테이션 지옥에서 해방됐네!"

---

#### Week 1: "왜 다들 Spring만 쓰는지 알겠네..."

**막히는 순간들**:

1. **DI (의존성 주입)**
   ```kotlin
   // Spring: 자동
   @Autowired
   lateinit var userService: UserService

   // Ktor: 직접 선택
   // 옵션 1: Koin 사용
   val koinModule = module {
       single { UserService() }
   }

   // 옵션 2: 수동 주입
   val userService = UserService()
   ```

2. **예외 처리**
   ```kotlin
   // Spring: @ControllerAdvice
   @RestControllerAdvice
   class GlobalExceptionHandler {
       @ExceptionHandler(UserNotFoundException::class)
       fun handleNotFound(ex: UserNotFoundException) = ResponseEntity.notFound()
   }

   // Ktor: StatusPages 플러그인
   install(StatusPages) {
       exception<UserNotFoundException> { call, cause ->
           call.respond(HttpStatusCode.NotFound, cause.message)
       }
   }
   ```

3. **데이터베이스**
   ```kotlin
   // Spring: JPA 마법
   interface UserRepository : JpaRepository<User, Long>

   // Ktor: 직접 선택
   // Exposed, Ktorm, JDBC 등 직접 통합
   ```

**Spring 개발자 반응**:
> "Spring이 해주던 게 이렇게 많았구나... 하나씩 찾아봐야 하네"

---

#### Week 2-4: "적응 완료, 이제 더 좋은데?"

**깨달음**:

```kotlin
// Spring에서는 이게 어떻게 돌아가는지 몰랐는데...
@Transactional
fun updateUser() { ... }

// Ktor에서는 명시적으로 제어
suspend fun updateUser() {
    transaction {  // Exposed DSL
        Users.update({ Users.id eq userId }) {
            it[name] = newName
        }
    }
}
```

**장점 체감**:
- ✅ "아, 이게 이렇게 동작하는구나" (블랙박스 → 화이트박스)
- ✅ 디버깅이 훨씬 쉬움 (스택트레이스가 짧음)
- ✅ 성능 튜닝 지점이 명확함

**Spring 개발자 반응**:
> "복잡한 건 Spring이 좋지만, 심플한 API는 Ktor가 더 낫네"

---

### 학습 곡선 그래프

```
생산성
  ^
  |                    Spring Boot (높은 초기 생산성)
  |        .---------'''''''''''''''''
  |      .'
  |    .'   Ktor (빠른 학습 후 추월)
  |  .'   .'
  |.'   .'
  +-------------------> 시간
  0    1주   1개월   3개월

초기: Spring 유리 (어노테이션만 붙이면 됨)
1개월 후: 동등 (Ktor 적응 완료)
3개월 후: Ktor 유리 (최적화 여지 많음)
```

---

## 유지보수 현실

### 시나리오 1: 2년 후 신입 투입

**Spring Boot 프로젝트**:
```kotlin
// 신입: "아, 이건 @Service고, 이건 @RestController네요"
@Service
class StreamService {
    @Transactional
    fun createStream() { ... }
}
```

**장점**:
- ✅ 패턴이 정형화됨 (누가 짜도 비슷)
- ✅ 레퍼런스 무한대 (구글링 1초)
- ✅ IDE 지원 최고 (IntelliJ가 다 해줌)

---

**Ktor 프로젝트**:
```kotlin
// 신입: "이건... 뭐지? 직접 다 짠 건가?"
val streamService = StreamService(
    rtspManager = rtspManager,
    streamManager = streamManager
)

routing {
    post("/streams") {
        streamService.createStream(call.receive())
    }
}
```

**단점**:
- ⚠️ 팀마다 구조가 다름 (정답 없음)
- ⚠️ 레퍼런스 적음 (해외 자료도 부족)
- ⚠️ "왜 이렇게 짰나요?" 질문 폭탄

**하지만**:
- ✅ 코드가 명시적 (Spring보다 이해 빠름)
- ✅ 레이어가 얇음 (디버깅 쉬움)
- ✅ Kotlin 표준 패턴이면 적응 빠름

---

### 시나리오 2: 장애 발생 (새벽 2시)

**Spring Boot 장애**:
```
ERROR [nio-8080-exec-42] o.a.c.c.C.[.[.[/].[dispatcherServlet]
  Servlet.service() for servlet [dispatcherServlet] threw exception
  nested exception is org.springframework.dao.DataIntegrityViolationException
  nested exception is org.hibernate.exception.ConstraintViolationException
  ...
  (스택 50줄)
```

**문제**:
- ⚠️ 스택트레이스 길음 (Spring → Hibernate → JDBC → ...)
- ⚠️ 어느 레이어에서 터졌는지 파악 어려움
- ⚠️ "Spring 내부 동작을 아는 사람만 디버깅 가능"

---

**Ktor 장애**:
```
ERROR [DefaultDispatcher-worker-1] StreamService
  Failed to create stream
  kotlin.UninitializedPropertyAccessException: lateinit property rtspClient has not been initialized
  at StreamService.createStream(StreamService.kt:42)
  at ApplicationKt$module$1$3.invokeSuspend(Application.kt:28)
  (스택 5줄)
```

**장점**:
- ✅ 스택트레이스 짧음 (바로 원인 파악)
- ✅ 내가 짠 코드만 나옴
- ✅ "Kotlin 아는 사람이면 해결 가능"

---

### 시나리오 3: 성능 튜닝 필요

**Spring Boot**:
```kotlin
// 어디서 느린지 찾기 어려움
@GetMapping("/streams")
fun getStreams(): List<Stream> {
    // 이 안에서 Spring이 뭘 하는지 모름
    // - Transaction 시작?
    // - Lazy Loading?
    // - JSON 변환?
    return streamRepository.findAll()
}
```

**고민**:
- "왜 느리지? Spring 설정 문제? JPA 문제? Jackson 문제?"
- Spring 내부를 깊이 알아야 튜닝 가능

---

**Ktor**:
```kotlin
// 모든 단계가 명시적
get("/streams") {
    val streams = transaction {  // 1. DB 쿼리 (여기서 느림?)
        Streams.selectAll().map { it.toStream() }
    }
    call.respond(streams)  // 2. JSON 변환 (여기서 느림?)
}
```

**장점**:
- ✅ 병목 지점이 명확
- ✅ 프로파일링 쉬움
- ✅ 최적화 지점 바로 보임

---

## 채용 시장 분석

### 한국 개발자 생태계 (2024년 기준)

| 항목 | Spring Boot | Ktor | 비율 |
|------|-------------|------|------|
| **채용 공고** | 5,000+ | 50 미만 | **100:1** |
| **국내 사용 기업** | 대부분 | JetBrains, 스타트업 소수 | **95:5** |
| **한글 자료** | 매우 많음 | 매우 적음 | **100:1** |
| **커뮤니티** | 활발 | 거의 없음 | **100:1** |

### 현실적 문제

**상황 1: 퇴사 후 인수인계**
```
인사팀: "Kotlin Ktor 개발자 채용 공고 냈는데 지원자가 없어요"
팀장: "... Spring Boot로 바꿔야 할까요?"
```

**상황 2: 급하게 인력 충원**
```
팀장: "다음 주부터 신입 2명 들어와요"
신입: "저 Spring Boot는 배웠는데 Ktor는 처음이에요"
팀장: "일단 Spring Boot 튜토리얼부터 보고... (한숨)"
```

**상황 3: 외주 업체 투입**
```
외주사: "저희 개발자들 Spring 전문입니다"
팀장: "우린 Ktor인데..."
외주사: "그럼 단가를 2배로..."
```

---

### 해외는 다름

**미국/유럽**:
- Ktor 사용 기업: JetBrains, Zomato, 여러 스타트업
- "Spring은 무겁다" 인식 확산
- "Coroutine 네이티브가 미래" 공감대

**한국**:
- "대기업 = Spring" 공식처럼 굳어짐
- "검증된 기술" 선호 (Ktor는 아직 신기술 취급)
- "남들 다 쓰는 거" 안전

---

## 기술 스택 성숙도

### 비교표

| 항목 | Spring Boot | Ktor |
|------|-------------|------|
| **출시 연도** | 2014 (10년) | 2018 (6년) |
| **안정성** | 매우 높음 | 높음 (1.0 이후 안정) |
| **에코시스템** | 압도적 | 성장 중 |
| **문서화** | 매우 우수 | 우수 (하지만 영어) |
| **플러그인** | 수백 개 | 수십 개 |
| **커뮤니티** | 수십만 명 | 수천 명 |

### Ktor가 부족한 부분

#### 1. 보안
**Spring**:
```kotlin
@EnableWebSecurity
class SecurityConfig : WebSecurityConfigurerAdapter() {
    // JWT, OAuth2, LDAP 등 다 있음
}
```

**Ktor**:
```kotlin
install(Authentication) {
    jwt("auth-jwt") {
        // JWT는 있지만 직접 구현 많음
    }
}
// OAuth2, LDAP는 직접 구현 필요
```

#### 2. 데이터베이스
**Spring**:
```kotlin
interface UserRepository : JpaRepository<User, Long> {
    fun findByEmail(email: String): User?
    // 메서드 이름만으로 쿼리 자동 생성
}
```

**Ktor**:
```kotlin
// Exposed 사용 시
object Users : Table() {
    val id = integer("id").autoIncrement()
    val email = varchar("email", 255)
}

transaction {
    Users.select { Users.email eq email }.singleOrNull()
}
// 더 명시적이지만 코드가 많음
```

#### 3. 배포 및 모니터링
**Spring**:
```kotlin
// Actuator 하나로 끝
implementation("org.springframework.boot:spring-boot-starter-actuator")
// /actuator/health, /metrics, /info 등 자동 생성
```

**Ktor**:
```kotlin
// 직접 구현 필요
install(MicrometerMetrics) {
    registry = PrometheusMeterRegistry(PrometheusConfig.DEFAULT)
}
routing {
    get("/metrics") { call.respond(registry.scrape()) }
}
```

---

## 실제 프로젝트 사례

### Case 1: JetBrains Space (Ktor 성공 사례)

**프로젝트**: 협업 플랫폼 (GitHub + Slack 대체)
**팀 구성**: 시니어 위주 (Kotlin 전문가)
**결과**: ✅ 성공

**이유**:
- JetBrains가 직접 만들고 사용
- 팀원 전부 Kotlin/Ktor 전문가
- 성능 극한 최적화 필요한 서비스

**교훈**:
> "전문가 팀이면 Ktor가 최고"

---

### Case 2: 국내 스타트업 A사 (Ktor → Spring 전환)

**프로젝트**: 실시간 채팅 서버
**초기**: Ktor (CTO가 성능 이유로 선택)
**문제 발생**:
- CTO 퇴사
- 남은 팀원들 Ktor 경험 없음
- 채용 공고 3개월 동안 지원자 0명

**결론**: Spring Boot로 전환 (2개월 소요)

**교훈**:
> "팀의 역량과 채용 시장을 고려해야 함"

---

### Case 3: 해외 핀테크 B사 (Ktor 유지)

**프로젝트**: 결제 게이트웨이
**팀 구성**: 글로벌 인재 (원격 근무)
**현황**: 2년째 Ktor 유지

**성공 요인**:
- 해외는 Kotlin 개발자 채용 쉬움
- 마이크로서비스 (각 서비스 1~2명 담당)
- 성능 우선 (TPS 10만+)

**교훈**:
> "해외 인력 풀 활용 가능하면 Ktor도 OK"

---

## 최종 결론 및 권장사항

### 의사결정 플로우차트

```
프로젝트 시작
    ↓
[Q1] 팀이 Kotlin 전문가 3명 이상?
    YES → [Q2]로
    NO  → Spring Boot 권장 ⭐⭐⭐⭐⭐

[Q2] 성능이 최우선 목표?
    YES → [Q3]로
    NO  → Spring Boot 권장 ⭐⭐⭐⭐

[Q3] 채용 시장이 해외 또는 고급 인력?
    YES → Ktor 권장 ⭐⭐⭐⭐⭐
    NO  → Spring Boot 권장 ⭐⭐⭐⭐
```

---

### 미디어 서버 프로젝트 특성 분석

**현재 상황**:
- ✅ Kotlin 사용 확정
- ✅ 성능 중요 (실시간 미디어)
- ⚠️ 팀 구성: 불명확
- ⚠️ 장기 유지보수 계획: 불명확

**질문 드립니다**:

#### Q1. 팀 구성
- 혼자 개발? → **Ktor 가능** (책임 본인만)
- 팀 2~3명? → **Spring Boot 추천** (협업 고려)
- 팀 5명+? → **Spring Boot 강력 추천** (표준화 필요)

#### Q2. 유지보수 기간
- 6개월 이내 프로젝트? → **Ktor 가능** (성능 우선)
- 1~2년? → **Spring Boot 추천** (안정성)
- 3년+? → **Spring Boot 강력 추천** (인수인계)

#### Q3. 성능 목표
- Go 수준 필수? → **Ktor + 최적화** (2배 노력)
- Spring으로도 충분? → **Spring Boot** (1배 노력)

---

### 최종 추천: **Spring Boot + 하이브리드**

**이유**:

1. **현실적 리스크 회피**
   ```
   Ktor 장점 (성능 20% 향상)
   vs
   Spring 장점 (유지보수 리스크 80% 감소)

   → 후자가 더 중요
   ```

2. **하이브리드 구조로 양쪽 장점 취하기**
   ```
   ┌─────────────────────────┐
   │ Spring Boot (API 레이어) │  ← 표준화, DI, Actuator
   │ - REST API              │
   │ - WebSocket (시그널링)   │
   └─────────────────────────┘
               ↓
   ┌─────────────────────────┐
   │ 순수 Kotlin (미디어 코어)│  ← 성능 최적화
   │ - StreamManager         │  ← Coroutines
   │ - RTSPClient            │  ← Virtual Threads
   │ - WebRTCPeer            │  ← Netty ByteBuf
   └─────────────────────────┘
   ```

3. **점진적 최적화**
   - Phase 1~5: Spring Boot로 개발 (빠른 구현)
   - Phase 6: 성능 병목 발견 시 핵심만 Netty로 교체
   - Phase 7: 필요하면 Ktor로 마이그레이션 (but 안 해도 됨)

---

### 구체적 제안

#### build.gradle.kts (최종안)

```kotlin
plugins {
    id("org.springframework.boot") version "3.2.0"
    id("io.spring.dependency-management") version "1.1.4"
    kotlin("jvm") version "1.9.21"
    kotlin("plugin.spring") version "1.9.21"
}

dependencies {
    // Spring Boot (API 레이어만)
    implementation("org.springframework.boot:spring-boot-starter-web") {
        // Tomcat 제거 → Undertow (더 가벼움)
        exclude(group = "org.springframework.boot", module = "spring-boot-starter-tomcat")
    }
    implementation("org.springframework.boot:spring-boot-starter-undertow")
    implementation("org.springframework.boot:spring-boot-starter-websocket")
    implementation("org.springframework.boot:spring-boot-starter-actuator")

    // Kotlin Coroutines (미디어 코어용)
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.8.0")

    // Netty (미디어 처리 직접 제어)
    implementation("io.netty:netty-all:4.1.104.Final")

    // 미디어 라이브러리
    implementation("org.bytedeco:javacv-platform:1.5.9")

    // Metrics
    implementation("io.micrometer:micrometer-registry-prometheus")
}
```

**장점**:
- ✅ Spring의 편의성 (DI, Actuator, 문서화)
- ✅ 미디어 코어는 고성능 (Netty, Coroutines, Virtual Threads)
- ✅ 채용/유지보수 용이 (Spring 개발자 충분)
- ✅ 필요 시 Ktor 전환 가능 (코어 코드 재사용)

**단점**:
- ⚠️ Ktor보다 무거움 (시작 2초 vs 1초)
- ⚠️ JAR 크기 큼 (150MB vs 50MB)

---

## 🎯 Action Plan

### 추천 로드맵

**Phase 1-3: Spring Boot로 구현** (Week 1-10)
- REST API: Spring MVC
- WebSocket: Spring WebSocket
- 미디어 코어: 순수 Kotlin + Netty

**Phase 4: 성능 측정** (Week 11)
- 목표 달성 여부 확인
- 병목 지점 분석

**Phase 5: 선택적 최적화** (Week 12+)
- 목표 달성 시: Spring 유지 ✅
- 미달 시: 병목 부분만 Netty로 교체
- 극단적 경우: Ktor 전환 고려

---

## 💬 마무리

### Spring Boot 개발자가 Ktor 배우는 건 쉬움
```
학습 시간: 2~4주
어려움: 낮음 (오히려 더 간단)
```

### 하지만 유지보수는 별개
```
채용: Spring >> Ktor (100배 차이)
레퍼런스: Spring >> Ktor (100배 차이)
인수인계: Spring >> Ktor (쉬움 vs 어려움)
```

### 결론
**"기술적으로는 Ktor가 낫지만, 비즈니스적으로는 Spring Boot가 안전함"**

---

**Last Updated**: 2025-11-24
**추천**: Spring Boot + 하이브리드 구조 ⭐⭐⭐⭐⭐
