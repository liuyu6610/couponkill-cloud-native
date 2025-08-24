// com/aliyun/seckill/common/pojo/Coupon.java
package com.aliyun.seckill.common.pojo;

import com.fasterxml.jackson.annotation.JsonFormat;
import lombok.Data;
import java.io.Serializable;
import java.time.LocalDateTime;

@Data
public class Coupon implements Serializable {
    private static final long serialVersionUID = 1L;

    private Long id;
    private String virtualId; // 虚拟分片ID
    private String name;
    private String description;
    private Integer type;
    private java.math.BigDecimal faceValue;
    private java.math.BigDecimal minSpend;
    private Integer validDays;
    private Integer perUserLimit;
    private Integer totalStock;
    private Integer seckillTotalStock;
    private Integer remainingStock;
    private Integer seckillRemainingStock;
    private Integer status;

    @JsonFormat(pattern = "yyyy-MM-dd HH:mm:ss")
    private LocalDateTime createTime;

    @JsonFormat(pattern = "yyyy-MM-dd HH:mm:ss")
    private LocalDateTime updateTime;

    private Integer version;
    private Long userId;
    private java.math.BigDecimal amount;

    public Coupon() {}

    // 生成虚拟ID的方法
    public static String generateVirtualId(Long couponId, int shardIndex) {
        return couponId + "_" + shardIndex;
    }

    // 从虚拟ID中提取原始couponId
    public static Long extractCouponIdFromVirtualId(String virtualId) {
        if (virtualId == null || !virtualId.contains("_")) {
            return null;
        }
        return Long.valueOf(virtualId.substring(0, virtualId.lastIndexOf("_")));
    }

    // 从虚拟ID中提取分片索引
    public static int extractShardIndexFromVirtualId(String virtualId) {
        if (virtualId == null || !virtualId.contains("_")) {
            return 0;
        }
        return Integer.parseInt(virtualId.substring(virtualId.lastIndexOf("_") + 1));
    }
}
