package com.aliyun.seckill.couponkillorderservice.service;

import com.aliyun.seckill.couponkillorderservice.domain.UserNotification;
import com.aliyun.seckill.couponkillorderservice.mapper.UserNotificationMapper;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.util.List;

@Slf4j
@Service
@RequiredArgsConstructor
public class NotificationService {

    private final UserNotificationMapper notificationMapper;

    public void notifyReservation(Long userId, Long reservationId, String type, String title, String content) {
        if (userId == null) {
            return;
        }
        try {
            UserNotification n = new UserNotification();
            n.setUserId(userId);
            n.setType(type);
            n.setTitle(title);
            n.setContent(content);
            n.setRefId(reservationId);
            notificationMapper.insert(n);
        } catch (Exception e) {
            // 通知失败不回滚预约状态
            log.warn("写入站内通知失败: userId={}, type={}, err={}", userId, type, e.getMessage());
        }
    }

    public List<UserNotification> listMine(Long userId, int limit) {
        int lim = limit <= 0 || limit > 50 ? 20 : limit;
        return notificationMapper.selectByUserId(userId, lim);
    }

    public int unreadCount(Long userId) {
        return notificationMapper.countUnread(userId);
    }

    public boolean markRead(Long userId, Long id) {
        return notificationMapper.markRead(userId, id) > 0;
    }

    public int markAllRead(Long userId) {
        return notificationMapper.markAllRead(userId);
    }
}
