// Package cache 是 coupon 服务的 Redis 实现，满足 service.Cache：维护秒杀预扣计数器
// ck:stock:{couponId}（design §8，order 服务只通过 Lua 读写它）并删除券元数据缓存 ck:cpn:meta:{couponId}。
package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
)

// takeStockLua 在计数器存在且大于 0 时减一并返回新值，否则返回 -1 且不改动。
// 不使用裸 DECR：键不存在时它会凭空造出一个负数计数器，让 order 误判为「已预热」。
const takeStockLua = `
local v = redis.call('GET', KEYS[1])
if v and tonumber(v) > 0 then return redis.call('DECR', KEYS[1]) end
return -1`

// Redis 是基于 go-redis 的 service.Cache 实现，并发安全。
type Redis struct {
	rdb  redis.Cmdable
	take *redisx.Script
}

var _ service.Cache = (*Redis)(nil)

// New 创建 Redis 缓存。rdb 通常是 redisx.New 返回的 *redis.Client。
func New(rdb redis.UniversalClient) *Redis {
	return &Redis{rdb: rdb, take: redisx.NewScripts(rdb).Add("coupon-take-stock", takeStockLua)}
}

// PutStock 无条件把 ck:stock:{id} 设为 remaining（不设过期时间：它是账本，不是缓存）。
func (r *Redis) PutStock(ctx context.Context, couponID, remaining int64) error {
	if err := r.rdb.Set(ctx, keys.Stock(couponID), remaining, 0).Err(); err != nil {
		return fmt.Errorf("SET %s: %w", keys.Stock(couponID), err)
	}
	return nil
}

// PutStockIfAbsent 仅在计数器不存在时写入（SET NX），返回是否写入。
func (r *Redis) PutStockIfAbsent(ctx context.Context, couponID, remaining int64) (bool, error) {
	ok, err := r.rdb.SetNX(ctx, keys.Stock(couponID), remaining, 0).Result()
	if err != nil {
		return false, fmt.Errorf("SETNX %s: %w", keys.Stock(couponID), err)
	}
	return ok, nil
}

// DropStock 删除计数器；键不存在不是错误。
func (r *Redis) DropStock(ctx context.Context, couponID int64) error {
	if err := r.rdb.Del(ctx, keys.Stock(couponID)).Err(); err != nil {
		return fmt.Errorf("DEL %s: %w", keys.Stock(couponID), err)
	}
	return nil
}

// DropMeta 删除券元数据缓存；键不存在不是错误。
func (r *Redis) DropMeta(ctx context.Context, couponID int64) error {
	if err := r.rdb.Del(ctx, keys.CouponMeta(couponID)).Err(); err != nil {
		return fmt.Errorf("DEL %s: %w", keys.CouponMeta(couponID), err)
	}
	return nil
}

// TakeStock 在计数器存在且大于 0 时减一，否则什么也不做。
func (r *Redis) TakeStock(ctx context.Context, couponID int64) error {
	if err := r.take.Run(ctx, []string{keys.Stock(couponID)}).Err(); err != nil {
		return fmt.Errorf("take %s: %w", keys.Stock(couponID), err)
	}
	return nil
}
