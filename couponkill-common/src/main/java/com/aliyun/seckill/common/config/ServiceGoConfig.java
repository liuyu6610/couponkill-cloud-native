package com.aliyun.seckill.common.config;

import com.alibaba.nacos.shaded.com.google.common.util.concurrent.RateLimiter;
import lombok.Getter;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.cloud.context.config.annotation.RefreshScope;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.client.RestTemplate;

@Configuration
@RefreshScope
public class ServiceGoConfig {
    @Getter
    @Value("${service协同.java.qps-threshold:500}")
    private int javaQpsThreshold;

    @Getter
    @Value("${service协同.go.url:http://couponkill-go-service:8090}")
    private String goServiceUrl;

    @Getter
    @Value("${service协同.go.enabled:true}")
    private boolean goServiceEnabled;

    @Value("${service协同.fallback-to-go:false}")
    private boolean fallbackToGo;

    @Bean
    public RateLimiter javaServiceRateLimiter() {
        return RateLimiter.create(javaQpsThreshold);
    }

    @Bean
    public RestTemplate goServiceRestTemplate() {
        return new RestTemplate();
    }

    /**
     * 判断是否应该路由到Go服务
     * 1. 如果Go服务未启用，始终返回false
     * 2. 如果配置了强制fallback到Go服务，始终返回true
     * 3. 否则基于Java服务的QPS阈值进行判断
     */
    public boolean shouldRouteToGo() {
        // 如果Go服务未启用，直接返回false
        if (!goServiceEnabled) {
            return false;
        }
        
        // 如果配置了强制fallback到Go服务，直接返回true
        if (fallbackToGo) {
            return true;
        }
        
        // 否则基于Java服务的QPS阈值进行判断
        return !javaServiceRateLimiter().tryAcquire();
    }
}