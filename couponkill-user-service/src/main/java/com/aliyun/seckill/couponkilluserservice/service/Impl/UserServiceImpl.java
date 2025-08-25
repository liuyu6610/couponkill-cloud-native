// 文件路径: com/aliyun/seckill/couponkilluserservice/service/Impl/UserServiceImpl.java
package com.aliyun.seckill.couponkilluserservice.service.Impl;

import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.pojo.User;
import com.aliyun.seckill.common.pojo.UserCouponCount;
import com.aliyun.seckill.common.utils.JwtUtils;
import com.aliyun.seckill.common.utils.SnowflakeIdGenerator;
import com.aliyun.seckill.couponkilluserservice.mapper.UserCouponCountMapper;
import com.aliyun.seckill.couponkilluserservice.mapper.UserMapper;
import com.aliyun.seckill.couponkilluserservice.service.UserService;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
@Slf4j
@Service
public class UserServiceImpl implements UserService {

    @Autowired
    private UserMapper userMapper;

    @Autowired
    private PasswordEncoder passwordEncoder;

    @Autowired
    private JwtUtils jwtUtils;
    @Autowired
    private UserCouponCountMapper userCouponCountMapper;
    @Autowired
    private RedisTemplate<String, Object> redisTemplate;
    private final SnowflakeIdGenerator idGenerator=new SnowflakeIdGenerator(1, 1);
    // 使用Redis生成ID的键名
    private static final String USER_ID_KEY = "user:id:sequence";

    // 本地缓存ID段，减少Redis访问
    private final AtomicLong currentId = new AtomicLong(0);
    private final AtomicLong maxId = new AtomicLong(0);
    private static final int ID_BATCH_SIZE = 100; // 每次从Redis获取的ID数量

    private static final String USER_LOGIN_KEY = "user:login:";
    /**
     * 生成用户ID
     * 使用Redis实现分布式ID生成
     */
    private synchronized Long generateUserId() {
        // 检查是否需要从Redis获取新的ID段
        if (currentId.get() >= maxId.get()) {
            // 从Redis获取一批ID
            Long newMaxId = redisTemplate.opsForValue().increment(USER_ID_KEY, ID_BATCH_SIZE);
            if (newMaxId != null) {
                maxId.set(newMaxId);
                currentId.set(newMaxId - ID_BATCH_SIZE);
            } else {
                // 如果Redis不可用，使用雪花算法作为备选方案
                return idGenerator.nextId();
            }
        }
        return currentId.incrementAndGet();
    }

    @Override
    @Transactional
    public User register(String username, String password, String phone) {
        // 检查用户名是否已存在
        User existUser = userMapper.selectByUsername(username);
        if (existUser != null) {
            throw new BusinessException(ResultCode.USER_EXIST);
        }

        // 创建新用户
        User user = new User();
        user.setId(generateUserId()); // 提前生成 ID
        user.setUsername(username);
        user.setPassword(passwordEncoder.encode(password));
        user.setPhone(phone);
        user.setEmail(""); // 设置默认邮箱
        user.setStatus(1);
        user.setCreateTime(LocalDateTime.now());
        user.setUpdateTime(LocalDateTime.now());

        // 插入用户（注意：去掉 useGeneratedKeys）
        int result = userMapper.insertUser(user);
        if (result <= 0 || user.getId() == null) {
            throw new BusinessException(ResultCode.SYSTEM_BUSY.getCode(), "用户注册失败");
        }

        // 初始化用户优惠券统计
        UserCouponCount count = new UserCouponCount();
        count.setUserId(user.getId());
        count.setTotalCount(0);
        count.setSeckillCount(0);
        count.setNormalCount(0);
        count.setExpiredCount(0);
        userCouponCountMapper.insert(count);

        return user;
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
        return (User) userMapper.selectById(userId);
    }

    @Override
    public void handleInactiveUsers() {
        try {
            // 实现处理失效用户的逻辑
            // 例如：查找超过一定时间未活跃的用户，并进行相应处理
            LocalDateTime inactiveThreshold = LocalDateTime.now().minusMonths(6); // 6个月未活跃
            List<User> inactiveUsers = userMapper.selectInactiveUsers(inactiveThreshold);

            if (inactiveUsers != null && !inactiveUsers.isEmpty()) {
                for (User user : inactiveUsers) {
                    // 可以根据业务需求进行处理，例如：
                    // 1. 发送提醒邮件/短信
                    // 2. 降低用户等级
                    // 3. 冻结账户等

                    log.info("发现失效用户，userId: {}, lastActiveTime: {}",
                            user.getId(), user.getLastActiveTime());

                    // 更新用户最后活跃时间（示例）
                    user.setLastActiveTime(LocalDateTime.now());
                    user.setUpdateTime(LocalDateTime.now());
                    userMapper.updateUserLastActiveTime(user.getId(), user.getLastActiveTime());
                }
            }
        } catch (Exception e) {
            log.error("处理失效用户失败", e);
        }
    }

}
