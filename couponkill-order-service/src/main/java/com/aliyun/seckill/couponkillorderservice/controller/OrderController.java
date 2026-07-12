package com.aliyun.seckill.couponkillorderservice.controller;

import com.alibaba.csp.sentinel.annotation.SentinelResource;
import com.alibaba.csp.sentinel.slots.block.BlockException;
import com.aliyun.seckill.common.config.ServiceGoConfig;
import com.aliyun.seckill.common.context.UserContext;
import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.pojo.EnterSeckillResp;
import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.couponkillorderservice.feign.GoSeckillFeignClient;
import com.aliyun.seckill.couponkillorderservice.service.AsyncSeckillEnterService;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import io.swagger.v3.oas.annotations.tags.Tag;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.web.bind.annotation.*;

import java.util.List;
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

    @Autowired
    private AsyncSeckillEnterService asyncSeckillEnterService;

    @Value("${couponkill.seckill.cooldown-seconds:2}")
    int cooldownSeconds;

    @Value("${couponkill.seckill.deduct-ttl-seconds:300}")
    int deductTtlSeconds;

    /** 仅用于可观测：当前进程内在途秒杀数 */
    private final AtomicInteger currentRequestCount = new AtomicInteger(0);

    @Operation(summary = "创建普通订单", description = "身份取自网关 X-User-Id，禁止客户端伪造 userId")
    @PostMapping("/create")
    public Result<Order> createOrder(
            @Parameter(description = "优惠券ID") @RequestParam("couponId") Long couponId) {
        Long userId = UserContext.requireCurrentUserId();
        Order order = orderService.createOrder(userId, couponId);
        return Result.success(order);
    }

    @Operation(summary = "取消订单")
    @PostMapping("/cancel")
    public Result<Boolean> cancelOrder(
            @Parameter(description = "订单ID") @RequestParam("orderId") String orderId) {
        Long userId = UserContext.requireCurrentUserId();
        return Result.success(orderService.cancelOrder(orderId, userId));
    }

    @Operation(summary = "检查当前用户是否已领取优惠券")
    @GetMapping("/check/{couponId}")
    public Result<Boolean> checkUserReceived(
            @Parameter(description = "优惠券ID") @PathVariable("couponId") Long couponId) {
        Long userId = UserContext.requireCurrentUserId();
        return Result.success(orderService.hasUserReceivedCoupon(userId, couponId));
    }

    @Operation(summary = "查询当前用户订单")
    @GetMapping("/user/me")
    public Result<List<Order>> getMyOrders(
            @Parameter(description = "页码") @RequestParam(value = "pageNum", defaultValue = "1") Integer pageNum,
            @Parameter(description = "每页数量") @RequestParam(value = "pageSize", defaultValue = "10") Integer pageSize) {
        Long userId = UserContext.requireCurrentUserId();
        List<Order> orders = orderService.getOrderByUserId(userId, pageNum, pageSize);
        return Result.success(orders);
    }

    /** @deprecated 保留兼容；路径中的 userId 必须与登录身份一致，否则拒绝 */
    @Deprecated
    @Operation(summary = "查询用户订单（兼容旧路径，须与登录身份一致）")
    @GetMapping("/user/{userId}")
    public Result<List<Order>> getUserOrders(
            @Parameter(description = "用户ID") @PathVariable("userId") Long userId,
            @Parameter(description = "页码") @RequestParam(value = "pageNum", defaultValue = "1") Integer pageNum,
            @Parameter(description = "每页数量") @RequestParam(value = "pageSize", defaultValue = "10") Integer pageSize) {
        Long currentUserId = UserContext.requireCurrentUserId();
        if (!currentUserId.equals(userId)) {
            return Result.fail(ResultCode.AUTH_FAIL.getCode(), "禁止查询他人订单");
        }
        List<Order> orders = orderService.getOrderByUserId(userId, pageNum, pageSize);
        return Result.success(orders);
    }

    @Operation(summary = "管理员查询所有订单")
    @GetMapping("/admin")
    public Result<List<Order>> getAllOrders(
            @Parameter(description = "页码") @RequestParam(value = "pageNum", defaultValue = "1") Integer pageNum,
            @Parameter(description = "每页数量") @RequestParam(value = "pageSize", defaultValue = "10") Integer pageSize,
            @Parameter(description = "开始时间") @RequestParam(value = "startTime", required = false) String startTime,
            @Parameter(description = "结束时间") @RequestParam(value = "endTime", required = false) String endTime) {
        List<Order> orders = orderService.getAllOrderByCondition(pageNum, pageSize, startTime, endTime);
        return Result.success(orders);
    }

    @Operation(summary = "执行优惠券秒杀", description = "热路径：Lua 预扣 + Kafka 异步落单；前端可轮询 /order/check/{couponId}")
    @PostMapping("/seckill")
    @SentinelResource(
            value = "couponSeckill",
            blockHandler = "seckillBlockHandler",
            fallback = "seckillFallback"
    )
    public Result<EnterSeckillResp> doSeckill(
            @Parameter(description = "优惠券ID") @RequestParam("couponId") Long couponId) {
        Long userId = UserContext.requireCurrentUserId();
        int currentCount = currentRequestCount.incrementAndGet();
        try {
            log.info("收到秒杀请求: userId={}, couponId={}, inFlight={}", userId, couponId, currentCount);

            // Go 旁路仍走同步引擎（默认关闭）
            if (servicegoConfig.shouldRouteToGo()) {
                GoSeckillFeignClient.SeckillRequest request = new GoSeckillFeignClient.SeckillRequest(userId, couponId);
                Result<?> goResult = goSeckillFeignClient.seckill(request);
                if (goResult != null && goResult.isSuccess()) {
                    orderService.setUserCooldown(userId, couponId, cooldownSeconds);
                    return Result.success(EnterSeckillResp.builder()
                            .status("QUEUED")
                            .err(0)
                            .message("Go秒杀已受理")
                            .build());
                }
                return Result.fail(
                        goResult != null ? goResult.getCode() : ResultCode.FAIL.getCode(),
                        goResult != null ? goResult.getMessage() : "Go秒杀失败");
            }

            EnterSeckillResp resp = orderService.enterSeckillAsync(userId, couponId);
            if ("QUEUED".equals(resp.getStatus())) {
                return Result.success(resp);
            }
            int err = resp.getErr() != null ? resp.getErr() : ResultCode.FAIL.getCode();
            if (err == com.aliyun.seckill.common.api.ErrorCodes.COOLING_DOWN) {
                return Result.fail(ResultCode.COOLING_DOWN, resp.getMessage());
            }
            if (err == com.aliyun.seckill.common.api.ErrorCodes.OUT_OF_STOCK) {
                return Result.fail(ResultCode.COUPON_OUT_OF_STOCK, resp.getMessage());
            }
            if (err == com.aliyun.seckill.common.api.ErrorCodes.NOT_PREHEATED) {
                return Result.fail(ResultCode.STOCK_NOT_PREHEATED, resp.getMessage());
            }
            return Result.fail(err, resp.getMessage() != null ? resp.getMessage() : "秒杀失败");
        } finally {
            currentRequestCount.decrementAndGet();
        }
    }

    @Operation(summary = "查询秒杀异步结果", description = "按 requestId 查询 PENDING/SUCCESS/FAIL")
    @GetMapping("/seckill/result")
    public Result<String> seckillResult(
            @Parameter(description = "入队时返回的 requestId") @RequestParam("requestId") String requestId) {
        UserContext.requireCurrentUserId();
        String status = asyncSeckillEnterService.getResult(requestId);
        return Result.success(status != null ? status : "UNKNOWN");
    }

    @SuppressWarnings("unused")
    public Result<EnterSeckillResp> seckillBlockHandler(Long couponId, BlockException e) {
        return Result.fail(ResultCode.SYSTEM_BUSY, "系统繁忙，请稍后再试");
    }

    @SuppressWarnings("unused")
    public Result<EnterSeckillResp> seckillFallback(Long couponId, Throwable e) {
        return Result.fail(500, "秒杀失败：" + e.getMessage());
    }
}
