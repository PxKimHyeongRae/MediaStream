package com.pluxity.mediaserver.config

import com.pluxity.mediaserver.domain.rtsp.RTSPManager
import com.pluxity.mediaserver.domain.stream.StreamManager
import io.github.oshai.kotlinlogging.KotlinLogging
import org.springframework.boot.CommandLineRunner
import org.springframework.stereotype.Component

private val logger = KotlinLogging.logger {}

/**
 * 애플리케이션 시작 시 스트림 초기화.
 *
 * application.yaml의 streams 설정을 읽어:
 * 1. 모든 스트림을 StreamManager에 등록 (사용 가능한 스트림 목록)
 * 2. `source-on-demand: false`인 스트림은 즉시 RTSP 연결 시작
 */
@Component
class StreamInitializer(
    private val streamsConfig: StreamsConfig,
    private val streamManager: StreamManager,
    private val rtspManager: RTSPManager
) : CommandLineRunner {

    override fun run(vararg args: String?) {
        logger.info { "Initializing streams from configuration..." }

        val allStreams = streamsConfig.paths
        if (allStreams.isEmpty()) {
            logger.info { "No streams configured" }
            return
        }

        logger.info { "Found ${allStreams.size} configured streams" }

        // 1. 모든 스트림을 StreamManager에 등록
        allStreams.forEach { (streamId, config) ->
            try {
                streamManager.getOrCreateStream(streamId)
                logger.info { "Registered stream: $streamId (onDemand: ${config.sourceOnDemand})" }
            } catch (e: Exception) {
                logger.error(e) { "Failed to register stream: $streamId" }
            }
        }

        // 2. on-demand가 아닌 스트림은 즉시 RTSP 시작
        val autoStartStreams = allStreams.filterValues { !it.sourceOnDemand }

        if (autoStartStreams.isEmpty()) {
            logger.info { "All ${allStreams.size} streams are on-demand (will connect when requested)" }
        } else {
            logger.info { "Auto-starting ${autoStartStreams.size} streams..." }

            autoStartStreams.forEach { (streamId, config) ->
                try {
                    logger.info { "Starting RTSP: $streamId -> ${config.source}" }
                    rtspManager.startClient(streamId, config.source)
                    logger.info { "RTSP started: $streamId" }
                } catch (e: Exception) {
                    logger.error(e) { "Failed to start RTSP: $streamId" }
                }
            }
        }

        logger.info { "Stream initialization completed (${allStreams.size} registered, ${autoStartStreams.size} auto-started)" }
    }
}
