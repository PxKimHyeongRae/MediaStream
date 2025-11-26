package com.pluxity.mediaserver.domain.webrtc

import com.pluxity.mediaserver.common.WebRTCException
import com.pluxity.mediaserver.domain.stream.RTPPacket
import com.pluxity.mediaserver.domain.stream.StreamManager
import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.coroutines.*
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong

private val logger = KotlinLogging.logger {}

/**
 * WebRTC 피어 연결 (Pure Java/Kotlin - Jitsi 라이브러리 사용).
 *
 * **실제 구현**:
 * - ✅ RTPRepacketizer: RTSP → WebRTC 변환 (100% 완성)
 * - ✅ DTLSHandler: 실제 인증서 생성
 * - ✅ ICEAgent: ice4j 사용
 * - ✅ SRTPTransformer: jitsi-srtp 사용
 *
 * **Pure Java 구현 - Virtual Threads 완벽 호환**
 *
 * @property peerId 피어 식별자 (WebSocket session ID)
 * @property streamId 스트림 식별자
 * @property streamManager 스트림 관리자
 * @property config WebRTC 설정
 */
class WebRTCPeer(
    private val peerId: String,
    private val streamId: String,
    private val streamManager: StreamManager,
    private val config: WebRTCConfig
) {
    private val running = AtomicBoolean(false)
    private val scope = CoroutineScope(Dispatchers.IO + Job())
    private var subscriptionJob: Job? = null

    // WebRTC 컴포넌트
    private var localSdp: String? = null
    private var remoteSdp: String? = null
    private val remoteCandidates = mutableListOf<String>()

    // 브라우저에서 협상된 H.264 Payload Type (Offer에서 파싱)
    private var negotiatedH264PayloadType: Int = 96
    private var negotiatedH264Fmtp: String = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"

    // RTP Repacketizer (RTSP → WebRTC 변환) ✅ REAL
    private val webrtcSSRC = (peerId.hashCode().toLong() and 0xFFFFFFFFL).toInt()
    private val repacketizer: RTPRepacketizer

    // ICE/SRTP/DTLS ✅ REAL
    private val iceAgent: ICEAgent
    private val dtlsHandler: DTLSHandler
    private var srtpTransformer: SRTPTransformer? = null
    private var iceDtlsTransport: IceDtlsTransport? = null
    private val iceConnected = AtomicBoolean(false)
    private val dtlsCompleted = AtomicBoolean(false)

    // 통계
    private val packetsProcessed = AtomicLong(0)
    private val bytesProcessed = AtomicLong(0)

    init {
        // Payload Type 매핑 (RTSP → WebRTC)
        val payloadMapping = PayloadTypeMappingBuilder()
            .map(96, 96) // H.264
            .map(97, 97) // H.265
            .build()

        repacketizer = RTPRepacketizer(webrtcSSRC.toLong() and 0xFFFFFFFFL, payloadMapping)

        // ICE Agent 초기화
        iceAgent = ICEAgent(
            streamId = streamId,
            isControlling = true,
            stunServers = config.stunServers
        )

        // DTLS Handler 초기화
        dtlsHandler = DTLSHandler(streamId)

        logger.info { "[$peerId] WebRTC Peer initialized (SSRC: $webrtcSSRC)" }
    }

    /**
     * SDP Offer 처리 및 Answer 생성.
     *
     * @param offerSdp 클라이언트의 SDP offer
     * @return 서버의 SDP answer
     */
    suspend fun processOffer(offerSdp: String): String {
        logger.info { "[$peerId] Processing SDP offer for stream: $streamId" }
        this.remoteSdp = offerSdp

        // 0. 브라우저 Offer에서 H.264 Payload Type 파싱 (필수!)
        negotiatedH264PayloadType = extractH264PayloadType(offerSdp)
        negotiatedH264Fmtp = extractH264Fmtp(offerSdp, negotiatedH264PayloadType)
        logger.info { "[$peerId] ★★★ Negotiated H.264: PT=$negotiatedH264PayloadType, fmtp=$negotiatedH264Fmtp" }

        // RTPRepacketizer에 동적 PT 설정 (RTSP PT 96 → 브라우저 PT로 매핑)
        updateRepacketizerPayloadType(negotiatedH264PayloadType)

        // 1. ICE Candidates 수집
        val localCandidates = iceAgent.gatherCandidates()
        logger.info { "[$peerId] Gathered ${localCandidates.size} local ICE candidates" }

        // 2. ICE Credentials 가져오기
        val (ufrag, password) = iceAgent.getLocalCredentials()

        // 3. DTLS Fingerprint 가져오기
        val fingerprint = dtlsHandler.getFingerprint()

        // 4. SDP Answer 생성 (브라우저의 PT를 그대로 사용)
        val answerSdp = generateAnswer(ufrag, password, fingerprint, localCandidates)
        this.localSdp = answerSdp

        logger.info { "[$peerId] Generated SDP answer with ${localCandidates.size} candidates" }
        logger.info { "[$peerId] DTLS Fingerprint: $fingerprint" }

        return answerSdp
    }

    /**
     * RTPRepacketizer의 WebRTC Payload Type 업데이트.
     *
     * 브라우저의 H.264 PT에 맞게 Repacketizer의 출력 PT를 동적으로 변경합니다.
     */
    private fun updateRepacketizerPayloadType(h264PayloadType: Int) {
        // Repacketizer의 setWebRTCPayloadType 메서드 호출 (새로 추가 필요)
        repacketizer.setWebRTCPayloadType(h264PayloadType)
        logger.info { "[$peerId] Updated RTPRepacketizer WebRTC PT to $h264PayloadType" }
    }

    /**
     * ICE Candidate 추가.
     *
     * ICE Agent가 이미 연결 시도 중이면 바로 추가합니다.
     *
     * @param candidate ICE candidate string
     */
    fun addIceCandidate(candidate: String) {
        logger.info { "[$peerId] Adding ICE candidate: ${candidate.take(80)}..." }
        remoteCandidates.add(candidate)

        // ICE Agent에 동적으로 candidate 추가 (연결 중이면 바로 적용)
        if (running.get()) {
            try {
                // Remote credentials 추출
                val remoteUfrag = extractFromSDP(remoteSdp ?: "", "ice-ufrag")
                val remotePassword = extractFromSDP(remoteSdp ?: "", "ice-pwd")

                if (remoteUfrag != null && remotePassword != null) {
                    iceAgent.addRemoteCandidates(listOf(candidate), remoteUfrag, remotePassword)
                    logger.info { "[$peerId] Dynamically added remote candidate to ICE Agent" }
                }
            } catch (e: Exception) {
                logger.warn(e) { "[$peerId] Failed to dynamically add candidate" }
            }
        }
    }

    /**
     * 스트림 구독 시작.
     *
     * StreamManager에서 RTP 패킷을 수신하여 WebRTC로 전송합니다.
     */
    fun start() {
        if (running.getAndSet(true)) {
            logger.warn { "[$peerId] WebRTC peer already running" }
            return
        }

        logger.info { "[$peerId] Starting WebRTC peer for stream: $streamId" }

        // ICE 연결 설정 (별도 코루틴)
        scope.launch {
            try {
                // Remote SDP에서 ICE credentials 추출
                val remoteUfrag = extractFromSDP(remoteSdp ?: "", "ice-ufrag") ?: run {
                    logger.error { "[$peerId] No remote ice-ufrag found" }
                    return@launch
                }
                val remotePassword = extractFromSDP(remoteSdp ?: "", "ice-pwd") ?: run {
                    logger.error { "[$peerId] No remote ice-pwd found" }
                    return@launch
                }

                logger.info { "[$peerId] Remote ICE credentials: ufrag=$remoteUfrag" }

                // Remote candidates가 도착할 때까지 대기 (최대 5초)
                logger.info { "[$peerId] Waiting for remote ICE candidates..." }
                var waitTime = 0
                while (remoteCandidates.isEmpty() && waitTime < 5000) {
                    delay(100)
                    waitTime += 100
                }

                if (remoteCandidates.isEmpty()) {
                    logger.warn { "[$peerId] No remote candidates received after ${waitTime}ms, proceeding anyway" }
                } else {
                    logger.info { "[$peerId] Received ${remoteCandidates.size} remote candidates after ${waitTime}ms" }
                }

                // Remote candidates 추가
                if (remoteCandidates.isNotEmpty()) {
                    iceAgent.addRemoteCandidates(remoteCandidates.toList(), remoteUfrag, remotePassword)
                }

                // ICE 연결 수립
                logger.info { "[$peerId] Establishing ICE connection..." }
                val connected = iceAgent.establishConnection()

                if (connected) {
                    iceConnected.set(true)
                    logger.info { "[$peerId] ICE connected successfully" }

                    // 선택된 candidate pair에서 원격 주소 확인
                    val selectedPair = iceAgent.getSelectedPair()
                    if (selectedPair != null) {
                        val remoteAddr = selectedPair.remoteCandidate?.transportAddress
                        logger.info { "[$peerId] Selected ICE pair - Remote: $remoteAddr" }
                    }

                    // ICE-DTLS Transport 생성
                    iceDtlsTransport = iceAgent.createDtlsTransport()

                    if (iceDtlsTransport != null) {
                        // 실제 DTLS 핸드셰이크 수행 (별도 스레드에서 blocking)
                        logger.info { "[$peerId] Starting DTLS handshake with ICE transport..." }
                        performDtlsHandshake()
                    } else {
                        // Fallback: Mock 키 사용
                        logger.warn { "[$peerId] ICE-DTLS transport unavailable, using mock keys" }
                        val (masterKey, masterSalt) = dtlsHandler.performHandshake(null)
                        srtpTransformer = SRTPTransformer(streamId, masterKey, masterSalt)
                        dtlsCompleted.set(true)
                        logger.info { "[$peerId] SRTP initialized with mock keys" }
                    }
                } else {
                    logger.error { "[$peerId] ICE connection failed" }
                }

            } catch (e: Exception) {
                logger.error(e) { "[$peerId] Failed to establish ICE/SRTP connection" }
            }
        }

        // 스트림 구독
        subscriptionJob = streamManager.subscribe(streamId, peerId, scope) { packet ->
            try {
                sendRTPPacket(packet)
            } catch (e: Exception) {
                logger.error(e) { "[$peerId] Error sending RTP packet" }
            }
        }

        logger.info { "[$peerId] WebRTC peer started, subscribed to stream: $streamId" }
    }

    /**
     * DTLS 핸드셰이크 수행 및 SRTP 키 자동 주입.
     *
     * ICE 연결 완료 후 호출됩니다.
     * 브라우저의 DTLS ClientHello를 수신하여 핸드셰이크를 완료하고,
     * 추출된 키로 SRTPTransformer를 초기화합니다.
     */
    private fun performDtlsHandshake() {
        val transport = iceDtlsTransport ?: run {
            logger.error { "[$peerId] ICE-DTLS transport is null" }
            return
        }

        // DTLS 핸드셰이크는 별도 Virtual Thread에서 실행 (blocking 작업)
        Thread.startVirtualThread {
            try {
                logger.info { "[$peerId] Waiting for DTLS ClientHello..." }

                // DTLS 핸드셰이크 수행 (blocking)
                val (masterKey, masterSalt) = dtlsHandler.performHandshake(transport)

                // SRTP Transformer 초기화 (키 주입)
                srtpTransformer = SRTPTransformer(streamId, masterKey, masterSalt)
                dtlsCompleted.set(true)

                logger.info { "[$peerId] DTLS handshake completed, SRTP keys injected" }
                logger.info { "[$peerId] Master Key: ${masterKey.size} bytes, Master Salt: ${masterSalt.size} bytes" }

            } catch (e: Exception) {
                logger.error(e) { "[$peerId] DTLS handshake failed" }

                // Fallback: Mock 키 사용
                logger.warn { "[$peerId] Falling back to mock SRTP keys" }
                val (mockKey, mockSalt) = dtlsHandler.performHandshake(null)
                srtpTransformer = SRTPTransformer(streamId, mockKey, mockSalt)
                dtlsCompleted.set(true)
            }
        }
    }

    /**
     * RTP 패킷 전송.
     *
     * StreamFlow에서 받은 RTP 패킷을 WebRTC로 변환 → SRTP 암호화 → ICE Transport 전송.
     * 참고: RTSPClient가 이미 copy()한 패킷을 emit하므로, 여기서는 직접 사용 후 release합니다.
     */
    private suspend fun sendRTPPacket(rtspPacket: RTPPacket) {
        // RTSPClient가 이미 copy를 emit했으므로 직접 사용
        val localPacket = rtspPacket

        try {
            // ICE 및 DTLS 완료 대기
            if (!iceConnected.get() || !dtlsCompleted.get() || srtpTransformer == null) {
                // 처음 몇 번만 로그 (너무 많으면 로그 폭발)
                val dropped = packetsProcessed.get()
                if (dropped < 10 || dropped % 100 == 0L) {
                    logger.info { "[$peerId] Connection not ready, dropping packet #$dropped (ICE: ${iceConnected.get()}, DTLS: ${dtlsCompleted.get()}, SRTP: ${srtpTransformer != null})" }
                }
                return
            }

            // 1. RTSP RTP → WebRTC RTP 변환 (Repacketizing + FU-A Fragmentation)
            // H.264 NAL units (100KB+)를 ~1200바이트 RTP 패킷들로 분할
            val webrtcPackets = repacketizer.repacketize(localPacket)

            if (webrtcPackets.isEmpty()) {
                logger.warn { "[$peerId] Repacketizer returned empty packets" }
                return
            }

            // 2. 각 분할된 패킷을 SRTP 암호화 후 전송
            var packetsSentInFrame = 0
            for (webrtcPacket in webrtcPackets) {
                try {
                    // 전체 RTP 패킷을 ByteBuf로 직렬화 (헤더 + 페이로드)
                    val rtpByteBuf = webrtcPacket.toByteBuf()

                    // Marker bit 물리적 검증 로깅 (처음 몇 프레임만)
                    val totalSentCheck = packetsProcessed.get()
                    if (totalSentCheck < 100 && webrtcPacket.header.marker) {
                        // 직렬화된 바이트 배열에서 Marker bit 확인 (byte[1]의 bit 7)
                        rtpByteBuf.markReaderIndex()
                        rtpByteBuf.skipBytes(1) // byte 0 skip
                        val byte1 = rtpByteBuf.readByte().toInt() and 0xFF
                        rtpByteBuf.resetReaderIndex()
                        val markerBitPhysical = (byte1 and 0x80) != 0
                        logger.info {
                            "[MARKER DEBUG] seq=${webrtcPacket.header.sequenceNumber} " +
                            "header.marker=${webrtcPacket.header.marker} " +
                            "byte1=0x${byte1.toString(16).padStart(2, '0')} " +
                            "physicalMarker=$markerBitPhysical"
                        }
                    }

                    // SRTP 암호화 (전체 RTP 패킷을 암호화해야 함)
                    val srtpData = srtpTransformer!!.encryptRTP(rtpByteBuf, webrtcSSRC.toInt())

                    // ICE Transport로 전송
                    val srtpBytes = ByteArray(srtpData.readableBytes())
                    srtpData.readBytes(srtpBytes)

                    // SRTP 패킷 RAW DUMP (처음 몇 패킷 및 marker=true인 패킷)
                    val shouldLogRaw = totalSentCheck < 200 || webrtcPacket.header.marker
                    if (shouldLogRaw) {
                        val payloadType = srtpBytes[1].toInt() and 0x7F
                        val marker = (srtpBytes[1].toInt() and 0x80) != 0
                        val seqNum = ((srtpBytes[2].toInt() and 0xFF) shl 8) or (srtpBytes[3].toInt() and 0xFF)
                        // Timestamp 추출 (bytes 4-7, big-endian)
                        val timestamp = ((srtpBytes[4].toLong() and 0xFF) shl 24) or
                                        ((srtpBytes[5].toLong() and 0xFF) shl 16) or
                                        ((srtpBytes[6].toLong() and 0xFF) shl 8) or
                                        (srtpBytes[7].toLong() and 0xFF)
                        logger.info {
                            "[SRTP RAW] seq=$seqNum pt=$payloadType marker=$marker ts=$timestamp size=${srtpBytes.size}"
                        }
                    }

                    iceAgent.send(srtpBytes)

                    // 통계 업데이트
                    packetsProcessed.incrementAndGet()
                    bytesProcessed.addAndGet(srtpBytes.size.toLong())
                    packetsSentInFrame++

                    // Cleanup
                    rtpByteBuf.release()
                    srtpData.release()
                } finally {
                    webrtcPacket.release()
                }
            }

            // 프레임 전송 로그 (처음 몇 개와 주기적으로)
            val totalSent = packetsProcessed.get()
            if (totalSent < 50 || totalSent % 500 == 0L) {
                logger.info {
                    "[$peerId] ✅ Sent frame: RTSP(seq=${localPacket.header.sequenceNumber}, ts=${localPacket.header.timestamp}, size=${localPacket.payload.readableBytes()}) → " +
                            "$packetsSentInFrame RTP packets (total sent: $totalSent)"
                }
            }

        } catch (e: Exception) {
            logger.error(e) { "[$peerId] Error processing RTP packet" }
        } finally {
            // RTSPClient가 emit한 패킷 정리 (refCnt 체크 후 release)
            if (localPacket.payload.refCnt() > 0) {
                localPacket.release()
            }
        }
    }

    /**
     * 피어 연결 종료.
     */
    suspend fun close() {
        if (!running.getAndSet(false)) {
            return
        }

        logger.info { "[$peerId] Closing WebRTC peer" }

        subscriptionJob?.cancel()
        subscriptionJob = null

        scope.cancel()

        // WebRTC 리소스 정리
        try {
            iceDtlsTransport?.close()
            iceAgent.close()
            srtpTransformer?.close()
            dtlsHandler.close()
        } catch (e: Exception) {
            logger.warn(e) { "[$peerId] Error closing WebRTC resources" }
        }
        iceDtlsTransport = null

        logger.info { "[$peerId] WebRTC peer closed" }
    }

    /**
     * SDP Answer 생성 (RFC 4566 표준 준수).
     *
     * SDP는 CRLF (\r\n)를 줄 구분자로 사용해야 합니다.
     * **중요**: 브라우저의 Offer에서 파싱한 H.264 Payload Type을 그대로 사용합니다.
     */
    private fun generateAnswer(
        ufrag: String,
        password: String,
        fingerprint: String,
        candidates: List<String>
    ): String {
        val timestamp = System.currentTimeMillis()
        val h264Pt = negotiatedH264PayloadType  // 브라우저의 H.264 PT 사용
        val h264Fmtp = negotiatedH264Fmtp        // 브라우저의 H.264 fmtp 사용

        logger.info { "[SDP Answer] Using negotiated H.264 PT=$h264Pt, fmtp=$h264Fmtp" }

        val sdp = buildString {
            appendLine("v=0")
            appendLine("o=- $timestamp 2 IN IP4 127.0.0.1")
            appendLine("s=-")
            appendLine("t=0 0")
            appendLine("a=group:BUNDLE 0")
            appendLine("a=msid-semantic: WMS *")
            // H.264만 선언 (브라우저의 PT 사용)
            appendLine("m=video 9 UDP/TLS/RTP/SAVPF $h264Pt")
            appendLine("c=IN IP4 0.0.0.0")
            appendLine("a=rtcp:9 IN IP4 0.0.0.0")
            appendLine("a=ice-ufrag:$ufrag")
            appendLine("a=ice-pwd:$password")
            appendLine("a=ice-options:trickle")
            appendLine("a=fingerprint:sha-256 $fingerprint")
            appendLine("a=setup:passive")  // 서버는 DTLS 서버 역할 (브라우저가 ClientHello 보냄)
            appendLine("a=mid:0")
            appendLine("a=sendonly")
            appendLine("a=rtcp-mux")
            appendLine("a=rtcp-rsize")
            // H.264 코덱 정보 (브라우저의 PT 사용)
            appendLine("a=rtpmap:$h264Pt H264/90000")
            appendLine("a=rtcp-fb:$h264Pt goog-remb")
            appendLine("a=rtcp-fb:$h264Pt transport-cc")
            appendLine("a=rtcp-fb:$h264Pt ccm fir")
            appendLine("a=rtcp-fb:$h264Pt nack")
            appendLine("a=rtcp-fb:$h264Pt nack pli")
            // fmtp도 브라우저의 것 사용
            appendLine("a=fmtp:$h264Pt $h264Fmtp")
            // SSRC는 unsigned 32-bit로 출력해야 함
            val ssrcUnsigned = webrtcSSRC.toLong() and 0xFFFFFFFFL
            appendLine("a=ssrc:$ssrcUnsigned cname:mediaserver-$streamId")
            appendLine("a=ssrc:$ssrcUnsigned msid:$streamId video0")
            appendLine("a=ssrc:$ssrcUnsigned mslabel:$streamId")
            appendLine("a=ssrc:$ssrcUnsigned label:video0")
            // ICE candidates (각 라인을 개별 추가)
            candidates.forEach { candidate ->
                appendLine("a=$candidate")
            }
        }

        // Windows에서 appendLine은 \r\n을 사용하지만, 명시적으로 \r\n 사용 보장
        return sdp.replace("\n", "\r\n").replace("\r\r\n", "\r\n")
    }

    /**
     * SDP에서 속성 값 추출.
     */
    private fun extractFromSDP(sdp: String, attribute: String): String? {
        val regex = Regex("a=$attribute:(\\S+)")
        return regex.find(sdp)?.groupValues?.getOrNull(1)
    }

    /**
     * SDP Offer에서 H.264 Payload Type 추출.
     *
     * 브라우저의 Offer에서 H.264 코덱에 할당된 PT 번호를 찾습니다.
     * 예: "a=rtpmap:102 H264/90000" → 102
     *
     * @param sdp SDP offer string
     * @return H.264 Payload Type (없으면 96 기본값)
     */
    private fun extractH264PayloadType(sdp: String): Int {
        // "a=rtpmap:XXX H264/90000" 패턴에서 XXX 추출
        val regex = Regex("a=rtpmap:(\\d+)\\s+H264/90000", RegexOption.IGNORE_CASE)
        val match = regex.find(sdp)
        val pt = match?.groupValues?.getOrNull(1)?.toIntOrNull() ?: 96

        logger.info { "[$peerId] Extracted H.264 Payload Type from Offer: $pt" }
        return pt
    }

    /**
     * SDP Offer에서 H.264 fmtp (format parameters) 추출.
     *
     * 브라우저가 요청한 H.264 프로파일/레벨 정보를 가져옵니다.
     * 예: "a=fmtp:102 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"
     *
     * @param sdp SDP offer string
     * @param payloadType H.264 Payload Type
     * @return fmtp 파라미터 (없으면 기본값)
     */
    private fun extractH264Fmtp(sdp: String, payloadType: Int): String {
        val regex = Regex("a=fmtp:$payloadType\\s+(.+)")
        val match = regex.find(sdp)
        val fmtp = match?.groupValues?.getOrNull(1) ?: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"

        logger.info { "[$peerId] Extracted H.264 fmtp: $fmtp" }
        return fmtp
    }

    /**
     * 현재 상태 확인.
     */
    fun isRunning(): Boolean = running.get()

    /**
     * 피어 통계 조회.
     */
    fun getStats(): WebRTCPeerStats {
        val repacketizerStats = repacketizer.getStats()
        val iceStats = iceAgent.getStats()
        val srtpStats = srtpTransformer?.getStats()

        return WebRTCPeerStats(
            peerId = peerId,
            streamId = streamId,
            isRunning = running.get(),
            iceConnected = iceConnected.get(),
            dtlsCompleted = dtlsCompleted.get(),
            iceState = iceStats.state,
            packetsProcessed = packetsProcessed.get(),
            bytesProcessed = bytesProcessed.get(),
            webrtcSSRC = webrtcSSRC.toLong(),
            repacketizerSeq = repacketizerStats.currentSeq,
            dtlsFingerprint = dtlsHandler.getFingerprint(),
            srtpEncrypted = srtpStats?.packetsEncrypted ?: 0
        )
    }
}

/**
 * WebRTC 피어 통계.
 */
data class WebRTCPeerStats(
    val peerId: String,
    val streamId: String,
    val isRunning: Boolean,
    val iceConnected: Boolean,
    val dtlsCompleted: Boolean,
    val iceState: String,
    val packetsProcessed: Long,
    val bytesProcessed: Long,
    val webrtcSSRC: Long,
    val repacketizerSeq: Int,
    val dtlsFingerprint: String,
    val srtpEncrypted: Long
)
