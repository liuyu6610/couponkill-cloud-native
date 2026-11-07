package redisx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

var (
	// ErrNotAcquired 表示锁正被其他持有者占用。
	ErrNotAcquired = errors.New("redisx: lock is held by someone else")
	// ErrLockLost 表示本持有者已不再拥有该锁（过期或被他人获取）。
	ErrLockLost = errors.New("redisx: lock lost")
)

const lockKeyPrefix = "ck:lock:"

// 只有令牌匹配才删除 / 续期：避免过期后误删别人的锁。
const (
	releaseLua = `if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end return 0`
	renewLua   = `if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('PEXPIRE', KEYS[1], ARGV[2]) end return 0`
)

// Locker 创建分布式锁：SET NX PX 获取，令牌校验释放与续租。
//
// 不变式：任一时刻至多一个持有者令牌对应键值；持有者只能释放/续租自己的令牌。
// 注意：单 Redis 实例上的锁是“效率锁”，不是强一致的互斥——锁过期后旧持有者仍可能还在执行，
// 因此长任务应使用 Run（自动续租并在锁丢失时取消任务 ctx），且临界区写操作应自带幂等。
type Locker struct {
	rdb     redis.Cmdable
	release *Script
	renew   *Script
}

// NewLocker 创建 Locker。
func NewLocker(rdb redis.Cmdable) *Locker {
	scripts := NewScripts(rdb)
	return &Locker{rdb: rdb, release: scripts.Add("lock-release", releaseLua), renew: scripts.Add("lock-renew", renewLua)}
}

// Lock 是一把已持有的锁。
type Lock struct {
	l     *Locker
	key   string
	name  string
	token string
}

// Acquire 尝试获取名为 name 的锁（键 ck:lock:{name}），ttl 为租约时长。
// 已被占用返回 ErrNotAcquired，不阻塞。
func (l *Locker) Acquire(ctx context.Context, name string, ttl time.Duration) (*Lock, error) {
	if ttl <= 0 {
		return nil, fmt.Errorf("redisx: lock ttl must be positive, got %v", ttl)
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	token := hex.EncodeToString(b[:])
	key := lockKeyPrefix + name
	ok, err := l.rdb.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, fmt.Errorf("redisx: acquire lock %q: %w", name, err)
	}
	if !ok {
		return nil, ErrNotAcquired
	}
	return &Lock{l: l, key: key, name: name, token: token}, nil
}

// Release 释放锁。锁已过期或已被他人持有返回 ErrLockLost，且不会动他人的锁。
func (k *Lock) Release(ctx context.Context) error {
	n, err := k.l.release.Run(ctx, []string{k.key}, k.token).Int64()
	if err != nil {
		return fmt.Errorf("redisx: release lock %q: %w", k.name, err)
	}
	if n == 0 {
		return ErrLockLost
	}
	return nil
}

// Renew 把租约重置为 ttl。锁已丢失返回 ErrLockLost。
func (k *Lock) Renew(ctx context.Context, ttl time.Duration) error {
	n, err := k.l.renew.Run(ctx, []string{k.key}, k.token, ttl.Milliseconds()).Int64()
	if err != nil {
		return fmt.Errorf("redisx: renew lock %q: %w", k.name, err)
	}
	if n == 0 {
		return ErrLockLost
	}
	return nil
}

// Run 获取锁后执行 fn，期间每 ttl/3 自动续租，结束后释放（即使 ctx 已取消也会释放）。
//
//   - 锁被他人持有：返回 ErrNotAcquired，不调用 fn（定时任务据此跳过本轮）。
//   - 续租发现锁丢失，或连续 ttl 时长都续租不成功：取消传给 fn 的 ctx，Run 返回的错误满足
//     errors.Is(err, ErrLockLost)，调用方应视本轮结果为不可信。
//   - 其余情况原样返回 fn 的错误。
func (l *Locker) Run(ctx context.Context, name string, ttl time.Duration, fn func(ctx context.Context) error) error {
	lock, err := l.Acquire(ctx, name, ttl)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancelCause(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		lock.keepAlive(runCtx, ttl, cancel)
	}()

	fnErr := fn(runCtx)

	lost := errors.Is(context.Cause(runCtx), ErrLockLost)
	cancel(nil)
	wg.Wait()

	relCtx, relCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer relCancel()
	relErr := lock.Release(relCtx)
	switch {
	case errors.Is(relErr, ErrLockLost):
		lost = true
	case relErr != nil:
		// 释放失败只会让锁多占一个 ttl，不影响本轮结果，记录即可。
		logx.From(ctx).Warn("lock release failed; it will expire on its own", "lock", name, "err", relErr)
	}
	if lost {
		return errors.Join(fmt.Errorf("redisx: lock %q lost while running: %w", name, ErrLockLost), fnErr)
	}
	return fnErr
}

// keepAlive 周期性续租；锁丢失或长时间续租失败时以 ErrLockLost 取消 ctx。
func (k *Lock) keepAlive(ctx context.Context, ttl time.Duration, cancel context.CancelCauseFunc) {
	interval := max(ttl/3, time.Millisecond)
	t := time.NewTicker(interval)
	defer t.Stop()
	lastOK := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		rctx, rcancel := context.WithTimeout(ctx, max(interval, 50*time.Millisecond))
		err := k.Renew(rctx, ttl)
		rcancel()
		switch {
		case err == nil:
			lastOK = time.Now()
		case errors.Is(err, ErrLockLost):
			cancel(ErrLockLost)
			return
		case ctx.Err() != nil:
			return
		default:
			logx.From(ctx).Warn("lock renew failed", "lock", k.name, "err", err)
			if time.Since(lastOK) >= ttl { // 租约事实上已经过期
				cancel(ErrLockLost)
				return
			}
		}
	}
}
