package com.pluxity.mediaserver.domain.stream

import io.netty.buffer.Unpooled
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.assertThrows

class RTPPacketTest {

    @Test
    fun `should create RTPPacket with valid data`() {
        val payloadData = byteArrayOf(1, 2, 3, 4, 5)
        val packet = RTPPacket.create(
            streamId = "test-stream",
            sequenceNumber = 100,
            timestamp = 123456L,
            payloadData = payloadData
        )

        try {
            assertEquals("test-stream", packet.streamId)
            assertEquals(100, packet.header.sequenceNumber)
            assertEquals(123456L, packet.header.timestamp)
            assertEquals(2, packet.header.version)
            assertEquals(96, packet.header.payloadType)
            assertEquals(5, packet.payload.readableBytes())
        } finally {
            packet.release()
        }
    }

    @Test
    fun `should parse RTPPacket from ByteArray`() {
        // RTP 헤더 생성 (12 bytes)
        val header = byteArrayOf(
            0x80.toByte(),  // V=2, P=0, X=0, CC=0
            0x60.toByte(),  // M=0, PT=96
            0x00, 0x64,     // Sequence = 100
            0x00, 0x01, 0xE2.toByte(), 0x40.toByte(),  // Timestamp = 123456
            0x12, 0x34, 0x56, 0x78  // SSRC
        )

        val payload = byteArrayOf(1, 2, 3, 4, 5)
        val data = header + payload

        val packet = RTPPacket.fromByteArray("test-stream", data)

        try {
            assertEquals("test-stream", packet.streamId)
            assertEquals(2, packet.header.version)
            assertEquals(96, packet.header.payloadType)
            assertEquals(100, packet.header.sequenceNumber)
            assertEquals(123456L, packet.header.timestamp)
            assertEquals(5, packet.payload.readableBytes())

            // 페이로드 내용 확인
            val readPayload = ByteArray(5)
            packet.payload.getBytes(packet.payload.readerIndex(), readPayload)
            assertArrayEquals(payload, readPayload)
        } finally {
            packet.release()
        }
    }

    @Test
    fun `should throw exception for too small data`() {
        val tooSmallData = byteArrayOf(1, 2, 3)  // Less than 12 bytes

        assertThrows<IllegalArgumentException> {
            RTPPacket.fromByteArray("test", tooSmallData)
        }
    }

    @Test
    fun `should handle marker bit correctly`() {
        val packet = RTPPacket.create(
            streamId = "test",
            sequenceNumber = 1,
            timestamp = 1000L,
            payloadData = byteArrayOf(1, 2, 3),
            marker = true
        )

        try {
            assertTrue(packet.header.marker)
        } finally {
            packet.release()
        }
    }

    @Test
    fun `should calculate total size correctly`() {
        val payloadData = byteArrayOf(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
        val packet = RTPPacket.create(
            streamId = "test",
            sequenceNumber = 1,
            timestamp = 1000L,
            payloadData = payloadData
        )

        try {
            // 12 (header) + 10 (payload) = 22
            assertEquals(22, packet.totalSize)
        } finally {
            packet.release()
        }
    }

    @Test
    fun `should copy packet independently`() {
        val original = RTPPacket.create(
            streamId = "test",
            sequenceNumber = 100,
            timestamp = 123456L,
            payloadData = byteArrayOf(1, 2, 3, 4, 5)
        )

        try {
            val copy = original.copy()

            try {
                // 내용 같음
                assertEquals(original.streamId, copy.streamId)
                assertEquals(original.header.sequenceNumber, copy.header.sequenceNumber)
                assertEquals(original.header.timestamp, copy.header.timestamp)

                // 독립적인 ByteBuf
                assertNotSame(original.payload, copy.payload)

                // 페이로드 내용 같음
                val originalBytes = ByteArray(original.payload.readableBytes())
                val copyBytes = ByteArray(copy.payload.readableBytes())

                original.payload.getBytes(original.payload.readerIndex(), originalBytes)
                copy.payload.getBytes(copy.payload.readerIndex(), copyBytes)

                assertArrayEquals(originalBytes, copyBytes)
            } finally {
                copy.release()
            }
        } finally {
            original.release()
        }
    }

    @Test
    fun `should release ByteBuf correctly`() {
        val packet = RTPPacket.create(
            streamId = "test",
            sequenceNumber = 1,
            timestamp = 1000L,
            payloadData = byteArrayOf(1, 2, 3)
        )

        assertEquals(1, packet.payload.refCnt())

        packet.release()

        assertEquals(0, packet.payload.refCnt())

        // 중복 release는 안전해야 함
        packet.release()
        assertEquals(0, packet.payload.refCnt())
    }

    @Test
    fun `should parse packet from ByteBuf`() {
        val buffer = Unpooled.buffer()

        try {
            // RTP 헤더 작성
            buffer.writeByte(0x80)  // V=2, P=0, X=0, CC=0
            buffer.writeByte(0x60)  // M=0, PT=96
            buffer.writeShort(100)   // Sequence
            buffer.writeInt(123456)  // Timestamp
            buffer.writeInt(0x12345678)  // SSRC

            // 페이로드
            buffer.writeBytes(byteArrayOf(1, 2, 3, 4, 5))

            val packet = RTPPacket.fromByteBuf("test", buffer)

            try {
                assertEquals("test", packet.streamId)
                assertEquals(100, packet.header.sequenceNumber)
                assertEquals(123456L, packet.header.timestamp)
                assertEquals(5, packet.payload.readableBytes())
            } finally {
                packet.release()
            }
        } finally {
            buffer.release()
        }
    }

    @Test
    fun `should have proper toString format`() {
        val packet = RTPPacket.create(
            streamId = "test-stream",
            sequenceNumber = 100,
            timestamp = 123456L,
            payloadData = byteArrayOf(1, 2, 3, 4, 5)
        )

        try {
            val str = packet.toString()

            assertTrue(str.contains("test-stream"))
            assertTrue(str.contains("seq=100"))
            assertTrue(str.contains("ts=123456"))
            assertTrue(str.contains("pt=96"))
            assertTrue(str.contains("payloadSize=5"))
        } finally {
            packet.release()
        }
    }

    @Test
    fun `should equals work correctly`() {
        val packet1 = RTPPacket.create(
            streamId = "test",
            sequenceNumber = 100,
            timestamp = 1000L,
            payloadData = byteArrayOf(1, 2, 3)
        )

        val packet2 = RTPPacket.create(
            streamId = "test",
            sequenceNumber = 100,
            timestamp = 1000L,
            payloadData = byteArrayOf(1, 2, 3)
        )

        try {
            assertEquals(packet1, packet2)
            assertEquals(packet1.hashCode(), packet2.hashCode())
        } finally {
            packet1.release()
            packet2.release()
        }
    }
}
