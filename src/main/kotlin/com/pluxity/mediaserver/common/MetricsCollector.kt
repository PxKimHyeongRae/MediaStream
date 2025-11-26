package com.pluxity.mediaserver.common

import io.micrometer.core.instrument.Counter
import io.micrometer.core.instrument.DistributionSummary
import io.micrometer.core.instrument.Gauge
import io.micrometer.core.instrument.MeterRegistry
import io.micrometer.core.instrument.Timer
import org.springframework.stereotype.Component
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicLong

/**
 * Centralized metrics collector for the media server.
 * Uses Micrometer for metrics collection and export to Prometheus.
 */
@Component
class MetricsCollector(
    private val meterRegistry: MeterRegistry
) {
    // Active streams counter
    private val activeStreams = AtomicLong(0)

    // Active peers counter
    private val activePeers = AtomicLong(0)

    // Per-stream metrics
    private val streamMetrics = ConcurrentHashMap<String, StreamMetrics>()

    // Per-peer metrics
    private val peerMetrics = ConcurrentHashMap<String, PeerMetrics>()

    init {
        // Register gauges for active resources
        Gauge.builder("mediaserver.streams.active", activeStreams) { it.get().toDouble() }
            .description("Number of active streams")
            .register(meterRegistry)

        Gauge.builder("mediaserver.peers.active", activePeers) { it.get().toDouble() }
            .description("Number of active WebRTC peers")
            .register(meterRegistry)
    }

    /**
     * Record stream started event.
     */
    fun streamStarted(streamId: String) {
        activeStreams.incrementAndGet()
        streamMetrics.computeIfAbsent(streamId) { StreamMetrics(streamId, meterRegistry) }

        Counter.builder("mediaserver.streams.started")
            .tag("stream_id", streamId)
            .description("Number of times stream has started")
            .register(meterRegistry)
            .increment()
    }

    /**
     * Record stream stopped event.
     */
    fun streamStopped(streamId: String) {
        activeStreams.decrementAndGet()

        Counter.builder("mediaserver.streams.stopped")
            .tag("stream_id", streamId)
            .description("Number of times stream has stopped")
            .register(meterRegistry)
            .increment()
    }

    /**
     * Record RTP packet received.
     */
    fun rtpPacketReceived(streamId: String, payloadSize: Int) {
        streamMetrics[streamId]?.apply {
            packetsReceived.increment()
            bytesReceived.record(payloadSize.toDouble())
        }
    }

    /**
     * Record RTP packet sent to peer.
     */
    fun rtpPacketSent(peerId: String, payloadSize: Int) {
        peerMetrics[peerId]?.apply {
            packetsSent.increment()
            bytesSent.record(payloadSize.toDouble())
        }
    }

    /**
     * Record peer connected event.
     */
    fun peerConnected(peerId: String, streamId: String) {
        activePeers.incrementAndGet()
        peerMetrics.computeIfAbsent(peerId) { PeerMetrics(peerId, streamId, meterRegistry) }

        Counter.builder("mediaserver.peers.connected")
            .tag("peer_id", peerId)
            .tag("stream_id", streamId)
            .description("Number of peer connections")
            .register(meterRegistry)
            .increment()
    }

    /**
     * Record peer disconnected event.
     */
    fun peerDisconnected(peerId: String) {
        activePeers.decrementAndGet()
        peerMetrics.remove(peerId)

        Counter.builder("mediaserver.peers.disconnected")
            .tag("peer_id", peerId)
            .description("Number of peer disconnections")
            .register(meterRegistry)
            .increment()
    }

    /**
     * Record RTSP connection error.
     */
    fun rtspError(streamId: String, errorType: String) {
        Counter.builder("mediaserver.rtsp.errors")
            .tag("stream_id", streamId)
            .tag("error_type", errorType)
            .description("RTSP connection errors")
            .register(meterRegistry)
            .increment()
    }

    /**
     * Record WebRTC peer error.
     */
    fun webrtcError(peerId: String, errorType: String) {
        Counter.builder("mediaserver.webrtc.errors")
            .tag("peer_id", peerId)
            .tag("error_type", errorType)
            .description("WebRTC peer errors")
            .register(meterRegistry)
            .increment()
    }

    /**
     * Record operation execution time.
     */
    fun recordOperationTime(operation: String, block: () -> Unit) {
        Timer.builder("mediaserver.operation.duration")
            .tag("operation", operation)
            .description("Operation execution time")
            .register(meterRegistry)
            .record(block)
    }

    /**
     * Get stream metrics for a specific stream.
     */
    fun getStreamMetrics(streamId: String): StreamMetrics? = streamMetrics[streamId]

    /**
     * Get peer metrics for a specific peer.
     */
    fun getPeerMetrics(peerId: String): PeerMetrics? = peerMetrics[peerId]

    /**
     * Metrics for a single stream.
     */
    class StreamMetrics(
        streamId: String,
        registry: MeterRegistry
    ) {
        val packetsReceived: Counter = Counter.builder("mediaserver.stream.packets.received")
            .tag("stream_id", streamId)
            .description("Number of RTP packets received from RTSP source")
            .register(registry)

        val bytesReceived: DistributionSummary = DistributionSummary.builder("mediaserver.stream.bytes.received")
            .tag("stream_id", streamId)
            .description("Bytes received from RTSP source")
            .baseUnit("bytes")
            .register(registry)
    }

    /**
     * Metrics for a single WebRTC peer.
     */
    class PeerMetrics(
        peerId: String,
        streamId: String,
        registry: MeterRegistry
    ) {
        val packetsSent: Counter = Counter.builder("mediaserver.peer.packets.sent")
            .tag("peer_id", peerId)
            .tag("stream_id", streamId)
            .description("Number of RTP packets sent to peer")
            .register(registry)

        val bytesSent: DistributionSummary = DistributionSummary.builder("mediaserver.peer.bytes.sent")
            .tag("peer_id", peerId)
            .tag("stream_id", streamId)
            .description("Bytes sent to peer")
            .baseUnit("bytes")
            .register(registry)
    }
}
