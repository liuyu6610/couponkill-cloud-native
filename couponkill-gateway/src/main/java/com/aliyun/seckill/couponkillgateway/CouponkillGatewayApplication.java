package com.aliyun.seckill.couponkillgateway;

import io.seata.spring.boot.autoconfigure.SeataAutoConfiguration;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.autoconfigure.jdbc.DataSourceAutoConfiguration;
import org.springframework.cloud.client.discovery.EnableDiscoveryClient;

@EnableDiscoveryClient
@SpringBootApplication
public class CouponkillGatewayApplication {

    public static void main (String[] args) {
        SpringApplication.run( CouponkillGatewayApplication.class, args );
    }

}
