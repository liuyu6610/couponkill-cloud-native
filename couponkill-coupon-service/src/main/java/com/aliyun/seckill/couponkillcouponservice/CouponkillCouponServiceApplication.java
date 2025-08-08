package com.aliyun.seckill.couponkillcouponservice;

import io.seata.spring.boot.autoconfigure.SeataAutoConfiguration;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.autoconfigure.jdbc.DataSourceAutoConfiguration;

@SpringBootApplication(exclude = {SeataAutoConfiguration.class,
        DataSourceAutoConfiguration.class})
public class CouponkillCouponServiceApplication {

    public static void main (String[] args) {
        SpringApplication.run( CouponkillCouponServiceApplication.class, args );
    }

}
