-- coupon-service/src/main/resources/lua/enter_seckill.lua
-- KEYS[1]=stock:{couponId}  KEYS[2]=cd:{couponId}:{userId}  KEYS[3]=deduct:{requestId}
-- ARGV[1]=cooldownSeconds   ARGV[2]=deductTtlSeconds
if redis.call("EXISTS", KEYS[3]) == 1 then return -3 end
if redis.call("EXISTS", KEYS[2]) == 1 then return -2 end
local s = tonumber(redis.call("GET", KEYS[1]) or "0")
if s <= 0 then return 0 end
redis.call("DECR", KEYS[1])
redis.call("SETEX", KEYS[2], ARGV[1], "1")
redis.call("SETEX", KEYS[3], ARGV[2], "1")
return 1
