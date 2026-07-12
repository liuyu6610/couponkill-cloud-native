package com.aliyun.seckill.couponkillconnectorservice;

import org.mybatis.spring.annotation.MapperScan;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.cloud.openfeign.EnableFeignClients;
import org.springframework.scheduling.annotation.EnableScheduling;

@EnableScheduling
@EnableFeignClients(basePackages = "com.aliyun.seckill.couponkillconnectorservice.feign")
@MapperScan("com.aliyun.seckill.couponkillconnectorservice.mapper")
@SpringBootApplication
public class CouponkillConnectorServiceApplication {

    public static void main(String[] args) {
        SpringApplication.run(CouponkillConnectorServiceApplication.class, args);
    }
}
