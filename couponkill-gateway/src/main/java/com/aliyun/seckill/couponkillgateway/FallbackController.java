package com.aliyun.seckill.couponkillgateway;

import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.reactive.function.server.ServerResponse;
import reactor.core.publisher.Mono;

@RestController
public class FallbackController {
    @GetMapping("/fallback/user")
    public Mono<ServerResponse> userFallback() {
        return ServerResponse.status( HttpStatus.TOO_MANY_REQUESTS)
                .contentType( MediaType.APPLICATION_JSON)
                .bodyValue("{\"code\":429,\"message\":\"用户服务限流，请稍后再试\"}");
    }


    // 其他服务的fallback方法...
    @GetMapping("/fallback/coupon")
    public Mono<ServerResponse> couponFallback() {
        return ServerResponse.status( HttpStatus.TOO_MANY_REQUESTS)
                .contentType( MediaType.APPLICATION_JSON)
                .bodyValue("{\"code\":429,\"message\":\"优惠券服务限流，请稍后再试\"}");
    }
    @GetMapping("/fallback/order")
    public Mono<ServerResponse> orderFallback() {
        return ServerResponse.status( HttpStatus.TOO_MANY_REQUESTS)
                .contentType( MediaType.APPLICATION_JSON)
                .bodyValue("{\"code\":429,\"message\":\"订单服务限流，请稍后再试\"}");
    }
    @GetMapping("/fallback/admin")
    public Mono<ServerResponse> adminFallback() {
        return ServerResponse.status( HttpStatus.TOO_MANY_REQUESTS)
                .contentType( MediaType.APPLICATION_JSON)
                .bodyValue("{\"code\":429,\"message\":\"管理员服务限流，请稍后再试\"}");
    }
}