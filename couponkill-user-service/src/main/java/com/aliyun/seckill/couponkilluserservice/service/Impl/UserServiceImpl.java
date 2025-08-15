// 文件路径: com/aliyun/seckill/couponkilluserservice/service/Impl/UserServiceImpl.java
package com.aliyun.seckill.couponkilluserservice.service.Impl;

import com.aliyun.seckill.common.context.UserContext;
import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.pojo.User;
import com.aliyun.seckill.common.redis.RedisCache;
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
    // 注入RedisCache
    @Autowired
    private RedisCache redisCache;


    private static final String USER_LOGIN_KEY = "user:login:";
    private static final String USER_INFO_KEY = "user:info:";
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
        // 使用RedisCache获取用户信息，解决缓存穿透和击穿问题
        String key = USER_INFO_KEY + userId;

        // 将userId添加到布隆过滤器
        redisCache.addToBloomFilter(key);

        Object userObj = redisCache.get(
                key,
                () -> userMapper.selectById(userId), // 数据库查询方法
                30, // 过期时间30分钟
                TimeUnit.MINUTES
        );

        return (User) userObj;
    }

    @Override
    public void handleInactiveUsers() {
        try {
            // 定义失效阈值：30天未活跃的用户
            LocalDateTime inactiveThreshold = LocalDateTime.now().minusDays(30);

            // 实际应用中，应该结合以下信息判断用户是否失效：
            // 1. 用户最后活跃时间超过阈值
            // 2. 用户没有未过期的订单
            // 3. 用户没有其他业务关联

            // 由于当前数据库结构限制，这里仅作示例实现
            // 在实际项目中，应该结合订单服务查询用户是否有有效订单

            System.out.println("开始处理失效用户，阈值时间：" + inactiveThreshold);

            // 示例：更新超过30天未活跃且没有订单的用户状态为失效
            // 注意：这需要根据实际业务逻辑和表结构进行调整

            // 模拟处理逻辑
            // userMapper.updateInactiveUsersStatus(inactiveThreshold);

            System.out.println("失效用户处理完成");
        } catch (Exception e) {
            System.err.println("处理失效用户时发生错误: " + e.getMessage());
            e.printStackTrace();
        }
    }
    @Override
    public User getCurrentUserInfo() {
    // 从请求上下文中获取当前用户ID
    String userId = UserContext.getCurrentUserId();
    if (userId == null) {
        throw new BusinessException(ResultCode.AUTH_FAIL);
    }

    return getUserById(Long.valueOf(userId));
}

}
