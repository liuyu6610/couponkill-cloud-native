package com.aliyun.seckill.couponkillorderservice;

import org.mybatis.spring.annotation.MapperScan;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.cloud.client.discovery.EnableDiscoveryClient;

@EnableDiscoveryClient
@SpringBootApplication
@MapperScan("com.aliyun.seckill.common.mapper")
public class CouponkillOrderServiceApplication {

    public static void main (String[] args) {
        SpringApplication.run( CouponkillOrderServiceApplication.class, args );
    }

}
