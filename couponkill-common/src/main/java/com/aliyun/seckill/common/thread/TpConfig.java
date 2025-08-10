package com.aliyun.seckill.common.thread;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.scheduling.concurrent.ThreadPoolTaskExecutor;

import java.util.concurrent.Executor;
import java.util.concurrent.ThreadPoolExecutor;

//线程池配置
@Configuration
public class TpConfig {
    // 核心线程池大小（CPU核心数*2）
    private int corePoolSize = Runtime.getRuntime().availableProcessors() * 2;
    // 最大可创建的线程数
    private int maxPoolSize = corePoolSize * 2;
    // 队列最大长度（根据内存调整）
    private int queueCapacity = 5000;
    // 线程池维护线程所允许的空闲时间
    private int keepAliveSeconds = 60;

    @Bean("asyncExecutor")
    public Executor asyncExecutor() {
        ThreadPoolTaskExecutor executor = new ThreadPoolTaskExecutor();
        executor.setCorePoolSize(corePoolSize);
        executor.setMaxPoolSize(maxPoolSize);
        executor.setQueueCapacity(queueCapacity);
        executor.setKeepAliveSeconds(keepAliveSeconds);
        executor.setAllowCoreThreadTimeOut(true); // 允许核心线程超时
        // 拒绝策略改为丢弃最老请求
        executor.setRejectedExecutionHandler(new ThreadPoolExecutor.DiscardOldestPolicy());
        executor.initialize();
        return executor;
    }
}
