package com.aliyun.seckill.couponkillconnectorservice.domain;

import com.aliyun.seckill.common.connector.PlatformType;
import com.fasterxml.jackson.annotation.JsonSetter;
import lombok.Data;

import java.math.BigDecimal;

@Data
public class CouponPriceMapCommand {
    private Long couponId;
    private PlatformType platform;
    private String externalSkuId;
    private String title;
    /** 手工价；stub 平台建议必填，否则 probe 会失败 */
    private BigDecimal manualPrice;
    private String currency;
    private Boolean enabled;

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
