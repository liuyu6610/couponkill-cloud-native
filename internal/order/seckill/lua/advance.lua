-- 请求状态推进（终态一旦写入不得再改）：把请求置为 PROCESSING（消费者领取）或 SUCCESS（订单已落库）。
-- KEYS[1]=ck:sk:req:{requestId} KEYS[2]=ck:sk:inflight
-- ARGV[1]=requestId ARGV[2]=目标状态(PROCESSING|SUCCESS) ARGV[3]=nowMs ARGV[4]=哈希 TTL(秒)
-- ARGV[5]=userId ARGV[6]=couponId ARGV[7]=orderId（目标为 SUCCESS 时）
-- 目标 PROCESSING：QUEUED/PROCESSING/哈希不存在 → 置 PROCESSING（并刷新 updatedAt）返回 1；已 SUCCESS 返回 -1；已 FAILED 返回 -2。
-- 目标 SUCCESS：FAILED 返回 -2（调用方必须回收迟到的订单）；已 SUCCESS 返回 0；否则置 SUCCESS、写 orderId、移出在途集合，返回 1。
local status = redis.call('HGET', KEYS[1], 'status')
if ARGV[2] == 'PROCESSING' then
    if status == 'SUCCESS' then
        return -1
    end
    if status == 'FAILED' then
        return -2
    end
    redis.call('HSET', KEYS[1], 'status', 'PROCESSING', 'userId', ARGV[5], 'couponId', ARGV[6], 'updatedAt', ARGV[3])
    redis.call('EXPIRE', KEYS[1], ARGV[4])
    return 1
end

if status == 'FAILED' then
    return -2
end
if status == 'SUCCESS' then
    redis.call('ZREM', KEYS[2], ARGV[1])
    return 0
end
redis.call('HSET', KEYS[1], 'status', 'SUCCESS', 'userId', ARGV[5], 'couponId', ARGV[6], 'orderId', ARGV[7], 'updatedAt', ARGV[3])
redis.call('EXPIRE', KEYS[1], ARGV[4])
redis.call('ZREM', KEYS[2], ARGV[1])
return 1
