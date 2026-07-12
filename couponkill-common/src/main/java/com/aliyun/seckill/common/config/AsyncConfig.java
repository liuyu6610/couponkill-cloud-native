package com.aliyun.seckill.common.config;

import org.springframework.context.annotation.Configuration;
import org.springframework.scheduling.annotation.EnableAsync;

/**
 * 仅开启 @Async。执行器真源见 {@link com.aliyun.seckill.common.thread.TpConfig}（默认虚拟线程）。
 * 已移除未使用的平台线程池 seckillAsyncExecutor / orderAsyncExecutor，避免与 VT 叙事冲突。
 */
@Configuration
@EnableAsync
public class AsyncConfig {
}
