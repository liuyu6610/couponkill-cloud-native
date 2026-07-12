package com.aliyun.seckill.couponkillconnectorservice.job;

import com.aliyun.seckill.couponkillconnectorservice.service.BindingService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

@Slf4j
@Component
@RequiredArgsConstructor
public class StockSyncJob {

    private final BindingService bindingService;

    @Value("${connector.sync.enabled:true}")
    private boolean syncEnabled;

    @Scheduled(cron = "${connector.sync.cron:0 */1 * * * *}")
    public void syncEnabledBindings() {
        if (!syncEnabled) {
            return;
        }
        try {
            int ok = bindingService.syncAllEnabled(false).getSyncedOk();
            if (ok > 0) {
                log.info("定时库存同步完成: successCount={} (safe-merge)", ok);
            }
        } catch (Exception e) {
            log.warn("定时库存同步异常: {}", e.getMessage());
        }
    }
}
