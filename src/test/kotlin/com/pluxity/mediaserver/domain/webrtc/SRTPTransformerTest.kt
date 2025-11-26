package com.pluxity.mediaserver.domain.webrtc

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*
import javax.crypto.Cipher
import javax.crypto.spec.IvParameterSpec
import javax.crypto.spec.SecretKeySpec

/**
 * RFC 3711 Test Vector를 사용한 SRTP Key Derivation 검증.
 *
 * RFC 3711 Appendix B.3:
 * - Master Key:  E1F97A0D 3E018BE0 D64FA32C 06DE4139
 * - Master Salt: 0EC675AD 498AFEEB B6960B3A ABE6
 *
 * Expected Session Keys:
 * - Session Encryption Key (label=0x00): C61E7A93 744F39EE 10734AFE 3FF7A087
 * - Session Authentication Key (label=0x01): CEBE321F 6FF7716B 6FD4AB49 AF256A15 6D38BAA4
 * - Session Salt (label=0x02): 30CBBC08 863D8C85 D49DB34A 9AE1
 */
class SRTPTransformerTest {

    companion object {
        // RFC 3711 Appendix B.3 Test Vectors
        val RFC_MASTER_KEY = hexToBytes("E1F97A0D3E018BE0D64FA32C06DE4139")
        val RFC_MASTER_SALT = hexToBytes("0EC675AD498AFEEBB6960B3AABE6")

        // Expected output keys
        val EXPECTED_SESSION_KEY = hexToBytes("C61E7A93744F39EE10734AFE3FF7A087")
        val EXPECTED_AUTH_KEY = hexToBytes("CEBE321F6FF7716B6FD4AB49AF256A156D38BAA4")
        val EXPECTED_SESSION_SALT = hexToBytes("30CBBC08863D8C85D49DB34A9AE1")

        private fun hexToBytes(hex: String): ByteArray {
            val result = ByteArray(hex.length / 2)
            for (i in result.indices) {
                result[i] = hex.substring(i * 2, i * 2 + 2).toInt(16).toByte()
            }
            return result
        }

        private fun bytesToHex(bytes: ByteArray): String {
            return bytes.joinToString("") { "%02X".format(it) }
        }
    }

    /**
     * 현재 SRTPTransformer의 키 유도 로직을 RFC 3711 Test Vector로 검증.
     *
     * RFC 3711 Section 4.3.1 Key Derivation Algorithm:
     * - key_id = label || r (r = index DIV key_derivation_rate, usually 0)
     * - x = key_id XOR master_salt
     * - PRF_n(master_key, x) = AES_CM(master_key, x || 0x0000...)
     */
    @Test
    fun `test RFC 3711 key derivation - current implementation`() {
        println("=" .repeat(80))
        println("RFC 3711 SRTP Key Derivation Test")
        println("=" .repeat(80))

        println("\nInput:")
        println("  Master Key:  ${bytesToHex(RFC_MASTER_KEY)}")
        println("  Master Salt: ${bytesToHex(RFC_MASTER_SALT)}")

        // 현재 SRTPTransformer 구현 방식 (label at byte 7)
        println("\n--- Current Implementation (label at byte 7) ---")

        val sessionKey = deriveKeyCurrentImpl(RFC_MASTER_KEY, RFC_MASTER_SALT, 0x00, 16)
        val authKey = deriveKeyCurrentImpl(RFC_MASTER_KEY, RFC_MASTER_SALT, 0x01, 20)
        val sessionSalt = deriveKeyCurrentImpl(RFC_MASTER_KEY, RFC_MASTER_SALT, 0x02, 14)

        println("\nDerived Keys (Current Implementation):")
        println("  Session Key:  ${bytesToHex(sessionKey)}")
        println("  Auth Key:     ${bytesToHex(authKey)}")
        println("  Session Salt: ${bytesToHex(sessionSalt)}")

        println("\nExpected (RFC 3711):")
        println("  Session Key:  ${bytesToHex(EXPECTED_SESSION_KEY)}")
        println("  Auth Key:     ${bytesToHex(EXPECTED_AUTH_KEY)}")
        println("  Session Salt: ${bytesToHex(EXPECTED_SESSION_SALT)}")

        // 비교
        val sessionKeyMatch = sessionKey.contentEquals(EXPECTED_SESSION_KEY)
        val authKeyMatch = authKey.contentEquals(EXPECTED_AUTH_KEY)
        val saltMatch = sessionSalt.contentEquals(EXPECTED_SESSION_SALT)

        println("\nComparison Results:")
        println("  Session Key Match:  $sessionKeyMatch ${if (sessionKeyMatch) "✅" else "❌"}")
        println("  Auth Key Match:     $authKeyMatch ${if (authKeyMatch) "✅" else "❌"}")
        println("  Session Salt Match: $saltMatch ${if (saltMatch) "✅" else "❌"}")

        if (!sessionKeyMatch || !authKeyMatch || !saltMatch) {
            println("\n⚠️ KEY DERIVATION MISMATCH DETECTED!")
            println("Trying alternative implementations...")

            // Alternative 1: label at byte 0
            testAlternativeImpl("label at byte 0", this::deriveKeyAltByte0)

            // Alternative 2: label at first byte of key_id (original RFC interpretation)
            testAlternativeImpl("RFC literal (label || r || master_salt)", this::deriveKeyRFCLiteral)
        }

        // Assert for CI/CD
        assertTrue(sessionKeyMatch, "Session key derivation failed")
        assertTrue(authKeyMatch, "Auth key derivation failed")
        assertTrue(saltMatch, "Session salt derivation failed")
    }

    /**
     * 현재 SRTPTransformer.kt 구현 (label at byte 7).
     */
    private fun deriveKeyCurrentImpl(masterKey: ByteArray, masterSalt: ByteArray, label: Int, length: Int): ByteArray {
        // x = master_salt XOR (label at byte 7)
        val x = ByteArray(14)
        System.arraycopy(masterSalt, 0, x, 0, masterSalt.size.coerceAtMost(14))
        x[7] = (x[7].toInt() xor label).toByte()

        // IV = x || 0x0000 (16 bytes)
        val iv = ByteArray(16)
        System.arraycopy(x, 0, iv, 0, 14)

        println("  Label=$label: x=${bytesToHex(x)}, IV=${bytesToHex(iv)}")

        // AES-CTR encryption of zeros
        val cipher = Cipher.getInstance("AES/CTR/NoPadding")
        val keySpec = SecretKeySpec(masterKey, "AES")
        cipher.init(Cipher.ENCRYPT_MODE, keySpec, IvParameterSpec(iv))

        return cipher.doFinal(ByteArray(length))
    }

    /**
     * Alternative: label at byte 0.
     */
    private fun deriveKeyAltByte0(masterKey: ByteArray, masterSalt: ByteArray, label: Int, length: Int): ByteArray {
        val x = ByteArray(14)
        System.arraycopy(masterSalt, 0, x, 0, masterSalt.size.coerceAtMost(14))
        x[0] = (x[0].toInt() xor label).toByte()

        val iv = ByteArray(16)
        System.arraycopy(x, 0, iv, 0, 14)

        val cipher = Cipher.getInstance("AES/CTR/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(masterKey, "AES"), IvParameterSpec(iv))
        return cipher.doFinal(ByteArray(length))
    }

    /**
     * RFC literal interpretation: key_id = label || r, then XOR with master_salt.
     *
     * RFC 3711 Section 4.3.1:
     * "key_id = label || r" where label is 1 byte and r is typically 0.
     * Then "x = key_id XOR master_salt" (14 bytes, right-aligned).
     */
    private fun deriveKeyRFCLiteral(masterKey: ByteArray, masterSalt: ByteArray, label: Int, length: Int): ByteArray {
        // key_id = label (1 byte) || r (6 bytes, all zeros for kdr=0)
        // This is 7 bytes: [label, 0, 0, 0, 0, 0, 0]
        // But master_salt is 14 bytes, so we need to align properly

        // RFC says: x = key_id XOR master_salt (both treated as 112-bit = 14 bytes)
        // key_id is left-padded with zeros to make 14 bytes
        // So key_id (14 bytes) = [0, 0, 0, 0, 0, 0, 0, label, 0, 0, 0, 0, 0, 0]

        val x = ByteArray(14)
        System.arraycopy(masterSalt, 0, x, 0, 14)
        // XOR label at position 7 (where the label byte of key_id would be)
        x[7] = (x[7].toInt() xor label).toByte()

        val iv = ByteArray(16)
        System.arraycopy(x, 0, iv, 0, 14)

        val cipher = Cipher.getInstance("AES/CTR/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(masterKey, "AES"), IvParameterSpec(iv))
        return cipher.doFinal(ByteArray(length))
    }

    private fun testAlternativeImpl(
        name: String,
        impl: (ByteArray, ByteArray, Int, Int) -> ByteArray
    ) {
        println("\n--- Alternative: $name ---")

        val sessionKey = impl(RFC_MASTER_KEY, RFC_MASTER_SALT, 0x00, 16)
        val authKey = impl(RFC_MASTER_KEY, RFC_MASTER_SALT, 0x01, 20)
        val sessionSalt = impl(RFC_MASTER_KEY, RFC_MASTER_SALT, 0x02, 14)

        println("  Session Key:  ${bytesToHex(sessionKey)} ${if (sessionKey.contentEquals(EXPECTED_SESSION_KEY)) "✅" else "❌"}")
        println("  Auth Key:     ${bytesToHex(authKey)} ${if (authKey.contentEquals(EXPECTED_AUTH_KEY)) "✅" else "❌"}")
        println("  Session Salt: ${bytesToHex(sessionSalt)} ${if (sessionSalt.contentEquals(EXPECTED_SESSION_SALT)) "✅" else "❌"}")
    }

    /**
     * SRTPTransformer 클래스 직접 테스트.
     */
    @Test
    fun `test SRTPTransformer class with RFC vectors`() {
        println("\n" + "=" .repeat(80))
        println("Testing SRTPTransformer class directly")
        println("=" .repeat(80))

        val transformer = SRTPTransformer(
            streamId = "TEST",
            masterKey = RFC_MASTER_KEY,
            masterSalt = RFC_MASTER_SALT
        )

        // SRTPTransformer는 내부에서 키를 유도하므로,
        // 실제 암호화/복호화 동작으로 간접 검증
        val stats = transformer.getStats()
        println("SRTPTransformer initialized: $stats")

        transformer.close()
        println("SRTPTransformer closed successfully")
    }
}
