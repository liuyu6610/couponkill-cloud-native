package com.aliyun.seckill.couponkillorderservice.mapper;

import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.common.pojo.UserCouponCount;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.time.LocalDateTime;
import java.util.List;

@Mapper
public interface OrderMapper {
    int insert(Order order);

    Order selectById(@Param("id") String id);

    Order selectOrderById(@Param("id") String id);

    int updateStatus(@Param("id") String id, @Param("status") int status, @Param("updateTime") LocalDateTime updateTime);

    int updateStatusWithCancelTime(@Param("id") String id, @Param("status") int status, @Param("cancelTime") LocalDateTime cancelTime, @Param("updateTime") LocalDateTime updateTime);

    int updateOrderStatus(@Param("id") String id, @Param("status") int status);

    long countByUserAndCoupon(@Param("userId") Long userId, @Param("couponId") Long couponId);

    List<Order> selectByUserId(@Param("userId") Long userId, @Param("offset") int offset, @Param("pageSize") int pageSize);

    List<Order> selectAllByCondition(@Param("startTime") String startTime, @Param("endTime") String endTime, @Param("offset") int offset, @Param("pageSize") int pageSize);

    UserCouponCount selectUserCouponCountById(@Param("userId") Long userId);

    int insertUserCouponCount(UserCouponCount userCouponCount);

    int updateUserCouponCount(@Param("userId") Long userId, @Param("totalCount") int totalCount, @Param("seckillCount") int seckillCount);

    // 新增方法：插入订单
    int insertOrder(Order order);
}
