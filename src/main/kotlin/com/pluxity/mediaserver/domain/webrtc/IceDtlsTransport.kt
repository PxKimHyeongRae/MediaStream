package com.pluxity.mediaserver.domain.webrtc

import io.github.oshai.kotlinlogging.KotlinLogging
import org.bouncycastle.tls.DatagramTransport
import org.ice4j.ice.Component
import org.ice4j.socket.MultiplexedDatagramSocket
import java.io.IOException
import java.net.DatagramPacket
import java.net.InetSocketAddress
import java.net.SocketTimeoutException
import java.util.concurrent.BlockingQueue
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

private val logger = KotlinLogging.logger {}

/**
 * ICE-DTLS Transport 어댑터.
 *
 * ICE Component의 소켓과 Bouncy Castle DTLS를 연결합니다.
 * Multiplexing을 통해 DTLS 패킷만 필터링하여 처리합니다.
 *
 * **동작 방식**:
 * 1. ICE Component의 MultiplexingDatagramSocket을 가져옴
 * 2. DTLSPacketFilter로 DTLS 전용 소켓 생성
 * 3. 수신 스레드가 DTLS 패킷을 큐에 넣음
 * 4. Bouncy Castle DTLSServerProtocol이 큐에서 패킷을 읽음
 *
 * @property streamId 스트림 식별자
 * @property component ICE Component
 */
class IceDtlsTransport(
    private val streamId: String,
    private val component: Component
) : DatagramTransport, AutoCloseable {

    private val mtu = 1500
    private val running = AtomicBoolean(true)

    // DTLS 패킷 큐 (수신용)
    private val receiveQueue: BlockingQueue<DatagramPacket> = LinkedBlockingQueue(100)

    // DTLS Multiplexed Socket (오직 이것만 사용)
    private var dtlsSocket: MultiplexedDatagramSocket? = null

    // 수신 스레드
    private var receiveThread: Thread? = null

    // 원격 주소 (DTLS 응답 전송용)
    @Volatile
    private var remoteAddress: InetSocketAddress? = null

    init {
        logger.info { "[$streamId] Initializing ICE-DTLS Transport" }
        initializeSockets()
    }

    /**
     * ICE 소켓 초기화.
     *
     * ICE 연결이 완료된 후 Component의 MultiplexingDatagramSocket을 통해 DTLS 소켓을 획득합니다.
     *
     * **중요**: 절대 Raw UDP Socket을 직접 사용하지 않습니다.
     * - iceSocketWrapper, componentSocket 등의 내부 API 사용 금지
     * - 오직 component.socket.getSocket(DTLSPacketFilter())만 사용
     * - 이렇게 해야 ice4j의 STUN 처리와 충돌 없이 패킷 Multiplexing이 동작합니다.
     */
    private fun initializeSockets() {
        try {
            // 1. Selected pair에서 remote address 확인
            val selectedPair = component.selectedPair
            if (selectedPair == null) {
                throw IllegalStateException("No selected pair - ICE not completed")
            }

            val localCandidate = selectedPair.localCandidate
            logger.info { "[$streamId] Selected pair local: ${localCandidate.transportAddress}" }

            val remoteCandidate = selectedPair.remoteCandidate
            remoteAddress = InetSocketAddress(
                remoteCandidate.transportAddress.address,
                remoteCandidate.transportAddress.port
            )
            logger.info { "[$streamId] Remote address: $remoteAddress" }

            // 2. 오직 Component.socket (MultiplexingDatagramSocket)만 사용
            // ICEAgent에서 useComponentSocket=true로 설정되어 있어야 함
            val muxSocket = component.socket
            if (muxSocket == null) {
                logger.error { "[$streamId] component.socket is NULL!" }
                logger.error { "[$streamId] ICEAgent에서 useComponentSocket=true 설정을 확인하세요." }
                throw IllegalStateException(
                    "component.socket is null - ICEAgent must use useComponentSocket=true"
                )
            }

            logger.info { "[$streamId] Got MultiplexingDatagramSocket: ${muxSocket.localSocketAddress}" }

            // 3. DTLSPacketFilter로 DTLS 전용 가상 소켓 생성
            dtlsSocket = muxSocket.getSocket(DTLSPacketFilter())
            logger.info { "[$streamId] Created DTLS filtered socket via MultiplexingDatagramSocket" }

            // 4. 수신 스레드 시작
            startReceiveThread()

        } catch (e: Exception) {
            logger.error(e) { "[$streamId] Failed to initialize ICE sockets" }
            throw e
        }
    }

    /**
     * 패킷 수신 스레드 시작.
     *
     * MultiplexedDatagramSocket을 통해 DTLS 패킷만 수신합니다.
     * DTLSPacketFilter가 이미 적용되어 있으므로 DTLS 패킷만 이 소켓으로 전달됩니다.
     */
    private fun startReceiveThread() {
        val socket = dtlsSocket
        if (socket == null) {
            logger.error { "[$streamId] dtlsSocket is null - cannot start receive thread" }
            return
        }

        receiveThread = Thread({
            val buffer = ByteArray(mtu)
            val packet = DatagramPacket(buffer, buffer.size)

            logger.info { "[$streamId] DTLS receive thread started (using MultiplexedDatagramSocket)" }

            while (running.get()) {
                try {
                    socket.soTimeout = 1000  // 1초 타임아웃
                    socket.receive(packet)

                    // DTLS 패킷 수신 로그 (INFO 레벨로 변경하여 디버깅)
                    val firstByte = if (packet.length > 0) packet.data[packet.offset].toInt() and 0xFF else -1
                    logger.info { "[$streamId] ✅ DTLS packet received: ${packet.length} bytes, firstByte=$firstByte from ${packet.socketAddress}" }

                    // 원격 주소 업데이트
                    remoteAddress = packet.socketAddress as? InetSocketAddress

                    // 패킷 복사하여 큐에 추가
                    val copy = ByteArray(packet.length)
                    System.arraycopy(packet.data, packet.offset, copy, 0, packet.length)
                    val copyPacket = DatagramPacket(copy, copy.size, packet.socketAddress)

                    if (!receiveQueue.offer(copyPacket, 100, TimeUnit.MILLISECONDS)) {
                        logger.warn { "[$streamId] DTLS receive queue full" }
                    } else {
                        logger.info { "[$streamId] ✅ Queued DTLS packet: ${packet.length} bytes (queue size: ${receiveQueue.size})" }
                    }

                    // 버퍼 재사용을 위해 리셋
                    packet.length = buffer.size

                } catch (e: SocketTimeoutException) {
                    // 타임아웃은 정상 (주기적 체크)
                } catch (e: Exception) {
                    if (running.get()) {
                        logger.error(e) { "[$streamId] Error receiving packet" }
                    }
                }
            }

            logger.info { "[$streamId] DTLS receive thread stopped" }
        }, "dtls-receive-$streamId")

        receiveThread?.isDaemon = true
        receiveThread?.start()
    }

    // ===============================
    // DatagramTransport 인터페이스 구현
    // ===============================

    override fun getReceiveLimit(): Int = mtu

    override fun getSendLimit(): Int = mtu - 28  // IP + UDP 헤더

    /**
     * DTLS 패킷 수신 (Bouncy Castle이 호출).
     *
     * @param buf 버퍼
     * @param off 오프셋
     * @param len 최대 길이
     * @param waitMillis 대기 시간 (밀리초)
     * @return 수신된 바이트 수, -1이면 타임아웃
     */
    override fun receive(buf: ByteArray, off: Int, len: Int, waitMillis: Int): Int {
        val packet = if (waitMillis > 0) {
            receiveQueue.poll(waitMillis.toLong(), TimeUnit.MILLISECONDS)
        } else {
            receiveQueue.poll()
        }

        if (packet == null) {
            return -1  // 타임아웃
        }

        val copyLen = minOf(len, packet.length)
        System.arraycopy(packet.data, packet.offset, buf, off, copyLen)

        logger.info { "[$streamId] DTLS receive: $copyLen bytes from queue" }
        return copyLen
    }

    /**
     * DTLS 패킷 전송 (Bouncy Castle이 호출).
     *
     * MultiplexedDatagramSocket을 통해 전송합니다.
     *
     * @param buf 버퍼
     * @param off 오프셋
     * @param len 길이
     */
    override fun send(buf: ByteArray, off: Int, len: Int) {
        val socket = dtlsSocket
        if (socket == null) {
            throw IOException("dtlsSocket is null - cannot send")
        }

        val remote = remoteAddress
        if (remote == null) {
            // ICE가 완료되면 selected pair의 remote address 사용
            val selectedPair = component.selectedPair
            if (selectedPair != null) {
                val addr = selectedPair.remoteCandidate.transportAddress
                remoteAddress = InetSocketAddress(addr.address, addr.port)
            } else {
                throw IOException("No remote address available")
            }
        }

        val packet = DatagramPacket(buf, off, len, remoteAddress)
        socket.send(packet)

        logger.info { "[$streamId] DTLS send: $len bytes to $remoteAddress" }
    }

    override fun close() {
        logger.info { "[$streamId] Closing ICE-DTLS Transport" }
        running.set(false)

        receiveThread?.interrupt()
        receiveThread = null

        // dtlsSocket은 MultiplexingDatagramSocket의 가상 소켓이므로 직접 닫지 않음
        // (부모 MultiplexingDatagramSocket이 관리)
        dtlsSocket = null

        receiveQueue.clear()
    }

    /**
     * 통계 조회.
     */
    fun getStats(): IceDtlsTransportStats {
        return IceDtlsTransportStats(
            streamId = streamId,
            queueSize = receiveQueue.size,
            hasRemoteAddress = remoteAddress != null,
            remoteAddress = remoteAddress?.toString()
        )
    }
}

/**
 * ICE-DTLS Transport 통계.
 */
data class IceDtlsTransportStats(
    val streamId: String,
    val queueSize: Int,
    val hasRemoteAddress: Boolean,
    val remoteAddress: String?
)

/**
 * ICE Component에서 DTLS Transport 생성 확장 함수.
 */
fun Component.createDtlsTransport(streamId: String): IceDtlsTransport {
    return IceDtlsTransport(streamId, this)
}
