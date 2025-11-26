package com.pluxity.mediaserver.domain.webrtc

import io.github.oshai.kotlinlogging.KotlinLogging
import org.bouncycastle.asn1.x500.X500Name
import org.bouncycastle.cert.X509CertificateHolder
import org.bouncycastle.cert.jcajce.JcaX509CertificateConverter
import org.bouncycastle.cert.jcajce.JcaX509v3CertificateBuilder
import org.bouncycastle.jce.provider.BouncyCastleProvider
import org.bouncycastle.operator.jcajce.JcaContentSignerBuilder
import org.bouncycastle.tls.*
import org.bouncycastle.tls.crypto.impl.jcajce.JcaTlsCryptoProvider
import java.io.IOException
import java.math.BigInteger
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.InetSocketAddress
import java.security.*
import java.security.cert.X509Certificate
import java.util.*

private val logger = KotlinLogging.logger {}

/**
 * DTLS Handler: DTLS-SRTP 핸드셰이크 및 키 교환.
 *
 * Bouncy Castle TLS 라이브러리를 사용한 실제 DTLS 구현.
 * WebRTC는 DTLS-SRTP를 사용하여 미디어 암호화 키를 교환합니다.
 *
 * **주요 기능**:
 * - 자체 서명 인증서 생성 (ECDSA P-256)
 * - SHA-256 Fingerprint 계산 (SDP에 포함)
 * - DTLS 서버 모드 핸드셰이크
 * - SRTP 키 추출 (exportKeyingMaterial)
 *
 * @property streamId 스트림 식별자
 */
class DTLSHandler(
    private val streamId: String
) : AutoCloseable {

    private val keyPair: KeyPair
    private val certificate: X509Certificate
    private val fingerprint: String
    private val tlsCrypto: org.bouncycastle.tls.crypto.TlsCrypto

    // DTLS 핸드셰이크 후 컨텍스트 (SRTP 키 추출용)
    private var tlsContext: TlsContext? = null
    private var dtlsTransport: DTLSTransport? = null

    init {
        // Bouncy Castle Provider 등록 (tlsCrypto 생성 전에 반드시 필요!)
        if (Security.getProvider(BouncyCastleProvider.PROVIDER_NAME) == null) {
            Security.addProvider(BouncyCastleProvider())
            logger.info { "[DTLS $streamId] Bouncy Castle Provider registered" }
        }

        // TLS Crypto 생성 (Provider 등록 후)
        tlsCrypto = JcaTlsCryptoProvider()
            .setProvider(BouncyCastleProvider.PROVIDER_NAME)
            .create(SecureRandom())

        // ECDSA 키 쌍 생성 (WebRTC 표준)
        keyPair = generateECKeyPair()
        certificate = generateSelfSignedCertificate(keyPair)
        fingerprint = calculateFingerprint(certificate)

        logger.info { "[DTLS $streamId] Certificate fingerprint (SHA-256): $fingerprint" }
    }

    /**
     * DTLS 핸드셰이크 수행 (서버 모드) - DatagramSocket 버전.
     *
     * ICE 연결이 확립된 후 호출됩니다.
     * 클라이언트(브라우저)가 DTLS ClientHello를 보내면 핸드셰이크를 완료합니다.
     *
     * @param socket ICE로 확정된 UDP 소켓
     * @param remoteAddress 원격 주소
     * @return SRTP 키 (masterKey, masterSalt)
     */
    fun performHandshake(
        socket: DatagramSocket?,
        remoteAddress: InetSocketAddress? = null
    ): Pair<ByteArray, ByteArray> {
        if (socket == null) {
            logger.warn { "[DTLS $streamId] Socket is null, returning mock keys" }
            return generateMockKeys()
        }

        logger.info { "[DTLS $streamId] Starting DTLS handshake (server mode)" }

        try {
            // DatagramTransport 래퍼 생성
            val transport = UdpDatagramTransport(socket, remoteAddress, MTU_SIZE)

            // DTLS 핸드셰이크 수행
            return performHandshakeInternal(transport)

        } catch (e: Exception) {
            logger.error(e) { "[DTLS $streamId] DTLS handshake failed" }
            return generateMockKeys()
        }
    }

    /**
     * DTLS 핸드셰이크 수행 (서버 모드) - DatagramTransport 버전.
     *
     * ICE-DTLS Transport를 직접 전달받아 핸드셰이크를 수행합니다.
     * 이 메서드는 ICE Component의 Multiplexed Socket을 사용할 때 호출합니다.
     *
     * @param transport Bouncy Castle DatagramTransport 구현체
     * @return SRTP 키 (masterKey, masterSalt)
     */
    fun performHandshake(transport: DatagramTransport): Pair<ByteArray, ByteArray> {
        logger.info { "[DTLS $streamId] Starting DTLS handshake with provided transport" }

        return try {
            performHandshakeInternal(transport)
        } catch (e: Exception) {
            logger.error(e) { "[DTLS $streamId] DTLS handshake failed" }
            generateMockKeys()
        }
    }

    /**
     * DTLS 핸드셰이크 내부 구현.
     */
    private fun performHandshakeInternal(transport: DatagramTransport): Pair<ByteArray, ByteArray> {
        // DTLS 서버 프로토콜 생성
        val serverProtocol = DTLSServerProtocol()

        // TLS 서버 구현
        val tlsServer = WebRTCDTLSServer(tlsCrypto, keyPair, certificate)

        // DTLS 핸드셰이크 수행 (blocking)
        // notifyHandshakeComplete() 콜백이 accept() 내에서 호출됨
        logger.info { "[DTLS $streamId] Waiting for ClientHello..." }
        dtlsTransport = serverProtocol.accept(tlsServer, transport)
        tlsContext = tlsServer.tlsServerContext

        logger.info { "[DTLS $streamId] DTLS handshake completed successfully" }

        // notifyHandshakeComplete()에서 이미 추출된 키 사용
        val masterKey = tlsServer.exportedMasterKey
        val masterSalt = tlsServer.exportedMasterSalt

        if (masterKey != null && masterSalt != null) {
            logger.info { "[DTLS $streamId] Using SRTP keys from notifyHandshakeComplete: Key=${masterKey.size} bytes, Salt=${masterSalt.size} bytes" }
            return Pair(masterKey, masterSalt)
        } else {
            logger.warn { "[DTLS $streamId] SRTP keys not available from notifyHandshakeComplete, using fallback" }
            return generateMockKeys()
        }
    }

    /**
     * SRTP 키 추출 (RFC 5764).
     *
     * DTLS 핸드셰이크 완료 후 exportKeyingMaterial을 사용하여
     * SRTP 암호화에 필요한 키를 추출합니다.
     */
    private fun exportSRTPKeys(): Pair<ByteArray, ByteArray> {
        val context = tlsContext ?: throw IllegalStateException("DTLS handshake not completed")

        // SRTP_AES128_CM_HMAC_SHA1_80 기준:
        // - Master Key: 16 bytes (128 bits)
        // - Master Salt: 14 bytes (112 bits)
        // 총 60 bytes = 2 * (16 + 14) for client + server
        val keyingMaterial = context.exportKeyingMaterial(
            ExporterLabel.dtls_srtp,
            null,
            2 * (SRTP_MASTER_KEY_LENGTH + SRTP_MASTER_SALT_LENGTH)
        )

        // 서버 모드에서는:
        // - client_write_key (16) + server_write_key (16) + client_write_salt (14) + server_write_salt (14)
        // 우리는 서버이므로 server_write_key와 server_write_salt를 사용
        val serverKeyOffset = SRTP_MASTER_KEY_LENGTH  // client key 이후
        val serverSaltOffset = 2 * SRTP_MASTER_KEY_LENGTH + SRTP_MASTER_SALT_LENGTH  // client salt 이후

        val masterKey = keyingMaterial.copyOfRange(serverKeyOffset, serverKeyOffset + SRTP_MASTER_KEY_LENGTH)
        val masterSalt = keyingMaterial.copyOfRange(serverSaltOffset, serverSaltOffset + SRTP_MASTER_SALT_LENGTH)

        logger.info { "[DTLS $streamId] SRTP keys exported: Key=${masterKey.size} bytes, Salt=${masterSalt.size} bytes" }

        return Pair(masterKey, masterSalt)
    }

    /**
     * Mock 키 생성 (테스트/폴백용).
     */
    private fun generateMockKeys(): Pair<ByteArray, ByteArray> {
        logger.warn { "[DTLS $streamId] Using mock SRTP keys (not secure!)" }

        val masterKey = ByteArray(SRTP_MASTER_KEY_LENGTH)
        val masterSalt = ByteArray(SRTP_MASTER_SALT_LENGTH)
        SecureRandom().apply {
            nextBytes(masterKey)
            nextBytes(masterSalt)
        }

        return Pair(masterKey, masterSalt)
    }

    /**
     * Certificate Fingerprint 가져오기 (SDP에 포함).
     */
    fun getFingerprint(): String = fingerprint

    /**
     * 인증서 가져오기.
     */
    fun getCertificate(): X509Certificate = certificate

    /**
     * Certificate Fingerprint 계산 (SHA-256).
     */
    private fun calculateFingerprint(cert: X509Certificate): String {
        val digest = MessageDigest.getInstance("SHA-256")
        val hash = digest.digest(cert.encoded)
        return hash.joinToString(":") { "%02X".format(it) }
    }

    /**
     * ECDSA P-256 키 쌍 생성 (WebRTC 표준).
     */
    private fun generateECKeyPair(): KeyPair {
        val keyGen = KeyPairGenerator.getInstance("EC", BouncyCastleProvider.PROVIDER_NAME)
        keyGen.initialize(256) // P-256 curve
        return keyGen.generateKeyPair()
    }

    /**
     * 자체 서명 인증서 생성.
     */
    private fun generateSelfSignedCertificate(keyPair: KeyPair): X509Certificate {
        val now = System.currentTimeMillis()
        val notBefore = Date(now)
        val notAfter = Date(now + 365L * 24 * 60 * 60 * 1000) // 1년

        val serial = BigInteger.valueOf(now)
        val subject = X500Name("CN=MediaServer,O=Pluxity,C=KR")

        val certBuilder = JcaX509v3CertificateBuilder(
            subject,
            serial,
            notBefore,
            notAfter,
            subject,
            keyPair.public
        )

        val signer = JcaContentSignerBuilder("SHA256withECDSA")
            .setProvider(BouncyCastleProvider.PROVIDER_NAME)
            .build(keyPair.private)

        val certHolder: X509CertificateHolder = certBuilder.build(signer)

        return JcaX509CertificateConverter()
            .setProvider(BouncyCastleProvider.PROVIDER_NAME)
            .getCertificate(certHolder)
    }

    override fun close() {
        try {
            dtlsTransport?.close()
        } catch (e: Exception) {
            logger.warn(e) { "[DTLS $streamId] Error closing DTLS transport" }
        }
        dtlsTransport = null
        tlsContext = null
    }

    companion object {
        private const val MTU_SIZE = 1500
        private const val SRTP_MASTER_KEY_LENGTH = 16  // AES-128
        private const val SRTP_MASTER_SALT_LENGTH = 14  // 112 bits
    }
}

/**
 * WebRTC DTLS 서버 구현.
 *
 * DTLS-SRTP 확장을 지원하는 TLS 서버입니다.
 */
private class WebRTCDTLSServer(
    private val crypto: org.bouncycastle.tls.crypto.TlsCrypto,
    private val keyPair: KeyPair,
    private val certificate: X509Certificate
) : DefaultTlsServer(crypto) {

    var tlsServerContext: TlsContext? = null
        private set

    // SRTP keys exported in notifyHandshakeComplete()
    var exportedMasterKey: ByteArray? = null
        private set
    var exportedMasterSalt: ByteArray? = null
        private set

    override fun init(serverContext: TlsServerContext) {
        super.init(serverContext)
        this.tlsServerContext = serverContext
    }

    /**
     * DTLS 핸드셰이크 완료 콜백.
     * exportKeyingMaterial()은 여기서만 호출 가능!
     */
    override fun notifyHandshakeComplete() {
        super.notifyHandshakeComplete()

        val ctx = tlsServerContext ?: return

        try {
            // SRTP_AES128_CM_HMAC_SHA1_80 기준:
            // - Master Key: 16 bytes (128 bits)
            // - Master Salt: 14 bytes (112 bits)
            val keyingMaterial = ctx.exportKeyingMaterial(
                ExporterLabel.dtls_srtp,
                null,
                2 * (16 + 14)  // client + server
            )

            // 서버 모드에서는:
            // - client_write_key (16) + server_write_key (16) + client_write_salt (14) + server_write_salt (14)
            val serverKeyOffset = 16  // client key 이후
            val serverSaltOffset = 2 * 16 + 14  // client salt 이후

            exportedMasterKey = keyingMaterial.copyOfRange(serverKeyOffset, serverKeyOffset + 16)
            exportedMasterSalt = keyingMaterial.copyOfRange(serverSaltOffset, serverSaltOffset + 14)

            logger.info { "[DTLS] SRTP keys exported in notifyHandshakeComplete: Key=${exportedMasterKey?.size} bytes, Salt=${exportedMasterSalt?.size} bytes" }
        } catch (e: Exception) {
            logger.error(e) { "[DTLS] Failed to export SRTP keys in notifyHandshakeComplete" }
        }
    }

    /**
     * DTLS 지원 버전.
     * Bouncy Castle 1.77에서는 getSupportedVersions()를 통해 버전 협상.
     */
    override fun getSupportedVersions(): Array<ProtocolVersion> {
        return arrayOf(
            ProtocolVersion.DTLSv12,  // 브라우저에서 주로 사용
            ProtocolVersion.DTLSv10   // 폴백용
        )
    }

    override fun getSupportedCipherSuites(): IntArray {
        // WebRTC에서 지원하는 cipher suites (DTLS 1.2 호환)
        return intArrayOf(
            CipherSuite.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
            CipherSuite.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
            CipherSuite.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
            CipherSuite.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
        )
    }

    override fun getServerExtensions(): Hashtable<*, *> {
        val extensions = super.getServerExtensions() ?: Hashtable<Int, ByteArray>()

        // SRTP 확장 추가 (use_srtp)
        @Suppress("UNCHECKED_CAST")
        val ext = extensions as Hashtable<Int, ByteArray>

        // SRTP_AES128_CM_HMAC_SHA1_80 프로파일 사용
        val srtpData = UseSRTPData(
            intArrayOf(SRTPProtectionProfile.SRTP_AES128_CM_HMAC_SHA1_80),
            TlsUtils.EMPTY_BYTES  // No MKI
        )
        TlsSRTPUtils.addUseSRTPExtension(ext, srtpData)

        return ext
    }

    override fun processClientExtensions(clientExtensions: Hashtable<*, *>?) {
        super.processClientExtensions(clientExtensions)

        // 클라이언트의 SRTP 확장 확인
        if (clientExtensions != null) {
            @Suppress("UNCHECKED_CAST")
            val ext = clientExtensions as Hashtable<Int, ByteArray>
            val srtpData = TlsSRTPUtils.getUseSRTPExtension(ext)
            if (srtpData != null) {
                logger.info { "[DTLS] Client SRTP profiles: ${srtpData.protectionProfiles.toList()}" }
            }
        }
    }

    override fun getCredentials(): TlsCredentials {
        val x509Cert = this.certificate  // 외부 클래스의 X509Certificate
        val ecKeyPair = this.keyPair

        // ECDSA 서명 자격증명 반환
        return object : TlsCredentialedSigner {
            override fun getCertificate(): org.bouncycastle.tls.Certificate {
                val tlsCert = crypto.createCertificate(x509Cert.encoded)
                return org.bouncycastle.tls.Certificate(arrayOf(tlsCert))
            }

            override fun getSignatureAndHashAlgorithm(): SignatureAndHashAlgorithm {
                return SignatureAndHashAlgorithm(HashAlgorithm.sha256, SignatureAlgorithm.ecdsa)
            }

            override fun generateRawSignature(hash: ByteArray): ByteArray {
                // IMPORTANT: hash 파라미터는 이미 해시된 데이터입니다.
                // SHA256withECDSA를 사용하면 이중 해시가 발생하므로 NONEwithECDSA 사용
                val signature = Signature.getInstance("NONEwithECDSA", BouncyCastleProvider.PROVIDER_NAME)
                signature.initSign(ecKeyPair.private)
                signature.update(hash)
                return signature.sign()
            }

            override fun getStreamSigner(): org.bouncycastle.tls.crypto.TlsStreamSigner? {
                // Raw signature 사용 시 null 반환
                return null
            }
        }
    }

    companion object {
        private val logger = KotlinLogging.logger {}
    }
}

/**
 * UDP DatagramTransport 구현.
 *
 * Bouncy Castle DTLS에서 사용하는 DatagramTransport 인터페이스를 구현합니다.
 */
private class UdpDatagramTransport(
    private val socket: DatagramSocket,
    private val remoteAddress: InetSocketAddress?,
    private val mtu: Int
) : DatagramTransport {

    private val receiveBuffer = ByteArray(mtu)

    override fun getReceiveLimit(): Int = mtu

    override fun getSendLimit(): Int = mtu - 28  // IP + UDP 헤더

    override fun receive(buf: ByteArray, off: Int, len: Int, waitMillis: Int): Int {
        socket.soTimeout = waitMillis

        return try {
            val packet = DatagramPacket(receiveBuffer, receiveBuffer.size)
            socket.receive(packet)

            val copyLen = minOf(len, packet.length)
            System.arraycopy(packet.data, packet.offset, buf, off, copyLen)
            copyLen
        } catch (e: java.net.SocketTimeoutException) {
            -1  // Timeout
        }
    }

    override fun send(buf: ByteArray, off: Int, len: Int) {
        val packet = if (remoteAddress != null) {
            DatagramPacket(buf, off, len, remoteAddress)
        } else if (socket.isConnected) {
            DatagramPacket(buf, off, len)
        } else {
            throw IOException("No remote address and socket not connected")
        }
        socket.send(packet)
    }

    override fun close() {
        // Socket은 외부에서 관리하므로 닫지 않음
    }
}

/**
 * DTLS 통계.
 */
data class DTLSStats(
    val fingerprint: String,
    val certificateSubject: String,
    val handshakeCompleted: Boolean = false
)
