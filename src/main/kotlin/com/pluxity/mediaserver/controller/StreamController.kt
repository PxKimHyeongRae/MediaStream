package com.pluxity.mediaserver.controller

import com.pluxity.mediaserver.domain.rtsp.RTSPManager
import com.pluxity.mediaserver.domain.stream.StreamManager
import io.github.oshai.kotlinlogging.KotlinLogging
import org.springframework.http.HttpStatus
import org.springframework.http.ResponseEntity
import org.springframework.web.bind.annotation.*

private val logger = KotlinLogging.logger {}

/**
 * 스트림 관리 REST API.
 *
 * **엔드포인트**:
 * - GET /api/v1/streams - 모든 스트림 목록
 * - GET /api/v1/streams/{id} - 특정 스트림 정보
 * - POST /api/v1/streams/{id}/start - RTSP 스트림 시작
 * - POST /api/v1/streams/{id}/stop - RTSP 스트림 중지
 * - GET /api/v1/streams/{id}/stats - 스트림 통계
 */
@RestController
@RequestMapping("/api/v1/streams")
class StreamController(
    private val streamManager: StreamManager,
    private val rtspManager: RTSPManager
) {

    /**
     * 모든 스트림 목록 조회.
     *
     * @return 스트림 ID 목록
     */
    @GetMapping
    fun listStreams(): ResponseEntity<Map<String, Any>> {
        val streamIds = streamManager.listStreamIds()
        val rtspClients = rtspManager.listClients()

        return ResponseEntity.ok(mapOf(
            "streams" to streamIds,
            "rtspClients" to rtspClients,
            "totalStreams" to streamIds.size,
            "activeRtspClients" to rtspClients.size
        ))
    }

    /**
     * 특정 스트림 정보 조회.
     *
     * @param id 스트림 ID
     * @return 스트림 정보
     */
    @GetMapping("/{id}")
    fun getStream(@PathVariable id: String): ResponseEntity<Map<String, Any>> {
        val stream = streamManager.getStream(id)
            ?: return ResponseEntity.notFound().build()

        val rtspClient = rtspManager.getClient(id)
        val stats = stream.getStats()

        return ResponseEntity.ok(mapOf(
            "id" to id,
            "subscriberCount" to stream.subscriberCount.value,
            "rtspConnected" to (rtspClient?.isRunning() ?: false),
            "stats" to mapOf(
                "packetsPublished" to stats.packetsPublished,
                "packetsDelivered" to stats.packetsDelivered,
                "bytesPublished" to stats.bytesPublished,
                "uptimeSeconds" to stats.uptimeSeconds,
                "avgBitrate" to stats.avgBitrate,
                "avgBitrateFormatted" to stats.avgBitrateFormatted,
                "deliveryRate" to stats.deliveryRate
            )
        ))
    }

    /**
     * RTSP 스트림 시작.
     *
     * @param id 스트림 ID
     * @param request 시작 요청 (RTSP URL 포함)
     * @return 시작 결과
     */
    @PostMapping("/{id}/start")
    fun startStream(
        @PathVariable id: String,
        @RequestBody request: StartStreamRequest
    ): ResponseEntity<Map<String, Any>> {
        logger.info { "Starting stream: $id, url: ${request.url}" }

        return try {
            val client = rtspManager.startClient(id, request.url)

            ResponseEntity.ok(mapOf(
                "status" to "started",
                "streamId" to id,
                "url" to request.url,
                "message" to "RTSP stream started successfully"
            ))
        } catch (e: Exception) {
            logger.error(e) { "Failed to start stream: $id" }
            ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR).body(mapOf(
                "status" to "error",
                "streamId" to id,
                "message" to (e.message ?: "Unknown error")
            ))
        }
    }

    /**
     * RTSP 스트림 중지.
     *
     * @param id 스트림 ID
     * @return 중지 결과
     */
    @PostMapping("/{id}/stop")
    fun stopStream(@PathVariable id: String): ResponseEntity<Map<String, Any>> {
        logger.info { "Stopping stream: $id" }

        return try {
            rtspManager.stopClient(id)

            ResponseEntity.ok(mapOf(
                "status" to "stopped",
                "streamId" to id,
                "message" to "RTSP stream stopped successfully"
            ))
        } catch (e: Exception) {
            logger.error(e) { "Failed to stop stream: $id" }
            ResponseEntity.status(HttpStatus.NOT_FOUND).body(mapOf(
                "status" to "error",
                "streamId" to id,
                "message" to (e.message ?: "Stream not found")
            ))
        }
    }

    /**
     * 스트림 통계 조회.
     *
     * @param id 스트림 ID
     * @return 스트림 통계
     */
    @GetMapping("/{id}/stats")
    fun getStreamStats(@PathVariable id: String): ResponseEntity<Map<String, Any>> {
        return try {
            val stats = streamManager.getStreamStats(id)

            ResponseEntity.ok(mapOf(
                "streamId" to id,
                "packetsPublished" to stats.packetsPublished,
                "packetsDelivered" to stats.packetsDelivered,
                "bytesPublished" to stats.bytesPublished,
                "uptimeSeconds" to stats.uptimeSeconds,
                "avgBitrate" to stats.avgBitrate,
                "avgBitrateFormatted" to stats.avgBitrateFormatted,
                "deliveryRate" to stats.deliveryRate
            ))
        } catch (e: Exception) {
            logger.error(e) { "Failed to get stream stats: $id" }
            ResponseEntity.notFound().build()
        }
    }

    /**
     * 모든 RTSP 클라이언트 중지.
     *
     * @return 중지 결과
     */
    @PostMapping("/stop-all")
    fun stopAllStreams(): ResponseEntity<Map<String, Any>> {
        logger.info { "Stopping all RTSP clients" }

        return try {
            val count = rtspManager.getClientCount()
            rtspManager.stopAllClients()

            ResponseEntity.ok(mapOf(
                "status" to "stopped",
                "message" to "All RTSP clients stopped",
                "count" to count
            ))
        } catch (e: Exception) {
            logger.error(e) { "Failed to stop all streams" }
            ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR).body(mapOf(
                "status" to "error",
                "message" to (e.message ?: "Unknown error")
            ))
        }
    }
}

/**
 * 스트림 시작 요청.
 *
 * @property url RTSP URL
 */
data class StartStreamRequest(
    val url: String
)
