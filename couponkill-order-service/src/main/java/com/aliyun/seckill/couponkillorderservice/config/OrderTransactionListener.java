package com.aliyun.seckill.couponkillorderservice.config;

import com.aliyun.seckill.common.pojo.OrderMessage;
import lombok.extern.slf4j.Slf4j;
import org.apache.rocketmq.client.producer.LocalTransactionState;
import org.apache.rocketmq.client.producer.TransactionListener;
import org.apache.rocketmq.common.message.MessageExt;
import org.springframework.stereotype.Component;

@Slf4j
@Component
public class OrderTransactionListener implements TransactionListener {

    /**
     * 执行本地事务
     * @param msg 消息
     * @param arg 附加参数
     * @return 事务状态
     */
    @Override
    public LocalTransactionState executeLocalTransaction(org.apache.rocketmq.common.message.Message msg, Object arg) {
        // 在我们的场景中，订单已经创建成功，本地事务已经提交
        // 所以直接返回COMMIT状态
        log.debug("执行本地事务，消息: {}, 参数: {}", msg, arg);
        return LocalTransactionState.COMMIT_MESSAGE;
    }

    /**
     * 检查本地事务状态
     * @param msg 消息
     * @return 事务状态
     */
    @Override
    public LocalTransactionState checkLocalTransaction(MessageExt msg) {
        // 在我们的场景中，如果能收到回查消息，说明订单已经创建成功
        // 所以直接返回COMMIT状态
        log.debug("回查本地事务状态，消息: {}", msg);
        return LocalTransactionState.COMMIT_MESSAGE;
    }
}