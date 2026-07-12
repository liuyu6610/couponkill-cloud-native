package com.aliyun.seckill.couponkillconnectorservice.mapper;

import com.aliyun.seckill.common.connector.PlatformType;
import com.aliyun.seckill.couponkillconnectorservice.domain.PlatformSkuBinding;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.util.List;

@Mapper
public interface PlatformSkuBindingMapper {

    int insert(PlatformSkuBinding binding);

    int updateById(PlatformSkuBinding binding);

    int updateSyncResult(PlatformSkuBinding binding);

    PlatformSkuBinding selectById(@Param("id") Long id);

    PlatformSkuBinding selectByPlatformSku(@Param("platform") PlatformType platform,
                                           @Param("externalSkuId") String externalSkuId);

    PlatformSkuBinding selectByCouponId(@Param("couponId") Long couponId);

    List<PlatformSkuBinding> selectAll();

    List<PlatformSkuBinding> selectSyncEnabled();
}
