// com.aliyun.seckill.order.controller.SeckillController.java
package com.aliyun.seckill.order.controller;

import com.alibaba.csp.sentinel.annotation.SentinelResource;
import com.alibaba.csp.sentinel.slots.block.BlockException;
import com.aliyun.seckill.common.api.ApiResp;
import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.pojo.EnterSeckillResp;
import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.common.pojo.SeckillResultResp;
import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.common.service.order.OrderService;
import com.aliyun.seckill.order.service.CreateOrderService;
import com.aliyun.seckill.order.feign.SeckillClient;
import com.baomidou.mybatisplus.extension.plugins.pagination.Page;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/seckill")
@Tag(name = "秒杀管理", description = "秒杀相关接口")
public class SeckillController {

    @Autowired
    private OrderService orderService;
    @Autowired
    private CreateOrderService createOrderService;
    @Autowired
    private  SeckillClient seckillClient;

    @Value("${couponkill.seckill.cooldown-seconds:2}") int cooldownSeconds;
    @Value("${couponkill.seckill.deduct-ttl-seconds:300}") int deductTtlSeconds;
    @PostMapping("/seckill/{couponId}/enter")
    public ApiResp<EnterSeckillResp> enter(@PathVariable String couponId,
                                           @RequestHeader("X-User-Id") String userId){
        // 调用coupon-service的接口
        return seckillClient.enter(couponId, userId);
    }

    @GetMapping("/seckill/results/{requestId}")
    public ApiResp<SeckillResultResp> result(@PathVariable String requestId){
        // 调用coupon-service的接口
        return seckillClient.result(requestId);
    }

    @PostMapping("/seckill/internal/success")
    public ApiResp<Void> markSuccess(@RequestParam String requestId, @RequestParam String orderId){
        // 调用coupon-service的接口
        return seckillClient.markSuccess(requestId, orderId);
    }

    @PostMapping("/seckill/internal/compensate")
    public ApiResp<Void> compensate(@RequestParam String requestId, @RequestParam String couponId, @RequestParam String userId){
        // 调用coupon-service的接口
        return seckillClient.compensate(requestId, couponId, userId);
    }

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
        Result<Order> fail = Result.fail( ResultCode.SYSTEM_BUSY );
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
    public Result<Page<Order>> getUserOrders(
            @PathVariable Long userId,
            @RequestParam(defaultValue = "1") Integer pageNum,
            @RequestParam(defaultValue = "10") Integer pageSize) {
        Page<Order> orders = orderService.getOrderByUserId(userId, pageNum, pageSize);
        return Result.success(orders);
    }
    @GetMapping("/admin")
    @Operation(summary = "管理员查询所有订单")
    public Result<Page<Order>> getAllOrders(
            @RequestParam(defaultValue = "1") Integer pageNum,
            @RequestParam(defaultValue = "10") Integer pageSize,
            @RequestParam(required = false) String startTime,
            @RequestParam(required = false) String endTime) {
        Page<Order> orders = orderService.getAllOrderByCondition(pageNum, pageSize, startTime, endTime);
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