package seckill

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
)

// loadTimeout 是一次回源读券的整体时限（含客户端自己的重试）。
const loadTimeout = 5 * time.Second

// CouponMeta 读取券元数据：先查 Redis 缓存 ck:cpn:meta:{id}，未命中回源 coupon 内部 API 并回填缓存（TTL keys.CouponMetaTTL）。
// 并发的未命中对同一张券只会回源一次，避免开售瞬间的缓存击穿。并发安全。
type CouponMeta struct {
	rdb    redis.Cmdable
	getter internalapi.CouponGetter

	mu     sync.Mutex
	flight map[int64]*metaCall
}

type metaCall struct {
	done chan struct{}
	info internalapi.CouponInfo
	err  error
}

// NewCouponMeta 创建券元数据读取器。
func NewCouponMeta(rdb redis.Cmdable, getter internalapi.CouponGetter) *CouponMeta {
	return &CouponMeta{rdb: rdb, getter: getter, flight: map[int64]*metaCall{}}
}

// Get 返回券详情。缓存内容必须是 internalapi.CouponInfo 的 JSON 且 id 与请求一致，否则视为未命中并被覆盖。
// 回源错误原样返回（券不存在为 404/CouponNotFound，下游故障满足 internalapi.IsTransient）。
func (m *CouponMeta) Get(ctx context.Context, couponID int64) (internalapi.CouponInfo, error) {
	log := logx.From(ctx)
	info, hit, err := redisx.GetJSON[internalapi.CouponInfo](ctx, m.rdb, keys.CouponMeta(couponID))
	switch {
	case err != nil:
		log.Warn("coupon meta cache unreadable, falling back to coupon service", "coupon_id", couponID, "err", err)
	case hit && info.ID == couponID:
		return info, nil
	case hit:
		log.Warn("coupon meta cache holds a different coupon, ignoring", "coupon_id", couponID, "cached_id", info.ID)
	}

	m.mu.Lock()
	if c, ok := m.flight[couponID]; ok {
		m.mu.Unlock()
		select {
		case <-c.done:
			return c.info, c.err
		case <-ctx.Done():
			return internalapi.CouponInfo{}, fmt.Errorf("wait for coupon meta: %w", ctx.Err())
		}
	}
	c := &metaCall{done: make(chan struct{})}
	m.flight[couponID] = c
	m.mu.Unlock()

	// 回源与发起者的 ctx 解耦：发起者提前断开不应让等在同一次回源上的其他请求一起失败。
	loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), loadTimeout)
	c.info, c.err = m.load(loadCtx, couponID)
	cancel()
	m.mu.Lock()
	delete(m.flight, couponID)
	m.mu.Unlock()
	close(c.done)
	return c.info, c.err
}

func (m *CouponMeta) load(ctx context.Context, couponID int64) (internalapi.CouponInfo, error) {
	info, err := m.getter.GetCoupon(ctx, couponID)
	if err != nil {
		return internalapi.CouponInfo{}, err
	}
	if err := redisx.SetJSON(ctx, m.rdb, keys.CouponMeta(couponID), info, keys.CouponMetaTTL); err != nil {
		logx.From(ctx).Warn("coupon meta cache fill failed", "coupon_id", couponID, "err", err)
	}
	return info, nil
}
