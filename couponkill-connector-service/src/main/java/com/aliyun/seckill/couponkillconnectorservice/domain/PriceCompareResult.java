package com.aliyun.seckill.couponkillconnectorservice.domain;

import com.fasterxml.jackson.annotation.JsonFormat;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.ArrayList;
import java.util.List;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class PriceCompareResult {
    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Long couponId;
    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Long bindingId;
    @Builder.Default
    private List<PriceCompareItem> items = new ArrayList<>();
}
