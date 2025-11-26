package com.pluxity.mediaserver.config

import org.springframework.boot.context.properties.ConfigurationProperties

@ConfigurationProperties(prefix = "media")
data class MediaServerProperties(
    val rtsp: RtspConfig = RtspConfig(),
    val webrtc: WebRtcConfig = WebRtcConfig(),
    val hls: HlsConfig = HlsConfig(),
    val performance: PerformanceConfig = PerformanceConfig()
)

data class RtspConfig(
    val pool: PoolConfig = PoolConfig(),
    val transport: String = "tcp",
    val reconnect: ReconnectConfig = ReconnectConfig()
)

data class PoolConfig(
    val maxStreams: Int = 100
)

data class ReconnectConfig(
    val enabled: Boolean = true,
    val maxAttempts: Int = 5,
    val delayMs: Long = 5000
)

data class WebRtcConfig(
    val iceServers: List<IceServerConfig> = emptyList(),
    val settings: WebRtcSettingsConfig = WebRtcSettingsConfig()
)

data class IceServerConfig(
    val urls: List<String> = emptyList()
)

data class WebRtcSettingsConfig(
    val maxPeers: Int = 1000
)

data class HlsConfig(
    val enabled: Boolean = true,
    val outputDir: String = "hls",
    val segmentDuration: Int = 6,
    val playlistLength: Int = 5
)

data class PerformanceConfig(
    val gcPercent: Int = 100
)

@ConfigurationProperties(prefix = "streams")
data class StreamsProperties(
    val streams: Map<String, StreamConfig> = emptyMap()
)

data class StreamConfig(
    val source: String,
    val sourceOnDemand: Boolean = true,
    val rtspTransport: String = "tcp"
)
