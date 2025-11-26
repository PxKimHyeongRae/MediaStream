package com.pluxity.mediaserver.config

import com.pluxity.mediaserver.controller.SignalingController
import org.springframework.context.annotation.Configuration
import org.springframework.web.socket.config.annotation.EnableWebSocket
import org.springframework.web.socket.config.annotation.WebSocketConfigurer
import org.springframework.web.socket.config.annotation.WebSocketHandlerRegistry

/**
 * WebSocket 설정.
 *
 * WebRTC 시그널링을 위한 WebSocket 엔드포인트를 등록합니다.
 *
 * **엔드포인트**:
 * - `ws://localhost:8080/ws/signaling` - WebRTC 시그널링
 *
 * **CORS**: 모든 오리진 허용 (개발용, 프로덕션에서는 제한 필요)
 */
@Configuration
@EnableWebSocket
class WebSocketConfig(
    private val signalingController: SignalingController
) : WebSocketConfigurer {

    override fun registerWebSocketHandlers(registry: WebSocketHandlerRegistry) {
        registry.addHandler(signalingController, "/ws/signaling")
            .setAllowedOrigins("*") // CORS: 모든 오리진 허용 (개발용)
    }
}
