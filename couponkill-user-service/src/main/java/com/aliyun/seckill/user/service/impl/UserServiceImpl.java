// com.aliyun.seckill.user.service.impl.UserServiceImpl.java
package com.aliyun.seckill.user.service.impl;

import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.result.ResultCode;
import com.aliyun.seckill.common.utils.JwtUtils;
import com.aliyun.seckill.pojo.User;
import com.aliyun.seckill.user.mapper.UserMapper;
import com.aliyun.seckill.user.service.UserService;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import com.baomidou.mybatisplus.extension.service.impl.ServiceImpl;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;

@Service
public class UserServiceImpl extends ServiceImpl<UserMapper, User> implements UserService {

    @Autowired
    private PasswordEncoder passwordEncoder;

    @Autowired
    private JwtUtils jwtUtils;

    @Override
    public void register(String username, String password, String phone, String email) {
        // 检查用户是否已存在
        User existingUser = getOne(new QueryWrapper<User>().eq("username", username));
        if (existingUser != null) {
            throw new BusinessException(ResultCode.USER_EXIST.getCode(), "用户名已存在");
        }

        // 创建新用户
        User user = new User();
        user.setUsername(username);
        user.setPassword(passwordEncoder.encode(password));
        user.setPhone(phone);
        user.setEmail(email);
        user.setStatus(1); // 正常状态

        save(user);
    }

    @Override
    public String login(String username, String password) {
        // 查询用户
        User user = getOne(new QueryWrapper<User>().eq("username", username));
        if (user == null) {
            throw new BusinessException(ResultCode.USER_NOT_FOUND.getCode(), "用户不存在");
        }

        // 验证密码
        if (!passwordEncoder.matches(password, user.getPassword())) {
            throw new BusinessException(ResultCode.PASSWORD_ERROR.getCode(), "密码错误");
        }

        // 生成JWT令牌
        return jwtUtils.generateToken(user.getId());
    }

    @Override
    public User getUserById(Long userId) {
        return getById(userId);
    }
}