package com.pluxity.mediaserver.config

import org.springframework.boot.context.properties.ConfigurationProperties
import org.springframework.stereotype.Component

/**
 * 스트림 설정 (application.yaml의 media-streams 섹션).
 *
 * mediaMTX 스타일의 스트림 경로 설정을 지원합니다.
 *
 * ```yaml
 * media-streams:
 *   paths:
 *     CCTV-TEST1:
 *       source: "rtsp://..."
 *       sourceOnDemand: true
 *       rtspTransport: tcp
 * ```
 */
@Component
@ConfigurationProperties(prefix = "media-streams")
class StreamsConfig {
    /**
     * 스트림 ID → StreamConfig 맵.
     */
    val paths: MutableMap<String, StreamConfig> = mutableMapOf()

    /**
     * 개별 스트림 설정.
     *
     * @property source RTSP URL
     * @property sourceOnDemand true이면 클라이언트 연결 시에만 시작
     * @property rtspTransport RTSP transport (tcp, udp)
     */
    data class StreamConfig(
        var source: String = "",
        var sourceOnDemand: Boolean = true,
        var rtspTransport: String = "tcp"
    )
}
