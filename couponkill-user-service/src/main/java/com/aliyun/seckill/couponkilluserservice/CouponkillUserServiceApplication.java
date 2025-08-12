package com.aliyun.seckill.couponkilluserservice;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.cloud.client.discovery.EnableDiscoveryClient;

@EnableDiscoveryClient
@SpringBootApplication
public class CouponkillUserServiceApplication {

    public static void main (String[] args) {
        SpringApplication.run( CouponkillUserServiceApplication.class, args );
    }

}
