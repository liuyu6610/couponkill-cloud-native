package com.aliyun.seckill.couponkillorderservice.mapper;

import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.common.pojo.UserCouponCount;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.time.LocalDateTime;
import java.util.List;

@Mapper
public interface OrderMapper {
    int insertOrder(Order order);
    Order selectOrderById(@Param("id") Long id);
    int updateOrderStatus(@Param("orderId") Long orderId, @Param("status") String status);
    Order selectById(String orderId);
    long countByUserAndCoupon(@Param("userId") Long userId, @Param("couponId") Long couponId);
    List<Order> selectByUserId(@Param("userId") Long userId, @Param("offset") int offset, @Param("limit") int limit);
    List<Order> selectAllByCondition(@Param("startTime") String startTime,
                                   @Param("endTime") String endTime,
                                   @Param("offset") int offset,
                                   @Param("limit") int limit);
    long countAll();
    int insert(Order order);
    int updateStatus(@Param("orderId") String orderId,
                    @Param("status") Integer status,
                    @Param("updateTime") LocalDateTime updateTime);
    int updateStatusWithCancelTime(@Param("orderId") String orderId,
                                  @Param("status") Integer status,
                                  @Param("cancelTime") LocalDateTime cancelTime,
                                  @Param("updateTime") LocalDateTime updateTime);

    // 用户优惠券计数相关方法
    UserCouponCount selectUserCouponCountById(@Param("userId") Long userId);
    int insertUserCouponCount(UserCouponCount userCouponCount);
    int updateUserCouponCount(@Param("userId") Long userId,
                             @Param("totalCount") int totalCount,
                             @Param("seckillCount") int seckillCount);
}
