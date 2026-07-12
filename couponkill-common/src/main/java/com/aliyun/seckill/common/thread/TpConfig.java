// 文件路径: couponkill-common/src/main/java/com/aliyun/seckill/common/thread/TpConfig.java
package com.aliyun.seckill.common.thread;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;
import org.springframework.scheduling.concurrent.ThreadPoolTaskExecutor;

import java.util.concurrent.Executor;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.ThreadPoolExecutor;

/**
 * 异步执行器：默认优先虚拟线程（JDK 21+ / Boot 3.2+），避免固定平台线程池把 IO 密集秒杀后置任务卡死。
 * 可通过 thread-pool.async.virtual-enabled=false 回退到传统池。
 */
@Configuration
public class TpConfig {

    @Value("${thread-pool.async.core-pool-size:32}")
    private int corePoolSize;

    @Value("${thread-pool.async.max-pool-size:128}")
    private int maxPoolSize;

    @Value("${thread-pool.async.queue-capacity:1000}")
    private int queueCapacity;

    @Value("${thread-pool.async.keep-alive-seconds:60}")
    private int keepAliveSeconds;

    @Value("${thread-pool.async.rejected-policy:CALLER_RUNS}")
    private String rejectedPolicy;

    @Bean(name = "asyncExecutor")
    @Primary
    @ConditionalOnProperty(name = "thread-pool.async.virtual-enabled", havingValue = "true", matchIfMissing = true)
    public Executor virtualAsyncExecutor() {
        ExecutorService executor = Executors.newVirtualThreadPerTaskExecutor();
        return executor;
    }

    @Bean(name = "asyncExecutor")
    @ConditionalOnProperty(name = "thread-pool.async.virtual-enabled", havingValue = "false")
    public Executor platformAsyncExecutor() {
        ThreadPoolTaskExecutor executor = new ThreadPoolTaskExecutor();
        executor.setCorePoolSize(corePoolSize);
        executor.setMaxPoolSize(maxPoolSize);
        executor.setQueueCapacity(queueCapacity);
        executor.setKeepAliveSeconds(keepAliveSeconds);
        executor.setAllowCoreThreadTimeOut(true);
        executor.setThreadNamePrefix("async-");

        switch (rejectedPolicy) {
            case "CALLER_RUNS" -> executor.setRejectedExecutionHandler(new ThreadPoolExecutor.CallerRunsPolicy());
            case "DISCARD_OLDEST" -> executor.setRejectedExecutionHandler(new ThreadPoolExecutor.DiscardOldestPolicy());
            case "DISCARD" -> executor.setRejectedExecutionHandler(new ThreadPoolExecutor.DiscardPolicy());
            default -> executor.setRejectedExecutionHandler(new ThreadPoolExecutor.AbortPolicy());
        }

        executor.initialize();
        return executor;
    }
}
