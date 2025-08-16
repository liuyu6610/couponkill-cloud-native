package com.aliyun.seckill.couponkillorderservice;

import org.mybatis.spring.annotation.MapperScan;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.cloud.openfeign.EnableFeignClients;
import org.springframework.context.annotation.ComponentScan;

@SpringBootApplication
@EnableFeignClients(basePackages = "com.aliyun.seckill.couponkillorderservice.feign")
@MapperScan("com.aliyun.seckill.couponkillorderservice.mapper")
@ComponentScan(basePackages = {"com.aliyun.seckill.couponkillorderservice", "com.aliyun.seckill.common"})
public class CouponkillOrderServiceApplication {
    public static void main(String[] args) {
        SpringApplication.run(CouponkillOrderServiceApplication.class, args);
    }
}
