-- 修改 enter_seckill.lua
-- KEYS[1]=stock:{couponId}  KEYS[2]=cd:{couponId}:{userId}  KEYS[3]=deduct:{requestId}
-- ARGV[1]=cooldownSeconds   ARGV[2]=deductTtlSeconds
-- 检查是否已经处理过该请求
if redis.call("EXISTS", KEYS[3]) == 1 then return -3 end
-- 检查用户是否在冷却期
if redis.call("EXISTS", KEYS[2]) == 1 then return -2 end
-- 使用乐观锁扣减库存
local stock_key = KEYS[1]
local current_stock = tonumber(redis.call("GET", stock_key) or "0")
if current_stock <= 0 then return 0 end
-- 原子性扣减库存
local new_stock = redis.call("DECR", stock_key)
if new_stock < 0 then
    -- 如果扣减后库存为负，回滚并返回失败
    redis.call("INCR", stock_key)
    return 0
end
-- 设置冷却期和扣减标记
redis.call("SETEX", KEYS[2], ARGV[1], "1")
redis.call("SETEX", KEYS[3], ARGV[2], "1")
return 1
