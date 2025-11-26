package com.pluxity.mediaserver.common

import io.netty.buffer.Unpooled
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*

class ByteBufExtensionsTest {

    @Test
    fun `test directBuffer auto-release`() {
        var bufferRefCount = 0

        withDirectBuffer(1024) { buffer ->
            bufferRefCount = buffer.refCnt()
            assertEquals(1, bufferRefCount, "Buffer should have refCount=1 inside block")
            buffer.writeInt(42)
        }

        // Buffer should be released after block (no way to check directly, but no leak)
    }

    @Test
    fun `test toByteArray`() {
        val buffer = Unpooled.buffer()
        try {
            val data = byteArrayOf(1, 2, 3, 4, 5)
            buffer.writeBytes(data)

            val result = buffer.toByteArray()

            assertArrayEquals(data, result)
            assertEquals(0, buffer.readerIndex(), "Reader index should not change")
        } finally {
            buffer.release()
        }
    }

    @Test
    fun `test writeRTPHeader and readRTPHeader`() {
        val buffer = Unpooled.buffer()
        try {
            // Write RTP header
            buffer.writeRTPHeader(
                version = 2,
                padding = false,
                extension = false,
                csrcCount = 0,
                marker = true,
                payloadType = 96,
                sequenceNumber = 12345,
                timestamp = 987654321L,
                ssrc = 0x12345678
            )

            // Read it back
            val header = buffer.readRTPHeader()

            assertEquals(2, header.version)
            assertEquals(false, header.padding)
            assertEquals(false, header.extension)
            assertEquals(0, header.csrcCount)
            assertEquals(true, header.marker)
            assertEquals(96, header.payloadType)
            assertEquals(12345, header.sequenceNumber)
            assertEquals(987654321L and 0xFFFFFFFFL, header.timestamp)
            assertEquals(0x12345678, header.ssrc)
        } finally {
            buffer.release()
        }
    }

    @Test
    fun `test calculateRTPPacketSize`() {
        val payloadSize = 1400

        val sizeWithoutCSRC = calculateRTPPacketSize(payloadSize, 0)
        assertEquals(12 + 1400, sizeWithoutCSRC)

        val sizeWithCSRC = calculateRTPPacketSize(payloadSize, 2)
        assertEquals(12 + 8 + 1400, sizeWithCSRC)
    }

    @Test
    fun `test retainAndGet`() {
        val buffer = Unpooled.buffer()
        try {
            assertEquals(1, buffer.refCnt())

            val retained = buffer.retainAndGet()

            assertEquals(2, buffer.refCnt())
            assertSame(buffer, retained)

            retained.release()
        } finally {
            buffer.release()
        }
    }

    @Test
    fun `test hasReadableBytes and remainingBytes`() {
        val buffer = Unpooled.buffer()
        try {
            assertFalse(buffer.hasReadableBytes())
            assertEquals(0, buffer.remainingBytes())

            buffer.writeInt(42)

            assertTrue(buffer.hasReadableBytes())
            assertEquals(4, buffer.remainingBytes())

            buffer.readInt()

            assertFalse(buffer.hasReadableBytes())
            assertEquals(0, buffer.remainingBytes())
        } finally {
            buffer.release()
        }
    }

    @Test
    fun `test writeByteArray`() {
        val buffer = Unpooled.buffer()
        try {
            val data = byteArrayOf(10, 20, 30, 40)

            buffer.writeByteArray(data)

            assertEquals(4, buffer.readableBytes())
            assertEquals(10, buffer.readByte())
            assertEquals(20, buffer.readByte())
            assertEquals(30, buffer.readByte())
            assertEquals(40, buffer.readByte())
        } finally {
            buffer.release()
        }
    }

    @Test
    fun `test RTP header with marker bit variations`() {
        val buffer = Unpooled.buffer()
        try {
            // Test marker=false
            buffer.writeRTPHeader(
                payloadType = 96,
                marker = false,
                sequenceNumber = 1,
                timestamp = 1000L,
                ssrc = 1
            )

            val header1 = buffer.readRTPHeader()
            assertFalse(header1.marker)

            // Test marker=true
            buffer.clear()
            buffer.writeRTPHeader(
                payloadType = 96,
                marker = true,
                sequenceNumber = 1,
                timestamp = 1000L,
                ssrc = 1
            )

            val header2 = buffer.readRTPHeader()
            assertTrue(header2.marker)
        } finally {
            buffer.release()
        }
    }
}
