package com.pluxity.mediaserver.domain.stream

import com.pluxity.mediaserver.common.RTPHeader
import com.pluxity.mediaserver.common.readRTPHeader
import com.pluxity.mediaserver.common.toByteArray
import io.netty.buffer.ByteBuf
import io.netty.buffer.Unpooled

/**
 * RTP (Real-time Transport Protocol) 패킷 데이터 클래스.
 *
 * RTP 패킷은 미디어 스트림의 기본 전송 단위입니다.
 * Netty ByteBuf를 사용하여 Off-heap 메모리로 관리됩니다.
 *
 * **중요**: 이 클래스는 ByteBuf를 포함하므로 반드시 release()를 호출하거나
 * use {} 블록을 사용해야 합니다.
 *
 * @property streamId 스트림 식별자
 * @property header RTP 헤더 정보
 * @property payload RTP 페이로드 (미디어 데이터)
 */
data class RTPPacket(
    val streamId: String,
    val header: RTPHeader,
    val payload: ByteBuf
) {
    /**
     * 패킷의 총 크기 (헤더 + 페이로드)
     */
    val totalSize: Int
        get() = RTPHeader.FIXED_HEADER_SIZE + (header.csrcCount * 4) + payload.readableBytes()

    /**
     * ByteBuf 리소스 해제.
     * 반드시 호출해야 메모리 누수를 방지할 수 있습니다.
     */
    fun release() {
        if (payload.refCnt() > 0) {
            payload.release()
        }
    }

    /**
     * 전체 RTP 패킷을 ByteArray로 직렬화 (헤더 + 페이로드).
     *
     * SRTP 암호화 시 전체 RTP 패킷이 필요합니다.
     *
     * @return 직렬화된 RTP 패킷 (12바이트 헤더 + 페이로드)
     */
    fun toByteArray(): ByteArray {
        val headerSize = RTPHeader.FIXED_HEADER_SIZE + (header.csrcCount * 4)
        val payloadSize = payload.readableBytes()
        val result = ByteArray(headerSize + payloadSize)

        // RTP 헤더 직렬화 (12 bytes 고정 + CSRC)
        // Byte 0: V=2, P, X, CC
        result[0] = ((header.version shl 6) or
                (if (header.padding) 0x20 else 0) or
                (if (header.extension) 0x10 else 0) or
                (header.csrcCount and 0x0F)).toByte()

        // Byte 1: M, PT
        result[1] = ((if (header.marker) 0x80 else 0) or
                (header.payloadType and 0x7F)).toByte()

        // Bytes 2-3: Sequence Number (big-endian)
        result[2] = ((header.sequenceNumber shr 8) and 0xFF).toByte()
        result[3] = (header.sequenceNumber and 0xFF).toByte()

        // Bytes 4-7: Timestamp (big-endian)
        result[4] = ((header.timestamp shr 24) and 0xFF).toByte()
        result[5] = ((header.timestamp shr 16) and 0xFF).toByte()
        result[6] = ((header.timestamp shr 8) and 0xFF).toByte()
        result[7] = (header.timestamp and 0xFF).toByte()

        // Bytes 8-11: SSRC (big-endian)
        result[8] = ((header.ssrc shr 24) and 0xFF).toByte()
        result[9] = ((header.ssrc shr 16) and 0xFF).toByte()
        result[10] = ((header.ssrc shr 8) and 0xFF).toByte()
        result[11] = (header.ssrc and 0xFF).toByte()

        // 페이로드 복사
        payload.markReaderIndex()
        payload.readBytes(result, headerSize, payloadSize)
        payload.resetReaderIndex()

        return result
    }

    /**
     * 전체 RTP 패킷을 ByteBuf로 직렬화 (헤더 + 페이로드).
     *
     * @return 직렬화된 RTP 패킷 ByteBuf (호출자가 release 책임)
     */
    fun toByteBuf(): ByteBuf {
        return Unpooled.wrappedBuffer(toByteArray())
    }

    /**
     * ByteBuf를 복사하여 새로운 RTPPacket 생성.
     * 원본 패킷과 독립적인 생명주기를 가집니다.
     */
    fun copy(): RTPPacket {
        val copiedPayload = Unpooled.buffer(payload.readableBytes())
        payload.getBytes(payload.readerIndex(), copiedPayload)
        copiedPayload.writerIndex(payload.readableBytes())

        return RTPPacket(
            streamId = streamId,
            header = header.copy(),
            payload = copiedPayload
        )
    }

    companion object {
        /**
         * ByteArray로부터 RTPPacket 생성.
         *
         * @param streamId 스트림 식별자
         * @param data RTP 패킷 데이터 (헤더 + 페이로드)
         * @return 파싱된 RTPPacket
         * @throws IllegalArgumentException 데이터가 최소 헤더 크기보다 작을 경우
         */
        fun fromByteArray(streamId: String, data: ByteArray): RTPPacket {
            require(data.size >= RTPHeader.FIXED_HEADER_SIZE) {
                "Data size (${data.size}) is less than minimum RTP header size (${RTPHeader.FIXED_HEADER_SIZE})"
            }

            val buffer = Unpooled.wrappedBuffer(data)
            return try {
                fromByteBuf(streamId, buffer)
            } finally {
                // wrappedBuffer는 release 불필요 (참조만)
            }
        }

        /**
         * ByteBuf로부터 RTPPacket 생성.
         *
         * @param streamId 스트림 식별자
         * @param buffer RTP 패킷 데이터를 포함한 ByteBuf
         * @return 파싱된 RTPPacket
         * @throws IllegalArgumentException 버퍼가 최소 헤더 크기보다 작을 경우
         *
         * **주의**: 이 메서드는 버퍼의 readerIndex부터 읽습니다.
         * 헤더를 읽은 후 readerIndex가 페이로드 시작 위치로 이동합니다.
         */
        fun fromByteBuf(streamId: String, buffer: ByteBuf): RTPPacket {
            require(buffer.readableBytes() >= RTPHeader.FIXED_HEADER_SIZE) {
                "Buffer has ${buffer.readableBytes()} bytes, need at least ${RTPHeader.FIXED_HEADER_SIZE}"
            }

            // RTP 헤더 파싱
            val header = buffer.readRTPHeader()

            // CSRC 식별자 건너뛰기 (필요 시 나중에 파싱 가능)
            if (header.csrcCount > 0) {
                buffer.skipBytes(header.csrcCount * 4)
            }

            // 나머지가 페이로드
            val payloadSize = buffer.readableBytes()
            val payload = buffer.readRetainedSlice(payloadSize)

            return RTPPacket(
                streamId = streamId,
                header = header,
                payload = payload
            )
        }

        /**
         * ByteBuf로 RTPPacket 생성 (RTSP Client용).
         *
         * @param streamId 스트림 식별자
         * @param payloadType RTP payload type
         * @param sequenceNumber 시퀀스 번호
         * @param timestamp 타임스탬프
         * @param ssrc SSRC
         * @param marker Marker bit
         * @param payloadData Payload ByteBuf (호출자가 release 책임)
         */
        fun create(
            streamId: String,
            payloadType: Int,
            sequenceNumber: Int,
            timestamp: Long,
            ssrc: Long,
            marker: Boolean = false,
            payloadData: ByteBuf
        ): RTPPacket {
            val header = RTPHeader(
                version = 2,
                padding = false,
                extension = false,
                csrcCount = 0,
                marker = marker,
                payloadType = payloadType,
                sequenceNumber = sequenceNumber,
                timestamp = timestamp,
                ssrc = ssrc.toInt()
            )

            return RTPPacket(
                streamId = streamId,
                header = header,
                payload = payloadData
            )
        }

        /**
         * 테스트용 RTPPacket 생성 헬퍼.
         *
         * @param streamId 스트림 식별자
         * @param sequenceNumber 시퀀스 번호
         * @param timestamp 타임스탬프
         * @param payloadData 페이로드 데이터
         */
        fun create(
            streamId: String,
            sequenceNumber: Int,
            timestamp: Long,
            payloadData: ByteArray,
            payloadType: Int = 96,
            ssrc: Int = 0x12345678,
            marker: Boolean = false
        ): RTPPacket {
            val header = RTPHeader(
                version = 2,
                padding = false,
                extension = false,
                csrcCount = 0,
                marker = marker,
                payloadType = payloadType,
                sequenceNumber = sequenceNumber,
                timestamp = timestamp,
                ssrc = ssrc
            )

            val payload = Unpooled.wrappedBuffer(payloadData)

            return RTPPacket(
                streamId = streamId,
                header = header,
                payload = payload
            )
        }
    }

    override fun toString(): String {
        return "RTPPacket(streamId='$streamId', seq=${header.sequenceNumber}, " +
                "ts=${header.timestamp}, pt=${header.payloadType}, " +
                "marker=${header.marker}, payloadSize=${payload.readableBytes()})"
    }

    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (other !is RTPPacket) return false

        if (streamId != other.streamId) return false
        if (header != other.header) return false
        // ByteBuf는 내용 비교
        if (payload.readableBytes() != other.payload.readableBytes()) return false

        val thisBytes = payload.toByteArray()
        val otherBytes = other.payload.toByteArray()

        return thisBytes.contentEquals(otherBytes)
    }

    override fun hashCode(): Int {
        var result = streamId.hashCode()
        result = 31 * result + header.hashCode()
        result = 31 * result + payload.toByteArray().contentHashCode()
        return result
    }
}
