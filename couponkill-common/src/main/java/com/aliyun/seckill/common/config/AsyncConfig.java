package com.aliyun.seckill.common.config;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.scheduling.annotation.EnableAsync;
import org.springframework.scheduling.concurrent.ThreadPoolTaskExecutor;

import java.util.concurrent.Executor;
import java.util.concurrent.ThreadPoolExecutor;

@Configuration
@EnableAsync
public class AsyncConfig {
    @Value("${thread-pool.order.core-pool-size:20}")
    private int corePoolSize;

    @Value("${thread-pool.order.max-pool-size:40}")
    private int maxPoolSize;

    @Value("${thread-pool.order.queue-capacity:500}")
    private int queueCapacity;

    @Value("${thread-pool.order.keep-alive-seconds:60}")
    private int keepAliveSeconds;

    @Value("${thread-pool.order.thread-name-prefix:order-async-}")
    private String threadNamePrefix;

    @Value("${thread-pool.order.rejected-policy:CALLER_RUNS}")
    private String rejectedPolicy;

    @Bean("orderAsyncExecutor")
    public Executor orderAsyncExecutor() {
        ThreadPoolTaskExecutor executor = new ThreadPoolTaskExecutor();
        executor.setCorePoolSize(corePoolSize);
        executor.setMaxPoolSize(maxPoolSize);
        executor.setQueueCapacity(queueCapacity);
        executor.setKeepAliveSeconds(keepAliveSeconds);
        executor.setThreadNamePrefix(threadNamePrefix);

        // 根据配置设置拒绝策略
        switch (rejectedPolicy) {
            case "CALLER_RUNS":
                executor.setRejectedExecutionHandler(new ThreadPoolExecutor.CallerRunsPolicy());
                break;
            case "DISCARD_OLDEST":
                executor.setRejectedExecutionHandler(new ThreadPoolExecutor.DiscardOldestPolicy());
                break;
            case "DISCARD":
                executor.setRejectedExecutionHandler(new ThreadPoolExecutor.DiscardPolicy());
                break;
            default:
                executor.setRejectedExecutionHandler(new ThreadPoolExecutor.CallerRunsPolicy());
        }

        executor.initialize();
        return executor;
    }
}
