package com.aliyun.seckill.couponkillorderservice.controller;

import com.alibaba.csp.sentinel.annotation.SentinelResource;
import com.alibaba.csp.sentinel.slots.block.BlockException;
import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.api.ErrorCodes;
import com.aliyun.seckill.common.config.ServiceGoConfig;
import com.aliyun.seckill.common.context.UserContext;
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
@RequestMapping({"/order", "/api/v1/order"})
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

    /** 仅用于可观测：当前进程内在途秒杀数 */
    private final AtomicInteger currentRequestCount = new AtomicInteger(0);

    @Operation(summary = "创建普通订单", description = "身份取自网关 X-User-Id，禁止客户端伪造 userId")
    @PostMapping("/create")
    public ApiResponse<Order> createOrder(
            @Parameter(description = "优惠券ID") @RequestParam("couponId") Long couponId) {
        Long userId = UserContext.requireCurrentUserId();
        Order order = orderService.createOrder(userId, couponId);
        return ApiResponse.success(order);
    }

    @Operation(summary = "取消订单")
    @PostMapping("/cancel")
    public ApiResponse<Boolean> cancelOrder(
            @Parameter(description = "订单ID") @RequestParam("orderId") String orderId) {
        Long userId = UserContext.requireCurrentUserId();
        return ApiResponse.success(orderService.cancelOrder(orderId, userId));
    }

    @Operation(summary = "检查当前用户是否已领取优惠券")
    @GetMapping("/check/{couponId}")
    public ApiResponse<Boolean> checkUserReceived(
            @Parameter(description = "优惠券ID") @PathVariable("couponId") Long couponId) {
        Long userId = UserContext.requireCurrentUserId();
        return ApiResponse.success(orderService.hasUserReceivedCoupon(userId, couponId));
    }

    @Operation(summary = "查询当前用户订单")
    @GetMapping("/user/me")
    public ApiResponse<List<Order>> getMyOrders(
            @Parameter(description = "页码") @RequestParam(value = "pageNum", defaultValue = "1") Integer pageNum,
            @Parameter(description = "每页数量") @RequestParam(value = "pageSize", defaultValue = "10") Integer pageSize) {
        Long userId = UserContext.requireCurrentUserId();
        List<Order> orders = orderService.getOrderByUserId(userId, pageNum, pageSize);
        return ApiResponse.success(orders);
    }

    /** @deprecated 保留兼容；路径中的 userId 必须与登录身份一致，否则拒绝 */
    @Deprecated
    @Operation(summary = "查询用户订单（兼容旧路径，须与登录身份一致）")
    @GetMapping("/user/{userId}")
    public ApiResponse<List<Order>> getUserOrders(
            @Parameter(description = "用户ID") @PathVariable("userId") Long userId,
            @Parameter(description = "页码") @RequestParam(value = "pageNum", defaultValue = "1") Integer pageNum,
            @Parameter(description = "每页数量") @RequestParam(value = "pageSize", defaultValue = "10") Integer pageSize) {
        Long currentUserId = UserContext.requireCurrentUserId();
        if (!currentUserId.equals(userId)) {
            return ApiResponse.fail(ErrorCodes.AUTH_FAIL, "禁止查询他人订单");
        }
        List<Order> orders = orderService.getOrderByUserId(userId, pageNum, pageSize);
        return ApiResponse.success(orders);
    }

    @Operation(summary = "管理员查询所有订单")
    @GetMapping("/admin")
    public ApiResponse<List<Order>> getAllOrders(
            @Parameter(description = "页码") @RequestParam(value = "pageNum", defaultValue = "1") Integer pageNum,
            @Parameter(description = "每页数量") @RequestParam(value = "pageSize", defaultValue = "10") Integer pageSize,
            @Parameter(description = "开始时间") @RequestParam(value = "startTime", required = false) String startTime,
            @Parameter(description = "结束时间") @RequestParam(value = "endTime", required = false) String endTime) {
        List<Order> orders = orderService.getAllOrderByCondition(pageNum, pageSize, startTime, endTime);
        return ApiResponse.success(orders);
    }

    @Operation(summary = "执行优惠券秒杀", description = "热路径：Lua 预扣 + Kafka 异步落单；前端可轮询 /api/v1/order/check/{couponId}（旧 /order/check 仍兼容）")
    @PostMapping("/seckill")
    @SentinelResource(
            value = "couponSeckill",
            blockHandler = "seckillBlockHandler",
            fallback = "seckillFallback"
    )
    public ApiResponse<EnterSeckillResp> doSeckill(
            @Parameter(description = "优惠券ID") @RequestParam("couponId") Long couponId) {
        Long userId = UserContext.requireCurrentUserId();
        int currentCount = currentRequestCount.incrementAndGet();
        try {
            log.debug("收到秒杀请求: userId={}, couponId={}, inFlight={}", userId, couponId, currentCount);

            // Go 热路径已冻结：仅 enabled+fallback-to-go 双开时进沙箱（禁止过载自动切流）
            if (servicegoConfig.shouldRouteToGo()) {
                log.warn("显式沙箱路由到 Go 秒杀（非默认热路径）: userId={}, couponId={}", userId, couponId);
                GoSeckillFeignClient.SeckillRequest request = new GoSeckillFeignClient.SeckillRequest(userId, couponId);
                Result<?> goResult = goSeckillFeignClient.seckill(request);
                if (goResult != null && goResult.isSuccess()) {
                    orderService.setUserCooldown(userId, couponId, cooldownSeconds);
                    return ApiResponse.success(EnterSeckillResp.builder()
                            .status("QUEUED")
                            .err(0)
                            .message("Go秒杀已受理")
                            .build());
                }
                return ApiResponse.fail(
                        goResult != null ? goResult.getCode() : ErrorCodes.SYS_ERROR,
                        goResult != null ? goResult.getMessage() : "Go秒杀失败");
            }

            EnterSeckillResp resp = orderService.enterSeckillAsync(userId, couponId);
            if ("QUEUED".equals(resp.getStatus())) {
                return ApiResponse.success(resp);
            }
            int err = resp.getErr() != null ? resp.getErr() : ErrorCodes.SYS_ERROR;
            // Phase3：对外只透传 ErrorCodes 语义；未知码归并 SYS_ERROR
            if (err == ErrorCodes.COOLING_DOWN
                    || err == ErrorCodes.OUT_OF_STOCK
                    || err == ErrorCodes.NOT_PREHEATED
                    || err == ErrorCodes.NOT_STARTED
                    || err == ErrorCodes.ACTIVITY_ENDED
                    || err == ErrorCodes.INVALID_REQ
                    || err == ErrorCodes.RATE_LIMITED) {
                return ApiResponse.fail(err, resp.getMessage() != null ? resp.getMessage() : "秒杀失败");
            }
            return ApiResponse.fail(ErrorCodes.SYS_ERROR, resp.getMessage() != null ? resp.getMessage() : "秒杀失败");
        } finally {
            currentRequestCount.decrementAndGet();
        }
    }

    @Operation(summary = "查询秒杀异步结果", description = "按 requestId 查询 PENDING/SUCCESS/FAIL")
    @GetMapping("/seckill/result")
    public ApiResponse<String> seckillResult(
            @Parameter(description = "入队时返回的 requestId") @RequestParam("requestId") String requestId) {
        UserContext.requireCurrentUserId();
        String status = asyncSeckillEnterService.getResult(requestId);
        return ApiResponse.success(status != null ? status : "UNKNOWN");
    }

    @SuppressWarnings("unused")
    public ApiResponse<EnterSeckillResp> seckillBlockHandler(Long couponId, BlockException e) {
        return ApiResponse.fail(ErrorCodes.RATE_LIMITED, "系统繁忙，请稍后再试");
    }

    @SuppressWarnings("unused")
    public ApiResponse<EnterSeckillResp> seckillFallback(Long couponId, Throwable e) {
        return ApiResponse.fail(ErrorCodes.SYS_ERROR, "秒杀失败：" + e.getMessage());
    }
}
