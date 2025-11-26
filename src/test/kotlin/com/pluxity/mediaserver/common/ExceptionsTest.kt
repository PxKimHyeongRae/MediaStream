package com.pluxity.mediaserver.common

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*

class ExceptionsTest {

    @Test
    fun `test StreamException`() {
        val exception = StreamException("stream123", "Connection failed")

        assertEquals("Stream [stream123]: Connection failed", exception.message)
        assertEquals("stream123", exception.streamId)
        assertNull(exception.cause)
    }

    @Test
    fun `test StreamException with cause`() {
        val cause = RuntimeException("Network error")
        val exception = StreamException("stream123", "Connection failed", cause)

        assertEquals("Stream [stream123]: Connection failed", exception.message)
        assertEquals(cause, exception.cause)
    }

    @Test
    fun `test RTSPException`() {
        val url = "rtsp://example.com/stream"
        val exception = RTSPException(url, "Authentication failed")

        assertEquals("RTSP [$url]: Authentication failed", exception.message)
        assertEquals(url, exception.url)
    }

    @Test
    fun `test WebRTCException`() {
        val exception = WebRTCException("peer456", "ICE connection failed")

        assertEquals("WebRTC Peer [peer456]: ICE connection failed", exception.message)
        assertEquals("peer456", exception.peerId)
    }

    @Test
    fun `test CodecException`() {
        val exception = CodecException("H265", "Decoder initialization failed")

        assertEquals("Codec [H265]: Decoder initialization failed", exception.message)
        assertEquals("H265", exception.codecName)
    }

    @Test
    fun `test ConfigurationException`() {
        val exception = ConfigurationException("Missing required property: server.port")

        assertEquals("Configuration error: Missing required property: server.port", exception.message)
    }

    @Test
    fun `test ResourceLimitException`() {
        val exception = ResourceLimitException(
            resourceType = "connections",
            limit = 100,
            current = 101,
            message = "Too many concurrent connections"
        )

        assertEquals(
            "connections limit exceeded: 101/100 - Too many concurrent connections",
            exception.message
        )
        assertEquals("connections", exception.resourceType)
        assertEquals(100, exception.limit)
        assertEquals(101, exception.current)
    }

    @Test
    fun `test AuthenticationException`() {
        val exception = AuthenticationException("Invalid credentials")

        assertEquals("Authentication failed: Invalid credentials", exception.message)
    }

    @Test
    fun `test NotFoundException`() {
        val exception = NotFoundException("stream", "stream123")

        assertEquals("stream not found: stream123", exception.message)
        assertEquals("stream", exception.resourceType)
        assertEquals("stream123", exception.resourceId)
    }

    @Test
    fun `test TimeoutException`() {
        val exception = TimeoutException("RTSP connection", 5000)

        assertEquals("RTSP connection timed out after 5000ms", exception.message)
        assertEquals("RTSP connection", exception.operation)
        assertEquals(5000, exception.timeoutMs)
    }

    @Test
    fun `test RTPPacketException`() {
        val exception = RTPPacketException(12345, "Invalid payload size")

        assertEquals("RTP packet [seq=12345]: Invalid payload size", exception.message)
        assertEquals(12345, exception.sequenceNumber)
    }

    @Test
    fun `test exception inheritance`() {
        val exception: MediaServerException = StreamException("stream1", "Test")

        assertTrue(exception is RuntimeException)
        assertTrue(exception is MediaServerException)
    }
}
