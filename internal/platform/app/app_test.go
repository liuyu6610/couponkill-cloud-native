package app_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/app"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
)

// events 按发生顺序记录事件，用来断言关闭顺序。
type events struct {
	mu  sync.Mutex
	seq []string
}

func (e *events) add(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seq = append(e.seq, s)
}

func (e *events) all() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.seq...)
}

func (e *events) index(s string) int {
	for i, v := range e.all() {
		if v == s {
			return i
		}
	}
	return -1
}

type fakeHealth struct{ ev *events }

func (h fakeHealth) SetShuttingDown() { h.ev.add("not-ready") }

type harness struct {
	t      *testing.T
	cancel context.CancelFunc
	done   chan error
	url    string
}

// start 在真实监听器上启动 app.Run。
func start(t *testing.T, mut func(*app.Options)) *harness {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") })
	opts := app.Options{Name: "test", Listener: ln, Handler: mux, ShutdownTimeout: 3 * time.Second}
	if mut != nil {
		mut(&opts)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &harness{t: t, cancel: cancel, done: make(chan error, 1), url: "http://" + ln.Addr().String()}
	go func() { h.done <- app.Run(ctx, opts) }()
	t.Cleanup(cancel)
	waitFor(t, func() bool { return get(h.url+"/ok") == "ok" }, "server to start serving")
	return h
}

func get(url string) string {
	c := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := c.Get(url)
	if err != nil {
		return "ERR:" + err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (h *harness) result(timeout time.Duration) error {
	h.t.Helper()
	select {
	case err := <-h.done:
		return err
	case <-time.After(timeout):
		h.t.Fatal("app.Run did not return in time")
		return nil
	}
}

func TestRunServesAndStopsOnContextCancel(t *testing.T) {
	h := start(t, nil)
	h.cancel()
	if err := h.result(3 * time.Second); err != nil {
		t.Fatalf("clean shutdown must return nil, got %v", err)
	}
	if got := get(h.url + "/ok"); !strings.HasPrefix(got, "ERR:") {
		t.Fatalf("server must not accept connections after Run returns, got %q", got)
	}
}

func TestShutdownOrder(t *testing.T) {
	ev := &events{}
	release := make(chan struct{})
	var inflight atomic.Bool

	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") })
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		inflight.Store(true)
		<-release
		ev.add("inflight-done")
		_, _ = io.WriteString(w, "slow-done")
	})

	h := start(t, func(o *app.Options) {
		o.Handler = mux
		o.Health = fakeHealth{ev}
		o.Workers = []app.Worker{{Name: "w", Run: func(ctx context.Context) error {
			<-ctx.Done()
			ev.add("worker-stopped")
			return ctx.Err()
		}}}
		o.OnClose = []func(context.Context) error{
			func(context.Context) error { ev.add("close-first-registered"); return nil },
			func(context.Context) error { ev.add("close-second-registered"); return nil },
		}
	})

	slowResult := make(chan string, 1)
	go func() { slowResult <- get(h.url + "/slow") }()
	waitFor(t, inflight.Load, "slow request to be in flight")

	h.cancel()
	waitFor(t, func() bool { return ev.index("not-ready") >= 0 }, "readiness flip")
	// 关闭已经开始，但在途请求还没结束：worker 与关闭钩子都不应该已经动过。
	time.Sleep(50 * time.Millisecond)
	if ev.index("worker-stopped") >= 0 || ev.index("close-second-registered") >= 0 {
		t.Fatalf("workers/hooks must wait for in-flight requests: %v", ev.all())
	}
	close(release)

	if err := h.result(3 * time.Second); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := <-slowResult; got != "slow-done" {
		t.Fatalf("in-flight request must complete during graceful shutdown, got %q", got)
	}
	want := []string{"not-ready", "inflight-done", "worker-stopped", "close-second-registered", "close-first-registered"}
	got := ev.all()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("shutdown order wrong:\n got  %v\n want %v", got, want)
	}
}

func TestDrainDelayKeepsServingAfterReadinessFlips(t *testing.T) {
	ev := &events{}
	h := start(t, func(o *app.Options) { o.Health = fakeHealth{ev}; o.DrainDelay = 300 * time.Millisecond })
	h.cancel()
	waitFor(t, func() bool { return ev.index("not-ready") >= 0 }, "readiness flip")
	if got := get(h.url + "/ok"); got != "ok" {
		t.Fatalf("during the drain delay the server must still serve, got %q", got)
	}
	start := time.Now()
	if err := h.result(3 * time.Second); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 100*time.Millisecond {
		t.Fatal("Run returned before the drain delay elapsed")
	}
}

func TestWorkerFailureTriggersShutdownAndIsReturned(t *testing.T) {
	ev := &events{}
	boom := errors.New("consumer lost kafka")
	var other atomic.Bool
	h := start(t, func(o *app.Options) {
		o.Health = fakeHealth{ev}
		o.Workers = []app.Worker{
			{Name: "healthy", Run: func(ctx context.Context) error { <-ctx.Done(); other.Store(true); return nil }},
			{Name: "failing", Run: func(ctx context.Context) error { time.Sleep(100 * time.Millisecond); return boom }},
		}
		o.OnClose = []func(context.Context) error{func(context.Context) error { ev.add("closed"); return nil }}
	})
	err := h.result(3 * time.Second)
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "failing") {
		t.Fatalf("Run must return the worker's error, got %v", err)
	}
	if !other.Load() {
		t.Fatal("the healthy worker must be cancelled and awaited")
	}
	if ev.index("not-ready") < 0 || ev.index("closed") < 0 {
		t.Fatalf("a worker failure must run the full shutdown sequence: %v", ev.all())
	}
}

func TestWorkerPanicBecomesError(t *testing.T) {
	h := start(t, func(o *app.Options) {
		o.Workers = []app.Worker{{Name: "bad", Run: func(context.Context) error { time.Sleep(100 * time.Millisecond); panic("bug") }}}
	})
	err := h.result(3 * time.Second)
	if err == nil || !strings.Contains(err.Error(), "panic: bug") {
		t.Fatalf("worker panic must be reported as an error, got %v", err)
	}
}

func TestWorkerFinishingEarlyWithoutErrorDoesNotStopTheApp(t *testing.T) {
	var ran atomic.Bool
	h := start(t, func(o *app.Options) {
		o.Workers = []app.Worker{{Name: "one-shot", Run: func(context.Context) error { ran.Store(true); return nil }}}
	})
	waitFor(t, ran.Load, "one-shot worker")
	time.Sleep(50 * time.Millisecond)
	if get(h.url+"/ok") != "ok" {
		t.Fatal("app must keep serving after a worker completes cleanly")
	}
	h.cancel()
	if err := h.result(3 * time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerReturningContextCanceledOnShutdownIsNotAnError(t *testing.T) {
	h := start(t, func(o *app.Options) {
		o.Workers = []app.Worker{{Name: "w", Run: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }}}
	})
	h.cancel()
	if err := h.result(3 * time.Second); err != nil {
		t.Fatalf("context.Canceled during shutdown is normal, got %v", err)
	}
}

func TestShutdownTimeoutForcesClosureOfStuckRequests(t *testing.T) {
	unblock := make(chan struct{})
	var entered atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/stuck", func(w http.ResponseWriter, r *http.Request) { entered.Store(true); <-unblock })
	h := start(t, func(o *app.Options) {
		o.Handler = mux
		o.ShutdownTimeout = 200 * time.Millisecond
		mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") })
	})
	defer close(unblock)
	go get(h.url + "/stuck")
	waitFor(t, entered.Load, "stuck request")

	h.cancel()
	start := time.Now()
	err := h.result(3 * time.Second)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a shutdown that exceeds its budget must report the deadline, got %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Run must not hang past ShutdownTimeout, took %v", d)
	}
}

func TestStuckWorkerIsReportedNotAwaitedForever(t *testing.T) {
	unblock := make(chan struct{})
	defer close(unblock)
	h := start(t, func(o *app.Options) {
		o.ShutdownTimeout = 200 * time.Millisecond
		o.Workers = []app.Worker{{Name: "stuck", Run: func(context.Context) error { <-unblock; return nil }}}
	})
	h.cancel()
	err := h.result(3 * time.Second)
	if err == nil || !strings.Contains(err.Error(), "workers did not stop") {
		t.Fatalf("a worker ignoring ctx must be reported, got %v", err)
	}
}

func TestCloseHooksAllRunDespiteFailuresAndPanics(t *testing.T) {
	ev := &events{}
	bad := errors.New("redis close failed")
	h := start(t, func(o *app.Options) {
		o.OnClose = []func(context.Context) error{
			func(context.Context) error { ev.add("a"); return nil },
			func(context.Context) error { ev.add("b"); panic("hook bug") },
			func(context.Context) error { ev.add("c"); return bad },
		}
	})
	h.cancel()
	err := h.result(3 * time.Second)
	if strings.Join(ev.all(), "") != "cba" {
		t.Fatalf("every hook must run, in reverse order: %v", ev.all())
	}
	if !errors.Is(err, bad) || !strings.Contains(err.Error(), "panic: hook bug") {
		t.Fatalf("hook failures must be reported: %v", err)
	}
}

func TestListenFailureStillRunsHooksAndStartsNoWorkers(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	var closed, workerStarted atomic.Bool
	err = app.Run(context.Background(), app.Options{
		Name: "x", Addr: taken.Addr().String(), Handler: http.NewServeMux(),
		Workers: []app.Worker{{Name: "w", Run: func(context.Context) error { workerStarted.Store(true); return nil }}},
		OnClose: []func(context.Context) error{func(context.Context) error { closed.Store(true); return nil }},
	})
	if err == nil || !strings.Contains(err.Error(), "listen") {
		t.Fatalf("a bind failure must be returned, got %v", err)
	}
	if !closed.Load() {
		t.Fatal("resources opened before Run must still be closed when startup fails")
	}
	time.Sleep(20 * time.Millisecond)
	if workerStarted.Load() {
		t.Fatal("no worker may start when the server could not bind")
	}
}

func TestHandlerIsRequired(t *testing.T) {
	var closed atomic.Bool
	err := app.Run(context.Background(), app.Options{
		Name: "x", Addr: "127.0.0.1:0",
		OnClose: []func(context.Context) error{func(context.Context) error { closed.Store(true); return nil }},
	})
	if err == nil || !closed.Load() {
		t.Fatalf("missing handler must fail and still close resources: err=%v closed=%v", err, closed.Load())
	}
}

func TestReadyzReturns503DuringDrainWithRealEngine(t *testing.T) {
	eng := httpx.NewEngine(httpx.Options{Service: "t"})
	eng.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	h := start(t, func(o *app.Options) {
		o.Handler = eng
		o.Health = eng.Health
		o.DrainDelay = 400 * time.Millisecond
	})
	if got := get(h.url + "/readyz"); !strings.Contains(got, `"status":"ready"`) {
		t.Fatalf("must be ready while running, got %q", got)
	}
	h.cancel()
	waitFor(t, func() bool { return strings.Contains(get(h.url+"/readyz"), "shutting_down") }, "readyz to report shutting_down during the drain")
	if got := get(h.url + "/healthz"); !strings.Contains(got, "alive") {
		t.Fatalf("liveness must stay up while draining, got %q", got)
	}
	if err := h.result(3 * time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestAllGoroutinesExitAfterRun(t *testing.T) {
	var running atomic.Int32
	worker := func(ctx context.Context) error {
		running.Add(1)
		defer running.Add(-1)
		<-ctx.Done()
		return ctx.Err()
	}
	h := start(t, func(o *app.Options) {
		o.Workers = []app.Worker{{Name: "a", Run: worker}, {Name: "b", Run: worker}, {Name: "c", Run: worker}}
	})
	waitFor(t, func() bool { return running.Load() == 3 }, "workers to start")
	h.cancel()
	if err := h.result(3 * time.Second); err != nil {
		t.Fatal(err)
	}
	if n := running.Load(); n != 0 {
		t.Fatalf("%d workers still running after Run returned", n)
	}
}
