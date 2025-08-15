package com.aliyun.seckill.couponkilluserservice.controller;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.utils.JwtUtils;
import lombok.Data;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

@RestController
@RequestMapping("/api/v1/auth")
public class AuthController {

    @Value("${auth.jwt.secret:CHANGE_ME_256bit_secret_please_CHANGE_ME_1234567890}")
    private String secret;
    @Value("${auth.jwt.issuer:https://auth.couponkill}")
    private String issuer;
    @Value("${auth.jwt.audience:couponkill}")
    private String audience;
    @Value("${auth.jwt.mock-ttl-seconds:3600}")
    private long ttlSeconds;

    @PostMapping("/token/mock")
    public ApiResponse<TokenResp> mock(@RequestBody MockReq req){
        String token = JwtUtils.createToken(secret, issuer, audience, req.getUserId(), req.getRoles()==null? List.of("user"):req.getRoles(), ttlSeconds);
        TokenResp resp = new TokenResp();
        resp.setToken(token);
        resp.setTokenType("Bearer");
        resp.setExpiresIn(ttlSeconds);
        return ApiResponse.success(resp);
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
