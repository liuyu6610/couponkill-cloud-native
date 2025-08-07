// com.aliyun.seckill.order.controller.SeckillController.java
package com.aliyun.seckill.order.controller;

import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.order.service.OrderService;
import com.aliyun.seckill.pojo.Order;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/seckill")
@Tag(name = "秒杀管理", description = "秒杀相关接口")
public class SeckillController {

    @Autowired
    private OrderService orderService;

    @PostMapping("/create")
    @Operation(summary = "创建秒杀订单")
    public Result<Order> createSeckillOrder(
            @RequestParam Long userId,
            @RequestParam Long couponId) {
        Order order = orderService.createOrder(userId, couponId);
        return Result.success(order);
    }

    @PostMapping("/cancel")
    @Operation(summary = "取消秒杀订单")
    public Result<Boolean> cancelSeckillOrder(
            @RequestParam String orderId,
            @RequestParam Long userId) {
        return Result.success(orderService.cancelOrder(orderId, userId));
    }

    @GetMapping("/check/{userId}/{couponId}")
    @Operation(summary = "检查用户是否已领取优惠券")
    public Result<Boolean> checkUserReceived(
            @PathVariable Long userId,
            @PathVariable Long couponId) {
        return Result.success(orderService.hasUserReceivedCoupon(userId, couponId));
    }
}