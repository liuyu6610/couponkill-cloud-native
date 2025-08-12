// com.aliyun.seckill.common.pojo.SeckillActivity.java
package com.aliyun.seckill.common.pojo;

import lombok.Data;

import java.io.Serializable;
import java.time.LocalDateTime;

@Data
public class SeckillActivity implements Serializable {
    private static final long serialVersionUID = 1L;

    private Long id;

    private Long couponId;

    private LocalDateTime startTime;

    private LocalDateTime endTime;
    private Integer activityStock;
    private Integer activityPerUserLimit;

    /**
     * 状态(0-未开始,1-进行中,2-已结束)
     */
    private Integer status;

    private LocalDateTime createTime;
    private LocalDateTime updateTime;
}