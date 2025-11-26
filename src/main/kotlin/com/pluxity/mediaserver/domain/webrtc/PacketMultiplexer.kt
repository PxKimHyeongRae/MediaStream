package com.pluxity.mediaserver.domain.webrtc

import io.github.oshai.kotlinlogging.KotlinLogging
import org.ice4j.socket.DatagramPacketFilter
import java.net.DatagramPacket
import java.util.concurrent.BlockingQueue
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit

private val logger = KotlinLogging.logger {}

/**
 * WebRTC 패킷 타입 분류.
 *
 * RFC 5764 - Multiplexing RTP Data and STUN over DTLS
 * 첫 바이트로 패킷 타입 구분:
 * - 0~1: STUN
 * - 20~63: DTLS
 * - 128~191: RTP/RTCP
 */
enum class PacketType {
    STUN,
    DTLS,
    RTP_RTCP,
    UNKNOWN
}

/**
 * 패킷 타입 판별 유틸리티.
 */
object PacketClassifier {

    /**
     * 바이트 배열에서 패킷 타입 판별.
     */
    fun classify(data: ByteArray, offset: Int = 0, length: Int = data.size): PacketType {
        if (length <= 0) return PacketType.UNKNOWN

        val firstByte = data[offset].toInt() and 0xFF

        return when {
            // STUN: 0x00-0x01 (첫 두 비트가 00)
            firstByte <= 3 -> PacketType.STUN

            // DTLS: 20-63 (Content Type: 20=ChangeCipherSpec, 21=Alert, 22=Handshake, 23=Application)
            firstByte in 20..63 -> PacketType.DTLS

            // RTP/RTCP: 128-191 (첫 두 비트가 10)
            firstByte in 128..191 -> PacketType.RTP_RTCP

            else -> PacketType.UNKNOWN
        }
    }

    /**
     * DatagramPacket에서 패킷 타입 판별.
     */
    fun classify(packet: DatagramPacket): PacketType {
        return classify(packet.data, packet.offset, packet.length)
    }

    /**
     * DTLS 패킷인지 확인.
     */
    fun isDTLS(data: ByteArray, offset: Int = 0, length: Int = data.size): Boolean {
        return classify(data, offset, length) == PacketType.DTLS
    }

    /**
     * RTP/RTCP 패킷인지 확인.
     */
    fun isRTP(data: ByteArray, offset: Int = 0, length: Int = data.size): Boolean {
        return classify(data, offset, length) == PacketType.RTP_RTCP
    }

    /**
     * STUN 패킷인지 확인.
     */
    fun isSTUN(data: ByteArray, offset: Int = 0, length: Int = data.size): Boolean {
        return classify(data, offset, length) == PacketType.STUN
    }
}

/**
 * DTLS 패킷 필터 (ice4j DatagramPacketFilter 구현).
 *
 * MultiplexingDatagramSocket.getSocket()에 전달하여
 * DTLS 패킷만 수신하는 소켓을 생성합니다.
 */
class DTLSPacketFilter : DatagramPacketFilter {
    override fun accept(packet: DatagramPacket): Boolean {
        return PacketClassifier.classify(packet) == PacketType.DTLS
    }
}

/**
 * RTP/RTCP 패킷 필터.
 */
class RTPPacketFilter : DatagramPacketFilter {
    override fun accept(packet: DatagramPacket): Boolean {
        return PacketClassifier.classify(packet) == PacketType.RTP_RTCP
    }
}

/**
 * 패킷 Multiplexer.
 *
 * 수신된 UDP 패킷을 타입별로 분류하여 적절한 큐로 라우팅합니다.
 * - STUN: ICE Agent가 처리 (ice4j 내부)
 * - DTLS: DTLSHandler로 전달
 * - RTP/RTCP: SRTPTransformer로 전달
 *
 * @property streamId 스트림 식별자
 */
class PacketMultiplexer(
    private val streamId: String
) : AutoCloseable {

    // 패킷 큐
    private val dtlsQueue: BlockingQueue<DatagramPacket> = LinkedBlockingQueue(100)
    private val rtpQueue: BlockingQueue<DatagramPacket> = LinkedBlockingQueue(1000)

    private var running = true

    // 통계
    private var stunPackets = 0L
    private var dtlsPackets = 0L
    private var rtpPackets = 0L
    private var unknownPackets = 0L

    /**
     * 패킷 분류 및 큐에 추가.
     *
     * @param packet 수신된 UDP 패킷
     * @return 패킷 타입
     */
    fun demultiplex(packet: DatagramPacket): PacketType {
        val type = PacketClassifier.classify(packet)

        when (type) {
            PacketType.STUN -> {
                stunPackets++
                // STUN은 ice4j가 직접 처리하므로 여기서는 카운트만
                logger.trace { "[$streamId] STUN packet (${packet.length} bytes)" }
            }
            PacketType.DTLS -> {
                dtlsPackets++
                // 패킷 복사하여 큐에 추가 (원본은 재사용될 수 있음)
                val copy = copyPacket(packet)
                if (!dtlsQueue.offer(copy)) {
                    logger.warn { "[$streamId] DTLS queue full, dropping packet" }
                }
                logger.trace { "[$streamId] DTLS packet (${packet.length} bytes)" }
            }
            PacketType.RTP_RTCP -> {
                rtpPackets++
                val copy = copyPacket(packet)
                if (!rtpQueue.offer(copy)) {
                    logger.warn { "[$streamId] RTP queue full, dropping packet" }
                }
                logger.trace { "[$streamId] RTP/RTCP packet (${packet.length} bytes)" }
            }
            PacketType.UNKNOWN -> {
                unknownPackets++
                logger.debug { "[$streamId] Unknown packet type (first byte: ${packet.data[packet.offset].toInt() and 0xFF})" }
            }
        }

        return type
    }

    /**
     * DTLS 패킷 수신 (블로킹).
     *
     * @param timeoutMs 타임아웃 (밀리초), 0이면 무한 대기
     * @return DTLS 패킷 또는 null (타임아웃)
     */
    fun receiveDTLS(timeoutMs: Long = 0): DatagramPacket? {
        return if (timeoutMs > 0) {
            dtlsQueue.poll(timeoutMs, TimeUnit.MILLISECONDS)
        } else {
            dtlsQueue.take()
        }
    }

    /**
     * RTP/RTCP 패킷 수신 (논블로킹).
     *
     * @return RTP/RTCP 패킷 또는 null
     */
    fun receiveRTP(): DatagramPacket? {
        return rtpQueue.poll()
    }

    /**
     * DTLS 큐에 대기 중인 패킷 수.
     */
    fun dtlsQueueSize(): Int = dtlsQueue.size

    /**
     * RTP 큐에 대기 중인 패킷 수.
     */
    fun rtpQueueSize(): Int = rtpQueue.size

    /**
     * 패킷 복사.
     */
    private fun copyPacket(packet: DatagramPacket): DatagramPacket {
        val data = ByteArray(packet.length)
        System.arraycopy(packet.data, packet.offset, data, 0, packet.length)
        return DatagramPacket(data, data.size, packet.socketAddress)
    }

    /**
     * 통계 조회.
     */
    fun getStats(): MultiplexerStats {
        return MultiplexerStats(
            streamId = streamId,
            stunPackets = stunPackets,
            dtlsPackets = dtlsPackets,
            rtpPackets = rtpPackets,
            unknownPackets = unknownPackets,
            dtlsQueueSize = dtlsQueue.size,
            rtpQueueSize = rtpQueue.size
        )
    }

    override fun close() {
        running = false
        dtlsQueue.clear()
        rtpQueue.clear()
    }
}

/**
 * Multiplexer 통계.
 */
data class MultiplexerStats(
    val streamId: String,
    val stunPackets: Long,
    val dtlsPackets: Long,
    val rtpPackets: Long,
    val unknownPackets: Long,
    val dtlsQueueSize: Int,
    val rtpQueueSize: Int
)
