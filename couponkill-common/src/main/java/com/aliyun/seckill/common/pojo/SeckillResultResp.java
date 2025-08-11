package com.aliyun.seckill.common.pojo;

import lombok.*;

@Getter @Setter @Builder @NoArgsConstructor @AllArgsConstructor
public class SeckillResultResp {
    private String status;
    private String orderId;
}