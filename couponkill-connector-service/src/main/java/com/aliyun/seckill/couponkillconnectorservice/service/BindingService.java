package com.aliyun.seckill.couponkillconnectorservice.service;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.connector.EcommerceConnector;
import com.aliyun.seckill.common.connector.PlatformStockSnapshot;
import com.aliyun.seckill.common.connector.PlatformType;
import com.aliyun.seckill.common.connector.SkuBindingCommand;
import com.aliyun.seckill.common.connector.SyncStockRequest;
import com.aliyun.seckill.common.connector.SyncStockResult;
import com.aliyun.seckill.couponkillconnectorservice.domain.PlatformSkuBinding;
import com.aliyun.seckill.couponkillconnectorservice.domain.SyncBatchResult;
import com.aliyun.seckill.couponkillconnectorservice.feign.CouponStockFeignClient;
import com.aliyun.seckill.couponkillconnectorservice.mapper.PlatformSkuBindingMapper;
import com.aliyun.seckill.couponkillconnectorservice.spi.ConnectorRegistry;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;
import org.springframework.util.StringUtils;

import java.util.EnumSet;
import java.util.List;
import java.util.Set;

@Slf4j
@Service
@RequiredArgsConstructor
public class BindingService {

    private static final Set<PlatformType> BINDABLE =
            EnumSet.of(PlatformType.MOCK, PlatformType.JD);

    private final PlatformSkuBindingMapper bindingMapper;
    private final ConnectorRegistry connectorRegistry;
    private final CouponStockFeignClient couponStockFeignClient;

    @Value("${connector.internal-token:couponkill-internal}")
    private String internalToken;

    @Value("${connector.sync.default-mock-stock:500}")
    private long defaultMockStock;

    public PlatformSkuBinding createOrUpdate(SkuBindingCommand cmd) {
        if (cmd.getPlatform() == null || !StringUtils.hasText(cmd.getExternalSkuId()) || cmd.getCouponId() == null) {
            throw new IllegalArgumentException("platform / externalSkuId / couponId 不能为空");
        }
        if (!BINDABLE.contains(cmd.getPlatform())) {
            throw new IllegalArgumentException("平台暂不支持绑定: " + cmd.getPlatform() + "（仅 MOCK/JD）");
        }
        String sku = cmd.getExternalSkuId().trim();
        if (sku.isEmpty()) {
            throw new IllegalArgumentException("externalSkuId 不能为空");
        }

        // 同一 couponId 只允许一条绑定，避免多源覆盖 Redis
        PlatformSkuBinding byCoupon = bindingMapper.selectByCouponId(cmd.getCouponId());
        if (byCoupon != null
                && !(byCoupon.getPlatform() == cmd.getPlatform()
                && sku.equals(byCoupon.getExternalSkuId()))) {
            throw new IllegalArgumentException(
                    "couponId=" + cmd.getCouponId() + " 已绑定 "
                            + byCoupon.getPlatform() + "/" + byCoupon.getExternalSkuId()
                            + "，请先停用或改绑同一条记录");
        }

        PlatformSkuBinding existing = bindingMapper.selectByPlatformSku(cmd.getPlatform(), sku);
        boolean enabled = cmd.getSyncEnabled() == null || Boolean.TRUE.equals(cmd.getSyncEnabled());
        if (existing == null) {
            PlatformSkuBinding b = new PlatformSkuBinding();
            b.setPlatform(cmd.getPlatform());
            b.setExternalSkuId(sku);
            b.setCouponId(cmd.getCouponId());
            b.setSyncEnabled(enabled);
            b.setLastSyncStatus("NEW");
            bindingMapper.insert(b);
            return bindingMapper.selectById(b.getId());
        }
        existing.setCouponId(cmd.getCouponId());
        existing.setSyncEnabled(enabled);
        bindingMapper.updateById(existing);
        return bindingMapper.selectById(existing.getId());
    }

    public List<PlatformSkuBinding> listAll() {
        return bindingMapper.selectAll();
    }

    public PlatformSkuBinding get(Long id) {
        return bindingMapper.selectById(id);
    }

    /**
     * @param force true=校准覆盖（允许抬高库存）；false=安全合并（永不抬高）
     */
    public PlatformSkuBinding syncOne(Long bindingId, boolean force) {
        PlatformSkuBinding binding = bindingMapper.selectById(bindingId);
        if (binding == null) {
            throw new IllegalArgumentException("binding 不存在: " + bindingId);
        }
        return doSync(binding, force);
    }

    public SyncBatchResult syncAllEnabled(boolean force) {
        int ok = 0;
        int failed = 0;
        int skipped = 0;
        for (PlatformSkuBinding b : bindingMapper.selectSyncEnabled()) {
            try {
                PlatformSkuBinding r = doSync(b, force);
                if ("SKIP".equals(r.getLastSyncStatus())) {
                    skipped++;
                } else if ("SUCCESS".equals(r.getLastSyncStatus())) {
                    ok++;
                } else {
                    failed++;
                }
            } catch (Exception e) {
                failed++;
                log.warn("同步失败 bindingId={} platform={} sku={}: {}",
                        b.getId(), b.getPlatform(), b.getExternalSkuId(), e.getMessage());
            }
        }
        return SyncBatchResult.builder()
                .syncedOk(ok)
                .failed(failed)
                .skipped(skipped)
                .force(force)
                .build();
    }

    private PlatformSkuBinding doSync(PlatformSkuBinding binding, boolean force) {
        EcommerceConnector connector = connectorRegistry.require(binding.getPlatform());
        PlatformStockSnapshot stock = connector.getStock(binding.getExternalSkuId());
        Long target = resolveTargetStock(binding.getPlatform(), stock);
        if (target == null) {
            binding.setLastSyncStatus("SKIP");
            binding.setLastError("平台未返回可用精确库存，且无法推导目标值");
            bindingMapper.updateSyncResult(binding);
            return bindingMapper.selectById(binding.getId());
        }

        SyncStockRequest req = SyncStockRequest.builder()
                .couponId(binding.getCouponId())
                .targetStock(target)
                .sourcePlatform(binding.getPlatform())
                .externalSkuId(binding.getExternalSkuId())
                .force(force)
                .build();
        ApiResponse<SyncStockResult> resp = couponStockFeignClient.syncStock(req, internalToken);
        SyncStockResult result = resp != null ? resp.getData() : null;
        boolean success = result != null && result.isSuccess();
        // lastStock 记 Redis 实际 applied，而非平台目标（安全合并可能保持原值）
        Long applied = success ? result.getAppliedStock() : null;
        binding.setLastStock(applied != null ? applied : target);
        if (success) {
            binding.setLastSyncStatus("SUCCESS");
            if (Boolean.FALSE.equals(result.getChanged())
                    && result.getAppliedStock() != null
                    && !result.getAppliedStock().equals(target)) {
                binding.setLastError("safe-merge: target=" + target + ", applied=" + result.getAppliedStock());
            } else {
                binding.setLastError(null);
            }
        } else {
            binding.setLastSyncStatus("FAIL");
            binding.setLastError(result != null && result.getMessage() != null
                    ? result.getMessage()
                    : (resp != null ? resp.getMessage() : "coupon sync-stock empty response"));
        }
        bindingMapper.updateSyncResult(binding);
        log.info("库存同步完成 bindingId={} couponId={} target={} applied={} force={} status={}",
                binding.getId(), binding.getCouponId(), target, applied, force, binding.getLastSyncStatus());
        return bindingMapper.selectById(binding.getId());
    }

    private Long resolveTargetStock(PlatformType platform, PlatformStockSnapshot stock) {
        if (stock == null) {
            return null;
        }
        if (stock.getStockQty() != null && stock.getStockQty() >= 0) {
            return stock.getStockQty();
        }
        if (platform == PlatformType.JD && Boolean.TRUE.equals(stock.getStockAvailable())) {
            log.info("京东库存无精确数量，使用默认映射值 {}", defaultMockStock);
            return defaultMockStock;
        }
        if (platform == PlatformType.MOCK) {
            return defaultMockStock;
        }
        return 0L;
    }
}
