// couponkill-order-service/src/main/java/com/aliyun/seckill/order/feign/UserServiceFeignClient.java
package com.aliyun.seckill.couponkillorderservice.feign;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.config.FeignConfig;
import org.springframework.cloud.openfeign.FeignClient;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestParam;

@FeignClient(
        name = "couponkill-user-service",
        configuration = FeignConfig.class
)
public interface UserServiceFeignClient {
    
    /**
     * 更新用户秒杀优惠券数量
     * @param userId 用户ID
     * @param count 变化数量（正数表示增加，负数表示减少）
     * @return 是否更新成功
     */
    @PostMapping("/api/v1/user/coupon/seckill/update")
    ApiResponse<Void> updateSeckillCouponCount(@RequestParam("userId") Long userId, 
                                               @RequestParam("count") int count);
    
    /**
     * 更新用户普通优惠券数量
     * @param userId 用户ID
     * @param count 变化数量（正数表示增加，负数表示减少）
     * @return 是否更新成功
     */
    @PostMapping("/api/v1/user/coupon/normal/update")
    ApiResponse<Void> updateNormalCouponCount(@RequestParam("userId") Long userId, 
                                              @RequestParam("count") int count);
}