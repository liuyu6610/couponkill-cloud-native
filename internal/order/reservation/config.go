package reservation

import (
	"errors"
	"fmt"
	"time"
)

// Config 是预约调度器的配置（环境变量 CK_ORDER_RESERVATION_*）。
// order 服务把它内嵌进自己的配置结构体，由 config.Load 统一加载与校验。
type Config struct {
	// Enabled 是否启用到点调度（false 时 Scheduler.Run 空转到 ctx 取消；HTTP 接口不受影响）。
	Enabled bool `env:"CK_ORDER_RESERVATION_ENABLED" default:"true"`
	// ScanInterval 是调度扫描间隔。
	ScanInterval time.Duration `env:"CK_ORDER_RESERVATION_SCAN_INTERVAL" default:"1s" min:"10ms"`
	// BatchSize 是每次认领/回收/回查的最大条数；认领满一批会立即再认领，直到清空。
	BatchSize int `env:"CK_ORDER_RESERVATION_BATCH_SIZE" default:"50" min:"1" max:"1000"`
	// Concurrency 是单个实例并发调用秒杀入口的 worker 数。
	Concurrency int `env:"CK_ORDER_RESERVATION_CONCURRENCY" default:"4" min:"1" max:"64"`
	// MaxRetry 是单条预约调用秒杀入口的最大尝试次数（含首次，沿用 Java couponkill.reservation.max-retry 的语义）：
	// 第 MaxRetry 次仍是瞬态失败则置 FAILED。
	MaxRetry int `env:"CK_ORDER_RESERVATION_MAX_RETRY" default:"3" min:"1" max:"20"`
	// RetryBackoff 是首次重试前的退避，之后每次翻倍，上限 1 分钟。
	RetryBackoff time.Duration `env:"CK_ORDER_RESERVATION_RETRY_BACKOFF" default:"2s" min:"1ms"`
	// EnterTimeout 是单次调用秒杀入口的超时；必须小于 StaleFiringTimeout。
	EnterTimeout time.Duration `env:"CK_ORDER_RESERVATION_ENTER_TIMEOUT" default:"10s" min:"10ms"`
	// StaleFiringTimeout 是 FIRING 状态的最长停留时间：超过它视为认领方已崩溃，回收后重试。
	StaleFiringTimeout time.Duration `env:"CK_ORDER_RESERVATION_STALE_FIRING_TIMEOUT" default:"2m" min:"1s"`
	// QueuedSyncAfter 是 QUEUED 预约在多久没有结果后开始主动回查（仅配置了 ResultReader 时生效），
	// 回查过但仍无结果的预约会再等一个同样的间隔才会被回查。
	QueuedSyncAfter time.Duration `env:"CK_ORDER_RESERVATION_QUEUED_SYNC_AFTER" default:"1m" min:"1s"`
}

// maxRetryDelay 是退避的上限。
const maxRetryDelay = time.Minute

// Validate 校验配置的取值范围与相互约束（config.Load 的 tag 只能做单字段校验）。
func (c Config) Validate() error {
	var errs []error
	positive := func(name string, ok bool) {
		if !ok {
			errs = append(errs, fmt.Errorf("%s 必须为正数", name))
		}
	}
	positive("ScanInterval", c.ScanInterval > 0)
	positive("BatchSize", c.BatchSize > 0)
	positive("Concurrency", c.Concurrency > 0)
	positive("MaxRetry", c.MaxRetry > 0)
	positive("RetryBackoff", c.RetryBackoff > 0)
	positive("EnterTimeout", c.EnterTimeout > 0)
	positive("StaleFiringTimeout", c.StaleFiringTimeout > 0)
	positive("QueuedSyncAfter", c.QueuedSyncAfter > 0)
	if c.EnterTimeout > 0 && c.StaleFiringTimeout <= c.EnterTimeout {
		errs = append(errs, fmt.Errorf("StaleFiringTimeout(%s) 必须大于 EnterTimeout(%s)，否则仍在调用入口的认领会被误判为崩溃", c.StaleFiringTimeout, c.EnterTimeout))
	}
	return errors.Join(errs...)
}

// DefaultConfig 返回与 env tag 默认值一致的配置（测试与手工装配用）。
func DefaultConfig() Config {
	return Config{
		Enabled: true, ScanInterval: time.Second, BatchSize: 50, Concurrency: 4, MaxRetry: 3,
		RetryBackoff: 2 * time.Second, EnterTimeout: 10 * time.Second,
		StaleFiringTimeout: 2 * time.Minute, QueuedSyncAfter: time.Minute,
	}
}

// retryDelay 返回第 retryCount 次失败（从 0 计）之后的退避：RetryBackoff * 2^retryCount，封顶 maxRetryDelay。
func (c Config) retryDelay(retryCount int) time.Duration {
	d := c.RetryBackoff
	for i := 0; i < retryCount && d < maxRetryDelay; i++ {
		d *= 2
	}
	return min(d, maxRetryDelay)
}
