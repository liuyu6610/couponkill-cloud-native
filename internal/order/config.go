// Package order 装配 order 服务：环境变量配置（config.go）与路由/后台任务的组合（server.go）。
// 业务逻辑在子包：seckill（秒杀状态机）、orders（订单域），预约与通知由 reservation / notification 子包提供。
package order

import (
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
)

// Config 是 order 服务的环境变量配置（公共项见 internal/platform/README.md，私有项前缀 CK_ORDER_）。
type Config struct {
	config.Common
	config.Postgres
	config.Redis
	config.Kafka
	config.Internal
	config.GatewaySign
	config.Peers

	// SeckillCooldown 是同一用户对同一张券两次秒杀之间的冷却时间，0 关闭。
	SeckillCooldown time.Duration `env:"CK_ORDER_SECKILL_COOLDOWN" default:"0s"`
	// EnqueueTimeout 是秒杀入口等待 Kafka ack 的上限。
	EnqueueTimeout time.Duration `env:"CK_ORDER_ENQUEUE_TIMEOUT" default:"500ms" min:"10ms" max:"10s"`

	// ConsumeEnabled 控制本实例是否运行建单消费者（可把消费者拆到独立副本）。
	ConsumeEnabled bool `env:"CK_ORDER_CONSUME_ENABLED" default:"true"`
	// ConsumeConcurrency 是同时处理的分区数。
	ConsumeConcurrency int `env:"CK_ORDER_CONSUME_CONCURRENCY" default:"1" min:"1" max:"64"`
	// ConsumeMaxRetries 是消息处理失败后进入死信前的最大重试次数。
	ConsumeMaxRetries int `env:"CK_ORDER_CONSUME_MAX_RETRIES" default:"5" min:"1" max:"50"`

	// SweepEnabled 控制本实例是否运行超时清扫器（多实例靠分布式锁互斥）。
	SweepEnabled bool `env:"CK_ORDER_SWEEP_ENABLED" default:"true"`
	// SweepInterval 是清扫轮询间隔。
	SweepInterval time.Duration `env:"CK_ORDER_SWEEP_INTERVAL" default:"30s" min:"1s"`
	// SweepStale 是请求在途多久后算超时；必须远大于一次建单的最长耗时。
	SweepStale time.Duration `env:"CK_ORDER_SWEEP_STALE" default:"120s" min:"5s"`
	// SweepBatch 是每轮最多清扫的请求数。
	SweepBatch int `env:"CK_ORDER_SWEEP_BATCH" default:"100" min:"1" max:"5000"`

	// MaxActiveCoupons 是用户同时持有的券总数上限，0 关闭。
	MaxActiveCoupons int `env:"CK_ORDER_MAX_ACTIVE_COUPONS" default:"15" min:"0"`
	// MaxActiveSeckill 是用户同时持有的秒杀券数上限，0 关闭。
	MaxActiveSeckill int `env:"CK_ORDER_MAX_ACTIVE_SECKILL" default:"5" min:"0"`

	// MaxPageSize 是列表单页最大条数。
	MaxPageSize int `env:"CK_ORDER_MAX_PAGE_SIZE" default:"100" min:"1" max:"1000"`
	// AdminMaxOffset 是管理端订单列表的最大偏移，超过则要求缩小范围。
	AdminMaxOffset int `env:"CK_ORDER_ADMIN_MAX_OFFSET" default:"10000" min:"0"`
}
