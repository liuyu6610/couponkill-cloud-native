package com.aliyun.seckill.couponkilluserservice;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.cloud.client.discovery.EnableDiscoveryClient;
import org.springframework.cloud.openfeign.EnableFeignClients;
import org.springframework.context.annotation.ComponentScan;

@EnableDiscoveryClient
@SpringBootApplication // 如果有feign客户端需要启用
@EnableFeignClients
@ComponentScan(basePackages = {"com.aliyun.seckill.couponkilluserservice", "com.aliyun.seckill.common"})
public class CouponkillUserServiceApplication {

    public static void main (String[] args) {
        SpringApplication.run( CouponkillUserServiceApplication.class, args );
    }

}
