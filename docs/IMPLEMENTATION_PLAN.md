# Kotlin Media Server 구현 플랜

> **작성일**: 2025-11-24
> **목표**: Go → Kotlin 완전 마이그레이션
> **예상 기간**: 22주 (5.5개월)

---

## 📋 목차

1. [전체 개요](#전체-개요)
2. [Phase 1: 기반 인프라 (Week 1-2)](#phase-1-기반-인프라-week-1-2)
3. [Phase 2: 핵심 도메인 (Week 3-6)](#phase-2-핵심-도메인-week-3-6)
4. [Phase 3: RTSP 연동 (Week 7-10)](#phase-3-rtsp-연동-week-7-10)
5. [Phase 4: WebRTC 연동 (Week 11-14)](#phase-4-webrtc-연동-week-11-14)
6. [Phase 5: API & UI (Week 15-18)](#phase-5-api--ui-week-15-18)
7. [Phase 6: 테스트 & 최적화 (Week 19-20)](#phase-6-테스트--최적화-week-19-20)
8. [Phase 7: 프로덕션 준비 (Week 21-22)](#phase-7-프로덕션-준비-week-21-22)
9. [체크리스트](#체크리스트)

---

## 전체 개요

### 마이그레이션 전략

```
Go 코드 (참조용)
    ↓ 분석 및 이해
Kotlin 구현
    ↓ 단위 테스트
통합 테스트
    ↓ 성능 검증
프로덕션 배포
```

### 아키텍처 레이어

```
┌─────────────────────────────────────────┐
│         Presentation Layer              │
│  - REST API (StreamController)          │
│  - WebSocket (SignalingHandler)         │
│  - Static Files (Web UI)                │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│         Application Layer               │
│  - StreamService                        │
│  - RTSPService                          │
│  - WebRTCService                        │
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
│  - JavaCV (FFmpeg)                      │
│  - Netty (ByteBuf)                      │
│  - WebRTC Library                       │
└─────────────────────────────────────────┘
```

---

## Phase 1: 기반 인프라 (Week 1-2)

> **목표**: 프로젝트 뼈대 및 공통 유틸리티 구현
> **상태**: ✅ 50% 완료 (프로젝트 구조 완료)

### Week 1: 프로젝트 초기화 ✅

**완료된 작업**:
- [x] Go 파일 go-legacy로 이동
- [x] Kotlin 프로젝트 구조 생성
- [x] build.gradle.kts 설정
- [x] application.yaml 기본 설정
- [x] MediaServerApplication.kt
- [x] HealthController.kt

### Week 2: 공통 인프라 구현

**작업 목록**:

#### 2.1 로깅 유틸리티
**파일**: `src/main/kotlin/com/pluxity/mediaserver/common/logging/`

```kotlin
// LoggingExtensions.kt
fun <T : Any> T.logger(): KLogger = KotlinLogging.logger(this::class.java.name)

// StructuredLogger.kt
object StructuredLogger {
    fun logStreamEvent(streamId: String, event: String, details: Map<String, Any>)
    fun logRTPPacket(streamId: String, packet: RTPPacket)
    fun logWebRTCEvent(peerId: String, event: String, sdp: String? = null)
}
```

**테스트**:
```kotlin
class LoggingExtensionsTest {
    @Test
    fun `should create logger with class name`()
}
```

#### 2.2 예외 처리 체계
**파일**: `src/main/kotlin/com/pluxity/mediaserver/common/exception/`

```kotlin
// MediaServerException.kt
sealed class MediaServerException(message: String, cause: Throwable? = null) :
    RuntimeException(message, cause) {

    class StreamNotFoundException(streamId: String) :
        MediaServerException("Stream not found: $streamId")

    class RTSPConnectionException(url: String, cause: Throwable) :
        MediaServerException("Failed to connect to RTSP: $url", cause)

    class WebRTCPeerException(peerId: String, message: String) :
        MediaServerException("WebRTC Peer error [$peerId]: $message")

    class ConfigurationException(message: String) :
        MediaServerException("Configuration error: $message")
}

// GlobalExceptionHandler.kt
@RestControllerAdvice
class GlobalExceptionHandler {
    @ExceptionHandler(MediaServerException::class)
    fun handleMediaServerException(ex: MediaServerException): ResponseEntity<ErrorResponse>
}
```

#### 2.3 메트릭 수집 유틸리티
**파일**: `src/main/kotlin/com/pluxity/mediaserver/common/metrics/`

```kotlin
// MetricsCollector.kt
@Component
class MetricsCollector(private val meterRegistry: MeterRegistry) {
    private val streamCount = meterRegistry.gauge("media.streams.active", AtomicInteger(0))
    private val peerCount = meterRegistry.gauge("media.peers.active", AtomicInteger(0))

    fun incrementStreamCount()
    fun decrementStreamCount()
    fun recordPacketReceived(streamId: String)
    fun recordPacketSent(peerId: String)
}
```

#### 2.4 Netty ByteBuf 유틸리티
**파일**: `src/main/kotlin/com/pluxity/mediaserver/common/netty/`

```kotlin
// ByteBufExtensions.kt
inline fun <T> ByteBuf.use(block: (ByteBuf) -> T): T {
    try {
        return block(this)
    } finally {
        ReferenceCountUtil.safeRelease(this)
    }
}

// ByteBufPool.kt
@Component
class ByteBufPool {
    private val allocator = PooledByteBufAllocator.DEFAULT

    fun allocate(size: Int): ByteBuf = allocator.directBuffer(size)
    fun allocateHeap(size: Int): ByteBuf = allocator.heapBuffer(size)
}
```

**완료 기준**:
- [ ] 모든 유틸리티 클래스 구현
- [ ] 단위 테스트 작성 (커버리지 80%+)
- [ ] 로깅 패턴 확립
- [ ] 예외 처리 가이드 문서화

---

## Phase 2: 핵심 도메인 (Week 3-6)

> **목표**: StreamManager 구현 (미디어 서버의 핵심)

### Week 3-4: StreamManager 구현

**파일 구조**:
```
domain/stream/
├── StreamManager.kt          # 메인 매니저
├── StreamFlow.kt             # Flow 기반 스트림
├── RTPPacket.kt              # RTP 패킷 모델
├── StreamRepository.kt       # 스트림 저장소 인터페이스
└── InMemoryStreamRepository.kt  # 메모리 기반 구현
```

#### 3.1 RTPPacket 모델
**파일**: `domain/stream/RTPPacket.kt`

```kotlin
data class RTPPacket(
    val streamId: String,
    val timestamp: Long,
    val sequenceNumber: Int,
    val payloadType: Int,
    val payload: ByteBuf,  // Netty ByteBuf (Off-heap)
    val ssrc: Long,
    val marker: Boolean = false
) {
    companion object {
        fun fromByteArray(streamId: String, data: ByteArray): RTPPacket
        fun fromByteBuf(streamId: String, buffer: ByteBuf): RTPPacket
    }

    fun release() {
        payload.release()
    }
}
```

**테스트**:
```kotlin
class RTPPacketTest {
    @Test
    fun `should parse RTP header correctly`()

    @Test
    fun `should handle ByteBuf lifecycle`()
}
```

#### 3.2 StreamFlow (Flow 기반 Pub/Sub)
**파일**: `domain/stream/StreamFlow.kt`

```kotlin
class StreamFlow(val id: String) {
    private val logger = logger()

    // MutableSharedFlow: 여러 구독자에게 브로드캐스트
    private val _packets = MutableSharedFlow<RTPPacket>(
        replay = 0,  // 새 구독자에게 이전 패킷 전송 안 함
        extraBufferCapacity = 1000,  // 버퍼 크기
        onBufferOverflow = BufferOverflow.DROP_OLDEST  // 오버플로우 시 가장 오래된 패킷 버림
    )

    val packets: SharedFlow<RTPPacket> = _packets.asSharedFlow()

    // 구독자 수 추적
    private val _subscriberCount = MutableStateFlow(0)
    val subscriberCount: StateFlow<Int> = _subscriberCount.asStateFlow()

    // 통계
    private val stats = StreamStats()

    suspend fun publish(packet: RTPPacket) {
        logger.trace { "Publishing packet: seq=${packet.sequenceNumber}" }

        stats.incrementPublished()
        _packets.emit(packet)
    }

    suspend fun subscribe(handler: suspend (RTPPacket) -> Unit): Job = coroutineScope {
        _subscriberCount.value++
        logger.info { "Subscriber added. Total: ${_subscriberCount.value}" }

        launch {
            try {
                packets.collect { packet ->
                    handler(packet)
                    stats.incrementDelivered()
                }
            } finally {
                _subscriberCount.value--
                logger.info { "Subscriber removed. Total: ${_subscriberCount.value}" }
            }
        }
    }

    fun getStats(): StreamStats = stats.copy()
}

data class StreamStats(
    val packetsPublished: AtomicLong = AtomicLong(0),
    val packetsDelivered: AtomicLong = AtomicLong(0),
    val bytesPublished: AtomicLong = AtomicLong(0)
) {
    fun incrementPublished() = packetsPublished.incrementAndGet()
    fun incrementDelivered() = packetsDelivered.incrementAndGet()
    fun copy() = StreamStats(
        AtomicLong(packetsPublished.get()),
        AtomicLong(packetsDelivered.get()),
        AtomicLong(bytesPublished.get())
    )
}
```

**테스트**:
```kotlin
class StreamFlowTest {
    @Test
    fun `should publish and collect packets`() = runBlocking {
        val flow = StreamFlow("test")
        val received = mutableListOf<RTPPacket>()

        launch {
            flow.subscribe { packet ->
                received.add(packet)
            }
        }

        delay(100)

        repeat(10) { i ->
            flow.publish(createTestPacket(i))
        }

        delay(100)
        assertEquals(10, received.size)
    }

    @Test
    fun `should track subscriber count`() = runBlocking {
        val flow = StreamFlow("test")

        val job1 = flow.subscribe { }
        assertEquals(1, flow.subscriberCount.value)

        val job2 = flow.subscribe { }
        assertEquals(2, flow.subscriberCount.value)

        job1.cancel()
        delay(100)
        assertEquals(1, flow.subscriberCount.value)
    }
}
```

#### 3.3 StreamManager
**파일**: `domain/stream/StreamManager.kt`

```kotlin
@Component
class StreamManager(
    private val metricsCollector: MetricsCollector
) {
    private val logger = logger()
    private val streams = ConcurrentHashMap<String, StreamFlow>()

    fun createStream(id: String): StreamFlow {
        logger.info { "Creating stream: $id" }

        val stream = streams.computeIfAbsent(id) { StreamFlow(id) }
        metricsCollector.incrementStreamCount()

        return stream
    }

    fun getStream(id: String): StreamFlow? = streams[id]

    fun removeStream(id: String): Boolean {
        logger.info { "Removing stream: $id" }

        return streams.remove(id)?.let {
            metricsCollector.decrementStreamCount()
            true
        } ?: false
    }

    fun getAllStreams(): List<StreamInfo> = streams.values.map { stream ->
        StreamInfo(
            id = stream.id,
            subscriberCount = stream.subscriberCount.value,
            stats = stream.getStats()
        )
    }

    suspend fun publishToStream(streamId: String, packet: RTPPacket) {
        val stream = getStream(streamId)
            ?: throw MediaServerException.StreamNotFoundException(streamId)

        stream.publish(packet)
    }
}

data class StreamInfo(
    val id: String,
    val subscriberCount: Int,
    val stats: StreamStats
)
```

**테스트**:
```kotlin
class StreamManagerTest {
    private lateinit var manager: StreamManager

    @BeforeEach
    fun setup() {
        manager = StreamManager(mockMetricsCollector())
    }

    @Test
    fun `should create and retrieve stream`() {
        val stream = manager.createStream("test")
        assertNotNull(stream)
        assertEquals("test", stream.id)

        val retrieved = manager.getStream("test")
        assertSame(stream, retrieved)
    }

    @Test
    fun `should not create duplicate streams`() {
        val stream1 = manager.createStream("test")
        val stream2 = manager.createStream("test")
        assertSame(stream1, stream2)
    }

    @Test
    fun `should publish to stream`() = runBlocking {
        manager.createStream("test")
        val packet = createTestPacket(0)

        assertDoesNotThrow {
            manager.publishToStream("test", packet)
        }
    }
}
```

**완료 기준**:
- [ ] StreamFlow 구현 완료
- [ ] StreamManager 구현 완료
- [ ] 단위 테스트 작성 (커버리지 90%+)
- [ ] 동시성 테스트 (1000+ 동시 publish)
- [ ] 메모리 누수 테스트 (ByteBuf release 확인)

### Week 5-6: 통합 테스트 및 벤치마크

#### 5.1 통합 테스트
**파일**: `test/kotlin/.../integration/StreamManagerIntegrationTest.kt`

```kotlin
@SpringBootTest
class StreamManagerIntegrationTest {
    @Autowired
    private lateinit var streamManager: StreamManager

    @Test
    fun `should handle multiple streams concurrently`() = runBlocking {
        val streamCount = 100
        val packetsPerStream = 1000

        // 100개 스트림 생성
        val streams = (1..streamCount).map { i ->
            streamManager.createStream("stream-$i")
        }

        // 각 스트림에 1000개 패킷 발행
        coroutineScope {
            streams.forEach { stream ->
                launch {
                    repeat(packetsPerStream) { i ->
                        stream.publish(createTestPacket(i))
                    }
                }
            }
        }

        // 통계 확인
        streams.forEach { stream ->
            assertEquals(packetsPerStream.toLong(), stream.getStats().packetsPublished.get())
        }
    }
}
```

#### 5.2 성능 벤치마크
**파일**: `test/kotlin/.../benchmark/StreamBenchmark.kt`

```kotlin
@State(Scope.Benchmark)
class StreamBenchmark {
    private lateinit var streamManager: StreamManager
    private lateinit var stream: StreamFlow

    @Setup
    fun setup() {
        streamManager = StreamManager(MockMetricsCollector())
        stream = streamManager.createStream("benchmark")
    }

    @Benchmark
    fun publishPacket() = runBlocking {
        stream.publish(createTestPacket(0))
    }

    @Benchmark
    fun publishAndCollect() = runBlocking {
        val job = stream.subscribe { /* no-op */ }
        stream.publish(createTestPacket(0))
        job.cancel()
    }
}
```

**목표 성능**:
- Publish 지연시간: < 100μs (P99)
- 처리량: > 50,000 packets/sec (단일 스트림)
- 메모리: < 200MB (100 스트림, 1000 패킷/초)

**완료 기준**:
- [ ] 통합 테스트 통과
- [ ] 벤치마크 목표 달성
- [ ] 메모리 프로파일링 (JFR)
- [ ] 성능 리포트 작성

---

## Phase 3: RTSP 연동 (Week 7-10)

> **목표**: JavaCV로 RTSP 스트림 수신 및 RTP 패킷 추출

### Week 7-8: RTSP Client 구현

**파일 구조**:
```
domain/rtsp/
├── RTSPClient.kt             # RTSP 클라이언트 (Virtual Threads)
├── RTSPManager.kt            # RTSP 클라이언트 관리
├── RTSPConfig.kt             # RTSP 설정
└── RTSPPacketExtractor.kt    # RTP 패킷 추출
```

#### 7.1 RTSPClient (Virtual Threads)
**파일**: `domain/rtsp/RTSPClient.kt`

```kotlin
class RTSPClient(
    private val streamId: String,
    private val url: String,
    private val streamManager: StreamManager,
    private val config: RTSPConfig
) {
    private val logger = logger()
    private val running = AtomicBoolean(false)
    private var thread: Thread? = null

    fun start() {
        if (running.getAndSet(true)) {
            logger.warn { "RTSP client already running: $streamId" }
            return
        }

        logger.info { "Starting RTSP client: $streamId -> $url" }

        // Virtual Thread 사용
        thread = Thread.startVirtualThread {
            try {
                connectAndStream()
            } catch (e: Exception) {
                logger.error(e) { "RTSP client error: $streamId" }
                throw MediaServerException.RTSPConnectionException(url, e)
            } finally {
                running.set(false)
            }
        }
    }

    private fun connectAndStream() {
        // JavaCV FFmpegFrameGrabber 사용
        val grabber = FFmpegFrameGrabber(url).apply {
            videoOption("rtsp_transport", config.transport)  // tcp or udp
            format = "rtsp"

            // 성능 옵션
            videoOption("buffer_size", "1024000")
            videoOption("max_delay", "500000")

            start()
        }

        logger.info { "RTSP connected: $streamId" }

        val stream = streamManager.getStream(streamId)
            ?: streamManager.createStream(streamId)

        try {
            while (running.get()) {
                // Frame 읽기 (블로킹 호출이지만 Virtual Thread라 가벼움)
                val frame = grabber.grabFrame() ?: continue

                // Video frame만 처리
                if (frame.image == null) continue

                // AVPacket에서 RTP 패킷 추출
                val rtpPacket = extractRTPPacket(frame, streamId)

                // StreamManager에 발행
                runBlocking {
                    stream.publish(rtpPacket)
                }
            }
        } finally {
            grabber.stop()
            grabber.release()
            logger.info { "RTSP disconnected: $streamId" }
        }
    }

    private fun extractRTPPacket(frame: Frame, streamId: String): RTPPacket {
        // AVPacket → RTP 패킷 변환
        // frame.opaque는 AVPacket 포인터
        val avPacket = AVPacket(frame.opaque)

        return RTPPacket(
            streamId = streamId,
            timestamp = frame.timestamp,
            sequenceNumber = 0,  // TODO: 실제 시퀀스 번호 추출
            payloadType = 96,  // H.264/H.265
            payload = ByteBufPool.allocate(avPacket.size()).also { buf ->
                // AVPacket 데이터 → ByteBuf 복사
                buf.writeBytes(avPacket.data().asByteBuffer())
            },
            ssrc = 0,  // TODO: 실제 SSRC 추출
            marker = false
        )
    }

    fun stop() {
        logger.info { "Stopping RTSP client: $streamId" }
        running.set(false)
        thread?.join(5000)
    }

    fun isRunning(): Boolean = running.get()
}
```

**테스트**:
```kotlin
class RTSPClientTest {
    @Test
    fun `should connect to RTSP stream`() {
        val client = RTSPClient(
            streamId = "test",
            url = "rtsp://example.com/stream",
            streamManager = mockStreamManager(),
            config = RTSPConfig()
        )

        client.start()
        assertTrue(client.isRunning())

        Thread.sleep(1000)

        client.stop()
        assertFalse(client.isRunning())
    }
}
```

#### 7.2 RTSPManager
**파일**: `domain/rtsp/RTSPManager.kt`

```kotlin
@Component
class RTSPManager(
    private val streamManager: StreamManager,
    @Value("\${media.rtsp}") private val rtspConfig: RtspConfig
) {
    private val logger = logger()
    private val clients = ConcurrentHashMap<String, RTSPClient>()

    fun startStream(streamId: String, url: String): RTSPClient {
        logger.info { "Starting RTSP stream: $streamId -> $url" }

        val client = clients.computeIfAbsent(streamId) {
            RTSPClient(
                streamId = streamId,
                url = url,
                streamManager = streamManager,
                config = RTSPConfig(transport = rtspConfig.transport)
            )
        }

        client.start()
        return client
    }

    fun stopStream(streamId: String): Boolean {
        logger.info { "Stopping RTSP stream: $streamId" }

        return clients.remove(streamId)?.let { client ->
            client.stop()
            true
        } ?: false
    }

    fun getClient(streamId: String): RTSPClient? = clients[streamId]

    fun getAllClients(): List<RTSPClientInfo> = clients.map { (id, client) ->
        RTSPClientInfo(
            streamId = id,
            isRunning = client.isRunning()
        )
    }
}

data class RTSPClientInfo(
    val streamId: String,
    val isRunning: Boolean
)
```

**완료 기준**:
- [ ] RTSPClient 구현 완료
- [ ] Virtual Thread로 동작 확인
- [ ] 실제 RTSP 스트림 연결 테스트
- [ ] RTP 패킷 정상 추출 확인
- [ ] 재연결 로직 구현

### Week 9-10: RTSP 통합 테스트

#### 9.1 E2E 테스트
**파일**: `test/kotlin/.../e2e/RTSPStreamingTest.kt`

```kotlin
@SpringBootTest
class RTSPStreamingTest {
    @Autowired
    private lateinit var rtspManager: RTSPManager

    @Autowired
    private lateinit var streamManager: StreamManager

    @Test
    fun `should stream from RTSP to StreamFlow`() = runBlocking {
        val streamId = "test-rtsp"
        val rtspUrl = "rtsp://wowzaec2demo.streamlock.net/vod/mp4:BigBuckBunny_115k.mov"

        // RTSP 시작
        rtspManager.startStream(streamId, rtspUrl)

        // 구독자 추가
        val receivedPackets = mutableListOf<RTPPacket>()
        val stream = streamManager.getStream(streamId)!!

        val job = stream.subscribe { packet ->
            receivedPackets.add(packet)
        }

        // 5초 대기
        delay(5000)

        // 검증
        assertTrue(receivedPackets.size > 100)
        logger.info { "Received ${receivedPackets.size} packets" }

        // 정리
        job.cancel()
        rtspManager.stopStream(streamId)
    }
}
```

**완료 기준**:
- [ ] 공개 RTSP 스트림 테스트 성공
- [ ] 실제 CCTV 카메라 연결 테스트
- [ ] 장시간 안정성 테스트 (24시간)
- [ ] 재연결 테스트 (네트워크 끊김 시뮬레이션)

---

## Phase 4: WebRTC 연동 (Week 11-14)

> **목표**: WebRTC Peer 구현 및 브라우저 연결

### Week 11-12: WebRTC Peer 구현

**파일 구조**:
```
domain/webrtc/
├── WebRTCPeer.kt             # WebRTC 피어 (Coroutines)
├── WebRTCManager.kt          # 피어 관리
├── ICECandidateHandler.kt    # ICE 후보 처리
└── SDPHandler.kt             # SDP 처리
```

#### 11.1 WebRTC 라이브러리 선택

**옵션 1: Kurento** (권장)
```kotlin
dependencies {
    implementation("org.kurento:kurento-client:7.0.0")
}
```

**옵션 2: webrtc-java**
```kotlin
dependencies {
    implementation("dev.onvoid.webrtc:webrtc-java:0.8.0")
}
```

**선택**: Kurento (프로덕션 검증됨, SFU 기능)

#### 11.2 WebRTCPeer
**파일**: `domain/webrtc/WebRTCPeer.kt`

```kotlin
class WebRTCPeer(
    val id: String,
    private val streamId: String,
    private val streamManager: StreamManager,
    private val iceServers: List<IceServerConfig>
) {
    private val logger = logger()
    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())

    private lateinit var webRtcEndpoint: WebRtcEndpoint
    private var subscriptionJob: Job? = null

    suspend fun initialize(mediaPipeline: MediaPipeline) {
        webRtcEndpoint = WebRtcEndpoint.Builder(mediaPipeline).build()

        // ICE 후보 이벤트
        webRtcEndpoint.addIceCandidateFoundListener { event ->
            logger.debug { "ICE candidate found: ${event.candidate.candidate}" }
            // ICE 후보를 클라이언트에게 전송 (WebSocket)
        }

        // 연결 상태 이벤트
        webRtcEndpoint.addConnectionStateChangedListener { event ->
            logger.info { "Connection state changed: ${event.newState}" }
        }
    }

    suspend fun processOffer(offerSdp: String): String = suspendCoroutine { cont ->
        webRtcEndpoint.processOffer(offerSdp, object : Continuation<String> {
            override fun onSuccess(answerSdp: String) {
                logger.info { "Created answer SDP" }
                cont.resume(answerSdp)
            }

            override fun onError(error: Throwable) {
                cont.resumeWithException(error)
            }
        })
    }

    suspend fun addIceCandidate(candidateString: String) {
        val candidate = IceCandidate(candidateString, "", 0)
        webRtcEndpoint.addIceCandidate(candidate)
    }

    suspend fun startStreaming() {
        val stream = streamManager.getStream(streamId)
            ?: throw MediaServerException.StreamNotFoundException(streamId)

        logger.info { "Starting streaming for peer: $id" }

        subscriptionJob = scope.launch {
            stream.subscribe { packet ->
                // RTP 패킷을 WebRTC로 전송
                sendRTPPacket(packet)
            }
        }
    }

    private suspend fun sendRTPPacket(packet: RTPPacket) {
        // Kurento는 내부적으로 RTP 처리
        // 직접 전송은 불필요 (MediaElement 연결로 처리)
    }

    fun close() {
        logger.info { "Closing peer: $id" }
        subscriptionJob?.cancel()
        scope.cancel()
        webRtcEndpoint.release()
    }
}
```

**테스트**:
```kotlin
class WebRTCPeerTest {
    @Test
    fun `should create offer and answer`() = runBlocking {
        val peer = WebRTCPeer(
            id = "test-peer",
            streamId = "test-stream",
            streamManager = mockStreamManager(),
            iceServers = emptyList()
        )

        val mediaPipeline = mockMediaPipeline()
        peer.initialize(mediaPipeline)

        val offerSdp = createTestOfferSdp()
        val answerSdp = peer.processOffer(offerSdp)

        assertTrue(answerSdp.contains("v=0"))
        assertTrue(answerSdp.contains("a=sendonly"))
    }
}
```

**완료 기준**:
- [ ] WebRTCPeer 구현 완료
- [ ] Offer/Answer 교환 성공
- [ ] ICE 연결 성공
- [ ] RTP 패킷 전송 확인

### Week 13-14: WebRTC 통합 테스트

#### 13.1 브라우저 테스트
**파일**: `test/kotlin/.../e2e/WebRTCBrowserTest.kt`

```kotlin
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class WebRTCBrowserTest {
    @LocalServerPort
    private var port: Int = 0

    @Test
    fun `should establish WebRTC connection from browser`() = runBlocking {
        // Playwright 또는 Selenium으로 브라우저 자동화
        // 1. 브라우저 열기
        // 2. WebSocket 연결
        // 3. Offer 전송
        // 4. Answer 수신
        // 5. ICE 연결 확인
        // 6. 영상 수신 확인
    }
}
```

**완료 기준**:
- [ ] Chrome 브라우저 연결 성공
- [ ] Firefox 브라우저 연결 성공
- [ ] Edge 브라우저 연결 성공
- [ ] 영상 재생 확인

---

## Phase 5: API & UI (Week 15-18)

> **목표**: REST API 및 WebSocket 시그널링 구현

### Week 15-16: REST API 구현

**파일 구조**:
```
presentation/api/
├── StreamController.kt       # 스트림 CRUD
├── HealthController.kt       # 헬스체크 (완료)
└── dto/
    ├── StreamRequest.kt
    └── StreamResponse.kt
```

#### 15.1 StreamController
**파일**: `presentation/api/StreamController.kt`

```kotlin
@RestController
@RequestMapping("/api/v1/streams")
class StreamController(
    private val streamManager: StreamManager,
    private val rtspManager: RTSPManager
) {
    private val logger = logger()

    @GetMapping
    fun getAllStreams(): List<StreamResponse> {
        return streamManager.getAllStreams().map { it.toResponse() }
    }

    @GetMapping("/{id}")
    fun getStream(@PathVariable id: String): StreamResponse {
        val stream = streamManager.getStream(id)
            ?: throw MediaServerException.StreamNotFoundException(id)

        return StreamInfo(
            id = stream.id,
            subscriberCount = stream.subscriberCount.value,
            stats = stream.getStats()
        ).toResponse()
    }

    @PostMapping("/{id}/start")
    suspend fun startStream(
        @PathVariable id: String,
        @RequestBody request: StartStreamRequest
    ): StreamResponse {
        logger.info { "Starting stream: $id" }

        rtspManager.startStream(id, request.rtspUrl)

        return getStream(id)
    }

    @DeleteMapping("/{id}")
    fun stopStream(@PathVariable id: String): ResponseEntity<Void> {
        logger.info { "Stopping stream: $id" }

        rtspManager.stopStream(id)
        streamManager.removeStream(id)

        return ResponseEntity.noContent().build()
    }
}

data class StartStreamRequest(
    val rtspUrl: String
)

data class StreamResponse(
    val id: String,
    val subscriberCount: Int,
    val stats: StatsResponse
)

data class StatsResponse(
    val packetsPublished: Long,
    val packetsDelivered: Long,
    val bytesPublished: Long
)

fun StreamInfo.toResponse() = StreamResponse(
    id = id,
    subscriberCount = subscriberCount,
    stats = StatsResponse(
        packetsPublished = stats.packetsPublished.get(),
        packetsDelivered = stats.packetsDelivered.get(),
        bytesPublished = stats.bytesPublished.get()
    )
)
```

**테스트**:
```kotlin
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
@AutoConfigureMockMvc
class StreamControllerTest {
    @Autowired
    private lateinit var mockMvc: MockMvc

    @Test
    fun `should return all streams`() {
        mockMvc.get("/api/v1/streams")
            .andExpect {
                status { isOk() }
                content { contentType(MediaType.APPLICATION_JSON) }
            }
    }
}
```

**완료 기준**:
- [ ] 모든 엔드포인트 구현
- [ ] 단위 테스트 작성
- [ ] 통합 테스트 작성
- [ ] OpenAPI 문서 생성

### Week 17-18: WebSocket 시그널링

**파일 구조**:
```
presentation/websocket/
├── WebSocketConfig.kt        # WebSocket 설정
├── SignalingHandler.kt       # 시그널링 핸들러
└── dto/
    └── SignalingMessage.kt
```

#### 17.1 SignalingHandler
**파일**: `presentation/websocket/SignalingHandler.kt`

```kotlin
@Component
class SignalingHandler(
    private val webrtcManager: WebRTCManager
) : TextWebSocketHandler() {
    private val logger = logger()
    private val sessions = ConcurrentHashMap<String, WebSocketSession>()

    override fun afterConnectionEstablished(session: WebSocketSession) {
        val streamId = extractStreamId(session)
        sessions[session.id] = session

        logger.info { "WebSocket connected: ${session.id} for stream: $streamId" }
    }

    override fun handleTextMessage(session: WebSocketSession, message: TextMessage) {
        val signalingMessage = Json.decodeFromString<SignalingMessage>(message.payload)

        when (signalingMessage.type) {
            "offer" -> handleOffer(session, signalingMessage)
            "ice" -> handleIceCandidate(session, signalingMessage)
            else -> logger.warn { "Unknown message type: ${signalingMessage.type}" }
        }
    }

    private suspend fun handleOffer(session: WebSocketSession, message: SignalingMessage) {
        val streamId = extractStreamId(session)
        val peer = webrtcManager.createPeer(streamId, session.id)

        val answerSdp = peer.processOffer(message.sdp!!)

        val response = SignalingMessage(
            type = "answer",
            sdp = answerSdp
        )

        session.sendMessage(TextMessage(Json.encodeToString(response)))
    }

    private fun extractStreamId(session: WebSocketSession): String {
        val uri = session.uri ?: throw IllegalArgumentException("No URI")
        return uri.path.split("/").last()
    }

    override fun afterConnectionClosed(session: WebSocketSession, status: CloseStatus) {
        sessions.remove(session.id)
        webrtcManager.removePeer(session.id)

        logger.info { "WebSocket disconnected: ${session.id}" }
    }
}

@Serializable
data class SignalingMessage(
    val type: String,
    val sdp: String? = null,
    val candidate: String? = null
)
```

**완료 기준**:
- [ ] WebSocket 시그널링 구현
- [ ] Offer/Answer 교환 성공
- [ ] ICE candidate 교환 성공
- [ ] 브라우저 테스트 성공

---

## Phase 6: 테스트 & 최적화 (Week 19-20)

> **목표**: 전체 시스템 통합 테스트 및 성능 최적화

### Week 19: 통합 테스트

#### 19.1 E2E 테스트 시나리오

**시나리오 1: Full Streaming Pipeline**
```kotlin
@SpringBootTest
class FullPipelineTest {
    @Test
    fun `should stream from RTSP to browser`() = runBlocking {
        // 1. RTSP 스트림 시작
        rtspManager.startStream("test", testRtspUrl)

        // 2. WebSocket 연결
        val wsClient = createWebSocketClient()
        wsClient.connect("/ws/test")

        // 3. Offer 전송
        val offer = createTestOffer()
        wsClient.send(offer)

        // 4. Answer 수신
        val answer = wsClient.receive()
        assertNotNull(answer)

        // 5. 영상 수신 확인 (시뮬레이션)
        delay(5000)

        // 6. 통계 확인
        val stream = streamManager.getStream("test")!!
        assertTrue(stream.getStats().packetsPublished.get() > 100)
    }
}
```

**완료 기준**:
- [ ] 모든 E2E 시나리오 통과
- [ ] 다중 스트림 테스트 (10개 동시)
- [ ] 다중 클라이언트 테스트 (100명 동시)
- [ ] 장애 복구 테스트

### Week 20: 성능 최적화

#### 20.1 프로파일링

**JFR 프로파일링**:
```bash
java -XX:StartFlightRecording=filename=app.jfr,duration=60s \
     -XX:+UseZGC -XX:+ZGenerational \
     -jar media-server.jar
```

**분석 포인트**:
- CPU 핫스팟
- 메모리 할당
- GC 이벤트
- I/O 대기

#### 20.2 최적화 항목

**메모리 최적화**:
- [ ] ByteBuf 풀링 최적화
- [ ] GC 튜닝 (ZGC 파라미터)
- [ ] 메모리 누수 제거

**성능 최적화**:
- [ ] Coroutine dispatcher 최적화
- [ ] Virtual Threads 스케줄러 튜닝
- [ ] Netty 파이프라인 최적화

**목표 성능**:
- 처리량: > 50,000 packets/sec
- P99 레이턴시: < 10ms
- 메모리: < 500MB (100 스트림)
- CPU: < 50% (8 코어)

**완료 기준**:
- [ ] 성능 목표 달성
- [ ] 프로파일링 리포트 작성
- [ ] 최적화 가이드 문서화

---

## Phase 7: 프로덕션 준비 (Week 21-22)

> **목표**: 프로덕션 배포 준비

### Week 21: 프로덕션 설정

#### 21.1 Docker 이미지

**Dockerfile**:
```dockerfile
FROM amazoncorretto:21-alpine AS builder
WORKDIR /app
COPY . .
RUN ./gradlew bootJar --no-daemon

FROM amazoncorretto:21-alpine
WORKDIR /app
COPY --from=builder /app/build/libs/*.jar app.jar

ENV JAVA_OPTS="-XX:+UseZGC -XX:+ZGenerational -Xms2g -Xmx4g"
EXPOSE 8080
ENTRYPOINT exec java $JAVA_OPTS -jar app.jar
```

#### 21.2 Kubernetes 배포

**deployment.yaml**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: media-server
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: media-server
        image: media-server:latest
        resources:
          requests:
            memory: "4Gi"
            cpu: "2000m"
          limits:
            memory: "6Gi"
            cpu: "4000m"
```

**완료 기준**:
- [ ] Docker 이미지 빌드 성공
- [ ] Kubernetes 배포 성공
- [ ] 헬스체크 설정
- [ ] 로그 수집 설정

### Week 22: 모니터링 및 문서화

#### 22.1 모니터링 대시보드

**Grafana 대시보드**:
- JVM 메트릭 (힙, GC, 스레드)
- 스트림 메트릭 (활성 스트림 수, 패킷 처리량)
- WebRTC 메트릭 (활성 피어 수, 연결 상태)

#### 22.2 운영 문서

**문서 목록**:
- [ ] 배포 가이드
- [ ] 운영 가이드 (로그, 모니터링)
- [ ] 트러블슈팅 가이드
- [ ] API 문서 (OpenAPI)
- [ ] 아키텍처 문서

**완료 기준**:
- [ ] 모니터링 대시보드 완성
- [ ] 모든 운영 문서 작성
- [ ] 프로덕션 체크리스트 완료

---

## 체크리스트

### Phase 1: 기반 인프라 ✅
- [x] 프로젝트 구조 생성
- [x] build.gradle.kts 설정
- [x] application.yaml 설정
- [ ] 로깅 유틸리티
- [ ] 예외 처리 체계
- [ ] 메트릭 수집
- [ ] Netty ByteBuf 유틸리티

### Phase 2: 핵심 도메인
- [ ] RTPPacket 모델
- [ ] StreamFlow 구현
- [ ] StreamManager 구현
- [ ] 단위 테스트 (90%+ 커버리지)
- [ ] 통합 테스트
- [ ] 성능 벤치마크

### Phase 3: RTSP 연동
- [ ] RTSPClient 구현
- [ ] RTSPManager 구현
- [ ] RTP 패킷 추출
- [ ] 실제 RTSP 테스트
- [ ] 재연결 로직
- [ ] 장시간 안정성 테스트

### Phase 4: WebRTC 연동
- [ ] WebRTCPeer 구현
- [ ] WebRTCManager 구현
- [ ] Offer/Answer 교환
- [ ] ICE 연결
- [ ] 브라우저 테스트 (Chrome, Firefox, Edge)

### Phase 5: API & UI
- [ ] REST API 구현
- [ ] WebSocket 시그널링
- [ ] 정적 파일 서빙
- [ ] OpenAPI 문서
- [ ] API 테스트

### Phase 6: 테스트 & 최적화
- [ ] E2E 테스트 (모든 시나리오)
- [ ] 성능 테스트 (목표 달성)
- [ ] JFR 프로파일링
- [ ] 메모리 최적화
- [ ] 성능 최적화

### Phase 7: 프로덕션 준비
- [ ] Docker 이미지
- [ ] Kubernetes 배포
- [ ] 모니터링 대시보드
- [ ] 운영 문서
- [ ] 프로덕션 체크리스트

---

## 다음 단계

**즉시 시작할 작업**:

1. **Week 2 시작**: 공통 인프라 구현
   - [ ] LoggingExtensions.kt
   - [ ] MediaServerException.kt
   - [ ] MetricsCollector.kt
   - [ ] ByteBufExtensions.kt

2. **테스트 환경 구축**:
   - [ ] 공개 RTSP 테스트 스트림 확보
   - [ ] 로컬 RTSP 서버 설정 (mediamtx)
   - [ ] 브라우저 자동화 도구 설정 (Playwright)

3. **성능 목표 설정**:
   - [ ] 벤치마크 기준선 측정 (Go 버전)
   - [ ] 목표 메트릭 정의
   - [ ] 측정 도구 준비 (JMH, JFR)

---

**Last Updated**: 2025-11-24
**Status**: 🚀 Phase 1 완료, Phase 2 시작 준비
**Next Milestone**: Week 2 - 공통 인프라 구현
