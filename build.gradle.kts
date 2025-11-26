import org.jetbrains.kotlin.gradle.tasks.KotlinCompile

plugins {
    id("org.springframework.boot") version "3.2.0"
    id("io.spring.dependency-management") version "1.1.4"
    kotlin("jvm") version "1.9.21"
    kotlin("plugin.spring") version "1.9.21"
}

group = "com.pluxity"
version = "0.1.0-SNAPSHOT"

java {
    sourceCompatibility = JavaVersion.VERSION_21
    targetCompatibility = JavaVersion.VERSION_21
    toolchain {
        languageVersion.set(JavaLanguageVersion.of(21))
    }
}

repositories {
    mavenCentral()
    maven { url = uri("https://repo.spring.io/milestone") }

    // Jitsi Maven Repository (GitHub raw)
    maven {
        url = uri("https://raw.githubusercontent.com/jitsi/jitsi-maven-repository/master/releases/")
        metadataSources {
            artifact()
        }
    }

    maven { url = uri("https://jitpack.io") }
}

dependencies {
    // Spring Boot
    implementation("org.springframework.boot:spring-boot-starter-web")
    implementation("org.springframework.boot:spring-boot-starter-websocket")
    implementation("org.springframework.boot:spring-boot-starter-actuator")

    // Kotlin
    implementation("org.jetbrains.kotlin:kotlin-reflect")
    implementation("org.jetbrains.kotlin:kotlin-stdlib-jdk8")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.8.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-reactor:1.8.0")

    // Jackson for JSON
    implementation("com.fasterxml.jackson.module:jackson-module-kotlin")
    implementation("com.fasterxml.jackson.dataformat:jackson-dataformat-yaml")

    // Netty (for ByteBuf and high-performance networking)
    implementation("io.netty:netty-buffer:4.1.104.Final")
    implementation("io.netty:netty-codec:4.1.104.Final")
    implementation("io.netty:netty-handler:4.1.104.Final")

    // WebRTC - Direct implementation with Jitsi components
    // Jitsi 최신 버전 (2025년 7월 배포)

    // 1. Ice4j (ICE/STUN/TURN 처리)
    //webrtc-java
    // C++
    implementation("org.jitsi:ice4j:3.2-9-gb64c86f")

    // 2. Jitsi SRTP 제거 - Pure Java SRTPTransformer 사용 (BouncyCastle 기반)
    // implementation("org.jitsi:jitsi-srtp:1.1-21-g66f32c3")  // JNI 의존성 (jitsisrtp_3.dll) 있어서 제거

    // JSON 처리
    implementation("com.googlecode.json-simple:json-simple:1.1.1")

    // Bouncy Castle for cryptography backend
    implementation("org.bouncycastle:bcprov-jdk18on:1.77")
    implementation("org.bouncycastle:bcpkix-jdk18on:1.77")
    implementation("org.bouncycastle:bctls-jdk18on:1.77")  // DTLS support

    // RTSP/FFmpeg (JavaCV)
    implementation("org.bytedeco:javacv-platform:1.5.9")

    // Logging
    implementation("io.github.oshai:kotlin-logging-jvm:5.1.0")

    // Micrometer for metrics
    implementation("io.micrometer:micrometer-registry-prometheus")

    // Configuration
    implementation("org.springframework.boot:spring-boot-configuration-processor")
    annotationProcessor("org.springframework.boot:spring-boot-configuration-processor")

    // Dev tools
    developmentOnly("org.springframework.boot:spring-boot-devtools")

    // Test
    testImplementation("org.springframework.boot:spring-boot-starter-test")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.8.0")
    testImplementation("io.mockk:mockk:1.13.8")
}

tasks.withType<KotlinCompile> {
    kotlinOptions {
        freeCompilerArgs += "-Xjsr305=strict"
        jvmTarget = "21"
    }
}

tasks.withType<Test> {
    useJUnitPlatform()

    // JVM options for tests (requires Java 21+)
    jvmArgs(
        "-XX:+UseZGC",
        "-XX:+ZGenerational"
    )
}

tasks.register<JavaExec>("runWithZGC") {
    group = "application"
    description = "Run the application with ZGC enabled"

    classpath = sourceSets["main"].runtimeClasspath
    mainClass.set("com.pluxity.mediaserver.MediaServerApplicationKt")

    jvmArgs(
        "-XX:+UseZGC",
        "-XX:+ZGenerational",
        "-Xms2g",
        "-Xmx4g",
        "-XX:MaxDirectMemorySize=2g",
        "-XX:+AlwaysPreTouch"
    )
}

springBoot {
    buildInfo()
}
