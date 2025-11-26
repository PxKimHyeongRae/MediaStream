package com.pluxity.mediaserver.domain.rtsp

import com.pluxity.mediaserver.common.RTSPException
import com.pluxity.mediaserver.domain.stream.RTPPacket
import com.pluxity.mediaserver.domain.stream.StreamManager
import com.pluxity.mediaserver.util.VirtualThreads
import io.github.oshai.kotlinlogging.KotlinLogging
import io.netty.buffer.PooledByteBufAllocator
import org.bytedeco.ffmpeg.avcodec.AVPacket
import org.bytedeco.ffmpeg.global.avcodec
import org.bytedeco.ffmpeg.global.avutil
import org.bytedeco.javacv.FFmpegFrameGrabber
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger

private val logger = KotlinLogging.logger {}

/**
 * RTSP 클라이언트 (Virtual Threads 기반).
 *
 * JavaCV의 FFmpegFrameGrabber를 사용하여 RTSP 스트림을 수신하고,
 * Virtual Thread에서 blocking I/O를 처리합니다.
 *
 * **특징**:
 * - Virtual Thread로 경량 동시성
 * - 자동 재연결 로직
 * - H.264/H.265 코덱 지원
 * - Frame을 RTP 패킷으로 변환하여 StreamManager로 전달
 *
 * @property streamId 스트림 식별자
 * @property url RTSP URL (예: rtsp://camera.example.com/stream1)
 * @property streamManager 스트림 관리자
 * @property config RTSP 설정
 */
class RTSPClient(
    private val streamId: String,
    private val url: String,
    private val streamManager: StreamManager,
    private val config: RTSPConfig
) {
    private val running = AtomicBoolean(false)
    private val reconnectAttempts = AtomicInteger(0)
    private var virtualThread: Thread? = null
    private var grabber: FFmpegFrameGrabber? = null

    private val allocator = PooledByteBufAllocator.DEFAULT
    private var sequenceNumber = 0
    private var ssrc: Long = 0L

    /**
     * RTSP 클라이언트 시작.
     *
     * Virtual Thread를 생성하여 RTSP 스트림 수신을 시작합니다.
     */
    fun start() {
        if (running.getAndSet(true)) {
            logger.warn { "[$streamId] RTSP client already running" }
            return
        }

        logger.info { "[$streamId] Starting RTSP client: $url" }

        // SSRC 생성 (스트림 식별자)
        ssrc = streamId.hashCode().toLong() and 0xFFFFFFFFL

        // Virtual Thread로 시작
        virtualThread = VirtualThreads.startVirtualThread {
            try {
                runWithReconnect()
            } catch (e: Exception) {
                logger.error(e) { "[$streamId] RTSP client fatal error" }
            } finally {
                running.set(false)
                cleanup()
            }
        }
    }

    /**
     * RTSP 클라이언트 중지.
     */
    fun stop() {
        if (!running.getAndSet(false)) {
            logger.warn { "[$streamId] RTSP client not running" }
            return
        }

        logger.info { "[$streamId] Stopping RTSP client" }
        cleanup()
        virtualThread?.interrupt()
    }

    /**
     * 재연결 로직을 포함한 메인 루프.
     */
    private fun runWithReconnect() {
        while (running.get()) {
            try {
                connectAndStream()

                // 정상 종료된 경우 재연결 카운터 리셋
                reconnectAttempts.set(0)
            } catch (e: InterruptedException) {
                logger.info { "[$streamId] RTSP client interrupted" }
                break
            } catch (e: Exception) {
                logger.error(e) { "[$streamId] RTSP connection error" }

                val attempts = reconnectAttempts.incrementAndGet()
                if (attempts >= config.maxReconnectAttempts) {
                    logger.error { "[$streamId] Max reconnect attempts ($attempts) reached" }
                    throw RTSPException(streamId, "Max reconnect attempts exceeded", e)
                }

                logger.info { "[$streamId] Reconnecting in ${config.reconnectDelay}ms (attempt $attempts/${config.maxReconnectAttempts})" }
                Thread.sleep(config.reconnectDelay)
            }
        }
    }

    /**
     * RTSP 연결 및 스트리밍.
     */
    private fun connectAndStream() {
        // FFmpegFrameGrabber 생성 및 설정
        val grabber = FFmpegFrameGrabber(url).apply {
            setOption("rtsp_transport", config.transport)
            this.format = "rtsp"
            setOption("buffer_size", config.bufferSize.toString())
            setOption("max_delay", config.maxDelay.toString())
            setOption("stimeout", (config.readTimeout * 1000).toString()) // microseconds

            // 로깅 레벨 설정 (에러만 출력)
            avutil.av_log_set_level(avutil.AV_LOG_ERROR)
        }

        try {
            logger.info { "[$streamId] Connecting to RTSP: $url" }
            grabber.start()
            logger.info { "[$streamId] RTSP connected. Video codec: ${grabber.videoCodecName}, Format: ${grabber.format}" }

            this.grabber = grabber

            // 스트림이 없으면 생성
            streamManager.getOrCreateStream(streamId)

            // 프레임 읽기 루프
            processFrames(grabber)
        } finally {
            try {
                grabber.stop()
                grabber.release()
            } catch (e: Exception) {
                logger.warn(e) { "[$streamId] Error stopping grabber" }
            }
            this.grabber = null
        }
    }

    /**
     * 패킷 처리 루프.
     *
     * grabFrame() 대신 grabPacket()을 사용하여 인코딩된 H.264/H.265 NAL 유닛을 그대로 가져옵니다.
     * 이렇게 하면 디코딩/재인코딩 없이 WebRTC로 직접 전송할 수 있습니다.
     */
    private fun processFrames(grabber: FFmpegFrameGrabber) {
        var packetCount = 0L
        var lastLogTime = System.currentTimeMillis()

        // 비디오 스트림 인덱스 확인
        val videoStream = grabber.videoStream
        logger.info { "[$streamId] Video stream index: $videoStream, codec: ${grabber.videoCodecName}" }

        while (running.get()) {
            // AVPacket 읽기 (인코딩된 데이터)
            val avPacket: AVPacket? = try {
                grabber.grabPacket()
            } catch (e: Exception) {
                logger.error(e) { "[$streamId] Error grabbing packet" }
                throw e
            }

            if (avPacket == null) {
                logger.warn { "[$streamId] Null packet received, stream may have ended" }
                break
            }

            // 비디오 패킷만 처리 (오디오 스킵)
            if (avPacket.stream_index() != videoStream) {
                continue
            }

            // AVPacket을 RTP 패킷으로 변환
            val rtpPacket = avPacketToRTPPacket(avPacket, grabber)
            if (rtpPacket != null) {
                // StreamManager로 패킷 전달 (suspend function)
                // SharedFlow는 비동기로 전달하므로, copy를 emit하고 원본은 즉시 release
                try {
                    // 구독자에게 전달할 복사본 생성
                    val packetCopy = rtpPacket.copy()

                    kotlinx.coroutines.runBlocking {
                        streamManager.publishPacket(streamId, packetCopy)
                    }
                    // 참고: packetCopy는 구독자가 release 책임
                } catch (e: Exception) {
                    logger.error(e) { "[$streamId] Error publishing packet" }
                } finally {
                    // 원본 패킷 즉시 release (copy를 emit했으므로 안전)
                    rtpPacket.release()
                }
            }

            packetCount++

            // 통계 로깅 (10초마다)
            val now = System.currentTimeMillis()
            if (now - lastLogTime > 10000) {
                val pps = packetCount / ((now - lastLogTime) / 1000.0)
                logger.info { "[$streamId] Packets: $packetCount, PPS: ${"%.2f".format(pps)}" }
                packetCount = 0
                lastLogTime = now
            }
        }
    }

    /**
     * AVPacket을 RTP 패킷으로 변환.
     *
     * AVPacket에는 인코딩된 H.264/H.265 NAL 유닛이 들어있습니다.
     * 이것을 RTP 페이로드로 사용합니다.
     */
    private fun avPacketToRTPPacket(avPacket: AVPacket, grabber: FFmpegFrameGrabber): RTPPacket? {
        val size = avPacket.size()
        if (size <= 0) return null

        // AVPacket 데이터를 ByteArray로 복사
        val data = ByteArray(size)
        avPacket.data().get(data)

        // Netty ByteBuf 생성
        val byteBuf = allocator.buffer(size)
        byteBuf.writeBytes(data)

        // RTP 패킷 생성
        val payloadType = when (grabber.videoCodecName?.lowercase()) {
            "h264" -> 96
            "h265", "hevc" -> 97
            else -> 96
        }

        // 키프레임 확인 (AV_PKT_FLAG_KEY)
        val isKeyFrame = (avPacket.flags() and avcodec.AV_PKT_FLAG_KEY) != 0

        // PTS를 90kHz 타임스탬프로 변환 (RTP 표준)
        // FFmpeg PTS는 time_base 기준이므로 90kHz로 변환 필요
        val pts = avPacket.pts()
        val timestamp = if (pts != avutil.AV_NOPTS_VALUE && pts >= 0) {
            // FFmpeg의 time_base를 고려하여 90kHz로 변환
            // 대부분의 RTSP 스트림은 이미 90kHz 기준이거나 1/1000초 기준
            // time_base가 1/90000이면 그대로, 1/1000이면 *90
            val videoStream = grabber.formatContext.streams(grabber.videoStream)
            val timeBase = videoStream.time_base()
            val tbNum = timeBase.num()
            val tbDen = timeBase.den()

            // 90kHz 변환: pts * 90000 / (den/num) = pts * 90000 * num / den
            if (tbDen > 0 && tbNum > 0) {
                (pts * 90000L * tbNum / tbDen)
            } else {
                // time_base 정보가 없으면 기본 90kHz 가정
                pts
            }
        } else {
            // PTS가 없으면 프레임 카운터 기반 타임스탬프 사용
            // 30fps 가정: 90000/30 = 3000 ticks per frame
            (sequenceNumber.toLong() * 3000)
        }

        return RTPPacket.create(
            streamId = streamId,
            payloadType = payloadType,
            sequenceNumber = sequenceNumber++,
            timestamp = timestamp,
            ssrc = ssrc,
            marker = isKeyFrame,
            payloadData = byteBuf
        )
    }

    /**
     * 리소스 정리.
     */
    private fun cleanup() {
        grabber?.let {
            try {
                it.stop()
                it.release()
            } catch (e: Exception) {
                logger.warn(e) { "[$streamId] Error cleaning up grabber" }
            }
        }
        grabber = null
    }

    /**
     * 현재 상태 확인.
     */
    fun isRunning(): Boolean = running.get()
}
