package com.aliyun.seckill.couponkilluserservice.controller;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.pojo.User;
import com.aliyun.seckill.couponkilluserservice.service.UserService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import io.swagger.v3.oas.annotations.tags.Tag;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

import java.util.Map;
@Slf4j
@Tag(name = "用户管理", description = "用户相关操作接口")
@RestController
@RequestMapping("/api/v1/user")
public class UserController {
    private final UserService userService;

    @Autowired
    public UserController(UserService userService) {
        this.userService = userService;
    }

    @Operation(summary = "用户登录", description = "用户登录接口")
    @PostMapping("/login")
    public ApiResponse<Map<String, Object>> login(
            @Parameter(description = "用户名") @RequestParam String username,
            @Parameter(description = "密码") @RequestParam String password) {
        return ApiResponse.success( userService.login(username, password));
    }

    @Operation(summary = "用户注册", description = "用户注册接口")
    @PostMapping("/register")
    public ApiResponse<User> register(
            @Parameter(description = "用户名") @RequestParam String username,
            @Parameter(description = "密码") @RequestParam String password,
            @Parameter(description = "手机号") @RequestParam String phone) {

        // 参数校验
        if (username == null || username.trim().isEmpty()) {
            return ApiResponse.fail(400, "用户名不能为空");
        }

        if (password == null || password.length() < 6) {
            return ApiResponse.fail(400, "密码长度不能少于6位");
        }

        if (phone == null || !phone.matches("^1[3-9]\\d{9}$")) {
            return ApiResponse.fail(400, "手机号格式不正确");
        }

        try {
            User user = userService.register(username, password, phone);
            return ApiResponse.success(user);
        } catch (BusinessException e) {
            return ApiResponse.fail(e.getCode(), e.getMessage());
        } catch (Exception e) {
            log.error("用户注册失败，username: {}", username, e);
            return ApiResponse.fail(500, "注册失败: " + e.getMessage());
        }
    }


    @Operation(summary = "获取用户信息", description = "根据用户ID获取用户信息")
    @GetMapping("/profile")
    public ApiResponse<User> getProfile(
            @Parameter(description = "用户ID") @RequestParam Long userId) {
        return ApiResponse.success(userService.getUserById(userId));
    }
}
