package com.aliyun.seckill.couponkillcouponservice.mapper;

import com.aliyun.seckill.common.pojo.Coupon;
import org.apache.ibatis.annotations.Param;
import org.apache.ibatis.annotations.Select;
import org.apache.ibatis.annotations.Update;

import java.time.LocalDateTime;
import java.util.List;

public interface CouponMapper {
    List<Coupon> selectAvailableCoupons();

    List<Coupon> selectAll();

    Coupon selectById(Long id);

    int insertCoupon(Coupon coupon);

    void batchInsertVirtualCoupons(List<Coupon> virtualCoupons);

    List<Coupon> selectByCouponId(Long couponId);

    int updateStockByVirtualId(@Param("virtualId") String virtualId, @Param("change") int change, @Param("version") int version);

    int updateStock(@Param("couponId") Long couponId, @Param("change") int change, @Param("version") int version);

    // 新增方法：查询过期优惠券
    @Select("SELECT * FROM coupon WHERE status = 1 AND valid_days > 0 AND create_time < #{expireTime}")
    List<Coupon> selectExpiredCoupons(@Param("expireTime") LocalDateTime expireTime);

    // 新增方法：更新优惠券状态
    @Update("UPDATE coupon SET status = #{status}, update_time = NOW() WHERE id = #{couponId}")
    int updateCouponStatus(@Param("couponId") Long couponId, @Param("status") int status);
}
