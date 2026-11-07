package keys_test

import (
	"context"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

// 会话键、锁键由 redisx 写入、由各服务按 keys 读取，两处各写一遍字符串，任何一边改了前缀
// 都会让网关与下游对不上（整站登录失效 / 锁失效）。这里用真实写入验证二者始终一致。

func TestSessionKeyWrittenByRedisxMatchesContract(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	const jti = "jti-contract-1"
	if err := redisx.NewSessionStore(rdb).Open(context.Background(), jti, time.Minute); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !mr.Exists(keys.Session(jti)) {
		t.Fatalf("redisx 写入的会话键与 keys.Session(%q)=%q 不一致，现有键：%v", jti, keys.Session(jti), mr.Keys())
	}
}

func TestLockKeyWrittenByRedisxMatchesContract(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	const name = "contract-lock"
	l, err := redisx.NewLocker(rdb).Acquire(context.Background(), name, time.Minute)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer func() { _ = l.Release(context.Background()) }()
	if !mr.Exists(keys.Lock(name)) {
		t.Fatalf("redisx 写入的锁键与 keys.Lock(%q)=%q 不一致，现有键：%v", name, keys.Lock(name), mr.Keys())
	}
}
