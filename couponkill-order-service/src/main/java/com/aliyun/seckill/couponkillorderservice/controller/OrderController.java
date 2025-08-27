package com.aliyun.seckill.couponkillorderservice.controller;

import com.alibaba.csp.sentinel.annotation.SentinelResource;
import com.alibaba.csp.sentinel.slots.block.BlockException;
import com.aliyun.seckill.common.config.ServiceGoConfig;
import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.common.result.Result;
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
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.atomic.AtomicInteger;

@Slf4j
@Tag(name = "订单管理", description = "订单及秒杀相关接口")
@RestController
@RequestMapping("/order")
public class OrderController {

    @Autowired
    private OrderService orderService;

    @Autowired
    private GoSeckillFeignClient goSeckillFeignClient;

    @Autowired
    private ServiceGoConfig servicegoConfig;

    @Value("${couponkill.seckill.cooldown-seconds:2}")
    int cooldownSeconds;

    @Value("${couponkill.seckill.deduct-ttl-seconds:300}")
    int deductTtlSeconds;

    @Value("${couponkill.seckill.java.threshold:500}")
    int javaServiceThreshold;

    // 自定义线程池用于异步处理
    private final ExecutorService executorService = Executors.newFixedThreadPool(50);

    // 用于统计当前处理请求数量
    private final AtomicInteger currentRequestCount = new AtomicInteger(0);


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
     * 在秒杀过程中会从优惠券的所有分片中选择一个有库存的分片，以提高系统QPS
     */
    @Operation(summary = "执行优惠券秒杀", description = "执行指定优惠券的秒杀操作，系统会自动选择一个有库存的分片")
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

        // 增加当前请求数量计数
        int currentCount = currentRequestCount.incrementAndGet();

        try {
            // 检查用户是否在冷却期
            boolean inCooldown = orderService.checkUserInCooldown(userId, couponId);
            if (inCooldown) {
                log.info("用户 {} 在冷却期，无法参与秒杀 couponId={}", userId, couponId);
                return Result.fail(ResultCode.COOLING_DOWN, "请在" + cooldownSeconds + "秒后再试");
            }

            // 使用RateLimiter进行更精确的流量控制
            if (servicegoConfig.shouldRouteToGo()) {
                // 超过阈值或需要分流，由Go服务处理
                log.info("路由到Go服务处理秒杀请求: userId={}, couponId={}, currentCount={}", userId, couponId, currentCount);

                // 路由到Go服务处理完整逻辑
                GoSeckillFeignClient.SeckillRequest request = new GoSeckillFeignClient.SeckillRequest(userId, couponId);
                Result<?> result = goSeckillFeignClient.seckill(request);
                // 无论Go服务处理成功与否，都设置冷却时间
                orderService.setUserCooldown(userId, couponId, cooldownSeconds);
                return result;
            } else {
                // Java服务处理核心逻辑
                log.info("Java服务处理秒杀请求: userId={}, couponId={}, currentCount={}", userId, couponId, currentCount);
                boolean result = orderService.processSeckill(userId, couponId);
                if (result) {
                    // 设置冷却时间
                    orderService.setUserCooldown(userId, couponId, cooldownSeconds);
                    return Result.success("秒杀成功");
                } else {
                    return Result.fail(500, "秒杀失败");
                }
            }
        } finally {
            // 减少当前请求数量计数
            currentRequestCount.decrementAndGet();
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
                // 检查用户是否在冷却期
                boolean inCooldown = orderService.checkUserInCooldown(userId, couponId);
                if (inCooldown) {
                    log.info("用户 {} 在冷却期，无法参与秒杀 couponId={}", userId, couponId);
                    return Result.fail(ResultCode.COOLING_DOWN, "请在" + cooldownSeconds + "秒后再试");
                }

                // 当限流时，完全由Go服务处理
                GoSeckillFeignClient.SeckillRequest request = new GoSeckillFeignClient.SeckillRequest(userId, couponId);
                Result<?> result = goSeckillFeignClient.seckill(request);
                // 无论Go服务处理成功与否，都设置冷却时间
                orderService.setUserCooldown(userId, couponId, cooldownSeconds);
                return result;
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