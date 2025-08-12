package com.aliyun.seckill.user.config;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.config.annotation.web.configuration.EnableWebSecurity;
import org.springframework.security.web.SecurityFilterChain;

@Configuration
@EnableWebSecurity
public class SecurityConfig {

    @Bean
    public SecurityFilterChain filterChain(HttpSecurity http) throws Exception {
        http.csrf(csrf -> csrf.disable())
                .authorizeHttpRequests(auth -> auth
                        // 1. 放行Knife4j/Swagger核心路径
                        .requestMatchers(
                                "/doc.html",                    // 文档首页
                                "/swagger-ui.html",             // 兼容Swagger UI
                                "/swagger-ui/**",               // Swagger UI静态资源
                                "/v3/api-docs/**",              // OpenAPI文档数据
                                "/webjars/**"                   // 文档依赖的第三方静态资源
                        ).permitAll()
                        // 2. 放行认证相关接口（包括登录、获取token）
                        .requestMatchers("/auth/**", "/api/v1/auth/**").permitAll()
                        // 3. 其他请求需要认证
                        .anyRequest().authenticated()
                );
        return http.build();
    }
}
