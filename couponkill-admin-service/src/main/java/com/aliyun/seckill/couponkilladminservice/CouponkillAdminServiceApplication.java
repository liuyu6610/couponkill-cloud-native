package com.aliyun.seckill.couponkilladminservice;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.cloud.client.discovery.EnableDiscoveryClient;
import org.springframework.cloud.openfeign.EnableFeignClients;

@EnableDiscoveryClient
@SpringBootApplication
@EnableFeignClients(basePackages = "com.aliyun.seckill.couponkilladminservice.feign")
public class CouponkillAdminServiceApplication {

    public static void main (String[] args) {
        SpringApplication.run( CouponkillAdminServiceApplication.class, args );
    }

}
