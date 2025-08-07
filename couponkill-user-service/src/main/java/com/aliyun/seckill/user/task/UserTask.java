// couponkill-user-service/src/main/java/com/aliyun/seckill/user/task/UserTask.java
package com.aliyun.seckill.user.task;

import com.aliyun.seckill.user.service.UserService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

@Component
public class UserTask {

    @Autowired
    private UserService userService;

    // 每周日凌晨执行
    @Scheduled(cron = "0 0 0 ? * SUN")
    public void handleInactiveUsers() {
        // 处理20天未活跃的用户
        userService.handleInactiveUsers();
    }
}