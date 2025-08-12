package com.aliyun.seckill.couponkillgateway.config;

import com.alibaba.csp.sentinel.adapter.gateway.sc.SentinelGatewayFilter;
import org.springframework.cloud.gateway.filter.GatewayFilter;
import org.springframework.cloud.gateway.filter.factory.AbstractGatewayFilterFactory;
import org.springframework.stereotype.Component;

import java.util.Collections;
import java.util.List;

@Component
public class SentinelGatewayFilterFactory extends AbstractGatewayFilterFactory<SentinelGatewayFilterFactory.Config> {

    private final SentinelGatewayFilter sentinelGatewayFilter;

    public SentinelGatewayFilterFactory (SentinelGatewayFilter sentinelGatewayFilter) {
        super(Config.class);
        this.sentinelGatewayFilter = sentinelGatewayFilter;
    }

    @Override
    public GatewayFilter apply(Config config) {
        return sentinelGatewayFilter;
    }

    @Override
    public List<String> shortcutFieldOrder() {
        return Collections.singletonList("resourceName");
    }

    public static class Config {
        private String resourceName;

        public String getResourceName() {
            return resourceName;
        }

        public void setResourceName(String resourceName) {
            this.resourceName = resourceName;
        }
    }
}
