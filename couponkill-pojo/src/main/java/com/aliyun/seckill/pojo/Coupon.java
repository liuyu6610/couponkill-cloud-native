// com.aliyun.seckill.pojo.Coupon.java
package com.aliyun.seckill.pojo;

import com.baomidou.mybatisplus.annotation.IdType;
import com.baomidou.mybatisplus.annotation.TableId;
import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

import java.io.Serializable;
import java.math.BigDecimal;
import java.time.LocalDateTime;

@Data
@TableName("coupon")
public class Coupon implements Serializable {
    private static final long serialVersionUID = 1L;

    @TableId(type = IdType.AUTO)
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

    private Integer totalStock;

    private Integer remainingStock;

    private Integer status;

    private LocalDateTime createTime;

    private LocalDateTime updateTime;
}