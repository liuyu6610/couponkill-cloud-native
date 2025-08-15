package com.aliyun.seckill.couponkilluserservice.controller;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.pojo.User;
import com.aliyun.seckill.couponkilluserservice.service.UserService;
import io.swagger.v3.oas.annotations.tags.Tag;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

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
    public ApiResponse<User> login(
            @Parameter(description = "用户名") @RequestParam String username,
            @Parameter(description = "密码") @RequestParam String password) {
        return ApiResponse.success((User) userService.login(username, password));
    }

    @Operation(summary = "用户注册", description = "用户注册接口")
    @PostMapping("/register")
    public ApiResponse<User> register(@RequestBody User user) {
        return ApiResponse.success(userService.register(user.getUsername(), user.getPassword(), user.getPhone()));
    }

    @Operation(summary = "获取用户信息", description = "根据用户ID获取用户信息")
    @GetMapping("/profile")
    public ApiResponse<User> getProfile(
            @Parameter(description = "用户ID") @RequestParam Long userId) {
        return ApiResponse.success(userService.getUserById(userId));
    }
}
