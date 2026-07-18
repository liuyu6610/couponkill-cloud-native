package com.aliyun.seckill.couponkillorderservice.feign;

import com.aliyun.seckill.common.result.Result;
import org.springframework.cloud.openfeign.FeignClient;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;

/**
 * Go 秒杀 Feign（沙箱专用）。
 * 热路径已冻结：仅当 couponkill.seckill.go.enabled 与 fallback-to-go 同时为 true 才会被调用。
 * url 与 ServiceGoConfig / nacos couponkill.seckill.go.url 对齐。
 */
@FeignClient(
        name = "seckill-go-svc",
        url = "${couponkill.seckill.go.url:http://seckill-go-svc:8083}"
)
public interface GoSeckillFeignClient {

    @PostMapping(value = "/seckill", consumes = MediaType.APPLICATION_JSON_VALUE)
    Result<?> seckill(@RequestBody SeckillRequest request);

    class SeckillRequest {
        private Long user_id;
        private Long coupon_id;

        public SeckillRequest() {}

        public SeckillRequest(Long userId, Long couponId) {
            this.user_id = userId;
            this.coupon_id = couponId;
        }

        public Long getUser_id() { return user_id; }
        public void setUser_id(Long user_id) { this.user_id = user_id; }
        public Long getCoupon_id() { return coupon_id; }
        public void setCoupon_id(Long coupon_id) { this.coupon_id = coupon_id; }
    }
}
