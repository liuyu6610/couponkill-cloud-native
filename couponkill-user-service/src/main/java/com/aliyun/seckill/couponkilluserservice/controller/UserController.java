package com.aliyun.seckill.couponkilluserservice.controller;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.couponkilluserservice.service.UserService;
import io.swagger.annotations.Api;
import io.swagger.annotations.ApiOperation;
import org.springframework.web.bind.annotation.*;

@Api(tags = "用户管理")
@RestController
@RequestMapping("/api/v1/user")
public class UserController {

    private final UserService userService;

    public UserController(UserService userService) {
        this.userService = userService;
    }

    @ApiOperation("用户登录")
    @PostMapping("/login")
    public ApiResponse<User> login(@RequestParam String username, @RequestParam String password) {
        return ApiResponse.success(userService.login(username, password));
    }

    @ApiOperation("用户注册")
    @PostMapping("/register")
    public ApiResponse<User> register(@RequestBody User user) {
        return ApiResponse.success(userService.register(user));
    }

    @ApiOperation("获取用户信息")
    @GetMapping("/profile")
    public ApiResponse<User> getProfile(@RequestParam Long userId) {
        return ApiResponse.success(userService.getProfile(userId));
    }
}
