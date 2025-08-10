// couponkill-coupon-service/src/main/java/com/aliyun/seckill/coupon/task/CouponTask.java
package com.aliyun.seckill.coupon.task;
import com.aliyun.seckill.common.service.coupon.CouponService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

@Component
public class CouponTask {

    @Autowired
    private CouponService couponService;

    // 每天凌晨执行
    @Scheduled(cron = "0 0 0 * * ?")
    public void handleExpiredCoupons() {
        // 处理过期优惠券，自动退还
        couponService.handleExpiredCoupons();
    }
}

