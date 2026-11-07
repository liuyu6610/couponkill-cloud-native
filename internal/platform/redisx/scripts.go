package redisx

import (
	"context"
	"crypto/sha1" //nolint:gosec // Redis 的 EVALSHA 协议规定用 SHA1 标识脚本，并非安全用途
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

// Script 是一段由内嵌服务提供的 Lua 脚本。调用走 EVALSHA；服务端丢失脚本缓存
// （重启、故障转移、SCRIPT FLUSH）时会透明地重新 SCRIPT LOAD 并重试一次。
type Script struct {
	name string
	src  string
	sha  string
	rdb  redis.Scripter
}

// Name 返回注册名。
func (s *Script) Name() string { return s.name }

// SHA 返回脚本的 SHA1（与 Redis 一致）。
func (s *Script) SHA() string { return s.sha }

// Run 执行脚本。返回的 *redis.Cmd 用 .Int64()/.Result()/.Err() 取结果。
func (s *Script) Run(ctx context.Context, keys []string, args ...any) *redis.Cmd {
	cmd := s.rdb.EvalSha(ctx, s.sha, keys, args...)
	err := cmd.Err()
	if err == nil || !strings.HasPrefix(err.Error(), "NOSCRIPT") {
		return cmd
	}
	if lerr := s.rdb.ScriptLoad(ctx, s.src).Err(); lerr != nil {
		failed := redis.NewCmd(ctx)
		failed.SetErr(fmt.Errorf("redisx: reload script %q: %w", s.name, lerr))
		return failed
	}
	return s.rdb.EvalSha(ctx, s.sha, keys, args...)
}

// Scripts 是一组脚本的注册表，便于启动时统一预热。
type Scripts struct {
	rdb redis.Scripter

	mu   sync.Mutex
	list []*Script
}

// NewScripts 创建注册表。
func NewScripts(rdb redis.Scripter) *Scripts { return &Scripts{rdb: rdb} }

// Add 注册一段脚本源码（通常来自 go:embed）并返回可调用的 Script。
func (r *Scripts) Add(name, src string) *Script {
	sum := sha1.Sum([]byte(src)) //nolint:gosec // 见上
	s := &Script{name: name, src: src, sha: hex.EncodeToString(sum[:]), rdb: r.rdb}
	r.mu.Lock()
	r.list = append(r.list, s)
	r.mu.Unlock()
	return s
}

// Preload 把所有已注册脚本 SCRIPT LOAD 到服务端，让语法错误在启动时暴露而不是在第一个请求上。
func (r *Scripts) Preload(ctx context.Context) error {
	r.mu.Lock()
	list := append([]*Script(nil), r.list...)
	r.mu.Unlock()
	for _, s := range list {
		if err := r.rdb.ScriptLoad(ctx, s.src).Err(); err != nil {
			return fmt.Errorf("redisx: preload script %q: %w", s.name, err)
		}
	}
	return nil
}
