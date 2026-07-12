package com.aliyun.seckill.couponkillconnectorservice.connector.mock;

import com.aliyun.seckill.common.connector.ConnectorHealth;
import com.aliyun.seckill.common.connector.EcommerceConnector;
import com.aliyun.seckill.common.connector.PlatformProductSnapshot;
import com.aliyun.seckill.common.connector.PlatformStockSnapshot;
import com.aliyun.seckill.common.connector.PlatformType;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.math.BigDecimal;

/**
 * 本地联调 Mock：无京东密钥时也可完成「绑定→同步→预热→秒杀」。
 */
@Component
public class MockEcommerceConnector implements EcommerceConnector {

    @Value("${connector.sync.default-mock-stock:500}")
    private long defaultMockStock;

    @Override
    public PlatformType platform() {
        return PlatformType.MOCK;
    }

    @Override
    public PlatformProductSnapshot getProduct(String externalSkuId) {
        return PlatformProductSnapshot.builder()
                .platform(PlatformType.MOCK)
                .externalSkuId(externalSkuId)
                .title("Mock商品-" + externalSkuId)
                .price(new BigDecimal("99.00"))
                .onSale(true)
                .rawStatus("ON_SALE")
                .build();
    }

    @Override
    public PlatformStockSnapshot getStock(String externalSkuId) {
        // 允许用 sku 后缀编码库存，例如 mock-sku:800
        long qty = defaultMockStock;
        int idx = externalSkuId != null ? externalSkuId.lastIndexOf(':') : -1;
        if (idx > 0 && idx < externalSkuId.length() - 1) {
            try {
                qty = Long.parseLong(externalSkuId.substring(idx + 1));
            } catch (NumberFormatException ignored) {
                // keep default
            }
        }
        return PlatformStockSnapshot.builder()
                .platform(PlatformType.MOCK)
                .externalSkuId(externalSkuId)
                .stockQty(qty)
                .stockAvailable(qty > 0)
                .stockStateDesc("MOCK_IN_STOCK")
                .area("mock")
                .build();
    }

    @Override
    public ConnectorHealth health() {
        return ConnectorHealth.builder()
                .platform(PlatformType.MOCK)
                .status("UP")
                .message("mock connector ready")
                .build();
    }
}
