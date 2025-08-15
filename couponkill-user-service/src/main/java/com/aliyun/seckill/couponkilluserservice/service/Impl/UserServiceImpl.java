// 文件路径: com/aliyun/seckill/couponkilluserservice/service/Impl/UserServiceImpl.java
package com.aliyun.seckill.couponkilluserservice.service.Impl;

import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.pojo.User;
import com.aliyun.seckill.common.utils.JwtUtils;
import com.aliyun.seckill.couponkilluserservice.mapper.UserMapper;
import com.aliyun.seckill.couponkilluserservice.service.UserService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.TimeUnit;

@Service
public class UserServiceImpl implements UserService {

    @Autowired
    private UserMapper userMapper;

    @Autowired
    private PasswordEncoder passwordEncoder;

    @Autowired
    private JwtUtils jwtUtils;

    @Autowired
    private RedisTemplate<String, Object> redisTemplate;

    private static final String USER_LOGIN_KEY = "user:login:";

    @Override
    @Transactional
    public void register(String username, String password, String phone) {
        // 检查用户名是否已存在
        User existUser = userMapper.selectByUsername(username);
        if (existUser != null) {
            throw new BusinessException(ResultCode.USER_EXIST);
        }

        // 创建新用户
        User user = new User();
        user.setUsername(username);
        user.setPassword(passwordEncoder.encode(password));
        user.setPhone(phone);
        user.setStatus(1);
        user.setCreateTime(LocalDateTime.now());
        user.setUpdateTime(LocalDateTime.now());

        userMapper.insert(user);
    }

    @Override
    public Map<String, Object> login(String username, String password) {
        // 查询用户
        User user = userMapper.selectByUsername(username);
        if (user == null) {
            throw new BusinessException(ResultCode.USER_NOT_FOUND);
        }

        // 验证密码
        if (!passwordEncoder.matches(password, user.getPassword())) {
            throw new BusinessException(ResultCode.PASSWORD_ERROR);
        }

        // 生成令牌
        String token = jwtUtils.generateToken(user.getId());

        // 存入Redis，用于令牌黑名单
        redisTemplate.opsForValue().set(USER_LOGIN_KEY + user.getId(), token,
                24, TimeUnit.HOURS);

        // 返回结果
        Map<String, Object> result = new HashMap<>();
        result.put("token", token);
        result.put("userId", user.getId());
        result.put("username", user.getUsername());
        return result;
    }

    @Override
    public User getUserById(Long userId) {
        return userMapper.selectById(userId);
    }

    @Override
    public void handleInactiveUsers() {
        // 实现处理失效用户的逻辑
    }
}
