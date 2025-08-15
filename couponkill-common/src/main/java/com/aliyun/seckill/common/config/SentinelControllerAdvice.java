// 在couponkill-common/src/main/java/com/aliyun/seckill/common/config/SentinelControllerAdvice.java
package com.aliyun.seckill.common.config;

import com.alibaba.csp.sentinel.slots.block.BlockException;
import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.result.Result;
import org.springframework.web.bind.annotation.ControllerAdvice;
import org.springframework.web.bind.annotation.ExceptionHandler;

@ControllerAdvice
public class SentinelControllerAdvice {

    @ExceptionHandler(BlockException.class)
    public Result<?> handleBlockException(BlockException e) {
        return Result.fail(ResultCode.SYSTEM_BUSY);
    }
}
