package com.pluxity.mediaserver.controller

import com.fasterxml.jackson.databind.ObjectMapper
import com.fasterxml.jackson.module.kotlin.readValue
import com.pluxity.mediaserver.domain.webrtc.WebRTCManager
import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.coroutines.runBlocking
import org.springframework.stereotype.Component
import org.springframework.web.socket.CloseStatus
import org.springframework.web.socket.TextMessage
import org.springframework.web.socket.WebSocketSession
import org.springframework.web.socket.handler.TextWebSocketHandler
import java.util.concurrent.ConcurrentHashMap

private val logger = KotlinLogging.logger {}

/**
 * WebSocket 시그널링 핸들러.
 *
 * WebRTC 연결을 위한 SDP와 ICE candidate 교환을 처리합니다.
 *
 * **메시지 타입**:
 * - `offer`: 클라이언트 → 서버 (SDP offer)
 * - `answer`: 서버 → 클라이언트 (SDP answer)
 * - `candidate`: 양방향 (ICE candidate)
 * - `close`: 연결 종료
 *
 * **프로토콜**:
 * ```json
 * {
 *   "type": "offer",
 *   "streamId": "stream123",
 *   "sdp": "v=0\r\no=- ...",
 *   "candidate": null
 * }
 * ```
 */
@Component
class SignalingController(
    private val objectMapper: ObjectMapper,
    private val webrtcManager: WebRTCManager
) : TextWebSocketHandler() {

    // 활성 WebSocket 세션 (sessionId → WebSocketSession)
    private val sessions = ConcurrentHashMap<String, WebSocketSession>()

    // 스트림별 구독자 (streamId → Set<sessionId>)
    private val streamSubscribers = ConcurrentHashMap<String, MutableSet<String>>()

    /**
     * 클라이언트 연결 수립.
     */
    override fun afterConnectionEstablished(session: WebSocketSession) {
        val sessionId = session.id
        sessions[sessionId] = session

        logger.info { "WebSocket connected: $sessionId (total: ${sessions.size})" }

        // Welcome 메시지 전송
        val welcomeMsg = SignalingMessage(
            type = "welcome",
            message = "WebSocket signaling connected"
        )
        sendMessage(session, welcomeMsg)
    }

    /**
     * 클라이언트 메시지 수신.
     */
    override fun handleTextMessage(session: WebSocketSession, message: TextMessage) {
        val sessionId = session.id

        try {
            val payload = message.payload
            logger.debug { "[$sessionId] Received: ${payload.take(100)}..." }

            val msg = objectMapper.readValue<SignalingMessage>(payload)

            when (msg.type) {
                "offer" -> handleOffer(session, msg)
                "candidate" -> handleCandidate(session, msg)
                "subscribe" -> handleSubscribe(session, msg)
                "unsubscribe" -> handleUnsubscribe(session, msg)
                else -> {
                    logger.warn { "[$sessionId] Unknown message type: ${msg.type}" }
                    sendError(session, "Unknown message type: ${msg.type}")
                }
            }
        } catch (e: Exception) {
            logger.error(e) { "[$sessionId] Error handling message" }
            sendError(session, "Error: ${e.message}")
        }
    }

    /**
     * 클라이언트 연결 종료.
     */
    override fun afterConnectionClosed(session: WebSocketSession, status: CloseStatus) {
        val sessionId = session.id
        sessions.remove(sessionId)

        // 모든 스트림 구독 해제
        streamSubscribers.values.forEach { subscribers ->
            subscribers.remove(sessionId)
        }

        // 세션의 모든 WebRTC 피어 정리
        runBlocking {
            try {
                webrtcManager.removePeersBySession(sessionId)
            } catch (e: Exception) {
                logger.debug { "[$sessionId] Error removing peers: ${e.message}" }
            }
        }

        logger.info { "WebSocket disconnected: $sessionId (total: ${sessions.size})" }
    }

    /**
     * SDP Offer 처리.
     *
     * 클라이언트의 SDP offer를 받아 WebRTC 피어를 생성하고 answer를 반환합니다.
     */
    private fun handleOffer(session: WebSocketSession, msg: SignalingMessage) {
        val sessionId = session.id
        val streamId = msg.streamId ?: run {
            sendError(session, "Missing streamId")
            return
        }
        val sdp = msg.sdp ?: run {
            sendError(session, "Missing SDP")
            return
        }

        logger.info { "[$sessionId] Received offer for stream: $streamId" }

        try {
            // WebRTC 피어 생성
            val peer = webrtcManager.createPeer(sessionId, streamId)

            // SDP Offer 처리 및 Answer 생성 (suspend function → runBlocking)
            val answerSdp = runBlocking {
                peer.processOffer(sdp)
            }

            // 피어 시작 (스트림 구독)
            peer.start()

            // Answer 전송
            val answerMsg = SignalingMessage(
                type = "answer",
                streamId = streamId,
                sdp = answerSdp
            )
            sendMessage(session, answerMsg)

            logger.info { "[$sessionId] Sent answer for stream: $streamId" }
        } catch (e: Exception) {
            logger.error(e) { "[$sessionId] Error processing offer" }
            sendError(session, "Error processing offer: ${e.message}")
        }
    }

    /**
     * ICE Candidate 처리.
     */
    private fun handleCandidate(session: WebSocketSession, msg: SignalingMessage) {
        val sessionId = session.id
        val streamId = msg.streamId ?: run {
            sendError(session, "Missing streamId")
            return
        }
        val candidate = msg.candidate ?: run {
            sendError(session, "Missing candidate")
            return
        }

        logger.debug { "[$sessionId] Received ICE candidate for stream: $streamId" }

        try {
            // WebRTC Peer 조회 및 ICE candidate 추가 (sessionId + streamId)
            val peer = webrtcManager.getPeer(sessionId, streamId)
            if (peer != null) {
                peer.addIceCandidate(candidate)

                // ICE candidate 수신 확인
                val ackMsg = SignalingMessage(
                    type = "candidate_ack",
                    streamId = streamId,
                    message = "Candidate received"
                )
                sendMessage(session, ackMsg)
            } else {
                logger.warn { "[$sessionId] Peer not found for ICE candidate (stream: $streamId)" }
                sendError(session, "Peer not found")
            }
        } catch (e: Exception) {
            logger.error(e) { "[$sessionId] Error adding ICE candidate" }
            sendError(session, "Error adding candidate: ${e.message}")
        }
    }

    /**
     * 스트림 구독 처리.
     */
    private fun handleSubscribe(session: WebSocketSession, msg: SignalingMessage) {
        val sessionId = session.id
        val streamId = msg.streamId ?: run {
            sendError(session, "Missing streamId")
            return
        }

        streamSubscribers.computeIfAbsent(streamId) { ConcurrentHashMap.newKeySet() }
            .add(sessionId)

        logger.info { "[$sessionId] Subscribed to stream: $streamId" }

        val ackMsg = SignalingMessage(
            type = "subscribed",
            streamId = streamId,
            message = "Subscribed to stream: $streamId"
        )
        sendMessage(session, ackMsg)
    }

    /**
     * 스트림 구독 해제 처리.
     */
    private fun handleUnsubscribe(session: WebSocketSession, msg: SignalingMessage) {
        val sessionId = session.id
        val streamId = msg.streamId ?: run {
            sendError(session, "Missing streamId")
            return
        }

        streamSubscribers[streamId]?.remove(sessionId)

        logger.info { "[$sessionId] Unsubscribed from stream: $streamId" }

        val ackMsg = SignalingMessage(
            type = "unsubscribed",
            streamId = streamId,
            message = "Unsubscribed from stream: $streamId"
        )
        sendMessage(session, ackMsg)
    }

    /**
     * 메시지 전송.
     */
    private fun sendMessage(session: WebSocketSession, msg: SignalingMessage) {
        try {
            val json = objectMapper.writeValueAsString(msg)
            session.sendMessage(TextMessage(json))
        } catch (e: Exception) {
            logger.error(e) { "[${session.id}] Error sending message" }
        }
    }

    /**
     * 에러 메시지 전송.
     */
    private fun sendError(session: WebSocketSession, error: String) {
        val errorMsg = SignalingMessage(
            type = "error",
            message = error
        )
        sendMessage(session, errorMsg)
    }

    /**
     * 특정 스트림의 모든 구독자에게 메시지 브로드캐스트.
     */
    fun broadcast(streamId: String, msg: SignalingMessage) {
        val subscribers = streamSubscribers[streamId] ?: return

        subscribers.forEach { sessionId ->
            sessions[sessionId]?.let { session ->
                sendMessage(session, msg)
            }
        }

        logger.debug { "Broadcasted to $streamId: ${subscribers.size} subscribers" }
    }

    /**
     * 활성 세션 수 조회.
     */
    fun getSessionCount(): Int = sessions.size

    /**
     * 특정 스트림의 구독자 수 조회.
     */
    fun getSubscriberCount(streamId: String): Int {
        return streamSubscribers[streamId]?.size ?: 0
    }
}

/**
 * WebSocket 시그널링 메시지.
 *
 * @property type 메시지 타입 (offer, answer, candidate, subscribe, error 등)
 * @property streamId 스트림 식별자
 * @property sdp SDP (Session Description Protocol)
 * @property candidate ICE candidate
 * @property message 메시지 내용
 */
data class SignalingMessage(
    val type: String,
    val streamId: String? = null,
    val sdp: String? = null,
    val candidate: String? = null,
    val message: String? = null
)
