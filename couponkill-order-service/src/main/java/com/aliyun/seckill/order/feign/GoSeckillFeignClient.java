// couponkill-order-service/src/main/java/com/aliyun/seckill/order/feign/GoSeckillFeignClient.java
package com.aliyun.seckill.order.feign;

import com.aliyun.seckill.common.result.Result;
import org.springframework.cloud.openfeign.FeignClient;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestParam;

@FeignClient(name = "go-service", url = "${go.service.url:http://go-service:8090}")
public interface GoSeckillFeignClient {

    @PostMapping("/seckill")
    Result<?> seckill(@RequestParam("user_id") Long userId, @RequestParam("coupon_id") Long couponId);
}