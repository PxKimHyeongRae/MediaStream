package com.pluxity.mediaserver.common

/**
 * Base exception for all media server errors.
 * Provides structured error handling with context information.
 */
sealed class MediaServerException(
    message: String,
    cause: Throwable? = null
) : RuntimeException(message, cause)

/**
 * Exception thrown when stream operations fail.
 */
class StreamException(
    val streamId: String,
    message: String,
    cause: Throwable? = null
) : MediaServerException("Stream [$streamId]: $message", cause)

/**
 * Exception thrown when RTSP operations fail.
 */
class RTSPException(
    val url: String,
    message: String,
    cause: Throwable? = null
) : MediaServerException("RTSP [$url]: $message", cause)

/**
 * Exception thrown when WebRTC peer operations fail.
 */
class WebRTCException(
    val peerId: String,
    message: String,
    cause: Throwable? = null
) : MediaServerException("WebRTC Peer [$peerId]: $message", cause)

/**
 * Exception thrown when codec operations fail.
 */
class CodecException(
    val codecName: String,
    message: String,
    cause: Throwable? = null
) : MediaServerException("Codec [$codecName]: $message", cause)

/**
 * Exception thrown when configuration is invalid.
 */
class ConfigurationException(
    message: String,
    cause: Throwable? = null
) : MediaServerException("Configuration error: $message", cause)

/**
 * Exception thrown when resource limits are exceeded.
 */
class ResourceLimitException(
    val resourceType: String,
    val limit: Int,
    val current: Int,
    message: String
) : MediaServerException(
    "$resourceType limit exceeded: $current/$limit - $message"
)

/**
 * Exception thrown when authentication fails.
 */
class AuthenticationException(
    message: String,
    cause: Throwable? = null
) : MediaServerException("Authentication failed: $message", cause)

/**
 * Exception thrown when requested resource is not found.
 */
class NotFoundException(
    val resourceType: String,
    val resourceId: String
) : MediaServerException("$resourceType not found: $resourceId")

/**
 * Exception thrown when operation times out.
 */
class TimeoutException(
    val operation: String,
    val timeoutMs: Long,
    cause: Throwable? = null
) : MediaServerException("$operation timed out after ${timeoutMs}ms", cause)

/**
 * Exception thrown when RTP packet processing fails.
 */
class RTPPacketException(
    val sequenceNumber: Int,
    message: String,
    cause: Throwable? = null
) : MediaServerException("RTP packet [seq=$sequenceNumber]: $message", cause)
