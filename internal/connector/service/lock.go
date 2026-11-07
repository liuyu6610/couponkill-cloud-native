package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
)

// syncLockName 是库存同步分布式锁的名字，对应 Redis 键 ck:lock:connector-sync。
// 定时任务、手工同步、多个实例共用这一把锁，任一时刻只有一个执行者向 coupon 推送绝对库存。
const syncLockName = "connector-sync"

// guardTimeout 是单次续租 Redis 调用的超时；超时按丢锁处理（宁可少同步一轮，也不冒并发写的风险）。
const guardTimeout = 2 * time.Second

// errLeaseLost 表示租约已丢失（过期、被他人接管，或 Redis 不可用导致无法确认）。
var errLeaseLost = errors.New("service: sync lease lost")

// Lease 是一次已获得的同步锁租约。
type Lease interface {
	// Guard 续租到完整 TTL 并确认锁仍归自己；返回非 nil 表示已丢锁（包括无法确认），调用方必须停止写入。
	Guard(ctx context.Context) error
	// Release 释放锁（只删自己的令牌，丢失时静默）。
	Release(ctx context.Context)
}

// Locker 获取同步锁租约。接口在使用方（SyncService）定义，测试可替换。
type Locker interface {
	// TryAcquire 非阻塞加锁：被占用返回 domain.ErrSyncBusy，其余失败（如 Redis 不可用）返回普通错误。
	TryAcquire(ctx context.Context) (Lease, error)
}

// RedisLocker 用 redisx.Locker 实现 Locker。
type RedisLocker struct {
	locker *redisx.Locker
	ttl    time.Duration
}

// NewRedisLocker 创建基于 Redis 的同步锁，ttl 为租约时长（每次 Guard 都会续满）。
func NewRedisLocker(rdb redis.Cmdable, ttl time.Duration) *RedisLocker {
	return &RedisLocker{locker: redisx.NewLocker(rdb), ttl: ttl}
}

// TryAcquire 实现 Locker。
func (l *RedisLocker) TryAcquire(ctx context.Context) (Lease, error) {
	lock, err := l.locker.Acquire(ctx, syncLockName, l.ttl)
	switch {
	case errors.Is(err, redisx.ErrNotAcquired):
		return nil, domain.ErrSyncBusy
	case err != nil:
		return nil, fmt.Errorf("acquire sync lock: %w", err)
	}
	return &redisLease{lock: lock, ttl: l.ttl}, nil
}

type redisLease struct {
	lock *redisx.Lock
	ttl  time.Duration
}

// Guard 实现 Lease。
func (r *redisLease) Guard(ctx context.Context) error {
	gctx, cancel := context.WithTimeout(ctx, guardTimeout)
	defer cancel()
	if err := r.lock.Renew(gctx, r.ttl); err != nil {
		return fmt.Errorf("%w: %w", errLeaseLost, err)
	}
	return nil
}

// Release 实现 Lease：释放用不受调用方取消影响的上下文，保证关闭过程中也能让出锁。
func (r *redisLease) Release(ctx context.Context) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), guardTimeout)
	defer cancel()
	if err := r.lock.Release(rctx); err != nil && !errors.Is(err, redisx.ErrLockLost) {
		// 释放失败只会让锁多占一个 TTL，不影响正确性。
		logx.From(ctx).Warn("release sync lock failed; it will expire on its own", "err", err)
	}
}
