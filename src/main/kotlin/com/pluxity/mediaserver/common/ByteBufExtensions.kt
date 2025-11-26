package com.pluxity.mediaserver.common

import io.netty.buffer.ByteBuf
import io.netty.buffer.ByteBufAllocator
import io.netty.buffer.PooledByteBufAllocator

/**
 * Extensions for Netty ByteBuf to simplify off-heap memory management.
 * These utilities help prevent memory leaks and enable zero-copy operations.
 */

/**
 * Shared pooled allocator for all ByteBuf allocations.
 * Uses off-heap direct buffers for zero-copy network I/O.
 */
val DEFAULT_ALLOCATOR: ByteBufAllocator = PooledByteBufAllocator.DEFAULT

/**
 * Execute a block with a ByteBuf and automatically release it.
 * This is the recommended pattern to prevent memory leaks.
 *
 * @param size Initial buffer size
 * @param block Code block to execute with the buffer
 * @return Result of the block execution
 */
inline fun <T> ByteBufAllocator.directBuffer(size: Int, block: (ByteBuf) -> T): T {
    val buffer = directBuffer(size)
    return try {
        block(buffer)
    } finally {
        buffer.release()
    }
}

/**
 * Execute a block with a ByteBuf and automatically release it.
 * Uses the default allocator.
 *
 * @param size Initial buffer size
 * @param block Code block to execute with the buffer
 * @return Result of the block execution
 */
inline fun <T> withDirectBuffer(size: Int, block: (ByteBuf) -> T): T {
    return DEFAULT_ALLOCATOR.directBuffer(size, block)
}

/**
 * Copy data from this ByteBuf to a ByteArray.
 * Does not modify reader index.
 *
 * @return ByteArray containing a copy of the buffer data
 */
fun ByteBuf.toByteArray(): ByteArray {
    val bytes = ByteArray(readableBytes())
    getBytes(readerIndex(), bytes)
    return bytes
}

/**
 * Write an RTP header to the buffer.
 *
 * @param version RTP version (typically 2)
 * @param padding Padding flag
 * @param extension Extension flag
 * @param csrcCount CSRC count
 * @param marker Marker bit
 * @param payloadType Payload type
 * @param sequenceNumber Sequence number
 * @param timestamp Timestamp
 * @param ssrc SSRC identifier
 */
fun ByteBuf.writeRTPHeader(
    version: Int = 2,
    padding: Boolean = false,
    extension: Boolean = false,
    csrcCount: Int = 0,
    marker: Boolean = false,
    payloadType: Int,
    sequenceNumber: Int,
    timestamp: Long,
    ssrc: Int
) {
    // Byte 0: V(2), P(1), X(1), CC(4)
    val byte0 = (version shl 6) or
            (if (padding) 0x20 else 0) or
            (if (extension) 0x10 else 0) or
            (csrcCount and 0x0F)
    writeByte(byte0)

    // Byte 1: M(1), PT(7)
    val byte1 = (if (marker) 0x80 else 0) or (payloadType and 0x7F)
    writeByte(byte1)

    // Sequence number (2 bytes)
    writeShort(sequenceNumber)

    // Timestamp (4 bytes)
    writeInt(timestamp.toInt())

    // SSRC (4 bytes)
    writeInt(ssrc)
}

/**
 * Read RTP header from the buffer.
 *
 * @return RTPHeader containing parsed header fields
 */
fun ByteBuf.readRTPHeader(): RTPHeader {
    val byte0 = readUnsignedByte().toInt()
    val byte1 = readUnsignedByte().toInt()

    val version = (byte0 shr 6) and 0x03
    val padding = (byte0 and 0x20) != 0
    val extension = (byte0 and 0x10) != 0
    val csrcCount = byte0 and 0x0F

    val marker = (byte1 and 0x80) != 0
    val payloadType = byte1 and 0x7F

    val sequenceNumber = readUnsignedShort()
    val timestamp = readUnsignedInt()
    val ssrc = readInt()

    return RTPHeader(
        version = version,
        padding = padding,
        extension = extension,
        csrcCount = csrcCount,
        marker = marker,
        payloadType = payloadType,
        sequenceNumber = sequenceNumber,
        timestamp = timestamp,
        ssrc = ssrc
    )
}

/**
 * Data class representing an RTP header.
 */
data class RTPHeader(
    val version: Int,
    val padding: Boolean,
    val extension: Boolean,
    val csrcCount: Int,
    val marker: Boolean,
    val payloadType: Int,
    val sequenceNumber: Int,
    val timestamp: Long,
    val ssrc: Int
) {
    companion object {
        const val FIXED_HEADER_SIZE = 12 // bytes
    }
}

/**
 * Calculate total RTP packet size including header.
 *
 * @param payloadSize Size of the RTP payload
 * @param csrcCount Number of CSRC identifiers
 * @return Total packet size in bytes
 */
fun calculateRTPPacketSize(payloadSize: Int, csrcCount: Int = 0): Int {
    return RTPHeader.FIXED_HEADER_SIZE + (csrcCount * 4) + payloadSize
}

/**
 * Retain this ByteBuf and return it (for chaining).
 * Useful for passing buffers between components while maintaining reference count.
 *
 * @return This ByteBuf with incremented reference count
 */
fun ByteBuf.retainAndGet(): ByteBuf {
    retain()
    return this
}

/**
 * Slice this ByteBuf and retain the slice.
 * Creates a zero-copy view of a portion of this buffer.
 *
 * @param index Start index
 * @param length Length of the slice
 * @return Retained slice of this ByteBuf
 */
fun ByteBuf.retainedSlice(index: Int, length: Int): ByteBuf {
    return retainedSlice(index, length)
}

/**
 * Check if this ByteBuf is readable (has bytes available).
 *
 * @return True if buffer has readable bytes
 */
fun ByteBuf.hasReadableBytes(): Boolean = isReadable

/**
 * Get remaining readable bytes count.
 *
 * @return Number of readable bytes
 */
fun ByteBuf.remainingBytes(): Int = readableBytes()

/**
 * Write bytes from a ByteArray to this buffer.
 *
 * @param bytes Source byte array
 * @return This ByteBuf for chaining
 */
fun ByteBuf.writeByteArray(bytes: ByteArray): ByteBuf {
    writeBytes(bytes)
    return this
}
