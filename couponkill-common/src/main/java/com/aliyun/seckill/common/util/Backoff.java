package com.aliyun.seckill.common.util;

import java.util.concurrent.locks.LockSupport;

/**
 * VT 友好的有限退避：用 parkNanos 替代盲目 Thread.sleep，可中断。
 */
public final class Backoff {

    private Backoff() {
    }

    public static void parkMillis(long millis) {
        if (millis <= 0) {
            return;
        }
        LockSupport.parkNanos(millis * 1_000_000L);
        if (Thread.interrupted()) {
            Thread.currentThread().interrupt();
        }
    }

    /** 线性退避：baseMs * (attempt+1)，上限 maxMs */
    public static void linear(int attempt, long baseMs, long maxMs) {
        long delay = Math.min(maxMs, baseMs * (attempt + 1L));
        parkMillis(delay);
    }
}
