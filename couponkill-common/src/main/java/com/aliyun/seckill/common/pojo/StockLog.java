package com.aliyun.seckill.common.pojo;

import lombok.Data;

import java.io.Serializable;
import java.time.LocalDateTime;

@Data
public class StockLog implements Serializable {
    private static final long serialVersionUID = 1L;

    private Long id;
    private Long couponId;
    private Long orderId;
    private Long activityId;
    private Integer quantity;
    private Integer operateType;
    private Long operateId;
    private Integer stockAfter;
    private String remark;
    private LocalDateTime createTime;
}
