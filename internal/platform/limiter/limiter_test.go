package limiter_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *fakeClock { return &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)} }
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// ---------- 令牌桶 ----------

func TestBucketBurstThenRefill(t *testing.T) {
	clk := newClock()
	b := limiter.NewBuckets(limiter.BucketConfig{Rate: 10, Burst: 3, Now: clk.Now})
	for i := 0; i < 3; i++ {
		if !b.Allow("k") {
			t.Fatalf("request %d within burst must pass", i)
		}
	}
	if b.Allow("k") {
		t.Fatal("4th request must be limited")
	}
	clk.Advance(99 * time.Millisecond) // 0.99 个令牌：仍不足
	if b.Allow("k") {
		t.Fatal("not enough tokens yet")
	}
	clk.Advance(1 * time.Millisecond) // 恰好 1 个令牌
	if !b.Allow("k") {
		t.Fatal("one token should have refilled")
	}
	if b.Allow("k") {
		t.Fatal("only one token was refilled")
	}
}

func TestBucketRefillIsCappedAtBurst(t *testing.T) {
	clk := newClock()
	b := limiter.NewBuckets(limiter.BucketConfig{Rate: 100, Burst: 2, Now: clk.Now})
	b.Allow("k")
	clk.Advance(time.Hour)
	allowed := 0
	for i := 0; i < 10; i++ {
		if b.Allow("k") {
			allowed++
		}
	}
	if allowed != 2 {
		t.Fatalf("after a long idle only burst=2 tokens may be available, got %d", allowed)
	}
}

func TestBucketKeysAreIndependent(t *testing.T) {
	clk := newClock()
	b := limiter.NewBuckets(limiter.BucketConfig{Rate: 1, Burst: 1, Now: clk.Now})
	if !b.Allow("a") || b.Allow("a") {
		t.Fatal("key a: first passes, second limited")
	}
	if !b.Allow("b") {
		t.Fatal("key b must have its own bucket")
	}
}

func TestBucketTakeReportsRetryAfter(t *testing.T) {
	clk := newClock()
	b := limiter.NewBuckets(limiter.BucketConfig{Rate: 2, Burst: 1, Now: clk.Now})
	if d := b.Take("k"); !d.Allowed || d.RetryAfter != 0 {
		t.Fatalf("first take: %+v", d)
	}
	d := b.Take("k")
	if d.Allowed || d.RetryAfter != 500*time.Millisecond {
		t.Fatalf("second take must be limited with retry 500ms, got %+v", d)
	}
	clk.Advance(250 * time.Millisecond)
	if d := b.Take("k"); d.Allowed || d.RetryAfter != 250*time.Millisecond {
		t.Fatalf("after 250ms retry should be 250ms, got %+v", d)
	}
}

func TestBucketAllowNIsAllOrNothing(t *testing.T) {
	clk := newClock()
	b := limiter.NewBuckets(limiter.BucketConfig{Rate: 1, Burst: 5, Now: clk.Now})
	if b.AllowN("k", 6) {
		t.Fatal("asking for more than burst can never pass")
	}
	if !b.AllowN("k", 5) {
		t.Fatal("a failed AllowN must not consume tokens")
	}
	if b.AllowN("k", 1) {
		t.Fatal("bucket should be empty")
	}
}

func TestBucketIdleKeysAreSwept(t *testing.T) {
	clk := newClock()
	b := limiter.NewBuckets(limiter.BucketConfig{Rate: 10, Burst: 5, IdleTTL: time.Minute, Now: clk.Now})
	for i := 0; i < 100; i++ {
		b.Allow(fmt.Sprintf("ip-%d", i))
	}
	if b.Len() != 100 {
		t.Fatalf("Len = %d, want 100", b.Len())
	}
	clk.Advance(30 * time.Second)
	if n := b.Sweep(); n != 0 || b.Len() != 100 {
		t.Fatalf("keys younger than IdleTTL must be kept (swept %d, len %d)", n, b.Len())
	}
	clk.Advance(31 * time.Second)
	if n := b.Sweep(); n != 100 || b.Len() != 0 {
		t.Fatalf("idle keys must be removed (swept %d, len %d)", n, b.Len())
	}
	// 清理后的 key 重新获得满桶，等价于它本来就已回满。
	if !b.AllowN("ip-1", 5) {
		t.Fatal("fresh bucket after sweep must be full")
	}
}

func TestBucketSweepsOpportunisticallyWithoutGoroutine(t *testing.T) {
	clk := newClock()
	b := limiter.NewBuckets(limiter.BucketConfig{Rate: 10, Burst: 5, IdleTTL: time.Minute, Now: clk.Now})
	for i := 0; i < 50; i++ {
		b.Allow(fmt.Sprintf("old-%d", i))
	}
	clk.Advance(5 * time.Minute)
	b.Allow("fresh") // 这次调用顺带清理
	if b.Len() != 1 {
		t.Fatalf("expected opportunistic sweep to leave only the fresh key, Len=%d", b.Len())
	}
}

func TestBucketKeepsKeyUntilFullyRefilled(t *testing.T) {
	clk := newClock()
	// 补满需要 10 秒 > IdleTTL(1s)：清理不得早于补满，否则被限流的 key 会被偷偷“重置”。
	b := limiter.NewBuckets(limiter.BucketConfig{Rate: 1, Burst: 10, IdleTTL: time.Second, Now: clk.Now})
	b.AllowN("k", 10)
	clk.Advance(5 * time.Second)
	b.Sweep()
	if b.AllowN("k", 6) {
		t.Fatal("key was swept before it refilled: limiter state was reset early")
	}
}

func TestBucketConcurrentUseNeverExceedsBudget(t *testing.T) {
	clk := newClock() // 时钟不动：总放行数必须恰好等于 burst
	b := limiter.NewBuckets(limiter.BucketConfig{Rate: 1, Burst: 100, Now: clk.Now})
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				if b.Allow("shared") {
					allowed.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if got := allowed.Load(); got != 100 {
		t.Fatalf("allowed %d, want exactly burst=100", got)
	}
}

func TestBucketNormalizesBadConfig(t *testing.T) {
	b := limiter.NewBuckets(limiter.BucketConfig{Rate: -5, Burst: 0})
	if !b.Allow("k") {
		t.Fatal("burst is normalised to at least 1")
	}
}

// ---------- 熔断器 ----------

func newBreaker(clk *fakeClock, threshold int) *limiter.Breaker {
	return limiter.NewBreaker(limiter.BreakerConfig{FailureThreshold: threshold, OpenTimeout: 10 * time.Second, Now: clk.Now})
}

var errBoom = errors.New("boom")

func fail(b *limiter.Breaker) error { return b.Execute(func() error { return errBoom }) }
func ok(b *limiter.Breaker) error   { return b.Execute(func() error { return nil }) }

func TestBreakerOpensAfterConsecutiveFailures(t *testing.T) {
	clk := newClock()
	b := newBreaker(clk, 3)
	for i := 0; i < 2; i++ {
		if err := fail(b); !errors.Is(err, errBoom) {
			t.Fatalf("Execute must return fn's error, got %v", err)
		}
	}
	if err := ok(b); err != nil { // 成功清零连续失败计数
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		fail(b)
	}
	if b.State() != limiter.Closed {
		t.Fatalf("2 consecutive failures must not open (threshold 3), state=%v", b.State())
	}
	fail(b)
	if b.State() != limiter.Open {
		t.Fatalf("3rd consecutive failure must open, state=%v", b.State())
	}
	called := false
	err := b.Execute(func() error { called = true; return nil })
	if !errors.Is(err, limiter.ErrOpen) || called {
		t.Fatalf("open breaker must reject without calling fn: err=%v called=%v", err, called)
	}
}

func TestBreakerHalfOpenProbeClosesOnSuccess(t *testing.T) {
	clk := newClock()
	var transitions []string
	b := limiter.NewBreaker(limiter.BreakerConfig{
		FailureThreshold: 1, OpenTimeout: 10 * time.Second, Now: clk.Now,
		OnStateChange: func(from, to limiter.State) { transitions = append(transitions, from.String()+">"+to.String()) },
	})
	fail(b)
	clk.Advance(9 * time.Second)
	if err := ok(b); !errors.Is(err, limiter.ErrOpen) {
		t.Fatalf("still within OpenTimeout: want ErrOpen, got %v", err)
	}
	clk.Advance(1 * time.Second)
	if b.State() != limiter.HalfOpen {
		t.Fatalf("after OpenTimeout state must be half-open, got %v", b.State())
	}
	if err := ok(b); err != nil {
		t.Fatalf("probe must be allowed: %v", err)
	}
	if b.State() != limiter.Closed {
		t.Fatalf("successful probe must close, got %v", b.State())
	}
	want := "closed>open open>half-open half-open>closed"
	if got := fmt.Sprint(transitions[0], " ", transitions[1], " ", transitions[2]); got != want {
		t.Fatalf("transitions = %q, want %q", got, want)
	}
}

func TestBreakerHalfOpenFailureReopensAndRestartsTimer(t *testing.T) {
	clk := newClock()
	b := newBreaker(clk, 1)
	fail(b)
	clk.Advance(10 * time.Second)
	if err := fail(b); !errors.Is(err, errBoom) {
		t.Fatalf("probe must run and return its error, got %v", err)
	}
	if b.State() != limiter.Open {
		t.Fatalf("failed probe must reopen, got %v", b.State())
	}
	clk.Advance(9 * time.Second) // 计时从重新打开那一刻重新开始
	if err := ok(b); !errors.Is(err, limiter.ErrOpen) {
		t.Fatalf("timer must restart after a failed probe, got %v", err)
	}
}

func TestBreakerHalfOpenLimitsConcurrentProbes(t *testing.T) {
	clk := newClock()
	b := newBreaker(clk, 1)
	fail(b)
	clk.Advance(10 * time.Second)
	p1, err := b.Allow()
	if err != nil {
		t.Fatalf("first probe: %v", err)
	}
	if _, err := b.Allow(); !errors.Is(err, limiter.ErrOpen) {
		t.Fatalf("second concurrent probe must be rejected, got %v", err)
	}
	p1.Done(true)
	p1.Done(false) // 重复 Done 必须被忽略
	if b.State() != limiter.Closed {
		t.Fatalf("state = %v, a duplicate Done must not change the outcome", b.State())
	}
}

func TestBreakerLeakedProbeDoesNotWedgeForever(t *testing.T) {
	clk := newClock()
	b := newBreaker(clk, 1)
	fail(b)
	clk.Advance(10 * time.Second)
	if _, err := b.Allow(); err != nil { // 取得探测名额却永不 Done
		t.Fatal(err)
	}
	clk.Advance(10 * time.Second)
	if err := ok(b); err != nil {
		t.Fatalf("a leaked probe must expire after OpenTimeout, got %v", err)
	}
}

func TestBreakerLateDoneFromOldGenerationIsIgnored(t *testing.T) {
	clk := newClock()
	b := newBreaker(clk, 1)
	slow, err := b.Allow() // 在关闭状态拿到的许可，迟迟不完成
	if err != nil {
		t.Fatal(err)
	}
	fail(b) // 打开
	clk.Advance(10 * time.Second)
	if err := ok(b); err != nil { // 半开探测成功 → 关闭
		t.Fatal(err)
	}
	slow.Done(false) // 迟到的失败属于旧世代，不得重新打开
	if b.State() != limiter.Closed {
		t.Fatalf("stale permit changed state to %v", b.State())
	}
}

func TestBreakerIsFailureClassification(t *testing.T) {
	clk := newClock()
	clientErr := errors.New("4xx")
	b := limiter.NewBreaker(limiter.BreakerConfig{
		FailureThreshold: 1, Now: clk.Now,
		IsFailure: func(err error) bool { return !errors.Is(err, clientErr) },
	})
	b.Execute(func() error { return clientErr })
	if b.State() != limiter.Closed {
		t.Fatal("errors classified as non-failures must not trip the breaker")
	}
	b.Execute(func() error { return errBoom })
	if b.State() != limiter.Open {
		t.Fatal("real failures must trip it")
	}
}

func TestBreakerDefaultIgnoresContextCancellation(t *testing.T) {
	clk := newClock()
	b := newBreaker(clk, 1)
	b.Execute(func() error { return context.Canceled })
	if b.State() != limiter.Closed {
		t.Fatal("the caller giving up (context.Canceled) is not an upstream failure")
	}
	b.Execute(func() error { return context.DeadlineExceeded })
	if b.State() != limiter.Open {
		t.Fatal("timeouts count as failures")
	}
}

func TestBreakerExecuteCountsPanicAsFailure(t *testing.T) {
	clk := newClock()
	b := newBreaker(clk, 1)
	func() {
		defer func() { _ = recover() }()
		b.Execute(func() error { panic("bad") })
	}()
	if b.State() != limiter.Open {
		t.Fatal("a panicking call must count as a failure and not leak its permit")
	}
}

func TestBreakerConcurrentUse(t *testing.T) {
	clk := newClock()
	b := newBreaker(clk, 50)
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_ = b.Execute(func() error {
					if (g+i)%3 == 0 {
						return errBoom
					}
					return nil
				})
				_ = b.State()
			}
		}(g)
	}
	wg.Wait() // 只要没有死锁/数据竞争即可；状态机细节由上面的确定性测试覆盖
}
