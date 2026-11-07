// Package gateway 是 CouponKill 的 API 网关（设计 §5、§6）：路径规范化、身份校验与签名传播、
// 限流、熔断、CORS、反向代理到 user/coupon/order/connector 四个上游。
//
// 网关是唯一校验 JWT 的组件：它剥离客户端带来的身份头，校验令牌与会话，
// 再用 HMAC 签名的身份头把调用者交给下游；业务权限（例如 admin）由下游服务自己判断。
package gateway

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
)

// defaultPort 是网关的默认监听端口（设计 D3）。
const defaultPort = "8088"

// Config 是网关的全部配置。公共部分来自 platform/config 的配置组，网关私有项使用 CK_GATEWAY_ 前缀。
type Config struct {
	config.Common
	config.JWT
	config.GatewaySign
	config.Peers
	config.Redis

	// CORSOrigins 允许跨域的源（scheme://host[:port]，不含路径，不允许 "*"）。
	CORSOrigins []string `env:"CK_GATEWAY_CORS_ORIGINS" default:"http://localhost:5173,http://127.0.0.1:5173"`

	// IPRate / IPBurst 每个客户端 IP 的令牌桶（每秒补充数 / 突发容量）。
	IPRate  float64 `env:"CK_GATEWAY_IP_RPS" default:"50" min:"0.1"`
	IPBurst int     `env:"CK_GATEWAY_IP_BURST" default:"100" min:"1"`
	// UserRate / UserBurst 每个已登录用户的令牌桶。
	UserRate  float64 `env:"CK_GATEWAY_USER_RPS" default:"20" min:"0.1"`
	UserBurst int     `env:"CK_GATEWAY_USER_BURST" default:"40" min:"1"`
	// SeckillRate / SeckillBurst 针对 POST /api/v1/order/seckill 的每用户更严格令牌桶。
	SeckillRate  float64 `env:"CK_GATEWAY_SECKILL_RPS" default:"2" min:"0.1"`
	SeckillBurst int     `env:"CK_GATEWAY_SECKILL_BURST" default:"3" min:"1"`

	// UpstreamTimeout 等待上游返回响应头的上限（不限制响应体的传输时间）。
	UpstreamTimeout time.Duration `env:"CK_GATEWAY_UPSTREAM_TIMEOUT" default:"3s" min:"1ms"`
	// SeckillTimeout 是秒杀入口的上游超时。
	SeckillTimeout time.Duration `env:"CK_GATEWAY_SECKILL_TIMEOUT" default:"2s" min:"1ms"`
	// ConnectorTimeout 是 connector 上游的超时：同步/探测会调用外部平台，3s 不够用。
	ConnectorTimeout time.Duration `env:"CK_GATEWAY_CONNECTOR_TIMEOUT" default:"10s" min:"1ms"`

	// BreakerFailures 连续失败多少次后打开某个上游的熔断器。
	BreakerFailures int `env:"CK_GATEWAY_BREAKER_FAILURES" default:"5" min:"1"`
	// BreakerOpen 熔断器打开后多久进入半开探测。
	BreakerOpen time.Duration `env:"CK_GATEWAY_BREAKER_OPEN" default:"30s" min:"1ms"`

	// MaxBodyBytes 请求体上限（字节），超限返回 413。
	MaxBodyBytes int64 `env:"CK_GATEWAY_MAX_BODY_BYTES" default:"1048576" min:"1024"`
	// TrustedProxies 允许信任 X-Forwarded-For 的上游代理（IP 或 CIDR）；默认不信任任何代理。
	TrustedProxies []string `env:"CK_GATEWAY_TRUSTED_PROXIES"`

	// RevocationEnabled 为 false 时网关不访问 Redis，仅校验 JWT 本身。
	RevocationEnabled bool `env:"CK_GATEWAY_REVOCATION_ENABLED" default:"true"`
	// RevocationFailOpen 为 true 时 Redis 出错或超时放行（可用性优先），false 时返回 503（安全优先）。
	RevocationFailOpen bool `env:"CK_GATEWAY_REVOCATION_FAIL_OPEN" default:"true"`
	// RedisTimeout 单次会话查询的超时。
	RedisTimeout time.Duration `env:"CK_GATEWAY_REDIS_TIMEOUT" default:"50ms" min:"1ms"`
}

// LoadConfig 从环境变量加载并校验网关配置，所有问题一次性以 *config.ValidationError 返回。
// 测试可传 config.WithMap 注入环境。
func LoadConfig(opts ...config.Option) (Config, error) {
	var cfg Config
	all := append([]config.Option{config.WithDefaults(map[string]string{"CK_HTTP_PORT": defaultPort})}, opts...)
	if err := config.Load(&cfg, all...); err != nil {
		return Config{}, fmt.Errorf("gateway: load config: %w", err)
	}
	if problems := cfg.validate(); len(problems) > 0 {
		return Config{}, fmt.Errorf("gateway: load config: %w", &config.ValidationError{Problems: problems})
	}
	return cfg, nil
}

// upstreamURLs 返回按名称索引的上游基础 URL。
func (c Config) upstreamURLs() map[string]string {
	return map[string]string{
		upstreamUser:      c.UserURL,
		upstreamCoupon:    c.CouponURL,
		upstreamOrder:     c.OrderURL,
		upstreamConnector: c.ConnectorURL,
	}
}

// validate 检查 config 标签表达不了的约束，返回全部问题。
func (c Config) validate() []string {
	var problems []string
	for name, raw := range c.upstreamURLs() {
		if _, err := parseUpstreamURL(raw); err != nil {
			problems = append(problems, fmt.Sprintf("upstream %s: %v", name, err))
		}
	}
	for _, o := range c.CORSOrigins {
		if err := checkOrigin(o); err != nil {
			problems = append(problems, "CK_GATEWAY_CORS_ORIGINS: "+err.Error())
		}
	}
	for _, p := range c.TrustedProxies {
		if _, err := parseProxyPrefix(p); err != nil {
			problems = append(problems, "CK_GATEWAY_TRUSTED_PROXIES: "+err.Error())
		}
	}
	return problems
}

// parseUpstreamURL 解析上游基础 URL：必须是带主机的 http/https 地址。
func parseUpstreamURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid url %q: %w", raw, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("url %q must look like http(s)://host[:port]", raw)
	}
	return u, nil
}

// checkOrigin 校验 CORS 源：scheme://host[:port]，无路径、无通配符。
func checkOrigin(o string) error {
	if o == "*" || strings.Contains(o, "*") {
		return fmt.Errorf("wildcard origin %q is not allowed, list explicit origins", o)
	}
	u, err := url.Parse(o)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("origin %q must look like scheme://host[:port]", o)
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("origin %q must not carry a path, query or userinfo", o)
	}
	return nil
}

// parseProxyPrefix 把 IP 或 CIDR 解析成前缀（单个 IP 视为 /32 或 /128）。
func parseProxyPrefix(s string) (netip.Prefix, error) {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p, nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%q is neither an IP nor a CIDR", s)
	}
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}
