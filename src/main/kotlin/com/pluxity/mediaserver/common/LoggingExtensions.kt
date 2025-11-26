package com.pluxity.mediaserver.common

import io.github.oshai.kotlinlogging.KLogger

/**
 * Logging extensions for common patterns in the media server.
 * Provides structured logging with context and performance measurement.
 */

/**
 * Log an exception with context information.
 *
 * @param message Error message
 * @param exception The exception to log
 * @param context Additional context key-value pairs
 */
fun KLogger.errorWithContext(
    message: String,
    exception: Throwable,
    vararg context: Pair<String, Any?>
) {
    val contextStr = context.joinToString(", ") { "${it.first}=${it.second}" }
    error(exception) { "$message | Context: $contextStr" }
}

/**
 * Log with additional context information.
 *
 * @param message Log message
 * @param context Additional context key-value pairs
 */
fun KLogger.infoWithContext(
    message: String,
    vararg context: Pair<String, Any?>
) {
    val contextStr = context.joinToString(", ") { "${it.first}=${it.second}" }
    info { "$message | $contextStr" }
}

/**
 * Log warning with context information.
 *
 * @param message Warning message
 * @param context Additional context key-value pairs
 */
fun KLogger.warnWithContext(
    message: String,
    vararg context: Pair<String, Any?>
) {
    val contextStr = context.joinToString(", ") { "${it.first}=${it.second}" }
    warn { "$message | $contextStr" }
}

/**
 * Measure execution time of a block and log it.
 *
 * @param operationName Name of the operation being measured
 * @param block The code block to measure
 * @return Result of the block execution
 */
inline fun <T> KLogger.measureTime(operationName: String, block: () -> T): T {
    val startTime = System.nanoTime()
    return try {
        block().also {
            val elapsedMs = (System.nanoTime() - startTime) / 1_000_000
            debug { "$operationName completed in ${elapsedMs}ms" }
        }
    } catch (e: Exception) {
        val elapsedMs = (System.nanoTime() - startTime) / 1_000_000
        error(e) { "$operationName failed after ${elapsedMs}ms" }
        throw e
    }
}

/**
 * Log stream lifecycle events with consistent formatting.
 *
 * @param streamId Stream identifier
 * @param event Event type (e.g., "started", "stopped", "error")
 * @param additionalInfo Optional additional information
 */
fun KLogger.logStreamEvent(
    streamId: String,
    event: String,
    additionalInfo: String? = null
) {
    val message = if (additionalInfo != null) {
        "Stream [$streamId] $event: $additionalInfo"
    } else {
        "Stream [$streamId] $event"
    }
    info { message }
}

/**
 * Log peer connection events with consistent formatting.
 *
 * @param peerId Peer identifier
 * @param event Event type (e.g., "connected", "disconnected", "failed")
 * @param additionalInfo Optional additional information
 */
fun KLogger.logPeerEvent(
    peerId: String,
    event: String,
    additionalInfo: String? = null
) {
    val message = if (additionalInfo != null) {
        "Peer [$peerId] $event: $additionalInfo"
    } else {
        "Peer [$peerId] $event"
    }
    info { message }
}

/**
 * Log RTP packet events for debugging.
 *
 * @param streamId Stream identifier
 * @param sequenceNumber RTP sequence number
 * @param timestamp RTP timestamp
 * @param payloadSize Payload size in bytes
 */
fun KLogger.logRTPPacket(
    streamId: String,
    sequenceNumber: Int,
    timestamp: Long,
    payloadSize: Int
) {
    trace { "RTP [$streamId] seq=$sequenceNumber ts=$timestamp size=$payloadSize" }
}

/**
 * Log media codec information.
 *
 * @param streamId Stream identifier
 * @param codecName Codec name (e.g., "H264", "H265", "AAC")
 * @param codecParams Additional codec parameters
 */
fun KLogger.logCodecInfo(
    streamId: String,
    codecName: String,
    vararg codecParams: Pair<String, Any?>
) {
    val params = codecParams.joinToString(", ") { "${it.first}=${it.second}" }
    info { "Codec [$streamId] $codecName | $params" }
}
