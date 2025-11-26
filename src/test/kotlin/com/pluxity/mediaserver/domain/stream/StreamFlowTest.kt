package com.pluxity.mediaserver.domain.stream

import kotlinx.coroutines.*
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*
import java.util.concurrent.CopyOnWriteArrayList

class StreamFlowTest {

    private fun createTestPacket(seq: Int): RTPPacket {
        return RTPPacket.create(
            streamId = "test",
            sequenceNumber = seq,
            timestamp = seq * 1000L,
            payloadData = ByteArray(100) { it.toByte() }
        )
    }

    @Test
    fun `should publish and collect packets`() = runTest {
        val flow = StreamFlow("test-stream")
        val received = CopyOnWriteArrayList<RTPPacket>()
        val packets = mutableListOf<RTPPacket>()

        // 구독자 시작 (기본 Dispatchers.IO 사용)
        val job = flow.subscribe("subscriber-1") { packet ->
            received.add(packet.copy())  // copy해서 저장 (원본은 release됨)
        }

        // 구독자가 시작할 시간 대기
        delay(50)

        // 패킷 발행
        repeat(10) { i ->
            val packet = createTestPacket(i)
            packets.add(packet)
            flow.publish(packet)
        }

        // 패킷이 처리될 시간 대기
        delay(200)

        // 검증
        assertEquals(10, received.size)
        for (i in 0 until 10) {
            assertEquals(i, received[i].header.sequenceNumber)
        }

        // 정리
        job.cancel()
        delay(50)
        packets.forEach { it.release() }
        received.forEach { it.release() }
    }

    @Test
    fun `should track subscriber count`() = runTest {
        val flow = StreamFlow("test")

        assertEquals(0, flow.subscriberCount.value)

        val job1 = flow.subscribe("sub-1", scope = this) { }
        assertEquals(1, flow.subscriberCount.value)

        val job2 = flow.subscribe("sub-2", scope = this) { }
        assertEquals(2, flow.subscriberCount.value)

        job1.cancel()
        testScheduler.advanceUntilIdle()
        assertEquals(1, flow.subscriberCount.value)

        job2.cancel()
        testScheduler.advanceUntilIdle()
        assertEquals(0, flow.subscriberCount.value)
    }

    @Test
    fun `should broadcast to multiple subscribers`() = runTest {
        val flow = StreamFlow("test")

        val received1 = CopyOnWriteArrayList<Int>()
        val received2 = CopyOnWriteArrayList<Int>()
        val received3 = CopyOnWriteArrayList<Int>()
        val packets = mutableListOf<RTPPacket>()

        // 3개 구독자
        val job1 = flow.subscribe("sub-1", scope = this) { packet ->
            received1.add(packet.header.sequenceNumber)
        }

        val job2 = flow.subscribe("sub-2", scope = this) { packet ->
            received2.add(packet.header.sequenceNumber)
        }

        val job3 = flow.subscribe("sub-3", scope = this) { packet ->
            received3.add(packet.header.sequenceNumber)
        }

        // 패킷 발행
        repeat(5) { i ->
            val packet = createTestPacket(i)
            packets.add(packet)
            flow.publish(packet)
        }

        // Flow.collect는 무한 루프이므로 실제 delay 필요
        delay(100)

        // 모든 구독자가 동일하게 수신
        assertEquals(5, received1.size)
        assertEquals(5, received2.size)
        assertEquals(5, received3.size)

        assertEquals(received1, received2)
        assertEquals(received2, received3)

        job1.cancel()
        job2.cancel()
        job3.cancel()
        packets.forEach { it.release() }
    }

    @Test
    fun `should collect stats correctly`() = runTest {
        val flow = StreamFlow("test")

        val job = flow.subscribe("sub-1", scope = this) { /* consume */ }

        // 패킷 발행
        repeat(10) { i ->
            val packet = createTestPacket(i)
            flow.publish(packet)
            packet.release()
        }

        delay(100)

        val stats = flow.getStats()

        assertEquals(10, stats.packetsPublished)
        assertTrue(stats.packetsDelivered >= 10)  // 구독자가 받음
        assertTrue(stats.bytesPublished > 0)
        assertTrue(stats.avgBitrate > 0)

        job.cancel(); delay(50)
    }

    @Test
    fun `should handle subscriber errors gracefully`() = runTest {
        val flow = StreamFlow("test")

        val received = CopyOnWriteArrayList<Int>()
        var errorCount = 0
        val packets = mutableListOf<RTPPacket>()

        // 에러를 발생시키는 구독자
        val job = flow.subscribe("error-sub", scope = this) { packet ->
            if (packet.header.sequenceNumber == 5) {
                errorCount++
                throw RuntimeException("Intentional error")
            }
            received.add(packet.header.sequenceNumber)
        }

        // 패킷 발행
        repeat(10) { i ->
            val packet = createTestPacket(i)
            packets.add(packet)
            flow.publish(packet)
        }

        // Flow.collect는 무한 루프이므로 실제 delay 필요
        delay(100)

        // 에러가 발생했지만 다른 패킷은 처리됨
        assertEquals(1, errorCount)
        assertEquals(9, received.size)  // 5번 제외하고 9개

        job.cancel()
        packets.forEach { it.release() }
    }

    @Test
    fun `should work with no subscribers`() = runTest {
        val flow = StreamFlow("test")

        // 구독자 없이 패킷 발행
        repeat(5) { i ->
            val packet = createTestPacket(i)
            flow.publish(packet)
            packet.release()
        }

        // 예외 없이 정상 동작
        val stats = flow.getStats()
        assertEquals(5, stats.packetsPublished)
        assertEquals(0, stats.packetsDelivered)
    }

    @Test
    fun `should handle late subscriber`() = runTest {
        val flow = StreamFlow("test")
        val packets = mutableListOf<RTPPacket>()

        // 먼저 패킷 발행
        repeat(5) { i ->
            val packet = createTestPacket(i)
            packets.add(packet)
            flow.publish(packet)
        }

        val received = CopyOnWriteArrayList<Int>()

        // 나중에 구독 시작
        val job = flow.subscribe("late-sub", scope = this) { packet ->
            received.add(packet.header.sequenceNumber)
        }

        // 이후 패킷만 수신 (replay=0 설정)
        repeat(3) { i ->
            val packet = createTestPacket(100 + i)
            packets.add(packet)
            flow.publish(packet)
        }

        // Flow.collect는 무한 루프이므로 실제 delay 필요
        delay(100)

        // 나중에 발행된 패킷만 수신
        assertEquals(3, received.size)
        assertEquals(listOf(100, 101, 102), received)

        job.cancel()
        packets.forEach { it.release() }
    }

    @Test
    fun `should format stats correctly`() = runTest {
        val flow = StreamFlow("test")

        repeat(10) { i ->
            val packet = createTestPacket(i)
            flow.publish(packet)
            packet.release()
        }

        delay(100)

        val stats = flow.getStats()

        // 비트레이트 포맷 확인
        assertTrue(stats.avgBitrateFormatted.contains("bps") ||
                stats.avgBitrateFormatted.contains("Kbps") ||
                stats.avgBitrateFormatted.contains("Mbps"))

        // 전달률 (구독자 없으면 0)
        assertEquals(0.0, stats.deliveryRate, 0.01)
    }

    @Test
    fun `should cleanup resources on close`() = runTest {
        val flow = StreamFlow("test")

        val job = flow.subscribe("sub", scope = this) { }

        assertEquals(1, flow.subscriberCount.value)

        flow.close()

        // close 후에도 구독자는 명시적으로 cancel 필요
        // (SharedFlow는 complete 메서드가 없음)
        job.cancel()
        testScheduler.advanceUntilIdle()

        assertEquals(0, flow.subscriberCount.value)
    }
}
