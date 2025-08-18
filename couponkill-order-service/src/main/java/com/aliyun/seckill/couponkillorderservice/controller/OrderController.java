package com.aliyun.seckill.couponkillorderservice.controller;

import com.alibaba.csp.sentinel.annotation.SentinelResource;
import com.alibaba.csp.sentinel.slots.block.BlockException;
import com.aliyun.seckill.common.config.ServiceGoConfig;
import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.couponkillorderservice.feign.CouponServiceFeignClient;
import com.aliyun.seckill.couponkillorderservice.feign.GoSeckillFeignClient;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import io.swagger.v3.oas.annotations.tags.Tag;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.web.bind.annotation.*;

import java.util.List;
@Slf4j
@Tag(name = "订单管理", description = "订单及秒杀相关接口")
@RestController
@RequestMapping("api/v1/order")
public class OrderController {

    @Autowired
    private OrderService orderService;

    @Autowired
    private CouponServiceFeignClient couponServiceFeignClient;

    @Autowired
    private GoSeckillFeignClient goSeckillFeignClient;

    @Autowired
    private ServiceGoConfig servicegoConfig;

    @Value("${couponkill.seckill.cooldown-seconds:2}")
    int cooldownSeconds;

    @Value("${couponkill.seckill.deduct-ttl-seconds:300}")
    int deductTtlSeconds;

    @Operation(summary = "创建普通订单")
    @PostMapping("/create")
    public Result<Order> createOrder(
            @Parameter(description = "用户ID") @RequestParam Long userId,
            @Parameter(description = "优惠券ID") @RequestParam Long couponId) {
        Order order = orderService.createOrder(userId, couponId);
        return Result.success(order);
    }

    @Operation(summary = "取消订单")
    @PostMapping("/cancel")
    public Result<Boolean> cancelOrder(
            @Parameter(description = "订单ID") @RequestParam String orderId,
            @Parameter(description = "用户ID") @RequestParam Long userId) {
        return Result.success(orderService.cancelOrder(orderId, userId));
    }

    @Operation(summary = "检查用户是否已领取优惠券")
    @GetMapping("/check/{userId}/{couponId}")
    public Result<Boolean> checkUserReceived(
            @Parameter(description = "用户ID") @PathVariable Long userId,
            @Parameter(description = "优惠券ID") @PathVariable Long couponId) {
        return Result.success(orderService.hasUserReceivedCoupon(userId, couponId));
    }

    @Operation(summary = "查询用户订单")
    @GetMapping("/user/{userId}")
    public Result<List<Order>> getUserOrders(
            @Parameter(description = "用户ID") @PathVariable Long userId,
            @Parameter(description = "页码") @RequestParam(defaultValue = "1") Integer pageNum,
            @Parameter(description = "每页数量") @RequestParam(defaultValue = "10") Integer pageSize) {
        List<Order> orders = orderService.getOrderByUserId(userId, pageNum, pageSize);
        return Result.success(orders);
    }

    @Operation(summary = "管理员查询所有订单")
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
     * 秒杀接口 - 根据负载情况自动路由到Java或Go服务
     */
    @Operation(summary = "执行优惠券秒杀", description = "执行指定优惠券的秒杀操作")
    @PostMapping("/seckill")
    @SentinelResource(
            value = "couponSeckill",
            blockHandler = "seckillBlockHandler",
            fallback = "seckillFallback"
    )
    public Result<?> doSeckill(
            @Parameter(description = "用户ID") @RequestParam Long userId,
            @Parameter(description = "优惠券ID") @RequestParam Long couponId) {
        log.info("收到秒杀请求: userId={}, couponId={}", userId, couponId);
        // 检查用户是否在冷却期
        boolean inCooldown = orderService.checkUserInCooldown(userId, couponId);
        if (inCooldown) {
            log.info("用户 {} 在冷却期，无法参与秒杀 couponId={}", userId, couponId);
            return Result.fail(ResultCode.COOLING_DOWN, "请在" + cooldownSeconds + "秒后再试");
        }

        // 根据负载决定路由到Java还是Go服务
        if (servicegoConfig.shouldRouteToGo()) {
            log.info("路由到Go服务处理秒杀请求: userId={}, couponId={}", userId, couponId);
            // 路由到Go服务
            GoSeckillFeignClient.SeckillRequest request = new GoSeckillFeignClient.SeckillRequest(userId, couponId);
            Result<?> result = goSeckillFeignClient.seckill(request);
            // 设置冷却时间
            if (result.isSuccess()) {
                orderService.setUserCooldown(userId, couponId, cooldownSeconds);
                return Result.success("订单正在处理中，将在2秒内返回结果");
            }
            return result;
        } else {
            log.info("使用Java服务处理秒杀请求: userId={}, couponId={}", userId, couponId);
            // Java服务处理
            Order order = orderService.createOrder(userId, couponId);
            // 设置冷却时间
            orderService.setUserCooldown(userId, couponId, cooldownSeconds);
            return Result.success(order);
        }
    }

    /**
     * 限流/熔断处理方法
     */
    @SuppressWarnings("unused")//sentinel框架原因，需要加上
    public Result<?> seckillBlockHandler(Long userId, Long couponId, BlockException e) {
        // 限流时尝试路由到Go服务
        if (servicegoConfig.isGoServiceEnabled()) {
            try {
                GoSeckillFeignClient.SeckillRequest request = new GoSeckillFeignClient.SeckillRequest(userId, couponId);
                Result<?> result = goSeckillFeignClient.seckill(request);
                if (result.isSuccess()) {
                    orderService.setUserCooldown(userId, couponId, cooldownSeconds);
                    return Result.success("订单正在处理中，将在2秒内返回结果");
                }
            } catch (Exception ex) {
                // Go服务调用失败
                return Result.fail(ResultCode.SYSTEM_BUSY, "系统繁忙，请稍后再试");
            }
        }
        return Result.fail(ResultCode.SYSTEM_BUSY, "系统繁忙，请稍后再试");
    }

    /**
     * 业务异常处理方法
     */
    @SuppressWarnings("unused")
    public Result<?> seckillFallback(Long userId, Long couponId, Throwable e) {
        return Result.fail(500, "秒杀失败：" + e.getMessage());
    }
}