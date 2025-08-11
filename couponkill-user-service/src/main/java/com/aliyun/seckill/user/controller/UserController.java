// com.aliyun.seckill.user.controller.UserController.java
package com.aliyun.seckill.user.controller;

import com.aliyun.seckill.common.pojo.User;
import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.common.service.user.UserService;
import com.aliyun.seckill.common.utils.JwtUtils;
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
    @Autowired
    private JwtUtils jwtUtils;

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
                // 假设返回结果中包含 userId 字段
                Object userIdObj = result.get("userId");
                if (userIdObj instanceof Long) {
                    Long userId = (Long) userIdObj;
                    // 使用 userId 生成 token
                    String token = jwtUtils.generateToken(userId);
                    result.put("token", token);
                    return Result.success(result);
                } else {
                    return Result.error("登录失败，用户ID无效");
                }
            } else {
                return Result.error("登录失败，用户名或密码错误");
            }
        } catch (Exception e) {
            return Result.error("登录异常：" + e.getMessage());
        }
    }


    @GetMapping("/{userId}")
    @Operation(summary = "获取用户信息")
    public Result<User> getUserById(@PathVariable Long userId) {
        User user = userService.getUserById(userId);
        return Result.success(user);
    }
}