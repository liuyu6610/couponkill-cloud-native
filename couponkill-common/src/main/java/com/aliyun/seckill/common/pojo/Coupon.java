package com.aliyun.seckill.common.pojo;

import com.fasterxml.jackson.annotation.JsonFormat;
import lombok.Data;
import java.io.Serializable;
import java.math.BigDecimal;
import java.time.LocalDateTime;
import java.util.concurrent.atomic.AtomicLong;

@Data
public class Coupon implements Serializable {
    private static final long serialVersionUID = 1L;

    // ID生成器，用于生成唯一的优惠券ID
    private static final AtomicLong ID_GENERATOR = new AtomicLong(System.currentTimeMillis());

    private Long id;
    private String name;
    private String description;
    private Integer type; // 1-常驻, 2-秒抢
    private BigDecimal faceValue;
    private BigDecimal minSpend;
    private Integer validDays = 15;
    private Integer perUserLimit = 1;
    private Integer totalStock;
    private Integer seckillTotalStock = 0;
    private Integer remainingStock = 0;
    private Integer seckillRemainingStock = 0;
    private Integer status = 0; // 0-无效, 1-有效
    @JsonFormat(pattern = "yyyy-MM-dd HH:mm:ss", timezone = "GMT+8")
    private LocalDateTime createTime;
    @JsonFormat(pattern = "yyyy-MM-dd HH:mm:ss", timezone = "GMT+8")
    private LocalDateTime updateTime;
    private Integer version = 0;
    private Integer shardIndex; // 用于标识这是第几个分片

    // 生成新的优惠券ID
    public static Long generateId() {
        return ID_GENERATOR.incrementAndGet();
    }

    // 创建分片优惠券
    public static Coupon createShard(Coupon original, int shardIndex, int totalShards) {
        Coupon shard = new Coupon();
        shard.setId(original.getId());
        shard.setName(original.getName());
        shard.setDescription(original.getDescription());
        shard.setType(original.getType());
        shard.setFaceValue(original.getFaceValue());
        shard.setMinSpend(original.getMinSpend());
        shard.setValidDays(original.getValidDays());
        shard.setPerUserLimit(original.getPerUserLimit());
        
        // 添加空值检查，防止NullPointerException
        if (original.getTotalStock() != null) {
            shard.setTotalStock(original.getTotalStock() / totalShards);
        }
        if (original.getSeckillTotalStock() != null) {
            shard.setSeckillTotalStock(original.getSeckillTotalStock() / totalShards);
        }
        if (original.getRemainingStock() != null) {
            shard.setRemainingStock(original.getRemainingStock() / totalShards);
        }
        if (original.getSeckillRemainingStock() != null) {
            shard.setSeckillRemainingStock(original.getSeckillRemainingStock() / totalShards);
        }
        
        shard.setStatus(original.getStatus());
        shard.setCreateTime(original.getCreateTime());
        shard.setUpdateTime(original.getUpdateTime());
        shard.setVersion(0);
        shard.setShardIndex(shardIndex);
        return shard;
    }
}