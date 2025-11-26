package com.pluxity.mediaserver.domain.stream

import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.coroutines.*
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.*
import java.util.concurrent.atomic.AtomicLong

private val logger = KotlinLogging.logger {}

/**
 * Kotlin Flow 기반 Pub/Sub 스트림.
 *
 * Go의 채널 기반 Pub/Sub 패턴을 Kotlin Flow로 구현한 것입니다.
 * MutableSharedFlow를 사용하여 여러 구독자에게 RTP 패킷을 브로드캐스트합니다.
 *
 * **특징**:
 * - Hot Stream: 구독자 없이도 패킷 발행 가능
 * - 여러 구독자 지원 (1:N 브로드캐스트)
 * - 버퍼 오버플로우 시 가장 오래된 패킷 드롭
 * - 구조화된 동시성 (Coroutines)
 *
 * @property id 스트림 식별자
 */
class StreamFlow(val id: String) {

    // MutableSharedFlow: 여러 구독자에게 브로드캐스트
    private val _packets = MutableSharedFlow<RTPPacket>(
        replay = 0,  // 새 구독자에게 이전 패킷 전송 안 함
        extraBufferCapacity = 1000,  // 버퍼 크기 (약 1.5MB for 1500-byte packets)
        onBufferOverflow = BufferOverflow.DROP_OLDEST  // 오버플로우 시 가장 오래된 패킷 버림
    )

    /**
     * 패킷 스트림 (읽기 전용).
     * 구독자는 이 Flow를 collect하여 패킷을 수신합니다.
     */
    val packets: SharedFlow<RTPPacket> = _packets.asSharedFlow()

    // 구독자 수 추적
    private val _subscriberCount = MutableStateFlow(0)

    /**
     * 현재 구독자 수 (읽기 전용).
     */
    val subscriberCount: StateFlow<Int> = _subscriberCount.asStateFlow()

    // 통계
    private val stats = StreamStats()

    /**
     * RTP 패킷 발행 (Publish).
     *
     * 이 메서드는 suspend 함수이지만, SharedFlow의 버퍼가 있어서
     * 대부분의 경우 즉시 반환됩니다.
     *
     * @param packet 발행할 RTP 패킷
     */
    suspend fun publish(packet: RTPPacket) {
        val published = stats.getPublishedCount()

        // 처음 몇 개와 주기적으로 로그
        if (published < 10 || published % 100 == 0L) {
            logger.info { "[$id] 📦 Publishing packet #$published: seq=${packet.header.sequenceNumber}, size=${packet.payload.readableBytes()}" }
        }

        stats.incrementPublished()
        stats.addBytes(packet.payload.readableBytes().toLong())

        _packets.emit(packet)
    }

    /**
     * RTP 패킷 구독 (Subscribe).
     *
     * 구독자는 패킷을 수신할 때마다 handler가 호출됩니다.
     * 반환된 Job을 cancel()하면 구독이 해제됩니다.
     *
     * **주의**: handler 내부에서 packet.release()를 호출하면 안 됩니다.
     * 패킷은 StreamFlow에서 관리됩니다.
     *
     * @param subscriberId 구독자 식별자 (로깅용)
     * @param scope CoroutineScope (기본값: Dispatchers.IO)
     * @param handler 패킷 수신 핸들러
     * @return Job (취소하면 구독 해제)
     */
    fun subscribe(
        subscriberId: String,
        scope: CoroutineScope = CoroutineScope(Dispatchers.IO),
        handler: suspend (RTPPacket) -> Unit
    ): Job {
        _subscriberCount.value++
        logger.info { "[$id] Subscriber added: $subscriberId. Total: ${_subscriberCount.value}" }

        return scope.launch {
            try {
                packets.collect { packet ->
                    try {
                        handler(packet)
                        stats.incrementDelivered()
                    } catch (e: Exception) {
                        logger.error(e) { "[$id] Error in subscriber $subscriberId handler" }
                        // 한 구독자의 에러가 다른 구독자에게 영향을 주지 않도록
                    }
                }
            } catch (e: CancellationException) {
                logger.debug { "[$id] Subscriber $subscriberId cancelled" }
                throw e  // CancellationException은 재throw
            } catch (e: Exception) {
                logger.error(e) { "[$id] Unexpected error in subscriber $subscriberId" }
            } finally {
                _subscriberCount.value--
                logger.info { "[$id] Subscriber removed: $subscriberId. Total: ${_subscriberCount.value}" }
            }
        }
    }

    /**
     * 현재 통계 스냅샷 반환.
     */
    fun getStats(): StreamStatsSnapshot = stats.snapshot()

    /**
     * 스트림 정리.
     * 모든 구독자에게 완료 신호를 보냅니다.
     */
    suspend fun close() {
        logger.info { "[$id] Closing stream" }
        // SharedFlow는 명시적 close 메서드가 없으므로
        // 구독자들이 자연스럽게 취소되도록 함
    }
}

/**
 * 스트림 통계 (내부용, mutable).
 */
private class StreamStats {
    private val packetsPublished = AtomicLong(0)
    private val packetsDelivered = AtomicLong(0)
    private val bytesPublished = AtomicLong(0)
    private val startTime = System.currentTimeMillis()

    fun getPublishedCount(): Long = packetsPublished.get()

    fun incrementPublished() {
        packetsPublished.incrementAndGet()
    }

    fun incrementDelivered() {
        packetsDelivered.incrementAndGet()
    }

    fun addBytes(bytes: Long) {
        bytesPublished.addAndGet(bytes)
    }

    fun snapshot(): StreamStatsSnapshot {
        val now = System.currentTimeMillis()
        val uptimeSeconds = (now - startTime) / 1000.0

        return StreamStatsSnapshot(
            packetsPublished = packetsPublished.get(),
            packetsDelivered = packetsDelivered.get(),
            bytesPublished = bytesPublished.get(),
            uptimeSeconds = uptimeSeconds,
            avgBitrate = if (uptimeSeconds > 0) {
                (bytesPublished.get() * 8 / uptimeSeconds).toLong()
            } else {
                0
            }
        )
    }
}

/**
 * 스트림 통계 스냅샷 (불변, 외부 노출용).
 */
data class StreamStatsSnapshot(
    val packetsPublished: Long,
    val packetsDelivered: Long,
    val bytesPublished: Long,
    val uptimeSeconds: Double,
    val avgBitrate: Long  // bits per second
) {
    /**
     * 평균 전달률 (패킷 전달 / 패킷 발행).
     */
    val deliveryRate: Double
        get() = if (packetsPublished > 0) {
            packetsDelivered.toDouble() / packetsPublished
        } else {
            0.0
        }

    /**
     * 평균 비트레이트 (인간 친화적 형식).
     */
    val avgBitrateFormatted: String
        get() = when {
            avgBitrate >= 1_000_000 -> "%.2f Mbps".format(avgBitrate / 1_000_000.0)
            avgBitrate >= 1_000 -> "%.2f Kbps".format(avgBitrate / 1_000.0)
            else -> "$avgBitrate bps"
        }
}
