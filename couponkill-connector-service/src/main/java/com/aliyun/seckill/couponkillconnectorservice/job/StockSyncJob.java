package com.aliyun.seckill.couponkillconnectorservice.job;

import com.aliyun.seckill.couponkillconnectorservice.service.BindingService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

import java.time.Duration;
import java.util.UUID;

/**
 * 定时库存同步；多实例用 Redis 锁防重入（Connector 不在秒杀热路径）。
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class StockSyncJob {

    private static final String LOCK_KEY = "connector:sync:all:lock";

    private final BindingService bindingService;
    private final StringRedisTemplate stringRedisTemplate;

    @Value("${connector.sync.enabled:true}")
    private boolean syncEnabled;

    @Value("${connector.sync.lock-ttl-seconds:55}")
    private long lockTtlSeconds;

    @Scheduled(cron = "${connector.sync.cron:0 */1 * * * *}")
    public void syncEnabledBindings() {
        if (!syncEnabled) {
            return;
        }
        String token = UUID.randomUUID().toString();
        Boolean locked = stringRedisTemplate.opsForValue()
                .setIfAbsent(LOCK_KEY, token, Duration.ofSeconds(Math.max(10, lockTtlSeconds)));
        if (!Boolean.TRUE.equals(locked)) {
            log.debug("定时库存同步跳过：其它实例持有锁");
            return;
        }
        try {
            int ok = bindingService.syncAllEnabled(false).getSyncedOk();
            if (ok > 0) {
                log.info("定时库存同步完成: successCount={} (safe-merge)", ok);
            }
        } catch (Exception e) {
            log.warn("定时库存同步异常: {}", e.getMessage());
        } finally {
            releaseLock(token);
        }
    }

    private void releaseLock(String token) {
        try {
            String current = stringRedisTemplate.opsForValue().get(LOCK_KEY);
            if (token.equals(current)) {
                stringRedisTemplate.delete(LOCK_KEY);
            }
        } catch (Exception e) {
            log.debug("释放同步锁失败（TTL 会自行过期）: {}", e.getMessage());
        }
    }
}
