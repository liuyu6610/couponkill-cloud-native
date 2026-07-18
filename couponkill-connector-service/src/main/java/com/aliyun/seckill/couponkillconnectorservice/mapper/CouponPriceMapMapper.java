package com.aliyun.seckill.couponkillconnectorservice.mapper;

import com.aliyun.seckill.common.connector.PlatformType;
import com.aliyun.seckill.couponkillconnectorservice.domain.CouponPriceMap;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.util.List;

@Mapper
public interface CouponPriceMapMapper {

    int insert(CouponPriceMap row);

    int updateById(CouponPriceMap row);

    int deleteById(@Param("id") Long id);

    CouponPriceMap selectById(@Param("id") Long id);

    CouponPriceMap selectByCouponPlatform(@Param("couponId") Long couponId,
                                          @Param("platform") PlatformType platform);

    List<CouponPriceMap> selectByCouponId(@Param("couponId") Long couponId);

    List<CouponPriceMap> selectEnabledByCouponId(@Param("couponId") Long couponId);
}
