package httpx

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// 约定的请求头。
const (
	HeaderRequestID     = "X-Request-Id"
	HeaderInternalToken = "X-Internal-Token"
)

const defaultMaxBodyBytes = 1 << 20 // 1 MiB

// Options 配置 NewEngine。
type Options struct {
	// Service 服务名，作为指标常量标签。
	Service string
	// Logger 默认 slog.Default()。
	Logger *slog.Logger
	// Registry 默认新建一个（每个引擎独立，避免测试里重复注册 panic）。
	Registry *prometheus.Registry
	// MaxBodyBytes 请求体上限，默认 1 MiB。
	MaxBodyBytes int64
	// TrustedProxies 允许信任 X-Forwarded-For 的代理 CIDR；默认不信任任何代理（ClientIP=对端地址），
	// 这样客户端无法通过伪造 XFF 绕过按 IP 的限流。
	TrustedProxies []string
}

// Engine 是配置好的 gin 引擎，可直接作为 http.Handler。
type Engine struct {
	*gin.Engine
	// Health 聚合就绪检查；app.Run 在关闭时调用 Health.SetShuttingDown。
	Health *Health
	// Registry 是本引擎的 Prometheus 注册表，服务可注册自己的指标。
	Registry *prometheus.Registry
}

// NewEngine 创建引擎：recover → request-id → 访问日志 → 指标 → 请求体上限，
// 并挂好 /healthz、/readyz、/metrics，以及信封格式的 404/405。
func NewEngine(opts Options) *Engine {
	gin.SetMode(gin.ReleaseMode)
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Registry == nil {
		opts.Registry = prometheus.NewRegistry()
	}
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = defaultMaxBodyBytes
	}

	g := gin.New()
	g.HandleMethodNotAllowed = true
	g.RedirectTrailingSlash = false
	g.RedirectFixedPath = false
	// 非法的 CIDR 在这里只会让 gin 返回错误；退化为“不信任任何代理”更安全，并记录下来。
	if err := g.SetTrustedProxies(opts.TrustedProxies); err != nil {
		opts.Logger.Error("invalid trusted proxies, trusting none", "err", err)
		_ = g.SetTrustedProxies(nil)
	}

	health := &Health{}
	e := &Engine{Engine: g, Health: health, Registry: opts.Registry}

	m := newMetrics(opts.Service, opts.Registry)
	g.Use(recoverMiddleware(), requestIDMiddleware(opts.Logger), accessLogMiddleware(), m.middleware(), maxBodyMiddleware(opts.MaxBodyBytes))

	g.NoRoute(func(c *gin.Context) {
		Fail(c, apperr.New(apperr.CodeInvalidRequest, http.StatusNotFound, "接口不存在"))
	})
	g.NoMethod(func(c *gin.Context) {
		Fail(c, apperr.New(apperr.CodeInvalidRequest, http.StatusMethodNotAllowed, "请求方法不被允许"))
	})
	g.GET("/healthz", health.livez)
	g.GET("/readyz", health.readyz)
	g.GET("/metrics", gin.WrapH(promhttp.HandlerFor(opts.Registry, promhttp.HandlerOpts{})))
	return e
}

// ---------- recover ----------

func recoverMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			if r == http.ErrAbortHandler { //nolint:errorlint // net/http 约定直接比较该哨兵值
				panic(r) // net/http 约定：用它主动中止响应，必须继续向上抛
			}
			logx.From(c.Request.Context()).Error("panic recovered", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			if c.Writer.Written() {
				c.Abort()
				return
			}
			Fail(c, apperr.SysError.WithCause(fmt.Errorf("panic: %v", r)))
		}()
		c.Next()
	}
}

// ---------- request id ----------

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// requestIDMiddleware 沿用入口给的 X-Request-Id（仅限安全字符与长度，防日志注入），否则生成；
// 写入响应头与 ctx，并把带 request_id 的 logger 放进 ctx。
func requestIDMiddleware(base *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if !validRequestID.MatchString(id) {
			id = logx.NewRequestID()
		}
		c.Header(HeaderRequestID, id)
		ctx := logx.NewContext(c.Request.Context(), base)
		ctx = logx.WithRequestID(ctx, id)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// RequestID 返回当前请求的 request id。
func RequestID(c *gin.Context) string { return logx.RequestID(c.Request.Context()) }

// ---------- 访问日志 ----------

func isOpsPath(p string) bool { return p == "/healthz" || p == "/readyz" || p == "/metrics" }

func accessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		base := c.Request.Context() // 在 Next 之前取：之后 ctx 里可能被鉴权中间件追加了 user_id，避免重复字段
		c.Next()

		status := c.Writer.Status()
		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"route", routeOf(c),
			"status", status,
			"latency_ms", float64(time.Since(start).Microseconds()) / 1000,
			"bytes", c.Writer.Size(),
			"ip", c.ClientIP(),
		}
		if id, ok := IdentityFrom(c); ok {
			attrs = append(attrs, "user_id", id.UserID)
		}
		if v, ok := c.Get(ctxKeyError); ok {
			if e, ok := v.(*apperr.Error); ok {
				attrs = append(attrs, "code", e.Code)
			}
		}
		log := logx.From(base)
		switch {
		case isOpsPath(c.Request.URL.Path):
			log.Debug("http request", attrs...)
		case status >= http.StatusInternalServerError:
			log.Error("http request", attrs...)
		case status >= http.StatusBadRequest:
			log.Warn("http request", attrs...)
		default:
			log.Info("http request", attrs...)
		}
	}
}

func routeOf(c *gin.Context) string {
	if p := c.FullPath(); p != "" {
		return p
	}
	return "unmatched" // 未匹配路径不能直接当标签，否则扫描流量会让指标基数爆炸
}

// ---------- 指标 ----------

type metrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inflight prometheus.Gauge
}

func newMetrics(service string, reg *prometheus.Registry) *metrics {
	constLabels := prometheus.Labels{"service": service}
	m := &metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ck_http_requests_total", Help: "HTTP 请求总数。", ConstLabels: constLabels,
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "ck_http_request_duration_seconds", Help: "HTTP 请求耗时。", ConstLabels: constLabels,
			Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		}, []string{"method", "route"}),
		inflight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ck_http_inflight_requests", Help: "处理中的 HTTP 请求数。", ConstLabels: constLabels,
		}),
	}
	reg.MustRegister(m.requests, m.duration, m.inflight, collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return m
}

func (m *metrics) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		m.inflight.Inc()
		defer m.inflight.Dec()
		c.Next()
		route := routeOf(c)
		m.requests.WithLabelValues(c.Request.Method, route, strconv.Itoa(c.Writer.Status())).Inc()
		m.duration.WithLabelValues(c.Request.Method, route).Observe(time.Since(start).Seconds())
	}
}

// ---------- 请求体上限 ----------

func maxBodyMiddleware(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}
