// com.aliyun.seckill.user.controller.UserController.java
package com.aliyun.seckill.couponkilluserservice.controller;

import com.aliyun.seckill.common.pojo.User;
import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.couponkilluserservice.service.UserService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

import java.util.Map;
@Slf4j
@RestController
@RequestMapping("/user")
@Tag(name = "用户管理", description = "用户注册、登录接口")
public class UserController {

    @Autowired
    private UserService userService;

    @PostMapping("/register")
    @Operation(summary = "用户注册")
    public Result<?> register(
            @RequestParam("username") String username,
            @RequestParam("password") String password,
            @RequestParam("phone") String phone) {
        log.info("Received register request for username: {}", username);
        try {
            userService.register(username, password, phone);
            log.info("User registration successful for username: {}", username);
            return Result.success();
        } catch (Exception e) {
            log.error("User registration failed for username: {}", username, e);
            return Result.error("注册失败: " + e.getMessage());
        }
    }

    @PostMapping("/login")
    @Operation(summary = "用户登录")
    public Result<Map<String, Object>> login(
            @RequestParam("username") String username,
            @RequestParam("password") String password) {
        // 参数校验
        if (username == null || username.trim().isEmpty()) {
            return Result.error("用户名不能为空");
        }
        if (password == null || password.trim().isEmpty()) {
            return Result.error("密码不能为空");
        }

        try {
            Map<String, Object> result = userService.login(username, password);
            if (result != null && !result.isEmpty()) {
                return Result.success(result);
            } else {
                return Result.error("登录失败，用户名或密码错误");
            }
        } catch (Exception e) {
            return Result.error("登录异常：" + e.getMessage());
        }
    }

    @GetMapping("/{userId}")
    @Operation(summary = "获取用户信息")
    public Result<User> getUserById(@PathVariable("userId") Long userId) {
        User user = userService.getUserById(userId);
        return Result.success(user);
    }
}
