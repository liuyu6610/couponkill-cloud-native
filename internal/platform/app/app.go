// Package app 负责服务进程的生命周期：启动 HTTP 服务与后台 worker，监听 SIGINT/SIGTERM，
// 并按固定顺序优雅关闭。
//
// 关闭顺序（不变式）：
//  1. 就绪探针立即变为 not-ready（负载均衡开始摘流），可选再等待 DrainDelay；
//  2. 停止接收新连接，并等待在途请求完成（受 ShutdownTimeout 约束，超时强制关闭连接）；
//  3. 取消 worker 的 ctx 并等待它们退出（HTTP 在途请求可能依赖 worker，所以 worker 后停）；
//  4. 按注册的逆序执行 OnClose（先开的后关）。
//
// 每个 goroutine 都随 ctx 退出：HTTP 由 Shutdown 终止，worker 必须在 ctx 取消时返回。
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
	"time"
)

// Worker 是一个后台任务（消费者、定时任务……）。Run 必须在 ctx 取消时尽快返回。
type Worker struct {
	Name string
	Run  func(ctx context.Context) error
}

// Readiness 是就绪探针的开关；*httpx.Health 满足它。
type Readiness interface {
	SetShuttingDown()
}

// Options 配置 Run。
type Options struct {
	// Name 服务名，仅用于日志。
	Name string
	// Addr 监听地址（如 ":8082"）；Listener 非空时忽略。
	Addr string
	// Listener 注入监听器（测试用 127.0.0.1:0）。
	Listener net.Listener
	// Handler 是 HTTP 处理器（必填）。
	Handler http.Handler
	// Health 可选；关闭开始时被置为不就绪。
	Health Readiness
	// Workers 后台任务。关闭前任一 worker 返回非 nil 错误会触发关闭并作为 Run 的返回值。
	Workers []Worker
	// OnClose 关闭钩子，按注册逆序执行（用于关闭 Kafka 生产者、Redis、PG 等）。
	OnClose []func(ctx context.Context) error
	// ShutdownTimeout 整个关闭过程的预算，默认 25s（K8s 默认宽限期 30s）。
	ShutdownTimeout time.Duration
	// DrainDelay 置为不就绪后、停止接收连接前的等待，给负载均衡摘流留时间，默认 0。
	DrainDelay time.Duration
	// ReadHeaderTimeout / ReadTimeout / WriteTimeout / IdleTimeout 为 http.Server 的同名参数，
	// 默认 5s / 15s / 30s / 90s。
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	// Signals 触发关闭的信号，默认 SIGINT 与 SIGTERM。
	Signals []os.Signal
	// Logger 默认 slog.Default()。
	Logger *slog.Logger
}

func (o *Options) applyDefaults() {
	def := func(d *time.Duration, v time.Duration) {
		if *d <= 0 {
			*d = v
		}
	}
	def(&o.ShutdownTimeout, 25*time.Second)
	def(&o.ReadHeaderTimeout, 5*time.Second)
	def(&o.ReadTimeout, 15*time.Second)
	def(&o.WriteTimeout, 30*time.Second)
	def(&o.IdleTimeout, 90*time.Second)
	if len(o.Signals) == 0 {
		o.Signals = []os.Signal{os.Interrupt, syscall.SIGTERM}
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
}

// Run 启动服务并阻塞到关闭完成。ctx 取消或收到信号即触发优雅关闭；正常关闭返回 nil，
// 否则返回 worker 的致命错误、服务器错误以及关闭过程中的错误（errors.Join）。
func Run(ctx context.Context, opts Options) (runErr error) {
	opts.applyDefaults()
	log := opts.Logger.With("app", opts.Name)

	// 关闭钩子在任何退出路径上都要执行（包括监听失败）。
	deadline := time.Time{}
	defer func() {
		runErr = errors.Join(runErr, runCloseHooks(ctx, log, opts.OnClose, deadline))
	}()

	if opts.Handler == nil {
		return errors.New("app: Handler is required")
	}

	sigCtx, stopSignals := signal.NotifyContext(ctx, opts.Signals...)
	defer stopSignals()

	ln := opts.Listener
	if ln == nil {
		l, err := net.Listen("tcp", opts.Addr)
		if err != nil {
			return fmt.Errorf("app: listen %q: %w", opts.Addr, err)
		}
		ln = l
	}

	// 在途请求与 worker 的 ctx 都不能被信号立刻取消，否则“优雅”就无从谈起：
	// 它们只在各自的关闭阶段被显式取消。
	detached := context.WithoutCancel(ctx)
	reqCtx, cancelReqs := context.WithCancel(detached)
	defer cancelReqs()
	workerCtx, cancelWorkers := context.WithCancel(detached)
	defer cancelWorkers()

	srv := &http.Server{
		Handler:           opts.Handler,
		ReadHeaderTimeout: opts.ReadHeaderTimeout,
		ReadTimeout:       opts.ReadTimeout,
		WriteTimeout:      opts.WriteTimeout,
		IdleTimeout:       opts.IdleTimeout,
		BaseContext:       func(net.Listener) context.Context { return reqCtx },
	}

	// failures 收集所有致命错误（含关闭阶段才发生的），trigger 只负责唤醒主流程。
	var (
		failMu   sync.Mutex
		failures []error
	)
	trigger := make(chan struct{}, 1)
	fail := func(err error) {
		failMu.Lock()
		failures = append(failures, err)
		failMu.Unlock()
		select {
		case trigger <- struct{}{}:
		default:
		}
	}

	var serverDone sync.WaitGroup
	serverDone.Add(1)
	go func() {
		defer serverDone.Done()
		log.Info("http server listening", "addr", ln.Addr().String())
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fail(fmt.Errorf("app: http server: %w", err))
		}
	}()

	var workers sync.WaitGroup
	for _, w := range opts.Workers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			err := runWorker(workerCtx, log, w)
			switch {
			case err == nil:
				log.Info("worker finished", "worker", w.Name)
			case errors.Is(err, context.Canceled) && workerCtx.Err() != nil:
				// 关闭期间因 ctx 取消而返回：正常。
			default:
				log.Error("worker failed", "worker", w.Name, "err", err)
				fail(fmt.Errorf("worker %s: %w", w.Name, err))
			}
		}()
	}

	select {
	case <-sigCtx.Done():
		log.Info("shutdown requested", "cause", context.Cause(sigCtx))
	case <-trigger:
		log.Error("fatal error, shutting down")
	}

	// ---- 关闭 ----
	started := time.Now()
	deadline = started.Add(opts.ShutdownTimeout)
	if opts.Health != nil {
		opts.Health.SetShuttingDown()
	}
	shutCtx, cancelShut := context.WithDeadline(detached, deadline)
	defer cancelShut()

	if opts.DrainDelay > 0 {
		t := time.NewTimer(opts.DrainDelay)
		select {
		case <-t.C:
		case <-shutCtx.Done():
			t.Stop()
		}
	}

	var shutErrs []error
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error("graceful http shutdown failed, closing connections", "err", err)
		shutErrs = append(shutErrs, fmt.Errorf("app: http shutdown: %w", err))
		if cerr := srv.Close(); cerr != nil {
			shutErrs = append(shutErrs, fmt.Errorf("app: http close: %w", cerr))
		}
	}
	cancelReqs()
	serverDone.Wait()

	cancelWorkers()
	if !waitTimeout(shutCtx, &workers) {
		log.Error("workers did not stop before the shutdown deadline")
		shutErrs = append(shutErrs, errors.New("app: workers did not stop in time"))
	}

	log.Info("shutdown complete", "took", time.Since(started).String())
	failMu.Lock()
	all := append(append([]error(nil), failures...), shutErrs...)
	failMu.Unlock()
	return errors.Join(all...)
}

// runWorker 运行 worker，把 panic 转为错误。
func runWorker(ctx context.Context, log *slog.Logger, w Worker) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("worker panic", "worker", w.Name, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return w.Run(ctx)
}

// waitTimeout 等待 wg 完成或 ctx 结束；返回是否在 ctx 结束前完成。
func waitTimeout(ctx context.Context, wg *sync.WaitGroup) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

// runCloseHooks 按逆序执行关闭钩子；某个失败或 panic 不影响其余钩子。
func runCloseHooks(ctx context.Context, log *slog.Logger, hooks []func(context.Context) error, deadline time.Time) error {
	if len(hooks) == 0 {
		return nil
	}
	budget := 10 * time.Second // 钩子在关闭流程之外（如监听失败）执行时的预算
	if !deadline.IsZero() {
		budget = max(time.Until(deadline), time.Second)
	}
	hctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), budget)
	defer cancel()

	var errs []error
	for i := len(hooks) - 1; i >= 0; i-- {
		if err := runHook(hctx, hooks[i]); err != nil {
			log.Error("close hook failed", "index", i, "err", err)
			errs = append(errs, fmt.Errorf("app: close hook %d: %w", i, err))
		}
	}
	return errors.Join(errs...)
}

func runHook(ctx context.Context, h func(context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return h(ctx)
}
