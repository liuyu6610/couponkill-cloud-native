// com.aliyun.seckill.common.pojo.Coupon.java
package com.aliyun.seckill.common.pojo;

import lombok.Data;

import java.io.Serializable;
import java.math.BigDecimal;
import java.time.LocalDateTime;

@Data
public class Coupon implements Serializable {
    private static final long serialVersionUID = 1L;
    private Long id;

    private String name;

    private String description;

    /**
     * 类型(1-常驻,2-秒抢)
     */
    private Integer type;

    private BigDecimal faceValue;

    private BigDecimal minSpend;

    private Integer validDays;

    private Integer perUserLimit;
    private Integer totalStock;
    private Integer seckillTotalStock;
    private Integer remainingStock;
    private Integer seckillRemainingStock;
    private Integer status;
    private LocalDateTime createTime;
    private LocalDateTime updateTime;
    private Long userId;
    private BigDecimal amount;
}