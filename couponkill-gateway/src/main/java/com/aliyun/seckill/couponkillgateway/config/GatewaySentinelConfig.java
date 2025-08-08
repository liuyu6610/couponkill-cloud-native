package com.aliyun.seckill.couponkillgateway.config;

import com.alibaba.csp.sentinel.adapter.gateway.sc.callback.BlockRequestHandler;
import com.alibaba.csp.sentinel.adapter.gateway.sc.callback.GatewayCallbackManager;
import org.springframework.context.annotation.Configuration;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.web.reactive.function.server.ServerResponse;
import org.springframework.web.server.ServerWebExchange;
import reactor.core.publisher.Mono;
import java.util.HashMap;
import java.util.Map;
@Configuration
public class GatewaySentinelConfig {
    public GatewaySentinelConfig() {
        // 自定义网关限流响应
        GatewayCallbackManager.setBlockHandler(new BlockRequestHandler() {
            @Override
            public Mono<ServerResponse> handleRequest(ServerWebExchange exchange, Throwable t) {
                Map<String, Object> result = new HashMap<>();
                result.put("code", 429);
                result.put("message", "网关限流，请稍后再试");

                String responseStr = "{\"code\":429,\"message\":\"网关限流，请稍后再试\"}";

                return ServerResponse.status(HttpStatus.TOO_MANY_REQUESTS)
                        .contentType(MediaType.APPLICATION_JSON)
                        .bodyValue(responseStr);
            }
        });
    }
}
