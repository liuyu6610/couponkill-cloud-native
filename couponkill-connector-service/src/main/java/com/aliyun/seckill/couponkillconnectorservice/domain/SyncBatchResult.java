package com.aliyun.seckill.couponkillconnectorservice.domain;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class SyncBatchResult {
    private int syncedOk;
    private int failed;
    private int skipped;
    private boolean force;
}
