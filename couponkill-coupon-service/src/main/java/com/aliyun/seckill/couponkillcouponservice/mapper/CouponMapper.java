package com.aliyun.seckill.couponkillcouponservice.mapper;

import com.aliyun.seckill.common.pojo.Coupon;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.time.LocalDateTime;
import java.util.List;

@Mapper
public interface CouponMapper {
    List<Coupon> selectAvailableCoupons();

    List<Coupon> selectAll();

    Coupon selectById(Long id);

    int insertCoupon(Coupon coupon);

    // 查询指定优惠券的所有分片
    List<Coupon> selectByCouponId(@Param("couponId") Long couponId);

    // 查询所有主分片用于缓存预热
    List<Coupon> selectMainShardsForCache();

    // 根据分片索引更新库存
    int updateStockByShardIndex(@Param("couponId") Long couponId, 
                               @Param("shardIndex") Integer shardIndex, 
                               @Param("change") int change, 
                               @Param("version") int version);

    int updateStock(@Param("couponId") Long couponId, @Param("change") int change, @Param("version") int version);

    // 查询过期优惠券
    List<Coupon> selectExpiredCoupons(@Param("expireTime") LocalDateTime expireTime);

    // 更新优惠券状态
    int updateCouponStatus(@Param("couponId") Long couponId, @Param("status") int status);
    
    // 根据ID删除优惠券
    int deleteCouponById(@Param("id") Long id);
}
