package com.aliyun.seckill.couponkillorderservice.controller;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import io.swagger.annotations.Api;
import io.swagger.annotations.ApiOperation;
import org.springframework.web.bind.annotation.*;

@Api(tags = "订单管理")
@RestController
@RequestMapping("/api/v1/order")
public class OrderController {

    private final OrderService orderService;

    public OrderController(OrderService orderService) {
        this.orderService = orderService;
    }

    @ApiOperation("创建订单")
    @PostMapping("/create")
    public ApiResponse<Order> createOrder(@RequestBody Order order) {
        return ApiResponse.success(orderService.createOrder(order));
    }

    @ApiOperation("根据ID查询订单")
    @GetMapping("/{id}")
    public ApiResponse<Order> getOrderById(@PathVariable Long id) {
        return ApiResponse.success(orderService.getOrderById(id));
    }

    @ApiOperation("订单支付")
    @PostMapping("/pay")
    public ApiResponse<Boolean> payOrder(@RequestParam Long orderId) {
        return ApiResponse.success(orderService.payOrder(orderId));
    }
}
