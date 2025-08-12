// com/aliyun/seckill/couponkilladminservice/feign/CouponServiceFeignClient.java
package com.aliyun.seckill.couponkilladminservice.feign;

import com.aliyun.seckill.common.pojo.Coupon;
import org.springframework.cloud.openfeign.FeignClient;
import org.springframework.web.bind.annotation.*;

import java.util.List;

@FeignClient(name = "coupon-service")
public interface CouponServiceFeignClient {

    @PostMapping("/coupon/{id}/stock")
    void updateStock(@PathVariable("id") Long id, @RequestParam("newStock") int newStock);

    @GetMapping("/coupon")
    List<Coupon> list();

    @GetMapping("/coupon/{id}")
    Coupon getCouponById(@PathVariable("id") Long id);
}
