package com.aliyun.seckill.couponkillorderservice.mapper;

import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.common.pojo.UserCouponCount;
import org.apache.ibatis.annotations.Param;

import java.util.List;

public interface OrderMapper {
    int insert(Order order);

    Order selectById(String orderId);

    Order selectOrderById(Long id);

    List<Order> selectByUserId(@Param("userId") Long userId, @Param("offset") int offset, @Param("limit") int limit);

    List<Order> selectAllByCondition(@Param("startTime") String startTime, @Param("endTime") String endTime, @Param("offset") int offset, @Param("limit") int limit);

    int updateStatus(@Param("orderId") String orderId, @Param("status") int status, @Param("updateTime") java.time.LocalDateTime updateTime);

    int updateStatusWithCancelTime(@Param("orderId") String orderId, @Param("status") int status, @Param("cancelTime") java.time.LocalDateTime cancelTime, @Param("updateTime") java.time.LocalDateTime updateTime);

    int updateOrderStatus(@Param("orderId") Long orderId, @Param("status") int status);

    long countByUserAndCoupon(@Param("userId") Long userId, @Param("couponId") Long couponId);

    UserCouponCount selectUserCouponCountById(Long userId);

    int insertUserCouponCount(UserCouponCount userCouponCount);

    int updateUserCouponCount(@Param("userId") Long userId, @Param("totalCount") int totalCount, @Param("seckillCount") int seckillCount);

    long countAll();
}
