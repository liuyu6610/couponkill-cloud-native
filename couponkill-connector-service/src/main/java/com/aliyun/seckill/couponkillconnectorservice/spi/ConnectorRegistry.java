package com.aliyun.seckill.couponkillconnectorservice.spi;

import com.aliyun.seckill.common.connector.EcommerceConnector;
import com.aliyun.seckill.common.connector.PlatformType;
import org.springframework.stereotype.Component;

import java.util.EnumMap;
import java.util.List;
import java.util.Map;

@Component
public class ConnectorRegistry {

    private final Map<PlatformType, EcommerceConnector> byPlatform = new EnumMap<>(PlatformType.class);

    public ConnectorRegistry(List<EcommerceConnector> connectors) {
        for (EcommerceConnector c : connectors) {
            byPlatform.put(c.platform(), c);
        }
    }

    public EcommerceConnector require(PlatformType platform) {
        EcommerceConnector c = byPlatform.get(platform);
        if (c == null) {
            throw new IllegalArgumentException("未注册平台 Connector: " + platform);
        }
        return c;
    }

    public List<EcommerceConnector> all() {
        return List.copyOf(byPlatform.values());
    }
}
