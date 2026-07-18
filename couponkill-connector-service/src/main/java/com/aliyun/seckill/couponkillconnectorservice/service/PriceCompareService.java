package com.aliyun.seckill.couponkillconnectorservice.service;

import com.aliyun.seckill.common.connector.EcommerceConnector;
import com.aliyun.seckill.common.connector.PlatformProductSnapshot;
import com.aliyun.seckill.common.connector.PlatformType;
import com.aliyun.seckill.couponkillconnectorservice.domain.PlatformSkuBinding;
import com.aliyun.seckill.couponkillconnectorservice.domain.PriceCompareItem;
import com.aliyun.seckill.couponkillconnectorservice.domain.PriceCompareResult;
import com.aliyun.seckill.couponkillconnectorservice.spi.ConnectorRegistry;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.List;

/**
 * 同品比价：按券绑定探测平台现价（不写库、不挡热路径）。
 * 完整 price_compare_group 表为 P2；本实现以绑定 + probe 为最小可用真源。
 */
@Slf4j
@Service
@RequiredArgsConstructor
public class PriceCompareService {

    private final BindingService bindingService;
    private final ConnectorRegistry connectorRegistry;

    public PriceCompareResult compareByCoupon(Long couponId) {
        if (couponId == null) {
            throw new IllegalArgumentException("couponId 不能为空");
        }
        PlatformSkuBinding binding = bindingService.getByCouponId(couponId);
        PriceCompareResult.PriceCompareResultBuilder out = PriceCompareResult.builder()
                .couponId(couponId)
                .items(new ArrayList<>());
        if (binding == null) {
            return out.build();
        }
        out.bindingId(binding.getId());
        List<PriceCompareItem> items = new ArrayList<>();
        items.add(probeItem(binding));
        // 预留：未来可并入手工 TB/PDD 映射与多绑定
        out.items(items);
        return out.build();
    }

    private PriceCompareItem probeItem(PlatformSkuBinding binding) {
        PlatformType platform = binding.getPlatform();
        String sku = binding.getExternalSkuId();
        LocalDateTime now = LocalDateTime.now();
        try {
            EcommerceConnector connector = connectorRegistry.require(platform);
            PlatformProductSnapshot snap = connector.getProduct(sku);
            String confidence = confidenceFor(platform, snap);
            return PriceCompareItem.builder()
                    .platform(platform)
                    .externalSkuId(sku)
                    .title(snap != null ? snap.getTitle() : null)
                    .price(snap != null ? snap.getPrice() : null)
                    .currency("CNY")
                    .fetchedAt(now)
                    .source("PROBE")
                    .confidence(confidence)
                    .message(snap != null && snap.getPrice() != null ? null : "probe 无价格")
                    .build();
        } catch (Exception e) {
            log.warn("price-compare probe 失败: platform={}, sku={}, err={}", platform, sku, e.getMessage());
            return PriceCompareItem.builder()
                    .platform(platform)
                    .externalSkuId(sku)
                    .currency("CNY")
                    .fetchedAt(now)
                    .source("PROBE")
                    .confidence("LOW")
                    .message("探测失败: " + e.getMessage())
                    .build();
        }
    }

    private static String confidenceFor(PlatformType platform, PlatformProductSnapshot snap) {
        if (snap == null || snap.getPrice() == null) {
            return "LOW";
        }
        // MOCK / Stub 平台不当置信官方 API
        if (platform == PlatformType.MOCK || platform == PlatformType.TB || platform == PlatformType.PDD) {
            return "MEDIUM";
        }
        if (platform == PlatformType.JD) {
            return "HIGH";
        }
        return "MEDIUM";
    }
}
