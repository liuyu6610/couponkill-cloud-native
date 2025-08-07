// couponkill-common/src/main/java/com/aliyun/seckill/common/redis/RedisCache.java
package com.aliyun.seckill.common.redis;

import com.google.common.hash.BloomFilter;
import com.google.common.hash.Funnels;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.stereotype.Component;

import java.nio.charset.Charset;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.locks.ReentrantLock;

@Component
public class RedisCache {

    @Autowired
    private RedisTemplate<String, Object> redisTemplate;

    // 布隆过滤器，解决缓存穿透（抑制 Guava @Beta 警告）
    @SuppressWarnings("UnstableApiUsage")
    private final BloomFilter<String> bloomFilter = BloomFilter.create(
            Funnels.stringFunnel(Charset.defaultCharset()),
            1000000,
            0.01
    );

    // 解决缓存击穿的锁
    private final ReentrantLock lock = new ReentrantLock();

    // 添加到布隆过滤器
    public void addToBloomFilter(String key) {
        bloomFilter.put(key);
    }

    // 从缓存获取，解决缓存穿透、击穿问题
    public Object get(String key, DataLoader loader, long expireTime, TimeUnit timeUnit) {
        // 1. 先查布隆过滤器，如果不存在直接返回null
        if (!bloomFilter.mightContain(key)) {
            return null;
        }

        // 2. 查缓存
        Object value = redisTemplate.opsForValue().get(key);
        if (value != null) {
            return value;
        }

        // 3. 缓存未命中，加锁查询数据库
        try {
            lock.lock();
            // 双重检查
            value = redisTemplate.opsForValue().get(key);
            if (value != null) {
                return value;
            }

            // 4. 从数据库加载
            value = loader.load();
            if (value != null) {
                // 设置随机过期时间，解决缓存雪崩
                long randomExpire = expireTime + (long)(Math.random() * expireTime * 0.1);
                redisTemplate.opsForValue().set(key, value, randomExpire, timeUnit);
            }
            return value;
        } finally {
            lock.unlock();
        }
    }

    // 数据加载接口
    @FunctionalInterface
    public interface DataLoader {
        Object load();
    }
}
