package gateway

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
)

// 网关拒绝请求的原因标签（基数固定）。
const (
	reasonBadPath         = "bad_path"
	reasonNotFound        = "not_found"
	reasonBodyTooLarge    = "body_too_large"
	reasonUnauthenticated = "unauthenticated"
	reasonTokenInvalid    = "token_invalid"
	reasonSessionRevoked  = "session_revoked"
	reasonSessionStore    = "session_store_unavailable"
	reasonLimitIP         = "rate_limited_ip"
	reasonLimitUser       = "rate_limited_user"
	reasonLimitSeckill    = "rate_limited_seckill"
	reasonBreakerOpen     = "breaker_open"
	reasonCORS            = "cors_forbidden"
)

// gwMetrics 是网关自己的 Prometheus 指标。httpx 的通用指标按 gin 路由模板打标签，
// 而网关的请求都走 NoRoute（标签恒为 unmatched），所以按上游、状态、拒绝原因另行统计。
type gwMetrics struct {
	proxied  *prometheus.CounterVec
	latency  *prometheus.HistogramVec
	rejected *prometheus.CounterVec
	failOpen prometheus.Counter
	breaker  *prometheus.GaugeVec
}

func newGWMetrics(reg *prometheus.Registry, upstreams []string) *gwMetrics {
	constLabels := prometheus.Labels{"service": "gateway"}
	m := &gwMetrics{
		proxied: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ck_gateway_proxied_total", Help: "转发到上游并得到（或未得到）响应的请求数，按上游与最终状态码。",
			ConstLabels: constLabels,
		}, []string{"upstream", "status"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "ck_gateway_proxy_duration_seconds", Help: "经网关转发的请求耗时（含响应体传输）。",
			ConstLabels: constLabels,
			Buckets:     []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"upstream"}),
		rejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ck_gateway_rejected_total", Help: "网关在转发前拒绝的请求数，按原因。",
			ConstLabels: constLabels,
		}, []string{"reason"}),
		failOpen: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ck_gateway_session_check_fail_open_total", Help: "会话存储不可用而按 fail-open 放行的请求数。",
			ConstLabels: constLabels,
		}),
		breaker: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ck_gateway_breaker_state", Help: "各上游熔断器状态：0=closed，1=half-open，2=open。",
			ConstLabels: constLabels,
		}, []string{"upstream"}),
	}
	reg.MustRegister(m.proxied, m.latency, m.rejected, m.failOpen, m.breaker)
	for _, u := range upstreams {
		m.breaker.WithLabelValues(u).Set(breakerGauge(limiter.Closed))
	}
	return m
}

func breakerGauge(s limiter.State) float64 {
	switch s {
	case limiter.HalfOpen:
		return 1
	case limiter.Open:
		return 2
	}
	return 0
}
