package com.aliyun.seckill.couponkilluserservice;

import org.mybatis.spring.annotation.MapperScan;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.cloud.client.discovery.EnableDiscoveryClient;
import org.springframework.cloud.openfeign.EnableFeignClients;
import org.springframework.context.annotation.ComponentScan;

@EnableDiscoveryClient
@SpringBootApplication
@EnableFeignClients
@MapperScan("com.aliyun.seckill.couponkilluserservice.mapper")
@ComponentScan(basePackages = {"com.aliyun.seckill.couponkilluserservice", "com.aliyun.seckill.common"})
public class CouponkillUserServiceApplication {

    public static void main (String[] args) {
        SpringApplication.run( CouponkillUserServiceApplication.class, args );
    }

}
