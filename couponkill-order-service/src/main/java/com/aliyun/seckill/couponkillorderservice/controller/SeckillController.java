package com.aliyun.seckill.couponkillorderservice.controller;

import com.alibaba.csp.sentinel.annotation.SentinelResource;
import com.alibaba.csp.sentinel.slots.block.BlockException;
import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.couponkillorderservice.feign.CouponServiceFeignClient;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import io.swagger.v3.oas.annotations.tags.Tag;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.web.bind.annotation.*;

import java.util.List;

@Tag(name = "秒杀管理", description = "秒杀相关接口")
@RestController
@RequestMapping("/seckill")
public class SeckillController {

    @Autowired
    private OrderService orderService;

    @Autowired
    private CouponServiceFeignClient couponServiceFeignClient;

    @Value("${couponkill.seckill.cooldown-seconds:2}")
    int cooldownSeconds;

    @Value("${couponkill.seckill.deduct-ttl-seconds:300}")
    int deductTtlSeconds;

    @Operation(summary = "创建秒杀订单", description = "为用户创建秒杀订单")
    @PostMapping("/create")
    @SentinelResource(value = "createSeckillOrder", blockHandler = "createSeckillOrderBlockHandler")
    public Result<Order> createSeckillOrder(
            @Parameter(description = "用户ID") @RequestParam Long userId,
            @Parameter(description = "优惠券ID") @RequestParam Long couponId) {
        Order order = orderService.createOrder(userId, couponId);
        return Result.success(order);
    }

    // 降级处理方法
    public Result<Order> createSeckillOrderBlockHandler(Long userId, Long couponId, BlockException e) {
        return Result.fail(ResultCode.SYSTEM_BUSY);
    }

    @Operation(summary = "取消秒杀订单", description = "取消指定的秒杀订单")
    @PostMapping("/cancel")
    public Result<Boolean> cancelSeckillOrder(
            @Parameter(description = "订单ID") @RequestParam String orderId,
            @Parameter(description = "用户ID") @RequestParam Long userId) {
        return Result.success(orderService.cancelOrder(orderId, userId));
    }

    @Operation(summary = "检查用户是否已领取优惠券", description = "检查指定用户是否已领取指定优惠券")
    @GetMapping("/check/{userId}/{couponId}")
    public Result<Boolean> checkUserReceived(
            @Parameter(description = "用户ID") @PathVariable Long userId,
            @Parameter(description = "优惠券ID") @PathVariable Long couponId) {
        return Result.success(orderService.hasUserReceivedCoupon(userId, couponId));
    }

    @Operation(summary = "查询用户订单", description = "分页查询指定用户的订单列表")
    @GetMapping("/user/{userId}")
    public Result<List<Order>> getUserOrders(
            @Parameter(description = "用户ID") @PathVariable Long userId,
            @Parameter(description = "页码") @RequestParam(defaultValue = "1") Integer pageNum,
            @Parameter(description = "每页数量") @RequestParam(defaultValue = "10") Integer pageSize) {
        List<Order> orders = orderService.getOrderByUserId(userId, pageNum, pageSize);
        return Result.success(orders);
    }

    @Operation(summary = "管理员查询所有订单", description = "管理员分页查询所有订单")
    @GetMapping("/admin")
    public Result<List<Order>> getAllOrders(
            @Parameter(description = "页码") @RequestParam(defaultValue = "1") Integer pageNum,
            @Parameter(description = "每页数量") @RequestParam(defaultValue = "10") Integer pageSize,
            @Parameter(description = "开始时间") @RequestParam(required = false) String startTime,
            @Parameter(description = "结束时间") @RequestParam(required = false) String endTime) {
        List<Order> orders = orderService.getAllOrderByCondition(pageNum, pageSize, startTime, endTime);
        return Result.success(orders);
    }

    /**
     * 新增秒杀接口
     */
    @Operation(summary = "执行优惠券秒杀", description = "执行指定优惠券的秒杀操作")
    @PostMapping("/seckill/doSeckill")
    @SentinelResource(
            value = "couponSeckill",
            blockHandler = "seckillBlockHandler",
            fallback = "seckillFallback"
    )
    public Result<Order> doSeckill(
            @Parameter(description = "用户ID") @RequestParam Long userId,
            @Parameter(description = "优惠券ID") @RequestParam Long couponId) {
        Order order = orderService.createOrder(userId, couponId);
        return Result.success(order);
    }

    /**
     * 限流/熔断处理方法
     */
    public Result<Order> seckillBlockHandler(Long userId, Long couponId, BlockException e) {
        return Result.fail(ResultCode.SYSTEM_BUSY);
    }

    /**
     * 业务异常处理方法
     */
    public Result<Order> seckillFallback(Long userId, Long couponId, Throwable e) {
        return Result.fail(500, "秒杀失败：" + e.getMessage());
    }
}
