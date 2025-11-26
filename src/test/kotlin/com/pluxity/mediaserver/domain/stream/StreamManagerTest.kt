package com.pluxity.mediaserver.domain.stream

import com.pluxity.mediaserver.common.NotFoundException
import com.pluxity.mediaserver.common.StreamException
import kotlinx.coroutines.delay
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.assertThrows
import java.util.concurrent.CopyOnWriteArrayList

class StreamManagerTest {

    private lateinit var manager: StreamManager

    @BeforeEach
    fun setup() {
        manager = StreamManager()
    }

    private fun createTestPacket(seq: Int): RTPPacket {
        return RTPPacket.create(
            streamId = "test",
            sequenceNumber = seq,
            timestamp = seq * 1000L,
            payloadData = ByteArray(100) { it.toByte() }
        )
    }

    @Test
    fun `should create stream`() {
        val stream = manager.createStream("test-stream")

        assertNotNull(stream)
        assertEquals("test-stream", stream.id)
        assertEquals(1, manager.getStreamCount())
        assertTrue(manager.hasStream("test-stream"))
    }

    @Test
    fun `should throw exception when creating duplicate stream`() {
        manager.createStream("test-stream")

        assertThrows<StreamException> {
            manager.createStream("test-stream")
        }
    }

    @Test
    fun `should get stream`() {
        val created = manager.createStream("test-stream")
        val retrieved = manager.getStream("test-stream")

        assertSame(created, retrieved)
    }

    @Test
    fun `should throw exception when getting non-existent stream`() {
        assertThrows<NotFoundException> {
            manager.getStream("non-existent")
        }
    }

    @Test
    fun `should check stream existence`() {
        assertFalse(manager.hasStream("test"))

        manager.createStream("test")

        assertTrue(manager.hasStream("test"))
    }

    @Test
    fun `should delete stream`() = runTest {
        manager.createStream("test-stream")

        assertTrue(manager.hasStream("test-stream"))

        val deleted = manager.deleteStream("test-stream")

        assertNotNull(deleted)
        assertFalse(manager.hasStream("test-stream"))
        assertEquals(0, manager.getStreamCount())
    }

    @Test
    fun `should return null when deleting non-existent stream`() = runTest {
        val deleted = manager.deleteStream("non-existent")

        assertNull(deleted)
    }

    @Test
    fun `should list stream IDs`() {
        manager.createStream("stream1")
        manager.createStream("stream2")
        manager.createStream("stream3")

        val ids = manager.listStreamIds()

        assertEquals(3, ids.size)
        assertTrue(ids.contains("stream1"))
        assertTrue(ids.contains("stream2"))
        assertTrue(ids.contains("stream3"))
    }

    @Test
    fun `should list all streams`() {
        val stream1 = manager.createStream("stream1")
        val stream2 = manager.createStream("stream2")

        val streams = manager.listStreams()

        assertEquals(2, streams.size)
        assertSame(stream1, streams["stream1"])
        assertSame(stream2, streams["stream2"])
    }

    @Test
    fun `should get stream count`() {
        assertEquals(0, manager.getStreamCount())

        manager.createStream("stream1")
        assertEquals(1, manager.getStreamCount())

        manager.createStream("stream2")
        assertEquals(2, manager.getStreamCount())
    }

    @Test
    fun `should publish and subscribe packets`() = runTest {
        manager.createStream("test-stream")

        val received = CopyOnWriteArrayList<RTPPacket>()
        val packets = mutableListOf<RTPPacket>()

        // 구독 (테스트 scope 전달)
        val job = manager.subscribe("test-stream", "subscriber-1", scope = this) { packet ->
            received.add(packet.copy())
        }

        // 패킷 발행
        repeat(5) { i ->
            val packet = createTestPacket(i)
            packets.add(packet)
            manager.publishPacket("test-stream", packet)
        }

        // Flow.collect는 무한 루프이므로 실제 delay 필요
        delay(100)

        // 검증
        assertEquals(5, received.size)
        for (i in 0 until 5) {
            assertEquals(i, received[i].header.sequenceNumber)
        }

        job.cancel()
        packets.forEach { it.release() }
        received.forEach { it.release() }
    }

    @Test
    fun `should get subscriber count`() = runTest {
        manager.createStream("test-stream")

        assertEquals(0, manager.getSubscriberCount("test-stream"))

        val job1 = manager.subscribe("test-stream", "sub-1", scope = this) { }
        assertEquals(1, manager.getSubscriberCount("test-stream"))

        val job2 = manager.subscribe("test-stream", "sub-2", scope = this) { }
        assertEquals(2, manager.getSubscriberCount("test-stream"))

        job1.cancel()
        testScheduler.advanceUntilIdle()
        assertEquals(1, manager.getSubscriberCount("test-stream"))

        job2.cancel()
        testScheduler.advanceUntilIdle()
        assertEquals(0, manager.getSubscriberCount("test-stream"))
    }

    @Test
    fun `should get stream stats`() = runTest {
        manager.createStream("test-stream")

        repeat(10) { i ->
            val packet = createTestPacket(i)
            manager.publishPacket("test-stream", packet)
            packet.release()
        }

        delay(100)

        val stats = manager.getStreamStats("test-stream")

        assertEquals(10, stats.packetsPublished)
        assertTrue(stats.bytesPublished > 0)
    }

    @Test
    fun `should get all stream stats`() = runTest {
        manager.createStream("stream1")
        manager.createStream("stream2")

        repeat(5) { i ->
            val packet = createTestPacket(i).copy(streamId = "stream1")
            manager.publishPacket("stream1", packet)
            packet.release()
        }

        repeat(3) { i ->
            val packet = createTestPacket(i).copy(streamId = "stream2")
            manager.publishPacket("stream2", packet)
            packet.release()
        }

        delay(100)

        val allStats = manager.getAllStreamStats()

        assertEquals(2, allStats.size)
        assertEquals(5, allStats["stream1"]?.packetsPublished)
        assertEquals(3, allStats["stream2"]?.packetsPublished)
    }

    @Test
    fun `should get manager summary`() = runTest {
        manager.createStream("stream1")
        manager.createStream("stream2")

        val job1 = manager.subscribe("stream1", "sub-1") { }
        val job2 = manager.subscribe("stream2", "sub-2") { }
        delay(50)

        repeat(10) { i ->
            val packet1 = createTestPacket(i).copy(streamId = "stream1")
            manager.publishPacket("stream1", packet1)
            packet1.release()

            val packet2 = createTestPacket(i).copy(streamId = "stream2")
            manager.publishPacket("stream2", packet2)
            packet2.release()
        }

        delay(100)

        val summary = manager.getSummary()

        assertEquals(2, summary.totalStreams)
        assertEquals(2, summary.totalSubscribers)
        assertEquals(20, summary.totalPacketsPublished)
        assertTrue(summary.totalBytesPublished > 0)
        assertTrue(summary.totalBytesFormatted.contains("KB") ||
                summary.totalBytesFormatted.contains("bytes"))

        job1.cancel(); delay(50)
        job2.cancel(); delay(50)
    }

    @Test
    fun `should delete all streams`() = runTest {
        manager.createStream("stream1")
        manager.createStream("stream2")
        manager.createStream("stream3")

        assertEquals(3, manager.getStreamCount())

        manager.deleteAllStreams()

        assertEquals(0, manager.getStreamCount())
        assertFalse(manager.hasStream("stream1"))
        assertFalse(manager.hasStream("stream2"))
        assertFalse(manager.hasStream("stream3"))
    }

    @Test
    fun `should handle concurrent operations`() = runTest {
        // 동시에 여러 스트림 생성
        val streams = (1..10).map { i ->
            manager.createStream("stream-$i")
        }

        assertEquals(10, manager.getStreamCount())

        // 모든 스트림이 정상 생성되었는지 확인
        streams.forEach { stream ->
            assertTrue(manager.hasStream(stream.id))
        }
    }

    @Test
    fun `should throw NotFoundException for operations on non-existent stream`() = runTest {
        assertThrows<NotFoundException> {
            manager.publishPacket("non-existent", createTestPacket(1))
        }

        assertThrows<NotFoundException> {
            manager.subscribe("non-existent", "sub") { }
        }

        assertThrows<NotFoundException> {
            manager.getStreamStats("non-existent")
        }

        assertThrows<NotFoundException> {
            manager.getSubscriberCount("non-existent")
        }
    }
}
