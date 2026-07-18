package com.aliyun.seckill.common.config;

import com.alibaba.nacos.shaded.com.google.common.util.concurrent.RateLimiter;
import lombok.Getter;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.cloud.context.config.annotation.RefreshScope;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.client.RestTemplate;

/**
 * Go 秒杀分流配置（热路径已冻结）。
 * <p>
 * 决策：保留 Go 模块作沙箱，禁止过载自动切流。
 * 仅当 {@code go.enabled=true} 且 {@code fallback-to-go=true} 时才允许显式走 Go（联调沙箱），
 * 默认两者均为 false，生产热路径 100% 走 Java Lua→Kafka。
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

    /** 默认 false：Go 热路径冻结，禁止生产开启 */
    @Getter
    @Value("${couponkill.seckill.go.enabled:false}")
    private boolean goServiceEnabled;

    /**
     * 显式沙箱开关。与 {@link #goServiceEnabled} 同时为 true 时才路由到 Go。
     * 已废除「Java QPS 打满自动切 Go」。
     */
    @Getter
    @Value("${couponkill.seckill.fallback-to-go:false}")
    private boolean fallbackToGo;

    /**
     * 保留限流器 Bean 供观测/扩展；不再用于 Go 分流决策。
     */
    @Bean
    public RateLimiter javaServiceRateLimiter() {
        return RateLimiter.create(javaQpsThreshold);
    }

    @Bean
    public RestTemplate goServiceRestTemplate() {
        return new RestTemplate();
    }

    /**
     * 是否路由到 Go：仅显式双开（enabled + fallback-to-go），禁止过载隐式切流。
     */
    public boolean shouldRouteToGo() {
        return goServiceEnabled && fallbackToGo;
    }
}
