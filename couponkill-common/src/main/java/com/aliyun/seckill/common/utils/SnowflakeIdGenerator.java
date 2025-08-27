package com.aliyun.seckill.common.utils;

/**
 * 雪花算法 ID 生成器
 * 支持正序/倒序 ID
 */
public class SnowflakeIdGenerator {

    private final long workerId;
    private final long datacenterId;
    private long sequence = 0L;

    private final long twepoch = 1700000000000L; // 自定义时间戳起点

    private final long workerIdBits = 5L;
    private final long datacenterIdBits = 5L;
    // 修复整数溢出问题
    private final long maxWorkerId = ~(-1L << workerIdBits);
    private final long maxDatacenterId = ~(-1L << datacenterIdBits);
    private final long sequenceBits = 12L;

    private final long workerIdShift = sequenceBits;
    private final long datacenterIdShift = sequenceBits + workerIdBits;
    private final long timestampLeftShift = sequenceBits + workerIdBits + datacenterIdBits;
    // 修复整数溢出问题
    private final long sequenceMask = ~(-1L << sequenceBits);

    private long lastTimestamp = -1L;

    public SnowflakeIdGenerator(long workerId, long datacenterId) {
        if ( workerId > maxWorkerId || workerId < 0 ) {
            throw new IllegalArgumentException("worker Id out of range");
        }
        if ( datacenterId > maxDatacenterId || datacenterId < 0 ) {
            throw new IllegalArgumentException("datacenter Id out of range");
        }
        this.workerId = workerId;
        this.datacenterId = datacenterId;
    }

    public synchronized long nextId() {
        long timestamp = timeGen();
        if ( timestamp < lastTimestamp ) {
            throw new RuntimeException("Clock moved backwards.");
        }
        if ( lastTimestamp == timestamp ) {
            sequence = (sequence + 1) & sequenceMask;
            if ( sequence == 0 ) {
                timestamp = tilNextMillis(lastTimestamp);
            }
        } else {
            sequence = 0L;
        }
        lastTimestamp = timestamp;
        return ((timestamp - twepoch) << timestampLeftShift)
                | (datacenterId << datacenterIdShift)
                | (workerId << workerIdShift)
                | sequence;
    }

    private long tilNextMillis(long lastTimestamp) {
        long timestamp = timeGen();
        while (timestamp <= lastTimestamp) {
            timestamp = timeGen();
        }
        return timestamp;
    }

    private long timeGen() {
        return System.currentTimeMillis();
    }
}
