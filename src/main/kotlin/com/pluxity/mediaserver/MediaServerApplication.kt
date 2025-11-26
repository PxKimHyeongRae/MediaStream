package com.pluxity.mediaserver

import io.github.oshai.kotlinlogging.KotlinLogging
import org.springframework.boot.autoconfigure.SpringBootApplication
import org.springframework.boot.context.properties.ConfigurationPropertiesScan
import org.springframework.boot.runApplication

private val logger = KotlinLogging.logger {}

@SpringBootApplication
@ConfigurationPropertiesScan
class MediaServerApplication

fun main(args: Array<String>) {
    // Log JVM information
    logger.info {
        """
        Starting Media Server...
        JVM Info:
        - Version: ${System.getProperty("java.version")}
        - Vendor: ${System.getProperty("java.vendor")}
        - Max Memory: ${Runtime.getRuntime().maxMemory() / 1024 / 1024}MB
        - Available CPUs: ${Runtime.getRuntime().availableProcessors()}
        """.trimIndent()
    }

    // Check if ZGC is enabled
    val gcBeans = java.lang.management.ManagementFactory.getGarbageCollectorMXBeans()
    val usingZGC = gcBeans.any { it.name.contains("ZGC") }

    if (usingZGC) {
        logger.info { "✅ ZGC is enabled: ${gcBeans.filter { it.name.contains("ZGC") }.joinToString { it.name }}" }
    } else {
        logger.warn { "⚠️ ZGC not enabled! Add -XX:+UseZGC -XX:+ZGenerational to JVM options" }
        logger.warn { "Current GC: ${gcBeans.joinToString { it.name }}" }
    }

    runApplication<MediaServerApplication>(*args)
}
