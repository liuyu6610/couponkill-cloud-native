package platforms

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

// scriptedServer 按顺序返回预设状态码，超出脚本后一直返回最后一个。
type scriptedServer struct {
	*httptest.Server
	hits    atomic.Int32
	script  []int
	body    string
	latency time.Duration
}

func newScriptedServer(t *testing.T, body string, script ...int) *scriptedServer {
	t.Helper()
	s := &scriptedServer{script: script, body: body}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(s.hits.Add(1)) - 1
		if s.latency > 0 {
			time.Sleep(s.latency)
		}
		code := s.script[min(n, len(s.script)-1)]
		w.WriteHeader(code)
		_, _ = w.Write([]byte(s.body))
	}))
	t.Cleanup(s.Close)
	return s
}

// newTestCaller 构造不真正睡眠、熔断器用假时钟的 Caller。
func newTestCaller(t *testing.T, clk *testkit.Clock, mutate func(*CallerConfig)) *Caller {
	t.Helper()
	cfg := CallerConfig{
		Name:    "test",
		Retries: 2,
		Sleep:   func(context.Context, time.Duration) error { return nil },
		Breaker: limiter.BreakerConfig{FailureThreshold: 3, OpenTimeout: 30 * time.Second, Now: clk.Now},
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return NewCaller(cfg)
}

func get(url string) func(context.Context) (*http.Request, error) {
	return func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	}
}

func TestCallerReturnsBodyOnSuccess(t *testing.T) {
	srv := newScriptedServer(t, `{"ok":true}`, 200)
	c := newTestCaller(t, testkit.NewClock(time.Now()), nil)

	body, err := c.Do(context.Background(), get(srv.URL))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %q", body)
	}
}

func TestCallerRetriesTransientFailuresAndRebuildsRequestEachAttempt(t *testing.T) {
	srv := newScriptedServer(t, `{"ok":true}`, 503, 502, 200)
	c := newTestCaller(t, testkit.NewClock(time.Now()), nil)

	var builds int
	_, err := c.Do(context.Background(), func(ctx context.Context) (*http.Request, error) {
		builds++ // 每次重试都要重新构造（重新签名、刷新时间戳）
		return http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := srv.hits.Load(); got != 3 || builds != 3 {
		t.Fatalf("hits=%d builds=%d, want 3/3", got, builds)
	}
}

func TestCallerRetriesAreBounded(t *testing.T) {
	srv := newScriptedServer(t, "boom", 500)
	c := newTestCaller(t, testkit.NewClock(time.Now()), nil)

	_, err := c.Do(context.Background(), get(srv.URL))
	var up *UpstreamError
	if !errors.As(err, &up) || up.Status != 500 {
		t.Fatalf("err = %v, want UpstreamError status 500", err)
	}
	if got := srv.hits.Load(); got != 3 { // 首次 + 2 次重试
		t.Fatalf("hits = %d, want 3", got)
	}
}

func TestCallerDoesNotRetryClientErrors(t *testing.T) {
	srv := newScriptedServer(t, "nope", 403)
	c := newTestCaller(t, testkit.NewClock(time.Now()), nil)

	_, err := c.Do(context.Background(), get(srv.URL))
	var up *UpstreamError
	if !errors.As(err, &up) || up.Status != 403 || up.Retryable() {
		t.Fatalf("err = %v, want non-retryable 403", err)
	}
	if got := srv.hits.Load(); got != 1 {
		t.Fatalf("hits = %d, want 1 (4xx 不重试)", got)
	}
}

func TestCallerBreakerOpensFastFailsAndRecoversThroughHalfOpen(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newScriptedServer(t, "down", 503)
	c := newTestCaller(t, clk, func(cfg *CallerConfig) { cfg.Retries = 0 })

	for i := 0; i < 3; i++ { // FailureThreshold=3：三次连续失败后打开
		if _, err := c.Do(context.Background(), get(srv.URL)); err == nil {
			t.Fatal("want failure")
		}
	}
	if st := c.BreakerState(); st != limiter.Open {
		t.Fatalf("state = %v, want open", st)
	}

	before := srv.hits.Load()
	_, err := c.Do(context.Background(), get(srv.URL))
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
	if srv.hits.Load() != before {
		t.Fatal("breaker open must fast-fail without touching the platform")
	}

	// 平台恢复；打开超时过后进入半开，一次成功探测即关闭。
	srv.script = []int{200}
	srv.body = `{"ok":true}`
	clk.Advance(31 * time.Second)
	if st := c.BreakerState(); st != limiter.HalfOpen {
		t.Fatalf("state = %v, want half-open", st)
	}
	if _, err := c.Do(context.Background(), get(srv.URL)); err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if st := c.BreakerState(); st != limiter.Closed {
		t.Fatalf("state = %v, want closed after successful probe", st)
	}
}

func TestCallerBreakerIgnoresClientErrors(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newScriptedServer(t, "bad", 400)
	c := newTestCaller(t, clk, nil)

	for i := 0; i < 10; i++ {
		_, _ = c.Do(context.Background(), get(srv.URL))
	}
	if st := c.BreakerState(); st != limiter.Closed {
		t.Fatalf("state = %v: 平台明确拒绝(4xx)不应打开熔断器", st)
	}
}

func TestCallerTimeoutIsRetriedAndCountsAsFailure(t *testing.T) {
	srv := newScriptedServer(t, "slow", 200)
	srv.latency = 200 * time.Millisecond
	c := newTestCaller(t, testkit.NewClock(time.Now()), func(cfg *CallerConfig) {
		cfg.Client = &http.Client{Timeout: 30 * time.Millisecond}
	})

	_, err := c.Do(context.Background(), get(srv.URL))
	var up *UpstreamError
	if !errors.As(err, &up) || up.Status != 0 || !up.Retryable() {
		t.Fatalf("err = %v, want retryable transport error", err)
	}
	if got := srv.hits.Load(); got != 3 {
		t.Fatalf("hits = %d, want 3 attempts", got)
	}
}

func TestCallerCallerCancellationIsNotRetriedNorCounted(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newScriptedServer(t, "slow", 200)
	srv.latency = 100 * time.Millisecond
	c := newTestCaller(t, clk, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := c.Do(ctx, get(srv.URL))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context deadline", err)
	}
	if got := srv.hits.Load(); got != 1 {
		t.Fatalf("hits = %d, want 1 (调用方放弃后不重试)", got)
	}
}

func TestCallerCapsResponseSize(t *testing.T) {
	srv := newScriptedServer(t, strings.Repeat("x", 4096), 200)
	c := newTestCaller(t, testkit.NewClock(time.Now()), func(cfg *CallerConfig) { cfg.MaxBodyBytes = 1024 })

	_, err := c.Do(context.Background(), get(srv.URL))
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("err = %v, want ErrBadResponse for oversized body", err)
	}
}
