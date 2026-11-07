package limiter

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrOpen 表示熔断器处于打开状态（或半开且探测名额已满），调用被直接拒绝。
var ErrOpen = errors.New("limiter: circuit breaker is open")

// State 是熔断器状态。
type State int

// 熔断器三态。
const (
	Closed State = iota
	Open
	HalfOpen
)

// String 返回状态名（closed/open/half-open）。
func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	}
	return "unknown"
}

// BreakerConfig 配置熔断器。零值字段使用默认值。
type BreakerConfig struct {
	// FailureThreshold 关闭状态下连续失败多少次后打开，默认 5。
	FailureThreshold int
	// OpenTimeout 打开多久后进入半开，默认 30 秒；也是半开状态下“泄漏的探测名额”的过期时间。
	OpenTimeout time.Duration
	// HalfOpenMax 半开状态下同时允许的探测数，默认 1。
	HalfOpenMax int
	// SuccessThreshold 半开状态下需要多少次探测成功才关闭，默认 1。
	SuccessThreshold int
	// IsFailure 判定 Execute 的返回值是否算失败；默认 err != nil 且不是 context.Canceled
	// （调用方自己放弃不代表上游故障）。
	IsFailure func(error) bool
	// OnStateChange 状态迁移回调（在持锁外同步调用），用于日志与指标。
	OnStateChange func(from, to State)
	// Now 注入时钟，默认 time.Now。
	Now func() time.Time
}

// Breaker 是并发安全的三态熔断器：closed → open → half-open → closed/open。
type Breaker struct {
	cfg BreakerConfig

	mu          sync.Mutex
	state       State
	gen         uint64 // 每次状态迁移 +1，使旧状态下发出的许可失效
	failures    int    // closed：连续失败数
	successes   int    // half-open：探测成功数
	probes      int    // half-open：在途探测数
	openedAt    time.Time
	halfOpenAt  time.Time
	pendingHook []func()
}

// NewBreaker 创建熔断器。
func NewBreaker(cfg BreakerConfig) *Breaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.OpenTimeout <= 0 {
		cfg.OpenTimeout = 30 * time.Second
	}
	if cfg.HalfOpenMax <= 0 {
		cfg.HalfOpenMax = 1
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = 1
	}
	if cfg.IsFailure == nil {
		cfg.IsFailure = func(err error) bool { return err != nil && !errors.Is(err, context.Canceled) }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Breaker{cfg: cfg}
}

// Permit 是一次被放行调用的凭据，调用结束必须且只需调用一次 Done。
type Permit struct {
	b    *Breaker
	gen  uint64
	done atomic.Bool
}

// Done 上报调用结果。重复调用与来自旧状态世代的迟到上报都会被忽略。
func (p *Permit) Done(success bool) {
	if !p.done.CompareAndSwap(false, true) {
		return
	}
	p.b.report(p.gen, success)
}

// State 返回当前状态（会先按时间把过期的 open 推进到 half-open）。
func (b *Breaker) State() State {
	b.mu.Lock()
	b.advanceLocked()
	s := b.state
	hooks := b.takeHooksLocked()
	b.mu.Unlock()
	runHooks(hooks)
	return s
}

// Allow 申请一次调用许可；被拒绝时返回 ErrOpen。
func (b *Breaker) Allow() (*Permit, error) {
	b.mu.Lock()
	b.advanceLocked()
	var (
		p   *Permit
		err error
	)
	switch b.state {
	case Open:
		err = ErrOpen
	case HalfOpen:
		if b.probes >= b.cfg.HalfOpenMax && b.cfg.Now().Sub(b.halfOpenAt) >= b.cfg.OpenTimeout {
			// 之前的探测许可泄漏了（从未 Done）：开启新一轮探测，旧许可作废。
			b.gen++
			b.probes, b.successes = 0, 0
			b.halfOpenAt = b.cfg.Now()
		}
		if b.probes >= b.cfg.HalfOpenMax {
			err = ErrOpen
			break
		}
		b.probes++
		p = &Permit{b: b, gen: b.gen}
	default:
		p = &Permit{b: b, gen: b.gen}
	}
	hooks := b.takeHooksLocked()
	b.mu.Unlock()
	runHooks(hooks)
	return p, err
}

// Execute 在熔断保护下调用 fn：被拒绝时返回 ErrOpen 且不调用 fn；
// 否则按 IsFailure 判定结果。fn panic 时记为失败后继续向上抛出。
func (b *Breaker) Execute(fn func() error) error {
	p, err := b.Allow()
	if err != nil {
		return err
	}
	finished := false
	defer func() {
		if !finished {
			p.Done(false)
		}
	}()
	err = fn()
	finished = true
	p.Done(!b.cfg.IsFailure(err))
	return err
}

// advanceLocked 处理基于时间的迁移：open 超时 → half-open。
func (b *Breaker) advanceLocked() {
	if b.state == Open && b.cfg.Now().Sub(b.openedAt) >= b.cfg.OpenTimeout {
		b.transitionLocked(HalfOpen)
	}
}

func (b *Breaker) report(gen uint64, success bool) {
	b.mu.Lock()
	if gen != b.gen {
		b.mu.Unlock()
		return
	}
	switch b.state {
	case Closed:
		if success {
			b.failures = 0
		} else if b.failures++; b.failures >= b.cfg.FailureThreshold {
			b.transitionLocked(Open)
		}
	case HalfOpen:
		b.probes--
		if !success {
			b.transitionLocked(Open)
		} else if b.successes++; b.successes >= b.cfg.SuccessThreshold {
			b.transitionLocked(Closed)
		}
	}
	hooks := b.takeHooksLocked()
	b.mu.Unlock()
	runHooks(hooks)
}

func (b *Breaker) transitionLocked(to State) {
	from := b.state
	b.state = to
	b.gen++
	b.failures, b.successes, b.probes = 0, 0, 0
	now := b.cfg.Now()
	switch to {
	case Open:
		b.openedAt = now
	case HalfOpen:
		b.halfOpenAt = now
	}
	if cb := b.cfg.OnStateChange; cb != nil {
		b.pendingHook = append(b.pendingHook, func() { cb(from, to) })
	}
}

func (b *Breaker) takeHooksLocked() []func() {
	h := b.pendingHook
	b.pendingHook = nil
	return h
}

func runHooks(hooks []func()) {
	for _, h := range hooks {
		h()
	}
}
