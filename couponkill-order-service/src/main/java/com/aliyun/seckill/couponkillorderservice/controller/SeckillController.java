// 文件路径: com/aliyun/seckill/couponkillorderservice/controller/SeckillController.java
package com.aliyun.seckill.couponkillorderservice.controller;

import com.alibaba.csp.sentinel.annotation.SentinelResource;
import com.alibaba.csp.sentinel.slots.block.BlockException;
import com.aliyun.seckill.common.context.UserContext;
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
    private CouponServiceFeignClient couponServiceFeignClient;

    @Value("${couponkill.seckill.cooldown-seconds:2}")
    int cooldownSeconds;

    @Value("${couponkill.seckill.deduct-ttl-seconds:300}")
    int deductTtlSeconds;

    @PostMapping("/create")
    @Operation(summary = "创建秒杀订单")
    @SentinelResource(value = "createSeckillOrder", blockHandler = "createSeckillOrderBlockHandler")
    public Result<Order> createSeckillOrder(@RequestParam Long couponId) {
        // 从请求头中获取用户ID
        String userIdStr = UserContext.getCurrentUserId();
        if (userIdStr == null) {
            return Result.fail(ResultCode.TOKEN_INVALID);
        }

        Long userId = Long.valueOf(userIdStr);
        Order order = orderService.createOrder(userId, couponId);
        return Result.success(order);
    }

    // 降级处理方法
    public Result<Order> createSeckillOrderBlockHandler(Long couponId, BlockException e) {
        return Result.fail(ResultCode.SYSTEM_BUSY);
    }

    @PostMapping("/cancel")
    @Operation(summary = "取消秒杀订单")
    public Result<Boolean> cancelSeckillOrder(@RequestParam String orderId) {
        String userIdStr = UserContext.getCurrentUserId();
        if (userIdStr == null) {
            return Result.fail(ResultCode.TOKEN_INVALID);
        }

        Long userId = Long.valueOf(userIdStr);
        return Result.success(orderService.cancelOrder(orderId, userId));
    }

    @GetMapping("/check/{couponId}")
    @Operation(summary = "检查用户是否已领取优惠券")
    @SentinelResource(value = "checkUserReceived", blockHandler = "checkUserReceivedBlockHandler")
    public Result<Boolean> checkUserReceived(@PathVariable Long couponId) {
        String userIdStr = UserContext.getCurrentUserId();
        if (userIdStr == null) {
            return Result.fail(ResultCode.TOKEN_INVALID);
        }

        Long userId = Long.valueOf(userIdStr);
        return Result.success(orderService.hasUserReceivedCoupon(userId, couponId));
    }

    @GetMapping("/user")
    @Operation(summary = "查询当前用户订单")
    @SentinelResource(value = "getUserOrders", blockHandler = "getUserOrdersBlockHandler")
    public Result<List<Order>> getUserOrders(
            @RequestParam(defaultValue = "1") Integer pageNum,
            @RequestParam(defaultValue = "10") Integer pageSize) {
        String userIdStr = UserContext.getCurrentUserId();
        if (userIdStr == null) {
            return Result.fail(ResultCode.TOKEN_INVALID);
        }

        Long userId = Long.valueOf(userIdStr);
        List<Order> orders = orderService.getOrderByUserId(userId, pageNum, pageSize);
        return Result.success(orders);
    }

    // getUserOrders接口的降级处理方法
    public Result<List<Order>> getUserOrdersBlockHandler(Integer pageNum, Integer pageSize, BlockException e) {
        return Result.fail(ResultCode.SYSTEM_BUSY);
    }

    // getAllOrders接口的降级处理方法
    public Result<List<Order>> getAllOrdersBlockHandler(Integer pageNum, Integer pageSize,
            String startTime, String endTime, BlockException e) {
        return Result.fail(ResultCode.SYSTEM_BUSY);
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
    public Result<Order> doSeckill(@RequestParam Long couponId) {
        String userIdStr = UserContext.getCurrentUserId();
        if (userIdStr == null) {
            return Result.fail(ResultCode.TOKEN_INVALID);
        }

        Long userId = Long.valueOf(userIdStr);
        Order order = createOrderService.createOrder(userId, couponId);
        return Result.success(order);
    }

    /**
     * 限流/熔断处理方法
     */
    public Result<Order> seckillBlockHandler(Long couponId, BlockException e) {
        e.printStackTrace();
        return Result.fail(ResultCode.SYSTEM_BUSY);
    }

    /**
     * 业务异常处理方法
     */
    public Result<Order> seckillFallback(Long couponId, Throwable e) {
        e.printStackTrace();
        return Result.fail(500, "秒杀失败：" + e.getMessage());
    }
}
