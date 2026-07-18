package com.aliyun.seckill.couponkillcouponservice.service.Impl;

import com.aliyun.seckill.common.pojo.Coupon;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;

class CouponShardMergeTest {

    @Test
    void mergeCouponShards_sumsStockAcrossShards() {
        Coupon s0 = new Coupon();
        s0.setId(1001L);
        s0.setName("demo");
        s0.setType(2);
        s0.setShardIndex(0);
        s0.setSeckillTotalStock(1);
        s0.setSeckillRemainingStock(1);

        Coupon s1 = new Coupon();
        s1.setId(1001L);
        s1.setName("demo");
        s1.setType(2);
        s1.setShardIndex(1);
        s1.setSeckillTotalStock(1);
        s1.setSeckillRemainingStock(1);

        Coupon merged = CouponServiceImpl.mergeCouponShards(List.of(s1, s0));
        assertEquals(1001L, merged.getId().longValue());
        assertEquals(2, merged.getSeckillTotalStock().intValue());
        assertEquals(2, merged.getSeckillRemainingStock().intValue());
        assertNull(merged.getShardIndex());
        // 元数据取最小 shard_index
        assertEquals("demo", merged.getName());
    }
}
