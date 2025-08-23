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
    Coupon selectById(@Param("id") Long id);
    int insertCoupon(Coupon coupon);
    int batchGrantCoupons(@Param("userIds") List<Long> userIds);

    // 修改 updateStock 方法，添加版本号参数
    int updateStock(@Param("couponId") Long couponId,
                   @Param("change") int change,
                   @Param("version") int version);

    // 修改 updateRemainingStock 方法，添加版本号参数
    int updateRemainingStock(@Param("couponId") Long couponId,
                            @Param("remainingStock") Integer remainingStock,
                            @Param("updateTime") LocalDateTime updateTime,
                            @Param("version") int version);
}
