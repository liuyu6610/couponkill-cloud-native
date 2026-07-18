package com.aliyun.seckill.couponkillconnectorservice.service;

import com.aliyun.seckill.common.connector.EcommerceConnector;
import com.aliyun.seckill.common.connector.PlatformProductSnapshot;
import com.aliyun.seckill.common.connector.PlatformType;
import com.aliyun.seckill.couponkillconnectorservice.domain.CouponPriceMap;
import com.aliyun.seckill.couponkillconnectorservice.domain.PlatformSkuBinding;
import com.aliyun.seckill.couponkillconnectorservice.domain.PriceCompareItem;
import com.aliyun.seckill.couponkillconnectorservice.domain.PriceCompareResult;
import com.aliyun.seckill.couponkillconnectorservice.spi.ConnectorRegistry;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.math.BigDecimal;
import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

/**
 * 同品比价：主绑定 probe + 手工映射（MANUAL 价或二次 probe）。
 * 不写热路径；完整 price_compare_group 表仍为后续增强。
 */
@Slf4j
@Service
@RequiredArgsConstructor
public class PriceCompareService {

    private final BindingService bindingService;
    private final PriceMapService priceMapService;
    private final ConnectorRegistry connectorRegistry;

    public PriceCompareResult compareByCoupon(Long couponId) {
        if (couponId == null) {
            throw new IllegalArgumentException("couponId 不能为空");
        }
        PlatformSkuBinding binding = bindingService.getByCouponId(couponId);
        List<PriceCompareItem> items = new ArrayList<>();
        Set<String> seen = new HashSet<>();

        PriceCompareResult.PriceCompareResultBuilder out = PriceCompareResult.builder()
                .couponId(couponId)
                .items(items);

        if (binding != null) {
            out.bindingId(binding.getId());
            PriceCompareItem primary = probeItem(binding.getPlatform(), binding.getExternalSkuId(), null);
            items.add(primary);
            seen.add(key(binding.getPlatform(), binding.getExternalSkuId()));
        }

        List<CouponPriceMap> maps = priceMapService.listEnabledByCoupon(couponId);
        for (CouponPriceMap map : maps) {
            if (map.getPlatform() == null || map.getExternalSkuId() == null) {
                continue;
            }
            String k = key(map.getPlatform(), map.getExternalSkuId());
            if (!seen.add(k)) {
                continue;
            }
            items.add(fromManualMap(map));
        }
        return out.build();
    }

    private PriceCompareItem fromManualMap(CouponPriceMap map) {
        LocalDateTime now = LocalDateTime.now();
        String currency = map.getCurrency() != null ? map.getCurrency() : "CNY";
        if (map.getManualPrice() != null) {
            return PriceCompareItem.builder()
                    .platform(map.getPlatform())
                    .externalSkuId(map.getExternalSkuId())
                    .title(map.getTitle())
                    .price(map.getManualPrice())
                    .currency(currency)
                    .fetchedAt(now)
                    .source("MANUAL")
                    .confidence("MEDIUM")
                    .message(null)
                    .build();
        }
        // 无手工价则尝试 probe（TB/PDD stub 会失败 → LOW）
        return probeItem(map.getPlatform(), map.getExternalSkuId(), map.getTitle());
    }

    private PriceCompareItem probeItem(PlatformType platform, String sku, String fallbackTitle) {
        LocalDateTime now = LocalDateTime.now();
        try {
            EcommerceConnector connector = connectorRegistry.require(platform);
            PlatformProductSnapshot snap = connector.getProduct(sku);
            BigDecimal price = snap != null ? snap.getPrice() : null;
            String title = snap != null && snap.getTitle() != null ? snap.getTitle() : fallbackTitle;
            return PriceCompareItem.builder()
                    .platform(platform)
                    .externalSkuId(sku)
                    .title(title)
                    .price(price)
                    .currency("CNY")
                    .fetchedAt(now)
                    .source("PROBE")
                    .confidence(confidenceFor(platform, price))
                    .message(price != null ? null : "probe 无价格")
                    .build();
        } catch (Exception e) {
            log.warn("price-compare probe 失败: platform={}, sku={}, err={}", platform, sku, e.getMessage());
            return PriceCompareItem.builder()
                    .platform(platform)
                    .externalSkuId(sku)
                    .title(fallbackTitle)
                    .currency("CNY")
                    .fetchedAt(now)
                    .source("PROBE")
                    .confidence("LOW")
                    .message("探测失败: " + e.getMessage())
                    .build();
        }
    }

    private static String confidenceFor(PlatformType platform, BigDecimal price) {
        if (price == null) {
            return "LOW";
        }
        if (platform == PlatformType.JD) {
            return "HIGH";
        }
        if (platform == PlatformType.MOCK || platform == PlatformType.TB || platform == PlatformType.PDD) {
            return "MEDIUM";
        }
        return "MEDIUM";
    }

    private static String key(PlatformType platform, String sku) {
        return platform.name() + "|" + sku;
    }
}
