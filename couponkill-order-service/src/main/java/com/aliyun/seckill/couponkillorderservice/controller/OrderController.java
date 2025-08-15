package com.aliyun.seckill.couponkillorderservice.controller;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import io.swagger.v3.oas.annotations.tags.Tag;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import org.springframework.web.bind.annotation.*;

@Tag(name = "订单管理", description = "订单相关操作接口")
@RestController
@RequestMapping("/api/v1/order")
public class OrderController {

    private final OrderService orderService;

    public OrderController(OrderService orderService) {
        this.orderService = orderService;
    }

    @Operation(summary = "创建订单", description = "创建一个新的订单")
    @PostMapping("/create")
    public ApiResponse<Order> createOrder(
            @Parameter(description = "订单信息") @RequestBody Order order) {
        return ApiResponse.success(orderService.createOrder(order));
    }

    @Operation(summary = "根据ID查询订单", description = "根据订单ID获取订单详情")
    @GetMapping("/{id}")
    public ApiResponse<Order> getOrderById(
            @Parameter(description = "订单ID") @PathVariable Long id) {
        return ApiResponse.success(orderService.getOrderById(id));
    }

    @Operation(summary = "订单支付", description = "对指定订单进行支付操作")
    @PostMapping("/pay")
    public ApiResponse<Boolean> payOrder(
            @Parameter(description = "订单ID") @RequestParam Long orderId) {
        return ApiResponse.success(orderService.payOrder(orderId));
    }
}
