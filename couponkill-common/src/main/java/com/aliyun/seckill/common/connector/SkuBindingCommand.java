package com.aliyun.seckill.common.connector;

import com.fasterxml.jackson.annotation.JsonSetter;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * 外部 SKU ↔ 本地券绑定命令。
 * couponId 支持 JSON number 或 string，避免前端大整数精度丢失。
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class SkuBindingCommand {
    private PlatformType platform;
    private String externalSkuId;
    private Long couponId;
    /** 默认 true */
    private Boolean syncEnabled;

    @JsonSetter("couponId")
    public void setCouponIdFlexible(Object raw) {
        if (raw == null) {
            this.couponId = null;
            return;
        }
        if (raw instanceof Number n) {
            this.couponId = n.longValue();
            return;
        }
        String s = String.valueOf(raw).trim();
        if (s.isEmpty()) {
            this.couponId = null;
            return;
        }
        this.couponId = Long.parseLong(s);
    }
}
