// Package testkit 汇集其他包写测试时用到的替身与辅助：miniredis、内存 Kafka、信封断言、
// 假时钟，以及带有效网关签名的请求构造器。
//
// 本包只能被测试代码引用。它刻意不依赖 httpx/app/redisx，因此这些包的外部测试包（xxx_test）
// 可以放心 import testkit 而不会形成环。
package testkit

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// Redis 启动一个 miniredis（支持 Lua、EVALSHA）并返回连到它的 go-redis 客户端。
// 两者都在 t.Cleanup 时关闭。miniredis 的 TTL 不随真实时间流逝，需用 mr.FastForward 推进。
func Redis(t testing.TB) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb, mr
}
