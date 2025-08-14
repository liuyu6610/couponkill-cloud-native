// com.aliyun.seckill.couponkilluserservice.controller.AuthController.java
package com.aliyun.seckill.couponkilluserservice.controller;

import com.aliyun.seckill.common.api.ApiResp;
import com.aliyun.seckill.common.utils.JwtUtils;
import lombok.Data;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.web.bind.annotation.*;

import java.util.List;

@RestController
@RequestMapping("/api/v1/auth")
public class AuthController {

    @Autowired
    private JwtUtils jwtUtils; // 使用JwtUtils替代JwtUtil

    @Value("${auth.jwt.mock-ttl-seconds:3600}")
    private long ttlSeconds;

    @PostMapping("/token/mock")
    public ApiResp<TokenResp> mock(@RequestBody MockReq req){
        // 使用JwtUtils替代JwtUtil
        String token = jwtUtils.createToken(req.getUserId(),
            req.getRoles() == null ? List.of("user") : req.getRoles(), ttlSeconds);
        TokenResp resp = new TokenResp();
        resp.setToken(token);
        resp.setTokenType("Bearer");
        resp.setExpiresIn(ttlSeconds);
        return ApiResp.ok(resp, null);
    }

    @Data
    public static class MockReq {
        private String userId;
        private List<String> roles;
    }

    @Data
    public static class TokenResp {
        private String tokenType;
        private String token;
        private long expiresIn;
    }
}
