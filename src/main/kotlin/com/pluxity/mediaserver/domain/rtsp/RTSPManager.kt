package com.pluxity.mediaserver.domain.rtsp

import com.pluxity.mediaserver.common.NotFoundException
import com.pluxity.mediaserver.common.StreamException
import com.pluxity.mediaserver.domain.stream.StreamManager
import io.github.oshai.kotlinlogging.KotlinLogging
import org.springframework.stereotype.Component
import java.util.concurrent.ConcurrentHashMap

private val logger = KotlinLogging.logger {}

/**
 * RTSP 클라이언트 관리자.
 *
 * 여러 RTSP 클라이언트의 생명주기를 관리합니다.
 *
 * **기능**:
 * - RTSP 클라이언트 생성, 시작, 중지
 * - 클라이언트 상태 조회
 * - 자동 재연결 관리
 *
 * @property streamManager 스트림 관리자
 * @property config RTSP 설정
 */
@Component
class RTSPManager(
    private val streamManager: StreamManager,
    private val config: RTSPConfig
) {
    private val clients = ConcurrentHashMap<String, RTSPClient>()

    /**
     * RTSP 클라이언트 생성 및 시작.
     *
     * @param streamId 스트림 식별자
     * @param url RTSP URL
     * @return RTSPClient
     * @throws StreamException 이미 실행 중인 경우
     */
    fun startClient(streamId: String, url: String): RTSPClient {
        if (clients.containsKey(streamId)) {
            throw StreamException(streamId, "RTSP client already exists")
        }

        logger.info { "Starting RTSP client: $streamId -> $url" }

        val client = RTSPClient(streamId, url, streamManager, config)
        clients[streamId] = client

        client.start()

        return client
    }

    /**
     * RTSP 클라이언트 중지.
     *
     * @param streamId 스트림 식별자
     * @throws NotFoundException 클라이언트가 존재하지 않을 경우
     */
    fun stopClient(streamId: String) {
        val client = clients.remove(streamId)
            ?: throw NotFoundException("RTSP client", streamId)

        logger.info { "Stopping RTSP client: $streamId" }
        client.stop()
    }

    /**
     * RTSP 클라이언트 조회.
     *
     * @param streamId 스트림 식별자
     * @return RTSPClient (없으면 null)
     */
    fun getClient(streamId: String): RTSPClient? {
        return clients[streamId]
    }

    /**
     * 모든 RTSP 클라이언트 목록.
     *
     * @return 스트림 ID 리스트
     */
    fun listClients(): List<String> {
        return clients.keys().toList()
    }

    /**
     * 활성 클라이언트 개수.
     */
    fun getClientCount(): Int {
        return clients.size
    }

    /**
     * 모든 RTSP 클라이언트 중지.
     */
    fun stopAllClients() {
        logger.info { "Stopping all RTSP clients (${clients.size})" }

        clients.values.forEach { client ->
            try {
                client.stop()
            } catch (e: Exception) {
                logger.error(e) { "Error stopping RTSP client" }
            }
        }

        clients.clear()
    }

    /**
     * 클라이언트 상태 조회.
     *
     * @param streamId 스트림 식별자
     * @return true if running
     */
    fun isClientRunning(streamId: String): Boolean {
        return clients[streamId]?.isRunning() ?: false
    }
}
