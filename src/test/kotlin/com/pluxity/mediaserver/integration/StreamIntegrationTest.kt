package com.pluxity.mediaserver.integration

import com.pluxity.mediaserver.domain.rtsp.RTSPManager
import com.pluxity.mediaserver.domain.stream.StreamManager
import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.junit.jupiter.api.Assertions.assertTrue

private val logger = KotlinLogging.logger {}

/**
 * 스트림 통합 테스트.
 *
 * 실제 RTSP 스트림을 사용하므로 환경변수로 제어합니다:
 * - ENABLE_INTEGRATION_TEST=true
 *
 * 또한 실제 RTSP URL이 필요합니다 (네트워크 접근 가능해야 함).
 */
@SpringBootTest
@EnabledIfEnvironmentVariable(named = "ENABLE_INTEGRATION_TEST", matches = "true")
class StreamIntegrationTest {

    @Autowired
    private lateinit var streamManager: StreamManager

    @Autowired
    private lateinit var rtspManager: RTSPManager

    /**
     * 스트림 생성 및 삭제 테스트.
     */
    @Test
    fun `test stream creation and deletion`() = runBlocking {
        logger.info { "Testing stream creation and deletion" }

        // 스트림 생성
        val streamId = "test-stream-1"
        val stream = streamManager.createStream(streamId)

        assertTrue(streamManager.hasStream(streamId))
        logger.info { "✅ Stream created: $streamId" }

        // 스트림 삭제
        streamManager.deleteStream(streamId)

        assertTrue(!streamManager.hasStream(streamId))
        logger.info { "✅ Stream deleted: $streamId" }
    }

    /**
     * RTSP 클라이언트 시작 테스트 (Mock URL).
     *
     * 실제 RTSP 서버가 없으면 연결 실패하지만,
     * 클라이언트 생성 및 재연결 로직은 테스트할 수 있습니다.
     */
    @Test
    fun `test RTSP client lifecycle`() = runBlocking {
        logger.info { "Testing RTSP client lifecycle" }

        val streamId = "test-rtsp-stream"
        val mockUrl = "rtsp://example.com/stream" // Mock URL

        try {
            // RTSP 클라이언트 시작 (실패 예상)
            rtspManager.startClient(streamId, mockUrl)
            logger.info { "✅ RTSP client started (will fail to connect)" }

            // 클라이언트 존재 확인
            val client = rtspManager.getClient(streamId)
            assertTrue(client != null)
            logger.info { "✅ RTSP client exists" }

            // 잠시 대기 (재연결 시도 확인)
            delay(2000)

            // RTSP 클라이언트 중지
            rtspManager.stopClient(streamId)
            logger.info { "✅ RTSP client stopped" }

            // 클라이언트 삭제 확인
            val clientAfterStop = rtspManager.getClient(streamId)
            assertTrue(clientAfterStop == null)
            logger.info { "✅ RTSP client removed" }

        } catch (e: Exception) {
            logger.error(e) { "RTSP test error (expected if no real server)" }
        }
    }

    /**
     * 스트림 목록 조회 테스트.
     */
    @Test
    fun `test stream list`() {
        logger.info { "Testing stream list" }

        val initialCount = streamManager.getStreamCount()
        logger.info { "Initial stream count: $initialCount" }

        // 여러 스트림 생성
        val streamIds = listOf("test-1", "test-2", "test-3")
        streamIds.forEach { streamManager.createStream(it) }

        val newCount = streamManager.getStreamCount()
        assertTrue(newCount == initialCount + 3)
        logger.info { "✅ Stream count after creation: $newCount" }

        // 스트림 목록 조회
        val list = streamManager.listStreamIds()
        streamIds.forEach { streamId ->
            assertTrue(list.contains(streamId))
        }
        logger.info { "✅ All test streams found in list" }

        // 정리
        runBlocking {
            streamIds.forEach { streamManager.deleteStream(it) }
        }
        logger.info { "✅ Test streams cleaned up" }
    }
}
