package com.aliyun.seckill.couponkillconnectorservice.controller;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.connector.ConnectorHealth;
import com.aliyun.seckill.common.connector.EcommerceConnector;
import com.aliyun.seckill.common.connector.PlatformProductSnapshot;
import com.aliyun.seckill.common.connector.PlatformStockSnapshot;
import com.aliyun.seckill.common.connector.PlatformType;
import com.aliyun.seckill.common.connector.SkuBindingCommand;
import com.aliyun.seckill.couponkillconnectorservice.config.JdConnectorProperties;
import com.aliyun.seckill.couponkillconnectorservice.domain.CouponPriceMap;
import com.aliyun.seckill.couponkillconnectorservice.domain.CouponPriceMapCommand;
import com.aliyun.seckill.couponkillconnectorservice.domain.PlatformSkuBinding;
import com.aliyun.seckill.couponkillconnectorservice.domain.PriceCompareResult;
import com.aliyun.seckill.couponkillconnectorservice.domain.SyncBatchResult;
import com.aliyun.seckill.couponkillconnectorservice.service.BindingService;
import com.aliyun.seckill.couponkillconnectorservice.service.PriceCompareService;
import com.aliyun.seckill.couponkillconnectorservice.service.PriceMapService;
import com.aliyun.seckill.couponkillconnectorservice.spi.ConnectorRegistry;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import lombok.RequiredArgsConstructor;
import org.springframework.util.StringUtils;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

@Tag(name = "电商 Connector")
@RestController
@RequestMapping("/api/v1/connector")
@RequiredArgsConstructor
public class ConnectorController {

    private final BindingService bindingService;
    private final PriceCompareService priceCompareService;
    private final PriceMapService priceMapService;
    private final ConnectorRegistry connectorRegistry;
    private final JdConnectorProperties jdProps;

    @Operation(summary = "创建或更新 SKU 绑定")
    @PostMapping("/bindings")
    public ApiResponse<PlatformSkuBinding> createBinding(@RequestBody SkuBindingCommand command) {
        return ApiResponse.success(bindingService.createOrUpdate(command));
    }

    @Operation(summary = "绑定列表")
    @GetMapping("/bindings")
    public ApiResponse<List<PlatformSkuBinding>> listBindings() {
        return ApiResponse.success(bindingService.listAll());
    }

    @Operation(summary = "按券查询绑定（C 端只读）")
    @GetMapping("/bindings/by-coupon/{couponId}")
    public ApiResponse<PlatformSkuBinding> bindingByCoupon(@PathVariable Long couponId) {
        return ApiResponse.success(bindingService.getByCouponId(couponId));
    }

    @Operation(summary = "同品比价（C 端只读；绑定 probe + 手工映射）")
    @GetMapping("/price-compare")
    public ApiResponse<PriceCompareResult> priceCompare(@RequestParam Long couponId) {
        return ApiResponse.success(priceCompareService.compareByCoupon(couponId));
    }

    @Operation(summary = "创建或更新同品比价手工映射（admin）")
    @PostMapping("/price-maps")
    public ApiResponse<CouponPriceMap> upsertPriceMap(@RequestBody CouponPriceMapCommand command) {
        return ApiResponse.success(priceMapService.upsert(command));
    }

    @Operation(summary = "按券列出手工业价映射（admin）")
    @GetMapping("/price-maps")
    public ApiResponse<List<CouponPriceMap>> listPriceMaps(@RequestParam Long couponId) {
        return ApiResponse.success(priceMapService.listByCoupon(couponId));
    }

    @Operation(summary = "删除手工比价映射（admin）")
    @DeleteMapping("/price-maps/{id}")
    public ApiResponse<Void> deletePriceMap(@PathVariable Long id) {
        priceMapService.delete(id);
        return ApiResponse.success(null);
    }

    @Operation(summary = "手动同步单个绑定；force=true 允许抬高库存（校准）")
    @PostMapping("/sync/{id}")
    public ApiResponse<PlatformSkuBinding> syncOne(
            @PathVariable Long id,
            @RequestParam(defaultValue = "false") boolean force) {
        return ApiResponse.success(bindingService.syncOne(id, force));
    }

    @Operation(summary = "同步全部已启用绑定")
    @PostMapping("/sync")
    public ApiResponse<SyncBatchResult> syncAll(@RequestParam(defaultValue = "false") boolean force) {
        return ApiResponse.success(bindingService.syncAllEnabled(force));
    }

    @Operation(summary = "Connector 健康")
    @GetMapping("/health")
    public ApiResponse<List<ConnectorHealth>> health() {
        return ApiResponse.success(
                connectorRegistry.all().stream().map(EcommerceConnector::health).toList()
        );
    }

    @Operation(summary = "平台列表（含 JD 配置就绪态，不含密钥）")
    @GetMapping("/platforms")
    public ApiResponse<List<Map<String, Object>>> platforms() {
        return ApiResponse.success(connectorRegistry.all().stream().map(c -> {
            Map<String, Object> m = new LinkedHashMap<>();
            ConnectorHealth h = c.health();
            m.put("platform", c.platform().name());
            m.put("status", h.getStatus());
            m.put("message", h.getMessage());
            if (c.platform() == PlatformType.JD) {
                m.put("jdEnabled", jdProps.isEnabled());
                m.put("jdCredentialsConfigured", jdProps.credentialsPresent());
                m.put("jdServerUrl", jdProps.getServerUrl());
                m.put("jdDefaultArea", jdProps.getDefaultArea());
                m.put("jdAppKeyMasked", mask(jdProps.getAppKey()));
            }
            return m;
        }).toList());
    }

    @Operation(summary = "探测平台商品（不写 Redis）")
    @GetMapping("/probe/{platform}/product")
    public ApiResponse<PlatformProductSnapshot> probeProduct(
            @PathVariable String platform,
            @RequestParam String skuId) {
        return ApiResponse.success(requireConnector(platform).getProduct(requireSku(skuId)));
    }

    @Operation(summary = "探测平台库存（不写 Redis）")
    @GetMapping("/probe/{platform}/stock")
    public ApiResponse<PlatformStockSnapshot> probeStock(
            @PathVariable String platform,
            @RequestParam String skuId) {
        return ApiResponse.success(requireConnector(platform).getStock(requireSku(skuId)));
    }

    private EcommerceConnector requireConnector(String platform) {
        if (!StringUtils.hasText(platform)) {
            throw new IllegalArgumentException("platform 不能为空");
        }
        PlatformType type;
        try {
            type = PlatformType.valueOf(platform.trim().toUpperCase());
        } catch (IllegalArgumentException e) {
            throw new IllegalArgumentException("未知平台: " + platform);
        }
        return connectorRegistry.require(type);
    }

    private static String requireSku(String skuId) {
        if (!StringUtils.hasText(skuId)) {
            throw new IllegalArgumentException("skuId 不能为空");
        }
        return skuId.trim();
    }

    private static String mask(String key) {
        if (key == null || key.isBlank()) {
            return "";
        }
        if (key.length() <= 4) {
            return "****";
        }
        return key.substring(0, 2) + "****" + key.substring(key.length() - 2);
    }
}
