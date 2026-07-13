package com.aliyun.seckill.couponkillorderservice.support;

import com.aliyun.seckill.common.pojo.Coupon;

import java.util.concurrent.Callable;
import java.util.concurrent.StructuredTaskScope;
import java.util.function.Supplier;

/**
 * JDK 25 Structured Concurrency（JEP 505 preview）：消费侧真正可并行的只读准备。
 * 并行加载券元数据 + 用户计数快照，再串行做限领判断（不破坏扣库存→插单顺序）。
 */
public final class StructuredFulfillSupport {

    private StructuredFulfillSupport() {
    }

    public record UserCountSnapshot(Integer totalCount, Integer seckillCount) {
    }

    public record FulfillReadPrep(Coupon coupon, UserCountSnapshot counts) {
    }

    /** 单任务封装（兼容旧调用） */
    public static Coupon loadCoupon(Supplier<Coupon> loader) throws Exception {
        try (var scope = StructuredTaskScope.open()) {
            StructuredTaskScope.Subtask<Coupon> couponTask = scope.fork((Callable<Coupon>) loader::get);
            scope.join();
            return requireSuccess(couponTask, "load coupon");
        }
    }

    /**
     * 并行只读：券元数据 ∥ 用户计数；调用方再基于 coupon.type 做限领校验。
     */
    public static FulfillReadPrep prepareReads(
            Supplier<Coupon> couponLoader,
            Supplier<UserCountSnapshot> countsLoader) throws Exception {
        try (var scope = StructuredTaskScope.open()) {
            StructuredTaskScope.Subtask<Coupon> couponTask = scope.fork((Callable<Coupon>) couponLoader::get);
            StructuredTaskScope.Subtask<UserCountSnapshot> countsTask =
                    scope.fork((Callable<UserCountSnapshot>) countsLoader::get);
            scope.join();
            return new FulfillReadPrep(
                    requireSuccess(couponTask, "load coupon"),
                    requireSuccess(countsTask, "load user counts"));
        }
    }

    private static <T> T requireSuccess(StructuredTaskScope.Subtask<T> task, String label) {
        return switch (task.state()) {
            case SUCCESS -> task.get();
            case FAILED -> throw new IllegalStateException(label + " failed", task.exception());
            case UNAVAILABLE -> throw new IllegalStateException(label + " unavailable");
        };
    }
}
