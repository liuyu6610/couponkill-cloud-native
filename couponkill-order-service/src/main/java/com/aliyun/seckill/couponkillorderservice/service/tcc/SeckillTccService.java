package com.aliyun.seckill.couponkillorderservice.service.tcc;

import io.seata.rm.tcc.api.BusinessActionContext;
import io.seata.rm.tcc.api.BusinessActionContextParameter;
import io.seata.rm.tcc.api.LocalTCC;
import io.seata.rm.tcc.api.TwoPhaseBusinessAction;

@LocalTCC
public interface SeckillTccService {

    /**
     * 一阶段准备：锁定库存并创建订单
     */
    @TwoPhaseBusinessAction(name = "seckillTccAction", commitMethod = "commit", rollbackMethod = "rollback")
    boolean prepareSeckill(
            BusinessActionContext context,
            @BusinessActionContextParameter(paramName = "userId") Long userId,
            @BusinessActionContextParameter(paramName = "couponId") Long couponId);

    /**
     * 二阶段提交：确认订单
     */
    boolean commit(BusinessActionContext context);

    /**
     * 二阶段回滚：释放库存并取消订单
     */
    boolean rollback(BusinessActionContext context);
}