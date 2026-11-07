// Package redisx 封装 go-redis：带合理超时的客户端、EVALSHA Lua 脚本注册表、分布式锁，
// 以及几个多个服务都会用到的小工具（就绪探测、JSON 缓存、会话存储）。
package redisx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
)

// Options 描述 Redis 连接。零值字段取默认值。
type Options struct {
	Addr     string
	Password string
	DB       int
	// PoolSize 连接池大小，默认 64。
	PoolSize int
	// DialTimeout / ReadTimeout / WriteTimeout 默认均为 2s；PoolTimeout 默认 3s。
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolTimeout  time.Duration
	// MaxRetries 命令失败后的自动重试次数。默认 -1（关闭）：热路径上的 Lua 预扣不是幂等的，
	// 连接中途断开时重试可能导致重复扣减；幂等性由上层（requestId）保证，需要重试的调用方显式处理。
	MaxRetries int
}

// New 按 Options 创建客户端（懒连接，不会立刻拨号）。
func New(o Options) *redis.Client {
	def := func(d *time.Duration, v time.Duration) {
		if *d <= 0 {
			*d = v
		}
	}
	def(&o.DialTimeout, 2*time.Second)
	def(&o.ReadTimeout, 2*time.Second)
	def(&o.WriteTimeout, 2*time.Second)
	def(&o.PoolTimeout, 3*time.Second)
	if o.PoolSize <= 0 {
		o.PoolSize = 64
	}
	if o.MaxRetries == 0 {
		o.MaxRetries = -1
	}
	return redis.NewClient(&redis.Options{
		Addr:            o.Addr,
		Password:        o.Password,
		DB:              o.DB,
		PoolSize:        o.PoolSize,
		DialTimeout:     o.DialTimeout,
		ReadTimeout:     o.ReadTimeout,
		WriteTimeout:    o.WriteTimeout,
		PoolTimeout:     o.PoolTimeout,
		MaxRetries:      o.MaxRetries,
		ConnMaxIdleTime: 5 * time.Minute,
	})
}

// FromConfig 用 config.Redis 创建客户端。
func FromConfig(c config.Redis) *redis.Client {
	return New(Options{Addr: c.RedisAddr, Password: c.RedisPassword.Reveal(), DB: c.RedisDB})
}

// PingFn 返回适合 httpx.Health.Add 的就绪检查函数。
func PingFn(rdb redis.Cmdable) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := rdb.Ping(ctx).Err(); err != nil {
			return fmt.Errorf("redis ping: %w", err)
		}
		return nil
	}
}

// IsNil 报告 err 是否是“键不存在”（redis.Nil）。
func IsNil(err error) bool { return errors.Is(err, redis.Nil) }

// GetJSON 读取并反序列化 JSON 值。键不存在返回 (零值, false, nil)；内容损坏返回错误。
func GetJSON[T any](ctx context.Context, rdb redis.Cmdable, key string) (T, bool, error) {
	var out T
	raw, err := rdb.Get(ctx, key).Bytes()
	if IsNil(err) {
		return out, false, nil
	}
	if err != nil {
		return out, false, fmt.Errorf("redis get %s: %w", key, err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, false, fmt.Errorf("redis decode %s: %w", key, err)
	}
	return out, true, nil
}

// SetJSON 序列化为 JSON 写入，ttl<=0 表示不过期。
func SetJSON(ctx context.Context, rdb redis.Cmdable, key string, v any, ttl time.Duration) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("redis encode %s: %w", key, err)
	}
	if err := rdb.Set(ctx, key, raw, ttl).Err(); err != nil {
		return fmt.Errorf("redis set %s: %w", key, err)
	}
	return nil
}

// SessionStore 用 Redis 实现 authx.Revoker：会话键 ck:sess:{jti}。
type SessionStore struct{ rdb redis.Cmdable }

// NewSessionStore 创建会话存储。
func NewSessionStore(rdb redis.Cmdable) *SessionStore { return &SessionStore{rdb: rdb} }

const sessionKeyPrefix = "ck:sess:"

// Open 登记会话；ttl 必须为正。
func (s *SessionStore) Open(ctx context.Context, jti string, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("redisx: session ttl must be positive, got %v", ttl)
	}
	if err := s.rdb.Set(ctx, sessionKeyPrefix+jti, 1, ttl).Err(); err != nil {
		return fmt.Errorf("redis open session: %w", err)
	}
	return nil
}

// Revoke 删除会话；不存在不是错误。
func (s *SessionStore) Revoke(ctx context.Context, jti string) error {
	if err := s.rdb.Del(ctx, sessionKeyPrefix+jti).Err(); err != nil {
		return fmt.Errorf("redis revoke session: %w", err)
	}
	return nil
}

// Active 报告会话是否存在。
func (s *SessionStore) Active(ctx context.Context, jti string) (bool, error) {
	n, err := s.rdb.Exists(ctx, sessionKeyPrefix+jti).Result()
	if err != nil {
		return false, fmt.Errorf("redis check session: %w", err)
	}
	return n == 1, nil
}
