package com.aliyun.seckill.couponkillconnectorservice.connector.pdd;

import com.aliyun.seckill.common.connector.ConnectorHealth;
import com.aliyun.seckill.common.connector.EcommerceConnector;
import com.aliyun.seckill.common.connector.PlatformProductSnapshot;
import com.aliyun.seckill.common.connector.PlatformStockSnapshot;
import com.aliyun.seckill.common.connector.PlatformType;
import org.springframework.stereotype.Component;

/**
 * 拼多多开放平台 Connector 预留 stub（二期实现）。
 * 不进秒杀热路径；health 恒为 DISABLED，避免误绑后假装成功。
 */
@Component
public class PddStubConnector implements EcommerceConnector {

    @Override
    public PlatformType platform() {
        return PlatformType.PDD;
    }

    @Override
    public PlatformProductSnapshot getProduct(String externalSkuId) {
        throw new UnsupportedOperationException("拼多多 Connector 二期未实现，请使用 MOCK/JD");
    }

    @Override
    public PlatformStockSnapshot getStock(String externalSkuId) {
        throw new UnsupportedOperationException("拼多多 Connector 二期未实现，请使用 MOCK/JD");
    }

    @Override
    public ConnectorHealth health() {
        return ConnectorHealth.builder()
                .platform(PlatformType.PDD)
                .status("DISABLED")
                .message("拼多多 Connector 预留（二期），SPI 已挂载")
                .build();
    }
}
