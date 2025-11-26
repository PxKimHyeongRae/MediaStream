package com.pluxity.mediaserver.domain.stream

import com.pluxity.mediaserver.common.NotFoundException
import com.pluxity.mediaserver.common.StreamException
import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import org.springframework.stereotype.Component
import java.util.concurrent.ConcurrentHashMap

private val logger = KotlinLogging.logger {}

/**
 * 스트림 관리자.
 *
 * 모든 미디어 스트림의 생명주기를 관리합니다.
 * Go의 StreamManager를 Kotlin으로 포팅한 것입니다.
 *
 * **기능**:
 * - 스트림 생성/삭제
 * - 스트림 조회
 * - 스트림 목록
 * - 스트림 통계
 *
 * **Thread-safe**: ConcurrentHashMap 사용
 */
@Component
class StreamManager {

    // 활성 스트림 저장소 (Thread-safe)
    private val streams = ConcurrentHashMap<String, StreamFlow>()

    /**
     * 새로운 스트림 생성.
     *
     * @param id 스트림 식별자 (고유해야 함)
     * @return 생성된 StreamFlow
     * @throws StreamException 동일한 ID의 스트림이 이미 존재할 경우
     */
    fun createStream(id: String): StreamFlow {
        logger.info { "Creating stream: $id" }

        val existingStream = streams.putIfAbsent(id, StreamFlow(id))
        if (existingStream != null) {
            throw StreamException(id, "Stream already exists")
        }

        val stream = streams[id]!!
        logger.info { "Stream created: $id" }

        return stream
    }

    /**
     * 스트림 조회.
     *
     * @param id 스트림 식별자
     * @return StreamFlow
     * @throws NotFoundException 스트림이 존재하지 않을 경우
     */
    fun getStream(id: String): StreamFlow {
        return streams[id] ?: throw NotFoundException("stream", id)
    }

    /**
     * 스트림 존재 여부 확인.
     *
     * @param id 스트림 식별자
     * @return 존재 여부
     */
    fun hasStream(id: String): Boolean {
        return streams.containsKey(id)
    }

    /**
     * 스트림 삭제.
     *
     * @param id 스트림 식별자
     * @return 삭제된 StreamFlow (없으면 null)
     */
    suspend fun deleteStream(id: String): StreamFlow? {
        logger.info { "Deleting stream: $id" }

        val stream = streams.remove(id)
        if (stream != null) {
            stream.close()
            logger.info { "Stream deleted: $id" }
        } else {
            logger.warn { "Stream not found for deletion: $id" }
        }

        return stream
    }

    /**
     * 모든 스트림 ID 목록.
     *
     * @return 스트림 ID 리스트
     */
    fun listStreamIds(): List<String> {
        return streams.keys().toList()
    }

    /**
     * 모든 스트림 맵.
     *
     * @return 스트림 ID → StreamFlow 맵 (읽기 전용 복사본)
     */
    fun listStreams(): Map<String, StreamFlow> {
        return HashMap(streams)
    }

    /**
     * 스트림 조회 또는 생성.
     *
     * 스트림이 존재하지 않으면 새로 생성합니다.
     *
     * @param id 스트림 식별자
     * @return StreamFlow
     */
    fun getOrCreateStream(id: String): StreamFlow {
        return streams.computeIfAbsent(id) {
            logger.info { "Creating new stream: $id" }
            StreamFlow(id)
        }
    }

    /**
     * 활성 스트림 개수.
     */
    fun getStreamCount(): Int {
        return streams.size
    }

    /**
     * 특정 스트림의 통계.
     *
     * @param id 스트림 식별자
     * @return StreamStatsSnapshot
     * @throws NotFoundException 스트림이 존재하지 않을 경우
     */
    fun getStreamStats(id: String): StreamStatsSnapshot {
        val stream = getStream(id)
        return stream.getStats()
    }

    /**
     * 모든 스트림의 통계.
     *
     * @return 스트림 ID → 통계 맵
     */
    fun getAllStreamStats(): Map<String, StreamStatsSnapshot> {
        return streams.mapValues { (_, stream) ->
            stream.getStats()
        }
    }

    /**
     * 특정 스트림의 구독자 수.
     *
     * @param id 스트림 식별자
     * @return 구독자 수
     * @throws NotFoundException 스트림이 존재하지 않을 경우
     */
    fun getSubscriberCount(id: String): Int {
        val stream = getStream(id)
        return stream.subscriberCount.value
    }

    /**
     * RTP 패킷 발행.
     *
     * @param streamId 스트림 식별자
     * @param packet RTP 패킷
     * @throws NotFoundException 스트림이 존재하지 않을 경우
     */
    suspend fun publishPacket(streamId: String, packet: RTPPacket) {
        val stream = getStream(streamId)
        stream.publish(packet)
    }

    /**
     * 스트림 구독.
     *
     * 스트림이 존재하지 않으면 자동으로 생성합니다 (On-demand).
     * RTSP 연결이 완료되면 데이터가 흐르기 시작합니다.
     *
     * @param streamId 스트림 식별자
     * @param subscriberId 구독자 식별자
     * @param scope CoroutineScope (기본값: Dispatchers.IO)
     * @param handler 패킷 수신 핸들러
     * @return Job (취소하면 구독 해제)
     */
    fun subscribe(
        streamId: String,
        subscriberId: String,
        scope: CoroutineScope = CoroutineScope(Dispatchers.IO),
        handler: suspend (RTPPacket) -> Unit
    ): Job {
        // 스트림이 없으면 자동 생성 (On-demand)
        val stream = getOrCreateStream(streamId)
        return stream.subscribe(subscriberId, scope, handler)
    }

    /**
     * 모든 스트림 삭제 (정리).
     */
    suspend fun deleteAllStreams() {
        logger.info { "Deleting all streams (${streams.size})" }

        val streamIds = listStreamIds()
        for (id in streamIds) {
            deleteStream(id)
        }

        logger.info { "All streams deleted" }
    }

    /**
     * 스트림 관리자 상태 요약.
     */
    fun getSummary(): StreamManagerSummary {
        val totalStreams = streams.size
        val totalSubscribers = streams.values.sumOf { it.subscriberCount.value }

        val allStats = getAllStreamStats()
        val totalPackets = allStats.values.sumOf { it.packetsPublished }
        val totalBytes = allStats.values.sumOf { it.bytesPublished }

        return StreamManagerSummary(
            totalStreams = totalStreams,
            totalSubscribers = totalSubscribers,
            totalPacketsPublished = totalPackets,
            totalBytesPublished = totalBytes
        )
    }
}

/**
 * StreamManager 상태 요약.
 */
data class StreamManagerSummary(
    val totalStreams: Int,
    val totalSubscribers: Int,
    val totalPacketsPublished: Long,
    val totalBytesPublished: Long
) {
    val totalBytesFormatted: String
        get() = when {
            totalBytesPublished >= 1_000_000_000 -> "%.2f GB".format(totalBytesPublished / 1_000_000_000.0)
            totalBytesPublished >= 1_000_000 -> "%.2f MB".format(totalBytesPublished / 1_000_000.0)
            totalBytesPublished >= 1_000 -> "%.2f KB".format(totalBytesPublished / 1_000.0)
            else -> "$totalBytesPublished bytes"
        }
}
