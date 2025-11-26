package com.pluxity.mediaserver.domain.webrtc

import io.github.oshai.kotlinlogging.KotlinLogging
import io.netty.buffer.ByteBuf
import io.netty.buffer.Unpooled
import org.bouncycastle.crypto.engines.AESEngine
import org.bouncycastle.crypto.macs.HMac
import org.bouncycastle.crypto.digests.SHA1Digest
import org.bouncycastle.crypto.params.KeyParameter
import java.util.concurrent.atomic.AtomicLong
import javax.crypto.Cipher
import javax.crypto.spec.IvParameterSpec
import javax.crypto.spec.SecretKeySpec

private val logger = KotlinLogging.logger {}

/**
 * Pure Java SRTP Transformer using BouncyCastle.
 *
 * WebRTC 표준 암호화:
 * - AES-128-CM (Counter Mode) for encryption
 * - HMAC-SHA1-80 for authentication (10 bytes tag)
 *
 * SRTP Key Derivation (RFC 3711):
 * - Master Key (16 bytes) + Master Salt (14 bytes)
 * - 파생 키: Session Encryption Key, Session Auth Key, Session Salt
 *
 * @property streamId 스트림 식별자
 * @property masterKey SRTP Master Key (16 bytes for AES-128)
 * @property masterSalt SRTP Master Salt (14 bytes)
 */
class SRTPTransformer(
    private val streamId: String,
    private val masterKey: ByteArray,
    private val masterSalt: ByteArray
) : AutoCloseable {

    // Derived keys (RFC 3711 Key Derivation)
    private val sessionEncryptionKey: ByteArray  // 16 bytes
    private val sessionAuthenticationKey: ByteArray  // 20 bytes (for HMAC-SHA1)
    private val sessionSalt: ByteArray  // 14 bytes

    // SSRC별 ROC (Rollover Counter) 관리
    private val rocMap = mutableMapOf<Int, Int>()

    // Statistics
    private val packetsEncrypted = AtomicLong(0)
    private val packetsDecrypted = AtomicLong(0)
    private val encryptionErrors = AtomicLong(0)

    companion object {
        // Key Derivation labels (RFC 3711 Section 4.3)
        private const val LABEL_RTP_ENCRYPTION = 0x00
        private const val LABEL_RTP_AUTH = 0x01
        private const val LABEL_RTP_SALT = 0x02
        private const val LABEL_RTCP_ENCRYPTION = 0x03
        private const val LABEL_RTCP_AUTH = 0x04
        private const val LABEL_RTCP_SALT = 0x05

        // WebRTC standard: HMAC-SHA1-80 (10 bytes)
        private const val AUTH_TAG_LENGTH = 10
    }

    init {
        logger.info { "[SRTP $streamId] Initializing Pure Java SRTP Transformer" }
        logger.debug { "[SRTP $streamId] Master Key: ${masterKey.size} bytes, Salt: ${masterSalt.size} bytes" }

        // Key derivation (RFC 3711)
        sessionEncryptionKey = deriveKey(LABEL_RTP_ENCRYPTION, 16)
        sessionAuthenticationKey = deriveKey(LABEL_RTP_AUTH, 20)
        sessionSalt = deriveKey(LABEL_RTP_SALT, 14)

        logger.info { "[SRTP $streamId] SRTP Transformer initialized (Pure Java)" }
    }

    /**
     * RFC 3711 Key Derivation Function (Section 4.3).
     *
     * PRF_n(k_master, x) = AES_CM(k_master, x*2^16)
     *
     * x = <label> || r
     * label은 1바이트, r은 패킷 인덱스/key_derivation_rate (보통 0)
     *
     * For SRTP:
     * - session_key = PRF(master_key, (label=0x00 || master_salt || 0x0000...))
     * - session_salt = PRF(master_key, (label=0x02 || master_salt || 0x0000...))
     * - session_auth_key = PRF(master_key, (label=0x01 || master_salt || 0x0000...))
     */
    private fun deriveKey(label: Int, length: Int): ByteArray {
        // RFC 3711 Section 4.3.1:
        // key_id = <label> || r (where r = index DIV key_derivation_rate, usually 0)
        // x = key_id XOR master_salt (padded to 112 bits = 14 bytes)

        // Create x (14 bytes): label at position 0, rest is master_salt XOR'd appropriately
        val x = ByteArray(14)
        System.arraycopy(masterSalt, 0, x, 0, masterSalt.size.coerceAtMost(14))

        // XOR the label at byte 7 (label is at the 7th byte in the 14-byte key_id)
        // RFC 3711: key_id = label || r, but for kdr=0, r=0
        // Actually, label occupies the first byte of key_id, and it's XOR'd with master_salt
        x[7] = (x[7].toInt() xor label).toByte()

        // IV for AES-CM: x || 0x0000 (16 bytes total, last 2 bytes are counter, start at 0)
        val iv = ByteArray(16)
        System.arraycopy(x, 0, iv, 0, 14)
        // iv[14] and iv[15] are already 0 (block counter starts at 0)

        // AES-CTR (Counter Mode) to generate key stream
        val cipher = Cipher.getInstance("AES/CTR/NoPadding")
        val keySpec = SecretKeySpec(masterKey, "AES")
        cipher.init(Cipher.ENCRYPT_MODE, keySpec, IvParameterSpec(iv))

        // Encrypt zeros to get the key stream
        val zeros = ByteArray(length)
        val derived = cipher.doFinal(zeros)

        // Log for debugging
        val totalDerived = packetsEncrypted.get()
        if (totalDerived == 0L) {
            logger.info { "[SRTP $streamId] Derived key (label=$label): ${derived.take(8).joinToString("") { "%02x".format(it) }}... (${derived.size} bytes)" }
        }

        return derived
    }

    /**
     * RTP 패킷 암호화 (SRTP).
     *
     * SRTP = Encrypted Payload + Auth Tag
     *
     * @param rtpPacket 평문 RTP 패킷 (header + payload)
     * @param ssrc RTP SSRC
     * @return 암호화된 SRTP 패킷
     */
    fun encryptRTP(rtpPacket: ByteBuf, ssrc: Int): ByteBuf {
        try {
            // ByteBuf → ByteArray
            val plainData = ByteArray(rtpPacket.readableBytes())
            rtpPacket.markReaderIndex()
            rtpPacket.readBytes(plainData)
            rtpPacket.resetReaderIndex()

            if (plainData.size < 12) {
                throw IllegalArgumentException("RTP packet too small: ${plainData.size}")
            }

            // RTP 헤더 파싱 (12 bytes minimum)
            val sequenceNumber = ((plainData[2].toInt() and 0xFF) shl 8) or (plainData[3].toInt() and 0xFF)
            val rtpSsrc = ((plainData[8].toInt() and 0xFF) shl 24) or
                    ((plainData[9].toInt() and 0xFF) shl 16) or
                    ((plainData[10].toInt() and 0xFF) shl 8) or
                    (plainData[11].toInt() and 0xFF)

            // ROC 관리
            val roc = rocMap.getOrPut(rtpSsrc) { 0 }
            val packetIndex = ((roc.toLong() shl 16) or sequenceNumber.toLong())

            // AES-CM IV 생성 (RFC 3711 Section 4.1.1)
            val iv = generateIV(rtpSsrc, packetIndex)

            // 페이로드 암호화 (헤더 12바이트 이후)
            val headerLen = 12 + getCSRCCount(plainData) * 4 + getExtensionLength(plainData)
            val encrypted = ByteArray(plainData.size + AUTH_TAG_LENGTH)
            System.arraycopy(plainData, 0, encrypted, 0, headerLen) // 헤더 복사

            if (plainData.size > headerLen) {
                val payload = plainData.copyOfRange(headerLen, plainData.size)
                val encryptedPayload = encryptAESCM(payload, iv)
                System.arraycopy(encryptedPayload, 0, encrypted, headerLen, encryptedPayload.size)
            }

            // HMAC-SHA1-80 인증 태그 생성
            val authTag = generateAuthTag(encrypted, 0, plainData.size, roc)
            System.arraycopy(authTag, 0, encrypted, plainData.size, AUTH_TAG_LENGTH)

            packetsEncrypted.incrementAndGet()

            val totalEncrypted = packetsEncrypted.get()
            if (totalEncrypted <= 5 || totalEncrypted % 500 == 0L) {
                logger.info { "[SRTP $streamId] Encrypted packet #$totalEncrypted: ${plainData.size} → ${encrypted.size} bytes (seq=$sequenceNumber)" }
            }

            return Unpooled.wrappedBuffer(encrypted)
        } catch (e: Exception) {
            encryptionErrors.incrementAndGet()
            logger.error(e) { "[SRTP $streamId] Encryption failed" }
            throw e
        }
    }

    /**
     * AES-CM (Counter Mode) 암호화.
     */
    private fun encryptAESCM(data: ByteArray, iv: ByteArray): ByteArray {
        val cipher = Cipher.getInstance("AES/CTR/NoPadding")
        val keySpec = SecretKeySpec(sessionEncryptionKey, "AES")
        cipher.init(Cipher.ENCRYPT_MODE, keySpec, IvParameterSpec(iv))
        return cipher.doFinal(data)
    }

    /**
     * AES-CM IV 생성 (RFC 3711 Section 4.1.1).
     *
     * IV = session_salt XOR (SSRC || packet_index)
     */
    private fun generateIV(ssrc: Int, packetIndex: Long): ByteArray {
        val iv = ByteArray(16)

        // Session salt를 IV에 복사
        System.arraycopy(sessionSalt, 0, iv, 0, sessionSalt.size.coerceAtMost(14))

        // SSRC XOR (bytes 4-7)
        iv[4] = (iv[4].toInt() xor ((ssrc shr 24) and 0xFF)).toByte()
        iv[5] = (iv[5].toInt() xor ((ssrc shr 16) and 0xFF)).toByte()
        iv[6] = (iv[6].toInt() xor ((ssrc shr 8) and 0xFF)).toByte()
        iv[7] = (iv[7].toInt() xor (ssrc and 0xFF)).toByte()

        // Packet index XOR (bytes 8-13, 6 bytes for 48-bit index)
        iv[8] = (iv[8].toInt() xor ((packetIndex shr 40) and 0xFF).toInt()).toByte()
        iv[9] = (iv[9].toInt() xor ((packetIndex shr 32) and 0xFF).toInt()).toByte()
        iv[10] = (iv[10].toInt() xor ((packetIndex shr 24) and 0xFF).toInt()).toByte()
        iv[11] = (iv[11].toInt() xor ((packetIndex shr 16) and 0xFF).toInt()).toByte()
        iv[12] = (iv[12].toInt() xor ((packetIndex shr 8) and 0xFF).toInt()).toByte()
        iv[13] = (iv[13].toInt() xor (packetIndex and 0xFF).toInt()).toByte()

        return iv
    }

    /**
     * HMAC-SHA1-80 인증 태그 생성.
     */
    private fun generateAuthTag(data: ByteArray, offset: Int, length: Int, roc: Int): ByteArray {
        val hmac = HMac(SHA1Digest())
        hmac.init(KeyParameter(sessionAuthenticationKey))

        // 인증 대상: SRTP packet (without auth tag) || ROC
        hmac.update(data, offset, length)

        // ROC (4 bytes, big-endian)
        val rocBytes = byteArrayOf(
            ((roc shr 24) and 0xFF).toByte(),
            ((roc shr 16) and 0xFF).toByte(),
            ((roc shr 8) and 0xFF).toByte(),
            (roc and 0xFF).toByte()
        )
        hmac.update(rocBytes, 0, 4)

        // Full HMAC-SHA1 output (20 bytes)
        val fullTag = ByteArray(20)
        hmac.doFinal(fullTag, 0)

        // 첫 10 bytes만 사용 (HMAC-SHA1-80)
        return fullTag.copyOf(AUTH_TAG_LENGTH)
    }

    /**
     * CSRC 카운트 (RTP 헤더 첫 바이트의 하위 4비트).
     */
    private fun getCSRCCount(rtpPacket: ByteArray): Int {
        return rtpPacket[0].toInt() and 0x0F
    }

    /**
     * RTP 확장 헤더 길이.
     */
    private fun getExtensionLength(rtpPacket: ByteArray): Int {
        val hasExtension = (rtpPacket[0].toInt() and 0x10) != 0
        if (!hasExtension || rtpPacket.size < 16) return 0

        val csrcCount = getCSRCCount(rtpPacket)
        val extOffset = 12 + csrcCount * 4

        if (rtpPacket.size < extOffset + 4) return 0

        // Extension length (in 32-bit words, not including first 4 bytes)
        val extLength = ((rtpPacket[extOffset + 2].toInt() and 0xFF) shl 8) or
                (rtpPacket[extOffset + 3].toInt() and 0xFF)

        return 4 + extLength * 4
    }

    /**
     * RTP 패킷 복호화.
     */
    fun decryptRTP(srtpPacket: ByteBuf, ssrc: Int): ByteBuf {
        try {
            val encryptedData = ByteArray(srtpPacket.readableBytes())
            srtpPacket.markReaderIndex()
            srtpPacket.readBytes(encryptedData)
            srtpPacket.resetReaderIndex()

            if (encryptedData.size < 12 + AUTH_TAG_LENGTH) {
                throw IllegalArgumentException("SRTP packet too small: ${encryptedData.size}")
            }

            // 인증 태그 검증
            val payloadEnd = encryptedData.size - AUTH_TAG_LENGTH
            val receivedTag = encryptedData.copyOfRange(payloadEnd, encryptedData.size)

            val rtpSsrc = ((encryptedData[8].toInt() and 0xFF) shl 24) or
                    ((encryptedData[9].toInt() and 0xFF) shl 16) or
                    ((encryptedData[10].toInt() and 0xFF) shl 8) or
                    (encryptedData[11].toInt() and 0xFF)
            val sequenceNumber = ((encryptedData[2].toInt() and 0xFF) shl 8) or
                    (encryptedData[3].toInt() and 0xFF)

            val roc = rocMap.getOrPut(rtpSsrc) { 0 }
            val expectedTag = generateAuthTag(encryptedData, 0, payloadEnd, roc)

            if (!receivedTag.contentEquals(expectedTag)) {
                throw SecurityException("SRTP authentication failed")
            }

            // 복호화
            val packetIndex = ((roc.toLong() shl 16) or sequenceNumber.toLong())
            val iv = generateIV(rtpSsrc, packetIndex)

            val headerLen = 12 + getCSRCCount(encryptedData) * 4 + getExtensionLength(encryptedData)
            val decrypted = ByteArray(payloadEnd)
            System.arraycopy(encryptedData, 0, decrypted, 0, headerLen)

            if (payloadEnd > headerLen) {
                val encPayload = encryptedData.copyOfRange(headerLen, payloadEnd)
                val decPayload = encryptAESCM(encPayload, iv) // CTR mode: encrypt = decrypt
                System.arraycopy(decPayload, 0, decrypted, headerLen, decPayload.size)
            }

            packetsDecrypted.incrementAndGet()

            return Unpooled.wrappedBuffer(decrypted)
        } catch (e: Exception) {
            encryptionErrors.incrementAndGet()
            logger.error(e) { "[SRTP $streamId] Decryption failed" }
            throw e
        }
    }

    /**
     * 통계 조회.
     */
    fun getStats(): SRTPStats {
        return SRTPStats(
            streamId = streamId,
            packetsEncrypted = packetsEncrypted.get(),
            packetsDecrypted = packetsDecrypted.get(),
            encryptionErrors = encryptionErrors.get(),
            activeContexts = rocMap.size
        )
    }

    override fun close() {
        logger.info { "[SRTP $streamId] Closing SRTP Transformer" }
        rocMap.clear()
    }
}

/**
 * SRTP 통계.
 */
data class SRTPStats(
    val streamId: String,
    val packetsEncrypted: Long,
    val packetsDecrypted: Long,
    val encryptionErrors: Long,
    val activeContexts: Int
)
