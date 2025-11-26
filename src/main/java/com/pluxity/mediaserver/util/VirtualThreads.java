package com.pluxity.mediaserver.util;

/**
 * Virtual Threads utility for Java 21.
 *
 * Java 21의 Virtual Thread API를 Kotlin에서 사용할 수 있도록 Java로 작성한 유틸리티입니다.
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
