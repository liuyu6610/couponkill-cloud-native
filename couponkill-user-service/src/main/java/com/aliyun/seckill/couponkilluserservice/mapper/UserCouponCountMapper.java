// UserCouponCountMapper.java
package com.aliyun.seckill.couponkilluserservice.mapper;

import com.aliyun.seckill.common.pojo.UserCouponCount;
import org.apache.ibatis.annotations.*;

@Mapper
public interface UserCouponCountMapper {

    // 根据用户ID查询优惠券统计
    @Select("SELECT user_id as userId, total_count as totalCount, seckill_count as seckillCount, " +
            "normal_count as normalCount, expired_count as expiredCount, update_time as updateTime, version " +
            "FROM `user_coupon_count` WHERE user_id = #{userId}")
    UserCouponCount selectByUserId(@Param("userId") Long userId);

    // 初始化用户优惠券统计
    @Insert("INSERT INTO `user_coupon_count`(user_id, total_count, seckill_count, normal_count, expired_count, version) " +
            "VALUES(#{count.userId}, #{count.totalCount}, #{count.seckillCount}, #{count.normalCount}, #{count.expiredCount}, 0)")
    int insert(@Param("count") UserCouponCount count);

    // 更新用户优惠券统计
    @Update("UPDATE `user_coupon_count` SET total_count = #{totalCount}, seckill_count = #{seckillCount}, " +
            "normal_count = #{normalCount}, expired_count = #{expiredCount}, update_time = NOW(), version = version + 1 " +
            "WHERE user_id = #{userId} AND version = #{version}") // 增加乐观锁控制
    int update(@Param("userId") Long userId,
               @Param("totalCount") int totalCount,
               @Param("seckillCount") int seckillCount,
               @Param("normalCount") int normalCount,
               @Param("expiredCount") int expiredCount,
               @Param("version") int version); // 传入当前版本号，避免并发问题

    // 新增或更新用户优惠券统计（使用ON DUPLICATE KEY UPDATE）
    @Insert("INSERT INTO `user_coupon_count`(user_id, total_count, seckill_count, normal_count, expired_count, version) " +
            "VALUES(#{count.userId}, #{count.totalCount}, #{count.seckillCount}, #{count.normalCount}, #{count.expiredCount}, 0) " +
            "ON DUPLICATE KEY UPDATE " +
            "total_count = total_count + #{count.totalCount}, " +
            "seckill_count = seckill_count + #{count.seckillCount}, " +
            "normal_count = normal_count + #{count.normalCount}, " +
            "expired_count = expired_count + #{count.expiredCount}")
    int insertOrUpdate(@Param("count") UserCouponCount count);
}
