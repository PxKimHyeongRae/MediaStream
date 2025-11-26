package com.pluxity.mediaserver.domain.webrtc

import com.pluxity.mediaserver.domain.stream.RTPPacket
import io.github.oshai.kotlinlogging.KotlinLogging
import io.netty.buffer.ByteBuf
import io.netty.buffer.PooledByteBufAllocator
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicLong

private val logger = KotlinLogging.logger {}

/**
 * RTP Repacketizer: RTSP RTP → WebRTC RTP 변환 + H.264 FU-A Fragmentation.
 *
 * RTSP 스트림에서 받은 큰 NAL unit을 RTP MTU에 맞게 분할합니다.
 *
 * **H.264 RTP Packetization (RFC 6184)**:
 * - NAL Unit ≤ MTU: Single NAL Unit Packet (Type 1-23)
 * - NAL Unit > MTU: Fragmentation Unit A (Type 28)
 *
 * **FU-A 포맷**:
 * ```
 * +---------------+---------------+---------------+
 * | FU indicator  | FU header     | FU payload    |
 * +---------------+---------------+---------------+
 *
 * FU indicator (1 byte):
 *   F|NRI|  Type  |  (Type = 28 for FU-A)
 *
 * FU header (1 byte):
 *   S|E|R|  Type  |  (S=Start, E=End, R=Reserved, Type=original NAL type)
 * ```
 *
 * @property webrtcSSRC WebRTC 세션용 SSRC
 * @property payloadTypeMapping RTSP PT → WebRTC PT 매핑
 * @property maxPayloadSize RTP 페이로드 최대 크기 (기본 1200, MTU 고려)
 */
class RTPRepacketizer(
    private val webrtcSSRC: Long,
    private val payloadTypeMapping: Map<Int, Int> = emptyMap(),
    private val maxPayloadSize: Int = 1200  // MTU - RTP header - SRTP overhead
) {
    private val allocator = PooledByteBufAllocator.DEFAULT

    // WebRTC Sequence Number (0부터 시작, 연속성 보장)
    private val webrtcSeq = AtomicInteger(0)

    // Timestamp 오프셋 (첫 패킷의 TS를 0으로 맞추기 위해)
    private val timestampOffset = AtomicLong(-1)

    // 동적으로 설정 가능한 WebRTC Payload Type (브라우저의 SDP에서 파싱)
    // -1이면 payloadTypeMapping 사용, 그 외에는 이 값을 H.264 PT로 사용
    @Volatile
    private var dynamicWebRTCPayloadType: Int = -1

    // SPS/PPS 버퍼링 (Chrome 호환성을 위해 IDR과 동일 타임스탬프로 전송)
    @Volatile
    private var bufferedSPS: ByteArray? = null
    @Volatile
    private var bufferedPPS: ByteArray? = null

    /**
     * WebRTC Payload Type을 동적으로 설정.
     *
     * 브라우저의 SDP Offer에서 파싱한 H.264 PT를 사용합니다.
     * 이 설정은 payloadTypeMapping보다 우선합니다.
     *
     * @param payloadType H.264 Payload Type (예: 102, 111 등)
     */
    fun setWebRTCPayloadType(payloadType: Int) {
        dynamicWebRTCPayloadType = payloadType
        logger.info { "[Repacketizer] Dynamic WebRTC PT set to $payloadType" }
    }

    companion object {
        // H.264 NAL Unit Types
        const val NAL_TYPE_SLICE = 1
        const val NAL_TYPE_DPA = 2
        const val NAL_TYPE_DPB = 3
        const val NAL_TYPE_DPC = 4
        const val NAL_TYPE_IDR = 5
        const val NAL_TYPE_SEI = 6
        const val NAL_TYPE_SPS = 7
        const val NAL_TYPE_PPS = 8
        const val NAL_TYPE_AUD = 9
        const val NAL_TYPE_STAP_A = 24
        const val NAL_TYPE_STAP_B = 25
        const val NAL_TYPE_MTAP16 = 26
        const val NAL_TYPE_MTAP24 = 27
        const val NAL_TYPE_FU_A = 28
        const val NAL_TYPE_FU_B = 29

        // Start codes
        val START_CODE_3 = byteArrayOf(0x00, 0x00, 0x01)
        val START_CODE_4 = byteArrayOf(0x00, 0x00, 0x00, 0x01)
    }

    /**
     * RTSP RTP 패킷 → WebRTC RTP 패킷 변환.
     *
     * H.264 데이터의 경우 NAL unit 크기에 따라 분할합니다.
     *
     * @param rtspPacket RTSP에서 받은 RTP 패킷 (전체 프레임 포함)
     * @return WebRTC용으로 분할된 RTP 패킷 리스트
     */
    fun repacketize(rtspPacket: RTPPacket): List<RTPPacket> {
        val rtspHeader = rtspPacket.header

        // 1. Timestamp 오프셋 초기화 (첫 패킷)
        if (timestampOffset.get() == -1L) {
            timestampOffset.set(rtspHeader.timestamp)
            logger.info { "[Repacketizer] First packet, timestamp offset: ${rtspHeader.timestamp}" }
        }

        // 2. Timestamp 조정 (오프셋 적용 + 32비트 랩어라운드)
        // RTP timestamp는 32비트 unsigned이므로 0xFFFFFFFF 마스킹
        val rawTimestamp = rtspHeader.timestamp - timestampOffset.get()
        val adjustedTimestamp = rawTimestamp and 0xFFFFFFFFL  // 32-bit unsigned

        // 3. Payload Type 매핑 (동적 PT 우선)
        val webrtcPayloadType = if (dynamicWebRTCPayloadType >= 0) {
            dynamicWebRTCPayloadType  // 브라우저에서 협상된 PT 우선 사용
        } else {
            payloadTypeMapping[rtspHeader.payloadType] ?: rtspHeader.payloadType
        }

        // 4. H.264 데이터 파싱 및 분할
        val payloadData = rtspPacket.payload
        val payloadSize = payloadData.readableBytes()

        // 데이터를 ByteArray로 복사
        val data = ByteArray(payloadSize)
        payloadData.markReaderIndex()
        payloadData.readBytes(data)
        payloadData.resetReaderIndex()

        // 5. NAL units 파싱 (Annex B 포맷)
        // 디버깅: 원본 데이터의 첫 16바이트 출력
        if (webrtcSeq.get() < 10) {
            val hexDump = data.take(32).joinToString(" ") { "%02x".format(it) }
            logger.info { "[Repacketizer] RAW DATA first 32 bytes: $hexDump" }
        }

        val nalUnits = parseNalUnits(data)

        if (nalUnits.isEmpty()) {
            // Annex B가 아닐 수 있음 - 원본 데이터를 그대로 NAL unit으로 사용 시도
            if (data.isNotEmpty()) {
                val nalType = data[0].toInt() and 0x1F
                logger.warn { "[Repacketizer] No start code found, raw NAL type=$nalType, size=${data.size}" }
                // Start code 없이 원본 데이터가 NAL unit 자체일 수 있음
                // 이 경우 data를 직접 NAL unit으로 사용
            }
            logger.warn { "[Repacketizer] No NAL units found in packet" }
            return emptyList()
        }

        // 6. 각 NAL unit을 RTP 패킷으로 변환 (SPS/PPS 버퍼링 적용)
        val packets = mutableListOf<RTPPacket>()

        // NAL units 분류: SPS, PPS, IDR, 기타
        val spsNals = mutableListOf<ByteArray>()
        val ppsNals = mutableListOf<ByteArray>()
        val idrNals = mutableListOf<ByteArray>()
        val otherNals = mutableListOf<ByteArray>()

        nalUnits.forEach { nalUnit ->
            val nalType = nalUnit[0].toInt() and 0x1F
            when (nalType) {
                NAL_TYPE_SPS -> {
                    spsNals.add(nalUnit)
                    bufferedSPS = nalUnit.copyOf()  // 항상 최신 SPS 저장
                    logger.info { "[Repacketizer] SPS buffered: ${nalUnit.size} bytes" }
                }
                NAL_TYPE_PPS -> {
                    ppsNals.add(nalUnit)
                    bufferedPPS = nalUnit.copyOf()  // 항상 최신 PPS 저장
                    logger.info { "[Repacketizer] PPS buffered: ${nalUnit.size} bytes" }
                }
                NAL_TYPE_IDR -> idrNals.add(nalUnit)
                NAL_TYPE_SEI, NAL_TYPE_AUD -> {} // SEI, AUD는 스킵 (디코딩에 불필요)
                else -> otherNals.add(nalUnit)
            }
        }

        // IDR이 있는 경우: STAP-A(SPS+PPS) → IDR 순서로 **동일 타임스탬프**로 전송
        if (idrNals.isNotEmpty()) {
            val sps = bufferedSPS
            val pps = bufferedPPS

            if (sps != null && pps != null) {
                logger.info { "[Repacketizer] IDR detected! Sending STAP-A(SPS+PPS) + IDR with timestamp=$adjustedTimestamp" }

                // STAP-A 패킷 생성 (SPS + PPS를 하나의 패킷으로 묶음)
                val stapAPacket = createStapAPacket(
                    nalUnits = listOf(sps, pps),
                    timestamp = adjustedTimestamp,
                    payloadType = webrtcPayloadType,
                    streamId = rtspPacket.streamId,
                    isLastNalUnitInFrame = false  // STAP-A 뒤에 IDR이 있으므로 marker=false
                )
                packets.add(stapAPacket)

                // IDR 전송 (marker=true) - 마지막 IDR만 marker 설정
                idrNals.forEachIndexed { index, idrNal ->
                    val isLastIdr = (index == idrNals.size - 1) && otherNals.isEmpty()
                    packets.addAll(packetizeNalUnit(
                        nalUnit = idrNal,
                        timestamp = adjustedTimestamp,
                        payloadType = webrtcPayloadType,
                        streamId = rtspPacket.streamId,
                        isLastNalUnitInFrame = isLastIdr
                    ))
                }
            } else {
                logger.warn { "[Repacketizer] IDR found but SPS/PPS not buffered yet! sps=${sps != null}, pps=${pps != null}" }
                // SPS/PPS가 없으면 IDR만 전송 (디코딩 실패 예상)
                idrNals.forEachIndexed { index, idrNal ->
                    val isLast = (index == idrNals.size - 1) && otherNals.isEmpty()
                    packets.addAll(packetizeNalUnit(
                        nalUnit = idrNal,
                        timestamp = adjustedTimestamp,
                        payloadType = webrtcPayloadType,
                        streamId = rtspPacket.streamId,
                        isLastNalUnitInFrame = isLast
                    ))
                }
            }
        }

        // 일반 slice NAL units 전송 (non-IDR frames)
        otherNals.forEachIndexed { index, nalUnit ->
            val isLast = (index == otherNals.size - 1)

            // NAL type 로깅 (처음 몇 프레임만)
            if (webrtcSeq.get() < 200) {
                val nalType = nalUnit[0].toInt() and 0x1F
                val nalTypeName = when (nalType) {
                    NAL_TYPE_SLICE -> "SLICE"
                    else -> "TYPE_$nalType"
                }
                logger.info { "[Repacketizer] NAL unit: type=$nalTypeName($nalType), size=${nalUnit.size}" }
            }

            packets.addAll(packetizeNalUnit(
                nalUnit = nalUnit,
                timestamp = adjustedTimestamp,
                payloadType = webrtcPayloadType,
                streamId = rtspPacket.streamId,
                isLastNalUnitInFrame = isLast
            ))
        }

        // SPS/PPS만 있고 IDR이 없는 프레임은 버퍼링만 하고 패킷 생성하지 않음
        if (idrNals.isEmpty() && otherNals.isEmpty() && (spsNals.isNotEmpty() || ppsNals.isNotEmpty())) {
            logger.info { "[Repacketizer] SPS/PPS only frame - buffered, no packets sent" }
            return emptyList()
        }

        if (packets.isNotEmpty()) {
            val seqRange = if (packets.size > 1) {
                "${packets.first().header.sequenceNumber}-${packets.last().header.sequenceNumber}"
            } else {
                "${packets.first().header.sequenceNumber}"
            }
            val markerPacket = packets.lastOrNull { it.header.marker }
            logger.info {
                "[Repacketizer] Frame: ${nalUnits.size} NALs → ${packets.size} pkts, " +
                "seq=$seqRange, ts=$adjustedTimestamp, marker=${markerPacket?.header?.sequenceNumber ?: "NONE"}"
            }
        }

        return packets
    }

    /**
     * Annex B 포맷에서 NAL units 파싱.
     *
     * Start code (0x000001 또는 0x00000001)로 구분된 NAL units를 추출합니다.
     */
    private fun parseNalUnits(data: ByteArray): List<ByteArray> {
        val nalUnits = mutableListOf<ByteArray>()
        var i = 0

        while (i < data.size - 3) {
            // Start code 찾기
            val startCodeLength = when {
                i < data.size - 3 && data[i] == 0.toByte() && data[i + 1] == 0.toByte() && data[i + 2] == 1.toByte() -> 3
                i < data.size - 4 && data[i] == 0.toByte() && data[i + 1] == 0.toByte() && data[i + 2] == 0.toByte() && data[i + 3] == 1.toByte() -> 4
                else -> {
                    i++
                    continue
                }
            }

            val nalStart = i + startCodeLength

            // 다음 start code 찾기
            var nalEnd = data.size
            var j = nalStart + 1
            while (j < data.size - 2) {
                if (data[j] == 0.toByte() && data[j + 1] == 0.toByte()) {
                    if (j + 2 < data.size && data[j + 2] == 1.toByte()) {
                        nalEnd = j
                        break
                    }
                    if (j + 3 < data.size && data[j + 2] == 0.toByte() && data[j + 3] == 1.toByte()) {
                        nalEnd = j
                        break
                    }
                }
                j++
            }

            if (nalStart < nalEnd) {
                nalUnits.add(data.copyOfRange(nalStart, nalEnd))
            }

            i = nalEnd
        }

        return nalUnits
    }

    /**
     * 단일 NAL unit을 RTP 패킷(들)로 변환.
     *
     * NAL unit 크기에 따라:
     * - 작은 NAL: Single NAL Unit Packet
     * - 큰 NAL: FU-A Fragmentation
     */
    private fun packetizeNalUnit(
        nalUnit: ByteArray,
        timestamp: Long,
        payloadType: Int,
        streamId: String,
        isLastNalUnitInFrame: Boolean
    ): List<RTPPacket> {
        return if (nalUnit.size <= maxPayloadSize) {
            // Single NAL Unit Packet
            listOf(createSingleNalPacket(nalUnit, timestamp, payloadType, streamId, isLastNalUnitInFrame))
        } else {
            // FU-A Fragmentation
            createFuAPackets(nalUnit, timestamp, payloadType, streamId, isLastNalUnitInFrame)
        }
    }

    /**
     * Single NAL Unit Packet 생성.
     *
     * NAL unit이 MTU보다 작은 경우 그대로 전송.
     */
    private fun createSingleNalPacket(
        nalUnit: ByteArray,
        timestamp: Long,
        payloadType: Int,
        streamId: String,
        isLastNalUnitInFrame: Boolean
    ): RTPPacket {
        val buffer = allocator.buffer(nalUnit.size)
        buffer.writeBytes(nalUnit)

        val seqNum = webrtcSeq.getAndIncrement() and 0xFFFF

        return RTPPacket.create(
            streamId = streamId,
            payloadType = payloadType,
            sequenceNumber = seqNum,
            timestamp = timestamp,
            ssrc = webrtcSSRC,
            marker = isLastNalUnitInFrame,  // 프레임의 마지막 NAL이면 marker=1
            payloadData = buffer
        )
    }

    /**
     * STAP-A (Single-Time Aggregation Packet) 생성.
     *
     * 여러 작은 NAL unit을 하나의 RTP 패킷으로 묶습니다.
     * Chrome WebRTC에서 SPS/PPS를 IDR과 함께 받아야 디코딩 가능.
     *
     * **RFC 6184 Section 5.7.1 STAP-A 포맷**:
     * ```
     * +---------------+---------------+---------------+---------------+
     * | STAP-A NAL HDR|  NALU 1 Size  |   NALU 1 HDR  |  NALU 1 Data  |
     * +---------------+---------------+---------------+---------------+
     * |  NALU 2 Size  |   NALU 2 HDR  |  NALU 2 Data  |     ...       |
     * +---------------+---------------+---------------+---------------+
     *
     * STAP-A NAL HDR (1 byte): F=0, NRI=max(NRI of NALUs), Type=24
     * NALU Size (2 bytes): Big-endian size of each NAL unit
     * ```
     *
     * @param nalUnits 묶을 NAL unit 리스트 (SPS, PPS 등)
     * @param timestamp RTP 타임스탬프
     * @param payloadType Payload Type
     * @param streamId 스트림 ID
     * @param isLastNalUnitInFrame 프레임의 마지막 NAL인지 (marker bit)
     * @return STAP-A RTP 패킷
     */
    private fun createStapAPacket(
        nalUnits: List<ByteArray>,
        timestamp: Long,
        payloadType: Int,
        streamId: String,
        isLastNalUnitInFrame: Boolean
    ): RTPPacket {
        // 전체 페이로드 크기 계산: 1 (STAP-A header) + sum(2 + nalUnit.size)
        val totalSize = 1 + nalUnits.sumOf { 2 + it.size }
        val buffer = allocator.buffer(totalSize)

        // STAP-A NAL Header 계산
        // F=0 (forbidden_zero_bit)
        // NRI = max NRI from all NAL units (중요도 보존)
        // Type = 24 (STAP-A)
        val maxNri = nalUnits.maxOf { (it[0].toInt() and 0x60) }  // NRI bits (bits 5-6)
        val stapAHeader = maxNri or NAL_TYPE_STAP_A  // F=0, NRI=max, Type=24

        buffer.writeByte(stapAHeader)

        // 각 NAL unit을 [2-byte size][NAL data] 형태로 추가
        nalUnits.forEach { nalUnit ->
            // 2-byte big-endian size
            buffer.writeShort(nalUnit.size)
            // NAL unit data (헤더 포함)
            buffer.writeBytes(nalUnit)
        }

        val seqNum = webrtcSeq.getAndIncrement() and 0xFFFF

        // 로깅
        val nalTypes = nalUnits.map { it[0].toInt() and 0x1F }
        val nalSizes = nalUnits.map { it.size }
        logger.info {
            "[STAP-A] Created: header=0x${"%02x".format(stapAHeader)}, " +
            "NALs=${nalTypes.zip(nalSizes).joinToString("+") { "(type${it.first}:${it.second}B)" }}, " +
            "total=${totalSize}B, seq=$seqNum, ts=$timestamp"
        }

        return RTPPacket.create(
            streamId = streamId,
            payloadType = payloadType,
            sequenceNumber = seqNum,
            timestamp = timestamp,
            ssrc = webrtcSSRC,
            marker = isLastNalUnitInFrame,
            payloadData = buffer
        )
    }

    /**
     * FU-A Fragmentation Packets 생성.
     *
     * 큰 NAL unit을 여러 FU-A 패킷으로 분할합니다.
     *
     * FU-A 포맷:
     * - FU indicator: (nalUnit[0] & 0xE0) | 28
     * - FU header: S|E|R|Type
     */
    private fun createFuAPackets(
        nalUnit: ByteArray,
        timestamp: Long,
        payloadType: Int,
        streamId: String,
        isLastNalUnitInFrame: Boolean
    ): List<RTPPacket> {
        val packets = mutableListOf<RTPPacket>()

        val nalHeader = nalUnit[0].toInt()
        val nalType = nalHeader and 0x1F
        val nri = nalHeader and 0x60  // NRI bits

        // FU indicator: (F|NRI|Type) where Type=28 (FU-A)
        val fuIndicator = (nalHeader and 0x80) or nri or NAL_TYPE_FU_A

        // NAL payload (헤더 제외)
        val nalPayload = nalUnit.copyOfRange(1, nalUnit.size)

        // 분할 크기 (FU indicator + FU header = 2 bytes)
        val fragmentSize = maxPayloadSize - 2

        var offset = 0
        var fragmentIndex = 0
        val totalFragments = (nalPayload.size + fragmentSize - 1) / fragmentSize

        while (offset < nalPayload.size) {
            val end = minOf(offset + fragmentSize, nalPayload.size)
            val fragmentData = nalPayload.copyOfRange(offset, end)

            val isStart = (offset == 0)
            val isEnd = (end >= nalPayload.size)

            // FU header: S|E|R|Type
            val fuHeader = (if (isStart) 0x80 else 0x00) or  // S bit
                    (if (isEnd) 0x40 else 0x00) or        // E bit
                    nalType                                  // Original NAL type

            // RTP 페이로드 생성 (FU indicator + FU header + fragment data)
            val buffer = allocator.buffer(2 + fragmentData.size)
            buffer.writeByte(fuIndicator)
            buffer.writeByte(fuHeader)
            buffer.writeBytes(fragmentData)

            val seqNum = webrtcSeq.getAndIncrement() and 0xFFFF

            // 마지막 프래그먼트이고 마지막 NAL unit이면 marker=1
            val marker = isEnd && isLastNalUnitInFrame

            val packet = RTPPacket.create(
                streamId = streamId,
                payloadType = payloadType,
                sequenceNumber = seqNum,
                timestamp = timestamp,  // 모든 프래그먼트는 같은 timestamp
                ssrc = webrtcSSRC,
                marker = marker,
                payloadData = buffer
            )

            // FU-A 타임스탬프 일관성 검증 로깅 (처음 몇 프레임만)
            if (webrtcSeq.get() < 500) {
                // 첫 번째 및 마지막 FU-A 패킷의 실제 바이트 출력
                if (isStart || isEnd) {
                    val fuIndicatorHex = "%02x".format(fuIndicator)
                    val fuHeaderHex = "%02x".format(fuHeader)
                    val firstDataBytes = fragmentData.take(4).joinToString(" ") { "%02x".format(it) }
                    logger.info {
                        "[FU-A PAYLOAD] frag=${fragmentIndex+1}/$totalFragments " +
                        "FU-Indicator=0x$fuIndicatorHex FU-Header=0x$fuHeaderHex " +
                        "data(first4)=[$firstDataBytes] nalType=$nalType"
                    }
                }
                logger.info {
                    "[FU-A DEBUG] frag=${fragmentIndex+1}/$totalFragments seq=$seqNum ts=$timestamp " +
                    "marker=$marker isStart=$isStart isEnd=$isEnd"
                }
            }

            packets.add(packet)
            offset = end
            fragmentIndex++
        }

        logger.trace {
            "[Repacketizer] FU-A: NAL type=$nalType, size=${nalUnit.size} → $totalFragments fragments"
        }

        return packets
    }

    /**
     * 통계 조회.
     */
    fun getStats(): RepacketizerStats {
        return RepacketizerStats(
            webrtcSSRC = webrtcSSRC,
            currentSeq = webrtcSeq.get(),
            timestampOffset = timestampOffset.get(),
            payloadTypeMapping = payloadTypeMapping
        )
    }

    /**
     * 리셋 (새 세션 시작 시).
     */
    fun reset() {
        webrtcSeq.set(0)
        timestampOffset.set(-1)
        bufferedSPS = null
        bufferedPPS = null
        logger.info { "[Repacketizer] Reset: seq=0, timestamp offset cleared, SPS/PPS buffer cleared" }
    }
}

/**
 * Repacketizer 통계.
 */
data class RepacketizerStats(
    val webrtcSSRC: Long,
    val currentSeq: Int,
    val timestampOffset: Long,
    val payloadTypeMapping: Map<Int, Int>
)

/**
 * Payload Type 매핑 빌더.
 */
class PayloadTypeMappingBuilder {
    private val mapping = mutableMapOf<Int, Int>()

    fun map(rtspPT: Int, webrtcPT: Int): PayloadTypeMappingBuilder {
        mapping[rtspPT] = webrtcPT
        return this
    }

    fun build(): Map<Int, Int> = mapping.toMap()
}
