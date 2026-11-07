-- 秒杀入口：原子完成「已占位? → 冷却? → 键存在? → 库存>0? → DECR → 占位 → 请求哈希(QUEUED) → 在途集合」。
-- KEYS[1]=ck:stock:{couponId}          秒杀预扣计数器（coupon 服务预热）
-- KEYS[2]=ck:sk:user:{couponId}:{uid}  用户占位标记，值=requestId
-- KEYS[3]=ck:sk:cool:{couponId}:{uid}  冷却标记
-- KEYS[4]=ck:sk:req:{requestId}        请求状态哈希
-- KEYS[5]=ck:sk:inflight               在途请求有序集合，score=入队毫秒时间戳
-- ARGV[1]=requestId ARGV[2]=userId ARGV[3]=couponId ARGV[4]=nowMs
-- ARGV[5]=占位标记 TTL(秒) ARGV[6]=请求哈希 TTL(秒) ARGV[7]=冷却秒数(<=0 不启用)
-- 返回：1 成功；0 售罄；-2 冷却中；-3 已占位（重复）；-4 库存键不存在（未预热）
if redis.call('EXISTS', KEYS[2]) == 1 then
    return -3
end
local cooldown = tonumber(ARGV[7]) or 0
if cooldown > 0 and redis.call('EXISTS', KEYS[3]) == 1 then
    return -2
end
if redis.call('EXISTS', KEYS[1]) == 0 then
    return -4
end
local stock = tonumber(redis.call('GET', KEYS[1]))
if stock == nil or stock <= 0 then
    return 0
end
redis.call('DECR', KEYS[1])
redis.call('SET', KEYS[2], ARGV[1], 'EX', ARGV[5])
if cooldown > 0 then
    redis.call('SET', KEYS[3], '1', 'EX', ARGV[7])
end
redis.call('HSET', KEYS[4], 'status', 'QUEUED', 'userId', ARGV[2], 'couponId', ARGV[3], 'updatedAt', ARGV[4])
redis.call('EXPIRE', KEYS[4], ARGV[6])
redis.call('ZADD', KEYS[5], ARGV[4], ARGV[1])
return 1
