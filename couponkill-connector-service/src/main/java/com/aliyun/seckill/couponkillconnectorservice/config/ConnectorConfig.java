package com.aliyun.seckill.couponkillconnectorservice.config;

import lombok.RequiredArgsConstructor;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.servlet.config.annotation.InterceptorRegistry;
import org.springframework.web.servlet.config.annotation.WebMvcConfigurer;

@Configuration
@EnableConfigurationProperties(JdConnectorProperties.class)
@RequiredArgsConstructor
public class ConnectorConfig implements WebMvcConfigurer {

    private final ConnectorAdminInterceptor connectorAdminInterceptor;

    @Override
    public void addInterceptors(InterceptorRegistry registry) {
        registry.addInterceptor(connectorAdminInterceptor)
                .addPathPatterns("/api/v1/connector/**");
    }
}
