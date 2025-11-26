package com.pluxity.mediaserver.domain.webrtc

import com.pluxity.mediaserver.common.NotFoundException
import com.pluxity.mediaserver.common.ResourceLimitException
import com.pluxity.mediaserver.config.StreamsConfig
import com.pluxity.mediaserver.domain.rtsp.RTSPManager
import com.pluxity.mediaserver.domain.stream.StreamManager
import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.coroutines.runBlocking
import org.springframework.stereotype.Component
import java.util.concurrent.ConcurrentHashMap

private val logger = KotlinLogging.logger {}

/**
 * WebRTC 피어 관리자.
 *
 * 여러 WebRTC 피어 연결의 생명주기를 관리합니다.
 *
 * **기능**:
 * - WebRTC 피어 생성, 시작, 중지
 * - SDP offer/answer 처리
 * - ICE candidate 관리
 * - 피어 통계 조회
 * - On-demand 스트림 자동 RTSP 시작
 *
 * @property streamManager 스트림 관리자
 * @property config WebRTC 설정
 * @property rtspManager RTSP 관리자 (on-demand 자동 시작)
 * @property streamsConfig 스트림 설정 (on-demand 확인)
 */
@Component
class WebRTCManager(
    private val streamManager: StreamManager,
    private val config: WebRTCConfig,
    private val rtspManager: RTSPManager,
    private val streamsConfig: StreamsConfig
) {
    private val peers = ConcurrentHashMap<String, WebRTCPeer>()

    /**
     * WebRTC 피어 생성.
     *
     * On-demand 스트림인 경우 RTSP 클라이언트를 자동으로 시작합니다.
     *
     * @param peerId 피어 식별자 (보통 WebSocket session ID)
     * @param streamId 스트림 식별자
     * @return WebRTCPeer
     * @throws ResourceLimitException 최대 피어 수 초과 시
     */
    fun createPeer(peerId: String, streamId: String): WebRTCPeer {
        // 기존 피어가 있으면 먼저 정리
        val existingPeer = peers.remove(peerId)
        if (existingPeer != null) {
            logger.warn { "Closing existing peer before creating new one: $peerId" }
            try {
                runBlocking { existingPeer.close() }
            } catch (e: Exception) {
                logger.error(e) { "Error closing existing peer: $peerId" }
            }
        }

        if (peers.size >= config.settings.maxPeers) {
            throw ResourceLimitException(
                resourceType = "peers",
                limit = config.settings.maxPeers,
                current = peers.size,
                message = "Cannot create more peers"
            )
        }

        logger.info { "Creating WebRTC peer: $peerId for stream: $streamId" }

        // On-demand 스트림인 경우 RTSP 자동 시작
        ensureRtspClientRunning(streamId)

        val peer = WebRTCPeer(peerId, streamId, streamManager, config)
        peers[peerId] = peer

        logger.info { "WebRTC peer created: $peerId (total: ${peers.size})" }
        return peer
    }

    /**
     * On-demand 스트림의 RTSP 클라이언트가 실행 중인지 확인하고, 아니면 시작합니다.
     *
     * @param streamId 스트림 식별자
     */
    private fun ensureRtspClientRunning(streamId: String) {
        val streamConfig = streamsConfig.paths[streamId]
        if (streamConfig == null) {
            logger.warn { "No stream config found for: $streamId (RTSP not started)" }
            return
        }

        // 이미 RTSP 클라이언트가 실행 중이면 스킵
        if (rtspManager.isClientRunning(streamId)) {
            logger.info { "RTSP client already running for: $streamId" }
            return
        }

        // On-demand 스트림이면 RTSP 시작
        if (streamConfig.sourceOnDemand) {
            logger.info { "Starting on-demand RTSP for stream: $streamId -> ${streamConfig.source}" }
            try {
                rtspManager.startClient(streamId, streamConfig.source)
                logger.info { "✅ On-demand RTSP started: $streamId" }
            } catch (e: Exception) {
                logger.error(e) { "Failed to start on-demand RTSP: $streamId" }
            }
        } else {
            logger.warn { "Stream $streamId is not on-demand but RTSP not running" }
        }
    }

    /**
     * WebRTC 피어 조회.
     *
     * @param peerId 피어 식별자
     * @return WebRTCPeer (없으면 null)
     */
    fun getPeer(peerId: String): WebRTCPeer? {
        return peers[peerId]
    }

    /**
     * WebRTC 피어 삭제.
     *
     * @param peerId 피어 식별자
     * @throws NotFoundException 피어가 존재하지 않을 경우
     */
    suspend fun removePeer(peerId: String) {
        val peer = peers.remove(peerId)
            ?: throw NotFoundException("WebRTC peer", peerId)

        logger.info { "Removing WebRTC peer: $peerId" }
        peer.close()

        logger.info { "WebRTC peer removed: $peerId (total: ${peers.size})" }
    }

    /**
     * 모든 WebRTC 피어 목록.
     *
     * @return 피어 ID 리스트
     */
    fun listPeers(): List<String> {
        return peers.keys().toList()
    }

    /**
     * 특정 스트림의 피어 목록.
     *
     * @param streamId 스트림 식별자
     * @return 피어 ID 리스트
     */
    fun listPeersByStream(streamId: String): List<String> {
        return peers.values
            .filter { it.getStats().streamId == streamId }
            .map { it.getStats().peerId }
    }

    /**
     * 활성 피어 개수.
     */
    fun getPeerCount(): Int {
        return peers.size
    }

    /**
     * 특정 피어의 통계 조회.
     *
     * @param peerId 피어 식별자
     * @return WebRTCPeerStats
     * @throws NotFoundException 피어가 존재하지 않을 경우
     */
    fun getPeerStats(peerId: String): WebRTCPeerStats {
        val peer = peers[peerId]
            ?: throw NotFoundException("WebRTC peer", peerId)
        return peer.getStats()
    }

    /**
     * 모든 피어 통계 조회.
     *
     * @return 피어 ID → 통계 맵
     */
    fun getAllPeerStats(): Map<String, WebRTCPeerStats> {
        return peers.mapValues { (_, peer) ->
            peer.getStats()
        }
    }

    /**
     * 모든 WebRTC 피어 중지.
     */
    suspend fun closeAllPeers() {
        logger.info { "Closing all WebRTC peers (${peers.size})" }

        peers.values.forEach { peer ->
            try {
                peer.close()
            } catch (e: Exception) {
                logger.error(e) { "Error closing WebRTC peer" }
            }
        }

        peers.clear()
        logger.info { "All WebRTC peers closed" }
    }

    /**
     * 특정 스트림의 모든 피어 중지.
     *
     * @param streamId 스트림 식별자
     */
    suspend fun closePeersByStream(streamId: String) {
        logger.info { "Closing all peers for stream: $streamId" }

        val peersToClose = peers.values
            .filter { it.getStats().streamId == streamId }
            .toList()

        peersToClose.forEach { peer ->
            try {
                val stats = peer.getStats()
                peers.remove(stats.peerId)
                peer.close()
            } catch (e: Exception) {
                logger.error(e) { "Error closing WebRTC peer" }
            }
        }

        logger.info { "Closed ${peersToClose.size} peers for stream: $streamId" }
    }
}
