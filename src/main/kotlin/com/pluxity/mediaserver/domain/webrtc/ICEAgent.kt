package com.pluxity.mediaserver.domain.webrtc

import io.github.oshai.kotlinlogging.KotlinLogging
import org.ice4j.Transport
import org.ice4j.TransportAddress
import org.ice4j.ice.*
import org.ice4j.ice.harvest.StunCandidateHarvester
import java.beans.PropertyChangeEvent
import java.beans.PropertyChangeListener
import java.net.InetAddress
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong

private val logger = KotlinLogging.logger {}

/**
 * ICE Agent: ICE (Interactive Connectivity Establishment) 처리.
 *
 * **실제 ice4j 3.2-9 API 사용**
 *
 * ice4j는 Pure Java 구현이므로 Virtual Threads와 완벽 호환됩니다.
 *
 * @property streamId 스트림 식별자
 * @property isControlling Controlling/Controlled 모드 (서버는 controlling)
 * @property stunServers STUN 서버 목록
 */
class ICEAgent(
    private val streamId: String,
    private val isControlling: Boolean = true,
    private val stunServers: List<String> = listOf("stun.l.google.com:19302")
) : AutoCloseable {

    private val agent: Agent
    private val mediaStream: IceMediaStream
    private var component: Component? = null

    // 패킷 카운터 (로깅용)
    private val packetsSent = AtomicLong(0)

    init {
        logger.info { "[ICE $streamId] Initializing ICE Agent (controlling: $isControlling)" }

        // Agent 생성
        agent = Agent()
        agent.isControlling = isControlling

        // Media Stream 생성
        mediaStream = agent.createMediaStream("video")

        // Component 생성 (RTP)
        // API: createComponent(stream, keepAliveStrategy, useComponentSocket)
        //
        // useComponentSocket 파라미터:
        // - true: component.socket (MultiplexingDatagramSocket) 사용 가능
        //         모든 candidate pair의 트래픽이 하나의 소켓으로 통합됨
        // - false: component.socket이 null, selectedPair.iceSocketWrapper 사용
        //
        // DTLS Multiplexing을 위해 useComponentSocket=true 설정 필요
        component = agent.createComponent(
            mediaStream,
            KeepAliveStrategy.SELECTED_ONLY, // Keep-alive 전략
            true // useComponentSocket=true -> component.socket (MultiplexingDatagramSocket) 사용 가능
        )

        logger.info { "[ICE $streamId] Component created: ${component?.toShortString()}" }

        // STUN Harvester 추가
        addStunHarvesters()

        logger.info { "[ICE $streamId] ICE Agent initialized" }
    }

    /**
     * STUN Harvester 추가.
     */
    private fun addStunHarvesters() {
        stunServers.forEach { stunServer ->
            try {
                val parts = stunServer.split(":")
                val host = parts[0]
                val port = parts.getOrNull(1)?.toIntOrNull() ?: 3478

                val stunAddress = TransportAddress(host, port, Transport.UDP)
                val harvester = StunCandidateHarvester(stunAddress)
                agent.addCandidateHarvester(harvester)

                logger.info { "[ICE $streamId] Added STUN server: $stunServer" }
            } catch (e: Exception) {
                logger.warn(e) { "[ICE $streamId] Failed to add STUN server: $stunServer" }
            }
        }
    }

    /**
     * Local ICE candidates 수집.
     *
     * @return SDP format ICE candidates
     */
    fun gatherCandidates(): List<String> {
        logger.info { "[ICE $streamId] Gathering ICE candidates..." }

        val comp = component ?: run {
            logger.error { "[ICE $streamId] Component is null" }
            return emptyList()
        }

        // Candidate Harvesting은 자동으로 실행됨
        // 하지만 명시적으로 대기할 수 있음
        Thread.sleep(1000) // Harvesting 대기 (1초)

        // Local Candidates 변환 (SDP format)
        val candidates = mutableListOf<String>()
        comp.localCandidates.forEach { candidate ->
            val sdpCandidate = candidateToSDP(candidate, comp.componentID)
            candidates.add(sdpCandidate)
            logger.debug { "[ICE $streamId] Local candidate: $sdpCandidate" }
        }

        logger.info { "[ICE $streamId] Gathered ${candidates.size} candidates" }
        return candidates
    }

    /**
     * Remote ICE candidates 추가.
     *
     * @param sdpCandidates SDP format ICE candidates
     * @param remoteUfrag Remote ICE username fragment
     * @param remotePassword Remote ICE password
     */
    fun addRemoteCandidates(
        sdpCandidates: List<String>,
        remoteUfrag: String,
        remotePassword: String
    ) {
        logger.info { "[ICE $streamId] Adding ${sdpCandidates.size} remote candidates" }

        val comp = component ?: run {
            logger.error { "[ICE $streamId] Component is null" }
            return
        }

        // Remote credentials 설정
        mediaStream.setRemoteUfrag(remoteUfrag)
        mediaStream.setRemotePassword(remotePassword)

        // Remote candidates 추가
        sdpCandidates.forEach { sdpCandidate ->
            try {
                val candidate = sdpToCandidate(sdpCandidate, comp)
                if (candidate != null) {
                    comp.addRemoteCandidate(candidate)
                    logger.debug { "[ICE $streamId] Added remote candidate: ${candidate.toShortString()}" }
                }
            } catch (e: Exception) {
                logger.warn(e) { "[ICE $streamId] Failed to parse candidate: $sdpCandidate" }
            }
        }

        logger.info { "[ICE $streamId] Remote candidates added" }
    }

    /**
     * ICE 연결 수립.
     *
     * @return true if connected, false otherwise
     */
    fun establishConnection(): Boolean {
        logger.info { "[ICE $streamId] Establishing ICE connection..." }

        val latch = CountDownLatch(1)
        var connected = false

        // State change listener
        val listener = PropertyChangeListener { event: PropertyChangeEvent ->
            if (event.propertyName == Agent.PROPERTY_ICE_PROCESSING_STATE) {
                val state = event.newValue as IceProcessingState
                logger.debug { "[ICE $streamId] ICE state: $state" }

                when (state) {
                    IceProcessingState.COMPLETED -> {
                        logger.info { "[ICE $streamId] ICE connection established" }
                        connected = true
                        latch.countDown()
                    }
                    IceProcessingState.FAILED -> {
                        logger.error { "[ICE $streamId] ICE connection failed" }
                        connected = false
                        latch.countDown()
                    }
                    IceProcessingState.TERMINATED -> {
                        logger.error { "[ICE $streamId] ICE connection terminated" }
                        connected = false
                        latch.countDown()
                    }
                    else -> {
                        // RUNNING, WAITING 등 진행 중
                    }
                }
            }
        }

        agent.addStateChangeListener(listener)

        try {
            // ICE 처리 시작
            agent.startConnectivityEstablishment()

            // 최대 10초 대기
            if (!latch.await(10, TimeUnit.SECONDS)) {
                logger.error { "[ICE $streamId] ICE connection timeout" }
                return false
            }

            return connected
        } finally {
            agent.removeStateChangeListener(listener)
        }
    }

    /**
     * 선택된 Candidate Pair 가져오기.
     */
    fun getSelectedPair(): CandidatePair? {
        return component?.selectedPair
    }

    /**
     * ICE Component 가져오기.
     *
     * DTLS Transport 생성에 필요합니다.
     */
    fun getComponent(): Component? = component

    /**
     * DTLS Transport 생성.
     *
     * ICE Component의 Multiplexed Socket을 사용하여
     * DTLS 패킷만 처리하는 Transport를 생성합니다.
     */
    fun createDtlsTransport(): IceDtlsTransport? {
        val comp = component ?: run {
            logger.error { "[ICE $streamId] Component is null, cannot create DTLS transport" }
            return null
        }
        return IceDtlsTransport(streamId, comp)
    }

    /**
     * 데이터 전송 (UDP).
     *
     * @param data 전송할 데이터
     */
    fun send(data: ByteArray) {
        val comp = component ?: run {
            logger.error { "[ICE $streamId] Component is null, cannot send" }
            return
        }

        try {
            // Component.send() 메서드 사용
            comp.send(data, 0, data.size)
            val sent = packetsSent.incrementAndGet()

            // 처음 몇 개와 주기적으로 로그
            if (sent < 10 || sent % 100 == 0L) {
                logger.info { "[ICE $streamId] ✅ Sent packet #$sent: ${data.size} bytes via ICE" }
            }
        } catch (e: Exception) {
            logger.error(e) { "[ICE $streamId] Failed to send data: ${data.size} bytes" }
        }
    }

    /**
     * Local ICE credentials 가져오기.
     *
     * @return Pair(ufrag, password)
     */
    fun getLocalCredentials(): Pair<String, String> {
        val ufrag = agent.localUfrag
        val password = agent.localPassword
        return Pair(ufrag, password)
    }

    /**
     * Candidate를 SDP 형식으로 변환.
     */
    private fun candidateToSDP(candidate: LocalCandidate, componentId: Int): String {
        val foundation = candidate.foundation
        val priority = candidate.priority
        val ip = candidate.transportAddress.hostAddress
        val port = candidate.transportAddress.port
        val type = candidate.type.toString().lowercase()
        val transport = candidate.transport.toString().lowercase()  // RFC5245: must be lowercase

        return "candidate:$foundation $componentId $transport $priority $ip $port typ $type"
    }

    /**
     * SDP 형식을 Candidate로 변환.
     */
    private fun sdpToCandidate(sdp: String, component: Component): RemoteCandidate? {
        try {
            // candidate:1 1 UDP 2130706431 192.168.1.100 54321 typ host
            val parts = sdp.removePrefix("candidate:").split(" ")
            if (parts.size < 8) return null

            val foundation = parts[0]
            val componentId = parts[1].toIntOrNull() ?: Component.RTP
            val transport = Transport.parse(parts[2])
            val priority = parts[3].toLongOrNull() ?: 0L
            val ip = parts[4]
            val port = parts[5].toIntOrNull() ?: return null
            val type = CandidateType.parse(parts[7])

            val transportAddress = TransportAddress(ip, port, transport)

            return RemoteCandidate(
                transportAddress,
                component,
                type,
                foundation,
                priority,
                null // Related address (optional)
            )
        } catch (e: Exception) {
            logger.warn(e) { "[ICE $streamId] Failed to parse SDP candidate: $sdp" }
            return null
        }
    }

    /**
     * 통계 조회.
     */
    fun getStats(): ICEStats {
        val comp = component
        return ICEStats(
            streamId = streamId,
            state = agent.state.toString(),
            localCandidates = comp?.localCandidateCount ?: 0,
            remoteCandidates = comp?.remoteCandidateCount ?: 0,
            selectedPair = comp?.selectedPair?.toShortString()
        )
    }

    override fun close() {
        logger.info { "[ICE $streamId] Closing ICE Agent" }
        try {
            agent.free()
        } catch (e: Exception) {
            logger.warn(e) { "[ICE $streamId] Error closing ICE Agent" }
        }
    }
}

/**
 * ICE 통계.
 */
data class ICEStats(
    val streamId: String,
    val state: String,
    val localCandidates: Int,
    val remoteCandidates: Int,
    val selectedPair: String?
)
