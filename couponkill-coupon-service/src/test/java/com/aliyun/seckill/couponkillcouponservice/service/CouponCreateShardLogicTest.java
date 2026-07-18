package com.aliyun.seckill.couponkillcouponservice.service;

import com.aliyun.seckill.common.pojo.Coupon;
import org.junit.jupiter.api.Test;

import java.math.BigDecimal;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

/**
 * 校验多分片拆分与库存余数分配（与 CouponServiceImpl.createCoupon 策略一致）。
 */
class CouponCreateShardLogicTest {

    private static final int TOTAL_SHARDS = 32;

    @Test
    void createShard_setsShardIndexAndSplitsStock() {
        Coupon original = new Coupon();
        original.setId(991001L);
        original.setName("ut");
        original.setType(2);
        original.setFaceValue(new BigDecimal("10"));
        original.setMinSpend(BigDecimal.ZERO);
        original.setSeckillTotalStock(100);
        original.setSeckillRemainingStock(100);
        original.setTotalStock(0);
        original.setRemainingStock(0);
        original.setStatus(1);

        int seckillBase = 100 / TOTAL_SHARDS;
        int seckillRem = 100 % TOTAL_SHARDS;
        int sum = 0;
        for (int i = 0; i < TOTAL_SHARDS; i++) {
            Coupon shard = Coupon.createShard(original, i, TOTAL_SHARDS);
            int stock = seckillBase + (i < seckillRem ? 1 : 0);
            shard.setSeckillTotalStock(stock);
            shard.setSeckillRemainingStock(stock);
            assertNotNull(shard.getShardIndex());
            assertEquals(i, shard.getShardIndex().intValue());
            assertEquals(991001L, shard.getId().longValue());
            sum += shard.getSeckillRemainingStock();
        }
        assertEquals(100, sum);
    }
}
