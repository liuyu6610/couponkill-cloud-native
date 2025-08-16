package com.aliyun.seckill.common.pojo;

import com.fasterxml.jackson.annotation.JsonFormat;
import lombok.Data;
import java.io.Serializable;
import java.time.LocalDateTime;

@Data
public class Coupon implements Serializable {
    private static final long serialVersionUID = 1L;

    private Long id;
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

    // 新增字段
    private Long userId;
    private java.math.BigDecimal amount;

    // 确保有无参构造函数
    public Coupon() {}
}
