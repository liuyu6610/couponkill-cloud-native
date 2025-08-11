package com.aliyun.seckill.common.pojo;

import lombok.*;

@Getter @Setter @Builder @NoArgsConstructor @AllArgsConstructor
public class SeckillOrderCommand {
    private String requestId;
    private String couponId;
    private String userId;
    private long ts;
}
