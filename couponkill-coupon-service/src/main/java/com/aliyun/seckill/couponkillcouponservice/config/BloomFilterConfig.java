package com.aliyun.seckill.couponkillcouponservice.config;

import com.google.common.hash.BloomFilter;
import com.google.common.hash.Funnels;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import java.nio.charset.StandardCharsets;

@Configuration
public class BloomFilterConfig {

    // 布隆过滤器，用于防止缓存穿透
    @Bean
    public BloomFilter<String> couponBloomFilter() {
        // 预估100万优惠券，误判率0.01
        return BloomFilter.create(Funnels.stringFunnel(StandardCharsets.UTF_8), 1000000, 0.01);
    }
}