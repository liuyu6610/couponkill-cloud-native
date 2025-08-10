package com.aliyun.seckill.common.config;

import com.alibaba.csp.sentinel.slots.block.RuleConstant;
import com.alibaba.csp.sentinel.slots.block.degrade.DegradeRule;
import com.alibaba.csp.sentinel.slots.block.degrade.DegradeRuleManager;
import com.alibaba.csp.sentinel.slots.block.flow.FlowRule;
import com.alibaba.csp.sentinel.slots.block.flow.FlowRuleManager;
import jakarta.annotation.PostConstruct;
import org.springframework.context.annotation.Configuration;

import java.util.ArrayList;
import java.util.List;

@Configuration
public class SentinelConfig {
    @PostConstruct
    public void initRules() {
        // 配置秒杀接口的限流规则
        initFlowRules();
        // 配置降级规则
        initDegradeRules();
    }

    // 初始化限流规则
    private void initFlowRules() {
        List<FlowRule> rules = new ArrayList<>();

        // 秒杀接口限流规则 - QPS限制
        FlowRule seckillFlowRule = new FlowRule();
        seckillFlowRule.setResource("couponSeckill");
        seckillFlowRule.setGrade( RuleConstant.FLOW_GRADE_QPS);
        // 设置阈值，超过此阈值的请求将被转发到Go服务
        seckillFlowRule.setCount(500);
        rules.add(seckillFlowRule);

        FlowRuleManager.loadRules(rules);
    }

    // 初始化降级规则
    private void initDegradeRules() {
        List<DegradeRule> rules = new ArrayList<>();

        DegradeRule seckillDegradeRule = new DegradeRule();
        seckillDegradeRule.setResource("couponSeckill");
        // 基于异常比例降级
        seckillDegradeRule.setGrade(RuleConstant.DEGRADE_GRADE_EXCEPTION_RATIO);
        // 异常比例阈值
        seckillDegradeRule.setCount(0.5);
        // 最小请求数
        seckillDegradeRule.setMinRequestAmount(100);
        // 统计时长
        seckillDegradeRule.setStatIntervalMs(1000);
        // 降级时间窗口
        seckillDegradeRule.setTimeWindow(10);
        rules.add(seckillDegradeRule);

        DegradeRuleManager.loadRules(rules);
    }
}
