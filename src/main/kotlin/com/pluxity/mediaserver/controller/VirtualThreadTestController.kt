package com.pluxity.mediaserver.controller

import com.pluxity.mediaserver.util.VirtualThreads
import io.github.oshai.kotlinlogging.KotlinLogging
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RestController

private val logger = KotlinLogging.logger {}

/**
 * Test controller to verify Virtual Threads are enabled.
 */
@RestController
@RequestMapping("/api/v1/test")
class VirtualThreadTestController {

    @GetMapping("/thread-info")
    fun getThreadInfo(): Map<String, Any> {
        val currentThread = Thread.currentThread()
        val isVirtual = VirtualThreads.isVirtual(currentThread)

        logger.info {
            "Thread Info - Name: ${currentThread.name}, IsVirtual: $isVirtual, Class: ${currentThread::class.simpleName}"
        }

        return mapOf<String, Any>(
            "threadName" to (currentThread.name as Any),
            "isVirtual" to (isVirtual as Any),
            "threadClass" to (currentThread::class.simpleName as Any),
            "threadId" to (VirtualThreads.threadId(currentThread) as Any),
            "state" to (currentThread.state.name as Any),
            "message" to (if (isVirtual) {
                "✅ Virtual Threads ENABLED - Spring Boot is using Virtual Threads!"
            } else {
                "❌ Virtual Threads DISABLED - Using platform threads"
            } as Any)
        )
    }

    @GetMapping("/blocking-test")
    fun blockingTest(): Map<String, Any> {
        val startThread = Thread.currentThread()
        logger.info { "Starting blocking operation on thread: ${startThread.name} (virtual: ${VirtualThreads.isVirtual(startThread)})" }

        // Simulate blocking I/O
        Thread.sleep(100)

        val endThread = Thread.currentThread()
        logger.info { "Finished blocking operation on thread: ${endThread.name} (virtual: ${VirtualThreads.isVirtual(endThread)})" }

        return mapOf<String, Any>(
            "startThread" to (startThread.name as Any),
            "endThread" to (endThread.name as Any),
            "isVirtual" to (VirtualThreads.isVirtual(endThread) as Any),
            "message" to ("Blocking operation completed on ${if (VirtualThreads.isVirtual(endThread)) "virtual" else "platform"} thread" as Any)
        )
    }
}
