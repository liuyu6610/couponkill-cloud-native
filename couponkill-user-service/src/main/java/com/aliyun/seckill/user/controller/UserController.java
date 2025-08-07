// com.aliyun.seckill.user.controller.UserController.java
package com.aliyun.seckill.user.controller;

import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.pojo.User;
import com.aliyun.seckill.user.service.UserService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

import java.util.Map;

@RestController
@RequestMapping("/user")
@Tag(name = "用户管理", description = "用户注册、登录接口")
public class UserController {

    @Autowired
    private UserService userService;

    @PostMapping("/register")
    @Operation(summary = "用户注册")
    public Result<?> register(
            @RequestParam String username,
            @RequestParam String password,
            @RequestParam String phone) {
        userService.register(username, password, phone);
        return Result.success();
    }

    @PostMapping("/login")
    @Operation(summary = "用户登录")
    public Result<Map<String, Object>> login(
            @RequestParam String username,
            @RequestParam String password) {
        Map<String, Object> result = userService.login(username, password);
        return Result.success(result);
    }

    @GetMapping("/{userId}")
    @Operation(summary = "获取用户信息")
    public Result<User> getUserById(@PathVariable Long userId) {
        User user = userService.getUserById(userId);
        return Result.success(user);
    }
}