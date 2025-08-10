// com.aliyun.seckill.coupon.feign.CouponFeignService.java
package com.aliyun.seckill.common.feign;

import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.common.pojo.Coupon;
import org.springframework.cloud.openfeign.FeignClient;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;

@FeignClient(value = "couponkill-coupon-service")
public interface CouponFeignService {

    @GetMapping("/coupon/{id}")
    Result<Coupon> getCouponById(@PathVariable("id") Long couponId);

    @PostMapping("/coupon/deduct/{id}")
    Result<Boolean> deductStock(@PathVariable("id") Long couponId);

    @PostMapping("/coupon/increase/{id}")
    Result<Boolean> increaseStock(@PathVariable("id") Long couponId);
}