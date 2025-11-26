package com.pluxity.mediaserver.util;

/**
 * Virtual Threads utility for Java 21.
 *
 * Kotlin compiler may not recognize Virtual Thread APIs,
 * so we provide Java wrappers here.
 */
public class VirtualThreads {

    /**
     * Check if a thread is virtual.
     */
    public static boolean isVirtual(Thread thread) {
        return thread.isVirtual();
    }

    /**
     * Get thread ID.
     */
    public static long threadId(Thread thread) {
        return thread.threadId();
    }

    /**
     * Start a virtual thread.
     */
    public static Thread startVirtualThread(Runnable task) {
        return Thread.startVirtualThread(task);
    }
}
