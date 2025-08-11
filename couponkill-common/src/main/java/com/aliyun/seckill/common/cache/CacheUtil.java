package com.aliyun.seckill.common.cache;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.springframework.data.redis.core.StringRedisTemplate;

import java.time.Duration;
import java.util.function.Supplier;

public class CacheUtil {
    private final StringRedisTemplate redisTemplate;
    private final ObjectMapper objectMapper = new ObjectMapper();

    public CacheUtil(StringRedisTemplate redisTemplate) {
        this.redisTemplate = redisTemplate;
    }

    /**
     * 缓存查询模板方法
     */
    public <T> T queryWithPassThrough(String key, Class<T> type, Supplier<T> dbFallback, Duration ttl) {
        String json = redisTemplate.opsForValue().get(key);
        if (json != null) {
            try {
                return objectMapper.readValue(json, type);
            } catch (JsonProcessingException e) {
                // JSON解析失败，从数据库重新获取
                e.printStackTrace();
            }
        }
        T data = dbFallback.get();
        if (data == null) {
            redisTemplate.opsForValue().set(key, "", Duration.ofMinutes(2)); // 空值短期缓存
            return null;
        }
        try {
            redisTemplate.opsForValue().set(key, objectMapper.writeValueAsString(data), ttl.plus(Duration.ofMillis(randomJitter())));
        } catch (JsonProcessingException e) {
            e.printStackTrace();
        }
        return data;
    }

    private long randomJitter() {
        return (long) (Math.random() * 1000 * 60); // 0-60秒随机
    }
}
