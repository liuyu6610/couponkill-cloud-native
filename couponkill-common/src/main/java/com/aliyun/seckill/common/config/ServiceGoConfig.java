package com.aliyun.seckill.common.config;

import com.alibaba.nacos.shaded.com.google.common.util.concurrent.RateLimiter;
import lombok.Getter;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.cloud.context.config.annotation.RefreshScope;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.client.RestTemplate;

/**
 * Go 秒杀分流配置。默认关闭：Go 引擎数据源/分片/键名未与 Java 对齐前不得在过载时切流。
 */
@Configuration
@RefreshScope
public class ServiceGoConfig {
    @Getter
    @Value("${couponkill.seckill.java.qps-threshold:500}")
    private int javaQpsThreshold;

    @Getter
    @Value("${couponkill.seckill.go.url:http://seckill-go-svc:8083}")
    private String goServiceUrl;

    /** 默认 false：Go 路径修好前禁止路由 */
    @Getter
    @Value("${couponkill.seckill.go.enabled:false}")
    private boolean goServiceEnabled;

    @Value("${couponkill.seckill.fallback-to-go:false}")
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
     */
    public boolean shouldRouteToGo() {
        if (!goServiceEnabled) {
            return false;
        }
        if (fallbackToGo) {
            return true;
        }
        return !javaServiceRateLimiter().tryAcquire();
    }
}
