-- 秒杀热路径：Redis 原子预扣
-- KEYS[1]=coupon:stock:{couponId}
-- KEYS[2]=seckill:cooldown:{userId}:{couponId}
-- KEYS[3]=seckill:deduct:{userId}:{couponId}   -- 用户维度幂等占位，value=requestId
-- ARGV[1]=cooldown TTL 秒（<=0 表示不启用冷却）
-- ARGV[2]=deduct TTL 秒
-- ARGV[3]=requestId
--
-- 返回：1 新扣成功；0 无库存；-2 冷却中；-3 已扣过（幂等）；-4 库存 key 未预热

local stock_key = KEYS[1]
local cooldown_key = KEYS[2]
local deduct_key = KEYS[3]
local cooldown_ttl = tonumber(ARGV[1]) or 0
local deduct_ttl = tonumber(ARGV[2]) or 300
local req_id = ARGV[3]

local existing = redis.call("GET", deduct_key)
if existing then
    return -3
end

if cooldown_ttl > 0 and redis.call("EXISTS", cooldown_key) == 1 then
    return -2
end

if redis.call("EXISTS", stock_key) == 0 then
    return -4
end

local current_stock = tonumber(redis.call("GET", stock_key) or "0")
if current_stock <= 0 then
    return 0
end

local new_stock = redis.call("DECR", stock_key)
if new_stock < 0 then
    redis.call("INCR", stock_key)
    return 0
end

if cooldown_ttl > 0 then
    redis.call("SETEX", cooldown_key, cooldown_ttl, "1")
end
if deduct_ttl > 0 then
    redis.call("SETEX", deduct_key, deduct_ttl, req_id)
else
    redis.call("SET", deduct_key, req_id)
end
return 1
