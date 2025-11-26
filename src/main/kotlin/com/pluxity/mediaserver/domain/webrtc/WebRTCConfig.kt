package com.pluxity.mediaserver.domain.webrtc

import org.springframework.boot.context.properties.ConfigurationProperties

/**
 * WebRTC 설정.
 *
 * application.yaml의 `media-server.webrtc` 섹션과 바인딩됩니다.
 *
 * @property iceServers STUN/TURN 서버 목록
 * @property settings WebRTC 설정
 */
@ConfigurationProperties(prefix = "media-server.webrtc")
data class WebRTCConfig(
    var iceServers: List<IceServerConfig> = emptyList(),
    var settings: WebRTCSettings = WebRTCSettings(),
    var stunServers: List<String> = listOf("stun.l.google.com:19302")
)

/**
 * ICE 서버 설정.
 *
 * @property urls STUN/TURN 서버 URL 목록
 * @property username 인증 사용자명 (TURN 서버용, 선택적)
 * @property credential 인증 비밀번호 (TURN 서버용, 선택적)
 */
data class IceServerConfig(
    var urls: List<String> = emptyList(),
    var username: String? = null,
    var credential: String? = null
)

/**
 * WebRTC 설정값.
 *
 * @property maxPeers 최대 동시 피어 수
 */
data class WebRTCSettings(
    var maxPeers: Int = 1000
)
