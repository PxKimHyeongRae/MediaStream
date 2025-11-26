package com.pluxity.mediaserver.controller

import io.github.oshai.kotlinlogging.KotlinLogging
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RestController
import java.time.Instant

private val logger = KotlinLogging.logger {}

@RestController
@RequestMapping("/api/v1")
class HealthController {

    @GetMapping("/health")
    fun health(): Map<String, Any> {
        logger.debug { "Health check requested" }

        return mapOf(
            "status" to "UP",
            "timestamp" to Instant.now().toString(),
            "jvm" to mapOf(
                "version" to System.getProperty("java.version"),
                "maxMemory" to "${Runtime.getRuntime().maxMemory() / 1024 / 1024}MB",
                "freeMemory" to "${Runtime.getRuntime().freeMemory() / 1024 / 1024}MB",
                "totalMemory" to "${Runtime.getRuntime().totalMemory() / 1024 / 1024}MB"
            )
        )
    }

    @GetMapping("/ready")
    fun ready(): Map<String, String> {
        return mapOf("status" to "READY")
    }
}
