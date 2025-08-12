package com.aliyun.seckill.couponkillorderservice;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.cloud.client.discovery.EnableDiscoveryClient;
import org.springframework.cloud.openfeign.EnableFeignClients;

@EnableDiscoveryClient
@SpringBootApplication
@EnableFeignClients(basePackages = "com.aliyun.seckill.couponkillorderservice.feign")
public class CouponkillOrderServiceApplication {

    public static void main (String[] args) {
        SpringApplication.run( CouponkillOrderServiceApplication.class, args );
    }

}
