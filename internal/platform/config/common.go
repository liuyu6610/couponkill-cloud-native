package config

import (
	"encoding/json"
	"log/slog"
	"time"
)

// redacted 是 Secret 在任何输出渠道里的占位文本。
const redacted = "[REDACTED]"

// Secret 是不得出现在日志/响应里的配置值（密钥、口令、含口令的 DSN）。
// 它的所有格式化、JSON、slog 输出都是 [REDACTED]（空值保持为空，以便看出“未设置”）；
// 需要真实值时显式调用 Reveal。
type Secret string

// Reveal 返回真实值。只应在把它交给 JWT/HMAC/连接池等消费者时调用。
func (s Secret) Reveal() string { return string(s) }

// String 返回脱敏文本。
func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return redacted
}

// GoString 让 %#v 同样脱敏。
func (s Secret) GoString() string { return `config.Secret("` + s.String() + `")` }

// LogValue 实现 slog.LogValuer。
func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }

// MarshalJSON 输出脱敏文本。
func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

// MarshalText 输出脱敏文本（覆盖 yaml/toml 等基于 TextMarshaler 的编码）。
func (s Secret) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// Common 是每个服务都内嵌的公共配置。
type Common struct {
	// Env 运行环境：dev 或 prod。prod 下带开发默认值的密钥必须显式设置。
	Env string `env:"CK_ENV" default:"dev" oneof:"dev,prod"`
	// HTTPPort 监听端口；各服务用 WithDefaults 设置自己的默认端口。
	HTTPPort int `env:"CK_HTTP_PORT" default:"8080" min:"1" max:"65535"`
	// LogLevel 日志级别：debug|info|warn|error。
	LogLevel string `env:"CK_LOG_LEVEL" default:"info" oneof:"debug,info,warn,error"`
	// WorkerID snowflake 工作节点号（0..1023）；-1 表示按主机名哈希推导。
	WorkerID int `env:"CK_WORKER_ID" default:"-1" min:"-1" max:"1023"`
}

// IsProd 报告是否为生产环境。
func (c Common) IsProd() bool { return c.Env == "prod" }

// Postgres 是 PostgreSQL 连接配置。
type Postgres struct {
	// PGDSN 含口令，因此是 Secret；默认值仅适用于本地 docker-compose。
	PGDSN Secret `env:"CK_PG_DSN" default:"postgres://couponkill:couponkill@127.0.0.1:5433/couponkill?sslmode=disable"`
}

// Redis 是 Redis 连接配置。
type Redis struct {
	RedisAddr     string `env:"CK_REDIS_ADDR" default:"127.0.0.1:6379"`
	RedisPassword Secret `env:"CK_REDIS_PASSWORD"`
	RedisDB       int    `env:"CK_REDIS_DB" default:"0" min:"0" max:"15"`
}

// Kafka 是 Kafka 连接配置。
type Kafka struct {
	// KafkaBrokers 逗号分隔的 broker 地址。
	KafkaBrokers []string `env:"CK_KAFKA_BROKERS" default:"127.0.0.1:9092"`
}

// JWT 是登录令牌配置（gateway 与 user 服务使用）。
type JWT struct {
	JWTSecret   Secret        `env:"CK_JWT_SECRET" default:"dev-only-jwt-secret-do-not-use-in-prod-0123456789" min:"32"`
	JWTIssuer   string        `env:"CK_JWT_ISSUER" default:"couponkill"`
	JWTAudience string        `env:"CK_JWT_AUDIENCE" default:"couponkill-api"`
	JWTTTL      time.Duration `env:"CK_JWT_TTL" default:"24h" min:"1m"`
}

// Internal 是服务间内部调用令牌。
type Internal struct {
	InternalToken Secret `env:"CK_INTERNAL_TOKEN" default:"dev-only-internal-token-do-not-use-in-prod" min:"16"`
}

// GatewaySign 是网关身份头的 HMAC 密钥（gateway 签名，其他服务验签）。
type GatewaySign struct {
	GatewaySignKey Secret `env:"CK_GATEWAY_SIGN_KEY" default:"dev-only-gateway-sign-key-do-not-use-in-prod-0123456789" min:"32"`
}

// Peers 是各服务的内部基础 URL（K8s DNS 或本地地址）。
type Peers struct {
	UserURL      string `env:"CK_USER_URL" default:"http://127.0.0.1:8081"`
	CouponURL    string `env:"CK_COUPON_URL" default:"http://127.0.0.1:8080"`
	OrderURL     string `env:"CK_ORDER_URL" default:"http://127.0.0.1:8082"`
	ConnectorURL string `env:"CK_CONNECTOR_URL" default:"http://127.0.0.1:8085"`
}
