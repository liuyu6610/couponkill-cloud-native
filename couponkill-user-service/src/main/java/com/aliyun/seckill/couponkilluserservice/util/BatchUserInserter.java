package com.aliyun.seckill.couponkilluserservice.util;

import com.aliyun.seckill.common.pojo.User;
import com.aliyun.seckill.common.pojo.UserCouponCount;
import com.aliyun.seckill.couponkilluserservice.mapper.UserCouponCountMapper;
import com.aliyun.seckill.couponkilluserservice.mapper.UserMapper;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Component;

import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.List;

/**
 * 批量用户插入工具类
 * 根据分库分表规则批量插入用户数据
 */
@Slf4j
@Component
public class BatchUserInserter {

    @Autowired
    private UserMapper userMapper;

    @Autowired
    private UserCouponCountMapper userCouponCountMapper;

    @Autowired
    private PasswordEncoder passwordEncoder;

    /**
     * 批量插入用户，根据分库分表规则分配到不同的数据库和表
     * @param users 用户列表
     */
    public void batchInsertUsers(List<User> users) {
        if (users == null || users.isEmpty()) {
            return;
        }

        log.info("开始批量插入 {} 个用户", users.size());
        long startTime = System.currentTimeMillis();

        // 按分库规则分组用户
        List<User> db0Users = new ArrayList<>();
        List<User> db1Users = new ArrayList<>();

        for (User user : users) {
            // 根据分库规则分配用户到不同的数据库
            // user-db-$->{id % 2}
            if (user.getId() % 2 == 0) {
                db0Users.add(user);
            } else {
                db1Users.add(user);
            }
        }

        log.info("分配结果: db0用户数={}, db1用户数={}", db0Users.size(), db1Users.size());

        try {
            // 批量插入用户到数据库0
            if (!db0Users.isEmpty()) {
                insertUsersToDatabase(db0Users, "user-db-0");
            }

            // 批量插入用户到数据库1
            if (!db1Users.isEmpty()) {
                insertUsersToDatabase(db1Users, "user-db-1");
            }

            long endTime = System.currentTimeMillis();
            log.info("批量插入完成，耗时: {} ms", (endTime - startTime));
        } catch (Exception e) {
            log.error("批量插入用户失败", e);
            throw new RuntimeException("批量插入用户失败", e);
        }
    }

    /**
     * 插入用户到指定数据库
     * @param users 用户列表
     * @param databaseName 数据库名称
     */
    private void insertUsersToDatabase(List<User> users, String databaseName) {
        log.info("开始向 {} 插入 {} 个用户", databaseName, users.size());

        // 分批处理，避免一次性处理太多数据
        int batchSize = 1000;
        for (int i = 0; i < users.size(); i += batchSize) {
            int endIndex = Math.min(i + batchSize, users.size());
            List<User> batchUsers = users.subList(i, endIndex);

            // 插入用户
            for (User user : batchUsers) {
                try {
                    // 加密密码
                    if (user.getPassword() != null && !user.getPassword().startsWith("$2a$")) {
                        user.setPassword(passwordEncoder.encode(user.getPassword()));
                    }

                    // 设置默认值
                    if (user.getEmail() == null) {
                        user.setEmail("");
                    }
                    if (user.getStatus() == null) {
                        user.setStatus(1);
                    }
                    if (user.getCreateTime() == null) {
                        user.setCreateTime(LocalDateTime.now());
                    }
                    if (user.getUpdateTime() == null) {
                        user.setUpdateTime(LocalDateTime.now());
                    }

                    userMapper.insertUser(user);

                    // 初始化用户优惠券统计
                    UserCouponCount count = new UserCouponCount();
                    count.setUserId(user.getId());
                    count.setTotalCount(0);
                    count.setSeckillCount(0);
                    count.setNormalCount(0);
                    count.setExpiredCount(0);
                    userCouponCountMapper.insert(count);
                } catch (Exception e) {
                    log.error("插入用户失败，用户ID: {}", user.getId(), e);
                    throw e;
                }
            }

            log.info("已处理 {} 个用户到 {}", endIndex, databaseName);
        }
    }

    /**
     * 生成测试用户数据
     * @param startId 起始用户ID
     * @param count 用户数量
     * @return 用户列表
     */
    public List<User> generateTestUsers(long startId, int count) {
        List<User> users = new ArrayList<>(count);
        for (int i = 0; i < count; i++) {
            User user = new User();
            long userId = startId + i;
            user.setId(userId);
            user.setUsername("testuser" + userId);
            user.setPassword("password123");
            user.setPhone("1380000" + String.format("%04d", userId % 10000));
            user.setEmail("testuser" + userId + "@example.com");
            user.setStatus(1);
            user.setCreateTime(LocalDateTime.now());
            user.setUpdateTime(LocalDateTime.now());
            users.add(user);
        }
        return users;
    }
}