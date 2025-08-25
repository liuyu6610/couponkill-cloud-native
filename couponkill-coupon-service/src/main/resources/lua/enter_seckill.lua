-- 使用原子操作减少网络往返
local stock_key = KEYS[1]
local cooldown_key = KEYS[2]
local deduct_key = KEYS[3]

-- 检查是否已处理
if redis.call("EXISTS", deduct_key) == 1 then
    return -3
end

-- 检查冷却期
if redis.call("EXISTS", cooldown_key) == 1 then
    return -2
end

-- 原子性扣减库存
local current_stock = tonumber(redis.call("GET", stock_key) or "0")
if current_stock <= 0 then
    return 0
end

local new_stock = redis.call("DECR", stock_key)
if new_stock < 0 then
    redis.call("INCR", stock_key)
    return 0
end

-- 设置冷却期和扣减标记
redis.call("SETEX", cooldown_key, ARGV[1], "1")
redis.call("SETEX", deduct_key, ARGV[2], "1")
return 1
