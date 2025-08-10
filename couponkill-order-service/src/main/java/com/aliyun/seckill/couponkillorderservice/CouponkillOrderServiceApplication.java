package com.aliyun.seckill.couponkillorderservice;

import io.seata.spring.boot.autoconfigure.SeataAutoConfiguration;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.autoconfigure.jdbc.DataSourceAutoConfiguration;
import org.springframework.cloud.client.discovery.EnableDiscoveryClient;

@EnableDiscoveryClient
@SpringBootApplication(exclude = {SeataAutoConfiguration.class, DataSourceAutoConfiguration.class})

public class CouponkillOrderServiceApplication {

    public static void main (String[] args) {
        SpringApplication.run( CouponkillOrderServiceApplication.class, args );
    }

}
