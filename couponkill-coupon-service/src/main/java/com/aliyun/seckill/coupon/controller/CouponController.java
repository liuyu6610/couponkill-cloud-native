// com.aliyun.seckill.coupon.controller.CouponController.java
package com.aliyun.seckill.coupon.controller;

import com.alibaba.csp.sentinel.annotation.SentinelResource;
import com.alibaba.csp.sentinel.slots.block.BlockException;
import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.common.result.ResultCode;
import com.aliyun.seckill.coupon.service.CouponService;
import com.aliyun.seckill.order.service.OrderService;
import com.aliyun.seckill.pojo.Coupon;
import com.aliyun.seckill.pojo.Order;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.context.annotation.Lazy;
import org.springframework.web.bind.annotation.*;

import java.util.List;

@RestController
@RequestMapping("/coupon")
@Tag(name = "优惠券管理", description = "优惠券查询、库存操作接口")
public class CouponController {

    @Autowired
    @Lazy
    private CouponService couponService;
    @Autowired
    @Lazy
    private OrderService orderService;

    @GetMapping("/available")
    @Operation(summary = "获取所有可用优惠券")
    public Result<List<Coupon>> getAvailableCoupons() {
        return Result.success(couponService.getAvailableCoupons());
    }

    @GetMapping("/{id}")
    @Operation(summary = "根据ID获取优惠券")
    public Result<Coupon> getCouponById(@PathVariable Long id) {
        return Result.success(couponService.getCouponById(id));
    }

    @PostMapping("/deduct/{id}")
    @Operation(summary = "扣减优惠券库存")
    public Result<Boolean> deductStock(@PathVariable Long id) {
        return Result.success(couponService.deductStock(id));
    }

    @PostMapping("/increase/{id}")
    @Operation(summary = "增加优惠券库存")
    public Result<Boolean> increaseStock(@PathVariable Long id) {
        return Result.success(couponService.increaseStock(id));
    }
    /**
     * 新增秒杀接口
     */
    @PostMapping("/seckill/doSeckill")
    @Operation(summary = "执行优惠券秒杀")
    @SentinelResource(
            value = "couponSeckill",
            blockHandler = "seckillBlockHandler",
            fallback = "seckillFallback"
    )
    public Result<Order> doSeckill(
            @RequestParam Long userId,
            @RequestParam Long couponId) {
        Order order = orderService.createOrder(userId, couponId);
        return Result.success(order);
    }

    /**
     * 限流/熔断处理方法
     */
    public Result<Order> seckillBlockHandler(Long userId, Long couponId, BlockException e) {
        e.printStackTrace();
        return Result.fail( ResultCode.SYSTEM_BUSY);
    }

    /**
     * 业务异常处理方法
     */
    public Result<Order> seckillFallback(Long userId, Long couponId, Throwable e) {
        e.printStackTrace();
        return Result.fail(500, "秒杀失败：" + e.getMessage());
    }
}