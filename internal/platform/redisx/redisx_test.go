package redisx_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

// ---------- 客户端构造 ----------

func TestNewAppliesSaneDefaults(t *testing.T) {
	rdb := redisx.New(redisx.Options{Addr: "127.0.0.1:1"})
	defer rdb.Close()
	o := rdb.Options()
	if o.DialTimeout <= 0 || o.ReadTimeout <= 0 || o.WriteTimeout <= 0 || o.PoolSize <= 0 {
		t.Fatalf("timeouts and pool must be bounded: %+v", o)
	}
	if o.MaxRetries != 0 { // go-redis 把 -1（关闭）规整为 0；默认值应为 3
		t.Fatalf("automatic command retries must be off by default (non-idempotent Lua), got %d", o.MaxRetries)
	}
}

func TestFromConfigMapsFields(t *testing.T) {
	rdb := redisx.FromConfig(config.Redis{RedisAddr: "10.0.0.1:6380", RedisPassword: "pw", RedisDB: 3})
	defer rdb.Close()
	o := rdb.Options()
	if o.Addr != "10.0.0.1:6380" || o.Password != "pw" || o.DB != 3 {
		t.Fatalf("FromConfig lost fields: %+v", o)
	}
}

func TestPingFnAndIsNil(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	if err := redisx.PingFn(rdb)(context.Background()); err != nil {
		t.Fatalf("PingFn: %v", err)
	}
	if _, err := rdb.Get(context.Background(), "missing").Result(); !redisx.IsNil(err) {
		t.Fatalf("IsNil must recognise redis.Nil, got %v", err)
	}
	if redisx.IsNil(errors.New("x")) || redisx.IsNil(nil) {
		t.Fatal("IsNil false positives")
	}
	mr.Close()
	if err := redisx.PingFn(rdb)(context.Background()); err == nil {
		t.Fatal("PingFn must fail when redis is down")
	}
}

func TestJSONHelpers(t *testing.T) {
	rdb, _ := testkit.Redis(t)
	ctx := context.Background()
	type meta struct {
		Status int    `json:"status"`
		Name   string `json:"name"`
	}
	if _, found, err := redisx.GetJSON[meta](ctx, rdb, "ck:cpn:meta:1"); err != nil || found {
		t.Fatalf("miss must be (zero,false,nil), got found=%v err=%v", found, err)
	}
	if err := redisx.SetJSON(ctx, rdb, "ck:cpn:meta:1", meta{1, "新人券"}, 5*time.Minute); err != nil {
		t.Fatal(err)
	}
	got, found, err := redisx.GetJSON[meta](ctx, rdb, "ck:cpn:meta:1")
	if err != nil || !found || got != (meta{1, "新人券"}) {
		t.Fatalf("round trip failed: %+v %v %v", got, found, err)
	}
	if ttl := rdb.TTL(ctx, "ck:cpn:meta:1").Val(); ttl != 5*time.Minute {
		t.Fatalf("ttl = %v", ttl)
	}
	rdb.Set(ctx, "bad", "{not json", 0)
	if _, _, err := redisx.GetJSON[meta](ctx, rdb, "bad"); err == nil {
		t.Fatal("corrupt cache entry must surface as an error")
	}
}

// ---------- Lua 脚本 ----------

const incrIfExists = `
if redis.call('EXISTS', KEYS[1]) == 1 then return redis.call('INCRBY', KEYS[1], ARGV[1]) end
return -1`

func TestScriptRunsViaEvalShaAndReloadsOnNoScript(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	ctx := context.Background()
	scripts := redisx.NewScripts(rdb)
	incr := scripts.Add("incr", incrIfExists)

	if n, err := incr.Run(ctx, []string{"k"}, 5).Int64(); err != nil || n != -1 {
		t.Fatalf("missing key: %d %v", n, err)
	}
	mr.Set("k", "10")
	if n, err := incr.Run(ctx, []string{"k"}, 5).Int64(); err != nil || n != 15 {
		t.Fatalf("incr: %d %v", n, err)
	}

	// 服务端 SCRIPT FLUSH（如 Redis 重启/故障转移）之后，调用方不应感知到 NOSCRIPT。
	if err := rdb.ScriptFlush(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if n, err := incr.Run(ctx, []string{"k"}, 1).Int64(); err != nil || n != 16 {
		t.Fatalf("after flush: %d %v (NOSCRIPT must be handled transparently)", n, err)
	}
	if ok := rdb.ScriptExists(ctx, incr.SHA()).Val(); len(ok) != 1 || !ok[0] {
		t.Fatal("script must be cached again after the transparent reload")
	}
}

func TestScriptPropagatesRealErrors(t *testing.T) {
	rdb, _ := testkit.Redis(t)
	s := redisx.NewScripts(rdb).Add("bad", `return redis.call('INCR', KEYS[1])`)
	rdb.Set(context.Background(), "str", "not-a-number", 0)
	if err := s.Run(context.Background(), []string{"str"}).Err(); err == nil {
		t.Fatal("a script runtime error must be returned, not swallowed")
	}
}

func TestScriptsPreload(t *testing.T) {
	rdb, _ := testkit.Redis(t)
	ctx := context.Background()
	scripts := redisx.NewScripts(rdb)
	a := scripts.Add("a", `return 1`)
	b := scripts.Add("b", `return 2`)
	if err := scripts.Preload(ctx); err != nil {
		t.Fatal(err)
	}
	if ok := rdb.ScriptExists(ctx, a.SHA(), b.SHA()).Val(); !ok[0] || !ok[1] {
		t.Fatalf("Preload must cache every script, got %v", ok)
	}
	bad := redisx.NewScripts(rdb)
	bad.Add("broken", `this is not lua`)
	if err := bad.Preload(ctx); err == nil {
		t.Fatal("Preload must surface syntax errors at startup")
	}
}

func TestScriptNameAndDuplicateRegistration(t *testing.T) {
	rdb, _ := testkit.Redis(t)
	scripts := redisx.NewScripts(rdb)
	s := scripts.Add("enter", `return 1`)
	if s.Name() != "enter" || len(s.SHA()) != 40 {
		t.Fatalf("metadata wrong: %s %s", s.Name(), s.SHA())
	}
}

// ---------- 分布式锁 ----------

func TestLockAcquireExcludesOthersUntilRelease(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	ctx := context.Background()
	lk := redisx.NewLocker(rdb)

	l1, err := lk.Acquire(ctx, "seckill-sweeper", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !mr.Exists("ck:lock:seckill-sweeper") {
		t.Fatal("lock key must be ck:lock:{name}")
	}
	if _, err := lk.Acquire(ctx, "seckill-sweeper", time.Second); !errors.Is(err, redisx.ErrNotAcquired) {
		t.Fatalf("second Acquire must fail with ErrNotAcquired, got %v", err)
	}
	if other, err := lk.Acquire(ctx, "other", time.Second); err != nil || other == nil {
		t.Fatalf("different names must not conflict: %v", err)
	}
	if err := l1.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := lk.Acquire(ctx, "seckill-sweeper", time.Second); err != nil {
		t.Fatalf("Acquire after Release must succeed: %v", err)
	}
}

func TestLockReleaseIsTokenChecked(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	ctx := context.Background()
	lk := redisx.NewLocker(rdb)

	a, _ := lk.Acquire(ctx, "job", 10*time.Second)
	mr.FastForward(11 * time.Second) // A 的锁过期
	b, err := lk.Acquire(ctx, "job", 10*time.Second)
	if err != nil {
		t.Fatalf("B must get the expired lock: %v", err)
	}
	if err := a.Release(ctx); !errors.Is(err, redisx.ErrLockLost) {
		t.Fatalf("A releasing someone else's lock must return ErrLockLost, got %v", err)
	}
	if !mr.Exists("ck:lock:job") {
		t.Fatal("A's stale Release must not delete B's lock")
	}
	if _, err := lk.Acquire(ctx, "job", time.Second); !errors.Is(err, redisx.ErrNotAcquired) {
		t.Fatal("B must still hold the lock")
	}
	if err := b.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestLockRenewExtendsOnlyForOwner(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	ctx := context.Background()
	lk := redisx.NewLocker(rdb)

	a, _ := lk.Acquire(ctx, "job", 10*time.Second)
	mr.FastForward(8 * time.Second)
	if err := a.Renew(ctx, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(8 * time.Second) // 距首次获取 16s，若没续租早已过期
	if _, err := lk.Acquire(ctx, "job", time.Second); !errors.Is(err, redisx.ErrNotAcquired) {
		t.Fatal("renewed lock must still be held")
	}
	mr.FastForward(3 * time.Second) // 过期
	if err := a.Renew(ctx, 10*time.Second); !errors.Is(err, redisx.ErrLockLost) {
		t.Fatalf("renewing an expired lock must return ErrLockLost, got %v", err)
	}
	b, _ := lk.Acquire(ctx, "job", 10*time.Second)
	if err := a.Renew(ctx, 10*time.Second); !errors.Is(err, redisx.ErrLockLost) {
		t.Fatalf("renewing a lock now owned by B must return ErrLockLost, got %v", err)
	}
	_ = b
}

func TestLockMutualExclusionUnderContention(t *testing.T) {
	rdb, _ := testkit.Redis(t)
	lk := redisx.NewLocker(rdb)
	const workers, rounds = 12, 15
	var (
		inside   atomic.Int32
		maxSeen  atomic.Int32
		counter  int // 故意不加原子/互斥：锁若失效，-race 或结果校验会暴露
		wg       sync.WaitGroup
		failures atomic.Int32
	)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				var l *redisx.Lock
				for {
					var err error
					l, err = lk.Acquire(context.Background(), "hot", 30*time.Second)
					if err == nil {
						break
					}
					if !errors.Is(err, redisx.ErrNotAcquired) {
						failures.Add(1)
						return
					}
					time.Sleep(time.Millisecond)
				}
				n := inside.Add(1)
				if n > maxSeen.Load() {
					maxSeen.Store(n)
				}
				v := counter
				time.Sleep(50 * time.Microsecond) // 放大竞态窗口
				counter = v + 1
				inside.Add(-1)
				if err := l.Release(context.Background()); err != nil {
					failures.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("%d unexpected errors", failures.Load())
	}
	if maxSeen.Load() != 1 {
		t.Fatalf("critical section was entered concurrently (max %d)", maxSeen.Load())
	}
	if counter != workers*rounds {
		t.Fatalf("lost updates: counter=%d want %d", counter, workers*rounds)
	}
}

func TestLockRunExecutesAndReleases(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	lk := redisx.NewLocker(rdb)
	ran := false
	err := lk.Run(context.Background(), "sync", time.Minute, func(ctx context.Context) error {
		ran = true
		if !mr.Exists("ck:lock:sync") {
			t.Error("lock must be held while fn runs")
		}
		return nil
	})
	if err != nil || !ran {
		t.Fatalf("Run: ran=%v err=%v", ran, err)
	}
	if mr.Exists("ck:lock:sync") {
		t.Fatal("Run must release the lock afterwards")
	}
	// fn 的错误原样返回，锁照样释放。
	boom := errors.New("boom")
	if err := lk.Run(context.Background(), "sync", time.Minute, func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("fn error must propagate, got %v", err)
	}
	if mr.Exists("ck:lock:sync") {
		t.Fatal("lock must be released even when fn fails")
	}
}

func TestLockRunSkipsWhenHeldElsewhere(t *testing.T) {
	rdb, _ := testkit.Redis(t)
	lk := redisx.NewLocker(rdb)
	if _, err := lk.Acquire(context.Background(), "sync", time.Minute); err != nil {
		t.Fatal(err)
	}
	called := false
	err := lk.Run(context.Background(), "sync", time.Minute, func(context.Context) error { called = true; return nil })
	if !errors.Is(err, redisx.ErrNotAcquired) || called {
		t.Fatalf("Run must not call fn when the lock is held: err=%v called=%v", err, called)
	}
}

func TestLockRunRenewsWhileRunning(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	lk := redisx.NewLocker(rdb)
	const ttl = 90 * time.Millisecond
	err := lk.Run(context.Background(), "long", ttl, func(ctx context.Context) error {
		mr.SetTTL("ck:lock:long", 5*time.Millisecond) // 人为把剩余寿命压低
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if mr.TTL("ck:lock:long") == ttl {
				return nil // 续租把它重置回了完整 ttl
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Error("lock was not renewed in the background")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLockRunCancelsFnWhenLockIsLost(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	lk := redisx.NewLocker(rdb)
	err := lk.Run(context.Background(), "steal", 60*time.Millisecond, func(ctx context.Context) error {
		mr.Del("ck:lock:steal") // 锁被抢走/过期
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
			t.Error("fn context was not cancelled after the lock was lost")
			return nil
		}
	})
	if !errors.Is(err, redisx.ErrLockLost) {
		t.Fatalf("Run must report ErrLockLost, got %v", err)
	}
}

func TestLockRunStopsWhenParentContextCancelled(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	lk := redisx.NewLocker(rdb)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- lk.Run(ctx, "cancelled", time.Minute, func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation (goroutine leak)")
	}
	if mr.Exists("ck:lock:cancelled") {
		t.Fatal("lock must be released even though the parent ctx was cancelled")
	}
}

// ---------- 会话存储 ----------

func TestSessionStoreImplementsRevoker(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	var store authx.Revoker = redisx.NewSessionStore(rdb)
	ctx := context.Background()

	if ok, err := store.Active(ctx, "j1"); err != nil || ok {
		t.Fatalf("unknown session: %v %v", ok, err)
	}
	if err := store.Open(ctx, "j1", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := store.Open(ctx, "j2", time.Hour); err != nil {
		t.Fatal(err)
	}
	if !mr.Exists("ck:sess:j1") || mr.TTL("ck:sess:j1") != time.Hour {
		t.Fatalf("key/ttl wrong: ttl=%v", mr.TTL("ck:sess:j1"))
	}
	if ok, _ := store.Active(ctx, "j1"); !ok {
		t.Fatal("j1 must be active")
	}
	if err := store.Revoke(ctx, "j1"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := store.Active(ctx, "j1"); ok {
		t.Fatal("revoked j1 must be inactive")
	}
	if ok, _ := store.Active(ctx, "j2"); !ok {
		t.Fatal("revoking j1 must not touch j2")
	}
	if err := store.Revoke(ctx, "nope"); err != nil {
		t.Fatalf("revoking unknown jti is a no-op: %v", err)
	}
	if err := store.Open(ctx, "j3", 0); err == nil {
		t.Fatal("a non-positive ttl must be rejected (would create an eternal or invalid session)")
	}
	mr.Close()
	if _, err := store.Active(ctx, "j2"); err == nil {
		t.Fatal("redis outage must surface as an error so the caller can pick fail-open/closed")
	}
}

var _ redis.Scripter = (*redis.Client)(nil)
