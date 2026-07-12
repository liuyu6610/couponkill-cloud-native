package com.aliyun.seckill.couponkillorderservice.support;

import com.aliyun.seckill.common.pojo.Coupon;

import java.util.concurrent.Callable;
import java.util.concurrent.StructuredTaskScope;
import java.util.function.Supplier;

/**
 * JDK 25 Structured Concurrency（JEP 505 preview）封装：消费侧只读准备。
 * 编译与运行均需 --enable-preview；由配置 couponkill.seckill.structured-concurrency 开关。
 */
public final class StructuredFulfillSupport {

    private StructuredFulfillSupport() {
    }

    public static Coupon loadCoupon(Supplier<Coupon> loader) throws Exception {
        try (var scope = StructuredTaskScope.open()) {
            StructuredTaskScope.Subtask<Coupon> couponTask = scope.fork((Callable<Coupon>) loader::get);
            scope.join();
            return switch (couponTask.state()) {
                case SUCCESS -> couponTask.get();
                case FAILED -> throw new IllegalStateException("load coupon failed", couponTask.exception());
                case UNAVAILABLE -> throw new IllegalStateException("load coupon unavailable");
            };
        }
    }
}
