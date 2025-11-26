package com.pluxity.mediaserver.domain.rtsp

import org.springframework.boot.context.properties.ConfigurationProperties
import org.springframework.stereotype.Component

/**
 * RTSP 클라이언트 설정.
 *
 * @property transport RTSP 전송 프로토콜 (tcp 또는 udp)
 * @property bufferSize 수신 버퍼 크기 (바이트)
 * @property maxDelay 최대 지연 시간 (마이크로초)
 * @property reconnectDelay 재연결 대기 시간 (밀리초)
 * @property maxReconnectAttempts 최대 재연결 시도 횟수
 * @property readTimeout 읽기 타임아웃 (밀리초)
 */
@Component
@ConfigurationProperties(prefix = "media-server.rtsp")
data class RTSPConfig(
    var transport: String = "tcp",
    var bufferSize: Int = 1024000,
    var maxDelay: Int = 500000,
    var reconnectDelay: Long = 5000,
    var maxReconnectAttempts: Int = 3,
    var readTimeout: Long = 10000
)
