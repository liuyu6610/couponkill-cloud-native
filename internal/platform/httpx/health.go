package httpx

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// defaultCheckTimeout 是就绪检查未指定超时时的上限。
const defaultCheckTimeout = time.Second

type namedCheck struct {
	name    string
	timeout time.Duration
	fn      func(context.Context) error
}

// Health 聚合就绪检查。/healthz 只表示进程存活，不依赖它；/readyz 并发执行所有检查。
type Health struct {
	mu       sync.RWMutex
	checks   []namedCheck
	shutting atomic.Bool
}

// Add 注册一个就绪检查。timeout 可选，默认 1s；超时按失败计。
func (h *Health) Add(name string, fn func(context.Context) error, timeout ...time.Duration) {
	t := defaultCheckTimeout
	if len(timeout) > 0 && timeout[0] > 0 {
		t = timeout[0]
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks = append(h.checks, namedCheck{name: name, timeout: t, fn: fn})
}

// SetShuttingDown 让 /readyz 立即返回不就绪，使负载均衡在优雅关闭前先摘流。
func (h *Health) SetShuttingDown() { h.shutting.Store(true) }

// Ready 并发执行全部检查，返回 name → "ok"/"fail" 与整体是否就绪。
func (h *Health) Ready(ctx context.Context) (map[string]string, bool) {
	h.mu.RLock()
	checks := append([]namedCheck(nil), h.checks...)
	h.mu.RUnlock()

	results := make([]error, len(checks))
	var wg sync.WaitGroup
	for i, ck := range checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, ck.timeout)
			defer cancel()
			results[i] = ck.fn(cctx)
		}()
	}
	wg.Wait()

	out := make(map[string]string, len(checks))
	ready := !h.shutting.Load()
	for i, ck := range checks {
		if results[i] != nil {
			out[ck.name] = "fail"
			ready = false
			logx.From(ctx).Warn("readiness check failed", "check", ck.name, "err", results[i])
			continue
		}
		out[ck.name] = "ok"
	}
	return out, ready
}

func (h *Health) livez(c *gin.Context) { OK(c, gin.H{"status": "alive"}) }

func (h *Health) readyz(c *gin.Context) {
	checks, ready := h.Ready(c.Request.Context())
	if ready {
		OK(c, gin.H{"status": "ready", "checks": checks})
		return
	}
	status := "not_ready"
	if h.shutting.Load() {
		status = "shutting_down"
	}
	c.AbortWithStatusJSON(http.StatusServiceUnavailable, Envelope{
		Code: apperr.CodeSystemBusy, Message: "服务未就绪", Data: gin.H{"status": status, "checks": checks},
	})
}
