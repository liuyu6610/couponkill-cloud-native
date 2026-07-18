package com.aliyun.seckill.couponkillorderservice.mapper;

import com.aliyun.seckill.couponkillorderservice.domain.UserNotification;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.util.List;

@Mapper
public interface UserNotificationMapper {

    int insert(UserNotification row);

    List<UserNotification> selectByUserId(@Param("userId") Long userId, @Param("limit") int limit);

    int countUnread(@Param("userId") Long userId);

    int markRead(@Param("userId") Long userId, @Param("id") Long id);

    int markAllRead(@Param("userId") Long userId);
}
