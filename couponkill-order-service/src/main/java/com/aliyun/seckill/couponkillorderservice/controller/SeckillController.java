// com.aliyun.seckill.order.controller.SeckillController.java
package com.aliyun.seckill.couponkillorderservice.controller;

import com.alibaba.csp.sentinel.annotation.SentinelResource;
import com.alibaba.csp.sentinel.slots.block.BlockException;
import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.couponkillorderservice.feign.CouponServiceFeignClient;
import com.aliyun.seckill.couponkillorderservice.service.CreateOrderService;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.web.bind.annotation.*;

import java.util.List;

@RestController
@RequestMapping("/seckill")
@Tag(name = "秒杀管理", description = "秒杀相关接口")
public class SeckillController {

    @Autowired
    private OrderService orderService;
    @Autowired
    private CreateOrderService createOrderService;
    @Autowired
    private CouponServiceFeignClient couponServiceFeignClient; // 使用 CouponServiceFeignClient 替代 SeckillClient

    @Value("${couponkill.seckill.cooldown-seconds:2}") int cooldownSeconds;
    @Value("${couponkill.seckill.deduct-ttl-seconds:300}") int deductTtlSeconds;

    // 注意：以下与秒杀活动相关的方法需要在 CouponServiceFeignClient 中添加对应接口
    // 或者创建一个新的 Feign Client 专门用于秒杀相关接口

    @PostMapping("/create")
    @Operation(summary = "创建秒杀订单")
    @SentinelResource(value = "createSeckillOrder", blockHandler = "createSeckillOrderBlockHandler")
    public Result<Order> createSeckillOrder(
            @RequestParam Long userId,
            @RequestParam Long couponId) {
        Order order = orderService.createOrder(userId, couponId);
        return Result.success(order);
    }

    // 降级处理方法
    public Result<Order> createSeckillOrderBlockHandler(Long userId, Long couponId, BlockException e) {
        Result<Order> fail = Result.fail(ResultCode.SYSTEM_BUSY);
        return fail;
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

    @GetMapping("/user/{userId}")
    @Operation(summary = "查询用户订单")
    public Result<List<Order>> getUserOrders(
            @PathVariable Long userId,
            @RequestParam(defaultValue = "1") Integer pageNum,
            @RequestParam(defaultValue = "10") Integer pageSize) {
        List<Order> orders = orderService.getOrderByUserId(userId, pageNum, pageSize);
        return Result.success(orders);
    }

    @GetMapping("/admin")
    @Operation(summary = "管理员查询所有订单")
    public Result<List<Order>> getAllOrders(
            @RequestParam(defaultValue = "1") Integer pageNum,
            @RequestParam(defaultValue = "10") Integer pageSize,
            @RequestParam(required = false) String startTime,
            @RequestParam(required = false) String endTime) {
        List<Order> orders = orderService.getAllOrderByCondition(pageNum, pageSize, startTime, endTime);
        return Result.success(orders);
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
        Order order = createOrderService.createOrder(userId, couponId);
        return Result.success(order);
    }

    /**
     * 限流/熔断处理方法
     */
    public Result<Order> seckillBlockHandler(Long userId, Long couponId, BlockException e) {
        e.printStackTrace();
        return Result.fail(ResultCode.SYSTEM_BUSY);
    }

    /**
     * 业务异常处理方法
     */
    public Result<Order> seckillFallback(Long userId, Long couponId, Throwable e) {
        e.printStackTrace();
        return Result.fail(500, "秒杀失败：" + e.getMessage());
    }
}
