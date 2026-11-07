package connector

import (
	"fmt"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
)

// DefaultPort 是 connector 的默认监听端口（设计 D3）。
const DefaultPort = "8085"

// Config 是 connector 的全部配置（环境变量，前缀 CK_）。公共项见 platform/config；
// 平台凭证全部是 config.Secret（日志与 JSON 里只显示 [REDACTED]），缺凭证的平台自动 DISABLED，
// 因此凭证没有开发默认值，生产环境也不会因为缺它们而拒绝启动。
type Config struct {
	config.Common
	config.Postgres
	config.Redis
	config.Internal
	config.GatewaySign
	config.Peers

	// SyncEnabled 是否启动定时库存同步任务（手工同步接口不受影响）。
	SyncEnabled bool `env:"CK_CONNECTOR_SYNC_ENABLED" default:"true"`
	// SyncInterval 是两轮定时同步之间的间隔。
	SyncInterval time.Duration `env:"CK_CONNECTOR_SYNC_INTERVAL" default:"1m" min:"1s"`
	// SyncBudget 是单轮定时同步的时间预算（仅在每条绑定开始前检查，在途的一条会跑完）。
	SyncBudget time.Duration `env:"CK_CONNECTOR_SYNC_BUDGET" default:"30s" min:"1s"`
	// SyncLockTTL 是同步锁租约；每条绑定开始前与写库存前都会续满，下限 10s。
	SyncLockTTL time.Duration `env:"CK_CONNECTOR_SYNC_LOCK_TTL" default:"60s" min:"10s"`
	// MockStock 是 MOCK 平台在 SKU 不带库存后缀时返回的库存。
	MockStock int64 `env:"CK_CONNECTOR_MOCK_STOCK" default:"500" min:"1"`
	// HealthTTL 是平台健康状态的缓存时长（惰性复查）。
	HealthTTL time.Duration `env:"CK_CONNECTOR_HEALTH_TTL" default:"30s" min:"1s"`
	// CompareCacheTTL 是同品比价结果的缓存时长（匿名接口会触发第三方探测，缓存防放大）。
	CompareCacheTTL time.Duration `env:"CK_CONNECTOR_COMPARE_CACHE_TTL" default:"30s" min:"1s"`
	// CallRetries 是对平台幂等读的重试次数（首次之外）。
	CallRetries int `env:"CK_CONNECTOR_CALL_RETRIES" default:"2" min:"0" max:"5"`
	// BreakerFailures 是连续失败多少次后打开平台熔断器。
	BreakerFailures int `env:"CK_CONNECTOR_BREAKER_FAILURES" default:"5" min:"1"`
	// BreakerOpenFor 是熔断器打开多久后进入半开探测。
	BreakerOpenFor time.Duration `env:"CK_CONNECTOR_BREAKER_OPEN_FOR" default:"30s" min:"1s"`

	JDServerURL   string        `env:"CK_CONNECTOR_JD_SERVER_URL" default:"https://api.jd.com/routerjson"`
	JDAppKey      config.Secret `env:"CK_CONNECTOR_JD_APP_KEY"`
	JDAppSecret   config.Secret `env:"CK_CONNECTOR_JD_APP_SECRET"`
	JDAccessToken config.Secret `env:"CK_CONNECTOR_JD_ACCESS_TOKEN"`
	JDDefaultArea string        `env:"CK_CONNECTOR_JD_DEFAULT_AREA" default:"1_72_2799_0"`

	TBServerURL string        `env:"CK_CONNECTOR_TB_SERVER_URL" default:"https://eco.taobao.com/router/rest"`
	TBAppKey    config.Secret `env:"CK_CONNECTOR_TB_APP_KEY"`
	TBAppSecret config.Secret `env:"CK_CONNECTOR_TB_APP_SECRET"`
	TBSession   config.Secret `env:"CK_CONNECTOR_TB_SESSION"`

	PDDServerURL   string        `env:"CK_CONNECTOR_PDD_SERVER_URL" default:"https://gw-api.pinduoduo.com/api/router"`
	PDDAppKey      config.Secret `env:"CK_CONNECTOR_PDD_APP_KEY"`
	PDDAppSecret   config.Secret `env:"CK_CONNECTOR_PDD_APP_SECRET"`
	PDDAccessToken config.Secret `env:"CK_CONNECTOR_PDD_ACCESS_TOKEN"`
}

// LoadConfig 从环境变量加载并校验配置（一次报告全部问题）。
func LoadConfig(opts ...config.Option) (Config, error) {
	var cfg Config
	opts = append([]config.Option{config.WithDefaults(map[string]string{"CK_HTTP_PORT": DefaultPort})}, opts...)
	if err := config.Load(&cfg, opts...); err != nil {
		return cfg, err
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate 检查字段之间的约束。
func (c Config) Validate() error {
	if c.SyncBudget >= c.SyncLockTTL {
		return fmt.Errorf("CK_CONNECTOR_SYNC_BUDGET(%s) 必须小于 CK_CONNECTOR_SYNC_LOCK_TTL(%s)：一轮应当在一个租期内结束", c.SyncBudget, c.SyncLockTTL)
	}
	return nil
}
