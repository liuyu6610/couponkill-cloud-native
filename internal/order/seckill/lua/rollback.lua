-- 回滚一次预扣：库存键存在才 INCR、清占位与冷却、（失败终态时）置 FAILED、移出在途集合。
-- 对同一 requestId 幂等：请求哈希里的 rolledBack 字段保证库存最多回补一次。
-- KEYS 与 enter.lua 相同：1 stock 2 userMarker 3 cooldown 4 reqHash 5 inflight
-- ARGV[1]=requestId ARGV[2]=nowMs ARGV[3]=reason（空串=仅回补，不改状态：取消订单用）
-- ARGV[4]=keepMarker（"1" 保留占位标记：用户仍持有订单时的 REPEAT 失败） ARGV[5]=请求哈希 TTL(秒)
-- 返回：1 本次完成回补；0 此前已回补（空操作）；-1 请求已 SUCCESS 而调用方要求置 FAILED（拒绝，不改任何东西）；
--       -2 请求哈希不存在且要求置 FAILED（过期，只清在途集合）
local fail = ARGV[3] ~= ''
local status = redis.call('HGET', KEYS[4], 'status')
if status == false then
    redis.call('ZREM', KEYS[5], ARGV[1])
    if fail then
        return -2
    end
elseif redis.call('HEXISTS', KEYS[4], 'rolledBack') == 1 then
    redis.call('ZREM', KEYS[5], ARGV[1])
    return 0
elseif fail and status == 'SUCCESS' then
    return -1
end

if redis.call('EXISTS', KEYS[1]) == 1 then
    redis.call('INCR', KEYS[1])
end
if ARGV[4] ~= '1' then
    -- 失败终态只删属于本请求的占位；取消订单（仅回补）时用户已不持有订单，无条件删除
    if (not fail) or redis.call('GET', KEYS[2]) == ARGV[1] then
        redis.call('DEL', KEYS[2])
    end
end
redis.call('DEL', KEYS[3])
if fail then
    redis.call('HSET', KEYS[4], 'status', 'FAILED', 'reason', ARGV[3])
end
redis.call('HSET', KEYS[4], 'rolledBack', '1', 'updatedAt', ARGV[2])
redis.call('EXPIRE', KEYS[4], ARGV[5])
redis.call('ZREM', KEYS[5], ARGV[1])
return 1
