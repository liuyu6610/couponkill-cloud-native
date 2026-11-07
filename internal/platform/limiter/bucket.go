// Package limiter 提供网关内置的限流与熔断原语（替代 Sentinel）：按 key 的令牌桶与三态熔断器。
// 两者都支持注入时钟，且都不启动后台 goroutine，因此没有泄漏与关闭顺序问题。
package limiter

import (
	"math"
	"sync"
	"time"
)

// BucketConfig 配置按 key 的令牌桶。
type BucketConfig struct {
	// Rate 每秒补充的令牌数（<0 视为 0，即不再补充）。
	Rate float64
	// Burst 桶容量，也是允许的瞬时突发（<1 视为 1）。
	Burst int
	// IdleTTL 空闲多久后回收 key，默认 10 分钟。实际回收时间不会早于“补满所需时间”。
	IdleTTL time.Duration
	// Now 注入时钟，默认 time.Now。
	Now func() time.Time
}

// Decision 是一次 Take 的结果。
type Decision struct {
	// Allowed 是否放行。
	Allowed bool
	// RetryAfter 被拒绝时，至少再等多久才会有一个令牌；Rate 为 0 时为 0（表示不会恢复）。
	RetryAfter time.Duration
}

type bucket struct {
	tokens float64
	last   time.Time
}

// Buckets 是按 key 独立的令牌桶集合，并发安全。
type Buckets struct {
	rate    float64
	burst   float64
	idleTTL time.Duration
	now     func() time.Time

	mu        sync.Mutex
	m         map[string]*bucket
	lastSweep time.Time
}

// NewBuckets 创建令牌桶集合。非法配置会被规整（见 BucketConfig 字段说明），不会 panic。
func NewBuckets(cfg BucketConfig) *Buckets {
	if cfg.Rate < 0 {
		cfg.Rate = 0
	}
	if cfg.Burst < 1 {
		cfg.Burst = 1
	}
	if cfg.IdleTTL <= 0 {
		cfg.IdleTTL = 10 * time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	b := &Buckets{rate: cfg.Rate, burst: float64(cfg.Burst), idleTTL: cfg.IdleTTL, now: cfg.Now, m: map[string]*bucket{}}
	b.lastSweep = b.now()
	return b
}

// Allow 尝试取 1 个令牌。
func (b *Buckets) Allow(key string) bool { return b.Take(key).Allowed }

// AllowN 尝试一次取 n 个令牌，全有或全无；n 超过 Burst 永远失败。
func (b *Buckets) AllowN(key string, n int) bool { return b.take(key, n).Allowed }

// Take 尝试取 1 个令牌并给出被拒绝时的等待时间（用于 Retry-After）。
func (b *Buckets) Take(key string) Decision { return b.take(key, 1) }

func (b *Buckets) take(key string, n int) Decision {
	need := float64(n)
	now := b.now()

	b.mu.Lock()
	defer b.mu.Unlock()
	if now.Sub(b.lastSweep) >= b.idleTTL/2 {
		b.sweepLocked(now)
	}
	bk, ok := b.m[key]
	if !ok {
		bk = &bucket{tokens: b.burst, last: now}
		b.m[key] = bk
	}
	b.refill(bk, now)
	if need > b.burst {
		return Decision{}
	}
	if bk.tokens >= need {
		bk.tokens -= need
		return Decision{Allowed: true}
	}
	return Decision{RetryAfter: b.waitFor(need - bk.tokens)}
}

// refill 按流逝时间补充令牌（上限为 burst）。时钟回拨时不补也不扣。
func (b *Buckets) refill(bk *bucket, now time.Time) {
	if elapsed := now.Sub(bk.last); elapsed > 0 {
		bk.tokens = math.Min(b.burst, bk.tokens+elapsed.Seconds()*b.rate)
		bk.last = now
	}
}

// waitFor 返回补充 missing 个令牌所需时间（向上取整到纳秒）。
func (b *Buckets) waitFor(missing float64) time.Duration {
	if b.rate == 0 {
		return 0
	}
	return time.Duration(math.Ceil(missing / b.rate * float64(time.Second)))
}

// Len 返回当前跟踪的 key 数量。
func (b *Buckets) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.m)
}

// Sweep 立即回收空闲 key，返回回收数量。Take 也会按 IdleTTL/2 的节奏顺带调用。
func (b *Buckets) Sweep() int {
	now := b.now()
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sweepLocked(now)
}

func (b *Buckets) sweepLocked(now time.Time) int {
	b.lastSweep = now
	keep := b.idleTTL
	if b.rate > 0 { // 不得早于补满：否则被限流的 key 会被偷偷重置
		if full := time.Duration(b.burst / b.rate * float64(time.Second)); full > keep {
			keep = full
		}
	}
	removed := 0
	for k, bk := range b.m {
		if now.Sub(bk.last) >= keep {
			delete(b.m, k)
			removed++
		}
	}
	return removed
}
