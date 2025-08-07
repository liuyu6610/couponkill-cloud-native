// couponkill-common/src/main/java/com/aliyun/seckill/common/config/SentinelConfig.java
package com.aliyun.seckill.common.config;

import com.alibaba.csp.sentinel.adapter.spring.webmvc.callback.RequestOriginParser;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class SentinelConfig {

    @Bean
    public RequestOriginParser requestOriginParser() {
        return request -> {
            // 从请求头获取来源标识
            String origin = request.getHeader("X-Origin");
            return origin == null ? "default" : origin;
        };
    }
}