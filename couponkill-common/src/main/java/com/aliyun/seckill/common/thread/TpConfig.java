// 文件路径: couponkill-common/src/main/java/com/aliyun/seckill/common/thread/TpConfig.java
package com.aliyun.seckill.common.thread;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.scheduling.concurrent.ThreadPoolTaskExecutor;

import java.util.concurrent.Executor;
import java.util.concurrent.ThreadPoolExecutor;

//线程池配置
@Configuration
public class TpConfig {
    @Value("${thread-pool.async.core-pool-size:16}")
    private int corePoolSize;

    @Value("${thread-pool.async.max-pool-size:32}")
    private int maxPoolSize;

    @Value("${thread-pool.async.queue-capacity:200}")
    private int queueCapacity;

    @Value("${thread-pool.async.keep-alive-seconds:60}")
    private int keepAliveSeconds;

    @Value("${thread-pool.async.rejected-policy:CALLER_RUNS}")
    private String rejectedPolicy;

    @Bean("asyncExecutor")
    public Executor asyncExecutor() {
        ThreadPoolTaskExecutor executor = new ThreadPoolTaskExecutor();
        executor.setCorePoolSize(corePoolSize);
        executor.setMaxPoolSize(maxPoolSize);
        executor.setQueueCapacity(queueCapacity);
        executor.setKeepAliveSeconds(keepAliveSeconds);
        executor.setAllowCoreThreadTimeOut(true);

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
                executor.setRejectedExecutionHandler(new ThreadPoolExecutor.AbortPolicy());
        }

        executor.initialize();
        return executor;
    }
}
