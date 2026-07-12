package com.aliyun.seckill.common.config;

import com.aliyun.seckill.common.utils.SnowflakeIdGenerator;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import java.net.InetAddress;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.Locale;
import java.util.UUID;

/**
 * 雪花 ID 生成器：workerId / datacenterId 按实例稳定推导，避免多 Pod 硬编码 (1,1) 撞号。
 * <p>
 * 优先级：显式配置 snowflake.worker-id / datacenter-id → HOSTNAME/POD_NAME 哈希 → 本机名哈希。
 */
@Slf4j
@Configuration
public class SnowflakeConfig {

    private static final long MAX_WORKER_ID = 31L;
    private static final long MAX_DATACENTER_ID = 31L;

    @Value("${snowflake.worker-id:#{null}}")
    private Long configuredWorkerId;

    @Value("${snowflake.datacenter-id:#{null}}")
    private Long configuredDatacenterId;

    @Bean
    public SnowflakeIdGenerator snowflakeIdGenerator() {
        long workerId = configuredWorkerId != null
                ? clamp(configuredWorkerId, MAX_WORKER_ID)
                : deriveId("worker");
        long datacenterId = configuredDatacenterId != null
                ? clamp(configuredDatacenterId, MAX_DATACENTER_ID)
                : deriveId("datacenter");

        // 同一实例两次哈希若撞到相同低 5 位，对 datacenter 再扰动一次
        if (configuredWorkerId == null && configuredDatacenterId == null && workerId == datacenterId) {
            datacenterId = (datacenterId + 1) & MAX_DATACENTER_ID;
        }

        log.info("SnowflakeIdGenerator ready: workerId={}, datacenterId={}, instanceKey={}",
                workerId, datacenterId, instanceKey());
        return new SnowflakeIdGenerator(workerId, datacenterId);
    }

    private long deriveId(String salt) {
        try {
            MessageDigest md = MessageDigest.getInstance("SHA-256");
            byte[] digest = md.digest((instanceKey() + "|" + salt).getBytes(StandardCharsets.UTF_8));
            int value = ((digest[0] & 0xff) << 8) | (digest[1] & 0xff);
            return value & MAX_WORKER_ID;
        } catch (Exception e) {
            long fallback = Math.abs(UUID.randomUUID().getMostSignificantBits()) & MAX_WORKER_ID;
            log.warn("Snowflake id derive failed, use random fallback {}: {}", fallback, e.getMessage());
            return fallback;
        }
    }

    private static String instanceKey() {
        String hostname = firstNonBlank(
                System.getenv("HOSTNAME"),
                System.getenv("POD_NAME"),
                System.getProperty("snowflake.instance-id")
        );
        if (hostname == null) {
            try {
                hostname = InetAddress.getLocalHost().getHostName();
            } catch (Exception ignored) {
                hostname = "unknown-" + UUID.randomUUID();
            }
        }
        return hostname.toLowerCase(Locale.ROOT);
    }

    private static String firstNonBlank(String... values) {
        if (values == null) {
            return null;
        }
        for (String v : values) {
            if (v != null && !v.isBlank()) {
                return v.trim();
            }
        }
        return null;
    }

    private static long clamp(long value, long max) {
        if (value < 0 || value > max) {
            throw new IllegalArgumentException("snowflake id out of range [0," + max + "]: " + value);
        }
        return value;
    }
}
