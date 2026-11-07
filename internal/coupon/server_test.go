package coupon_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type fixture struct {
	pool *pgxpool.Pool
	mr   *miniredis.Miniredis
	res  coupon.Resources
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := dbxtest.New(t)
	rdb, mr := testkit.Redis(t)
	return &fixture{pool: pool, mr: mr, res: coupon.Resources{Pool: pool, Redis: rdb}}
}

func devConfig(t *testing.T, env map[string]string) coupon.Config {
	t.Helper()
	merged := map[string]string{"CK_INTERNAL_TOKEN": testkit.InternalToken, "CK_GATEWAY_SIGN_KEY": testkit.GatewayKey}
	for k, v := range env {
		merged[k] = v
	}
	var cfg coupon.Config
	if err := config.Load(&cfg, config.WithDefaults(map[string]string{"CK_HTTP_PORT": coupon.DefaultPort}), config.WithMap(merged)); err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

func count(t *testing.T, pool *pgxpool.Pool, sql string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func TestStartupMigratesSeedsThenPreheatsOnlyEnabledSeckillCoupons(t *testing.T) {
	f := newFixture(t)
	cfg := devConfig(t, map[string]string{"CK_SEED_DEMO": "true"})
	app, err := coupon.Build(context.Background(), cfg, f.res, quietLogger())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if n := count(t, f.pool, `SELECT count(*) FROM cpn.coupons`); n != 2 {
		t.Fatalf("seed coupons = %d, want 2", n)
	}
	if got, err := f.mr.Get(keys.Stock(1001)); err != nil || got != "1600" {
		t.Fatalf("ck:stock:1001 = %q,%v want 1600 (seed runs BEFORE preheat)", got, err)
	}
	if f.mr.Exists(keys.Stock(1002)) {
		t.Fatal("the normal coupon 1002 must not get a seckill counter")
	}

	// 种子券经内部接口可正常扣减，并且由真实装配的路由提供服务。
	res := testkit.DoJSON(t, app.Engine, "POST", internalapi.PathStockDeduct,
		map[string]any{"requestId": "seed-1", "couponId": "1001", "userId": "42"}, testkit.Internal())
	if res.Status != 200 {
		t.Fatalf("deduct against the seeded coupon: %d %s", res.Status, res.Body)
	}

	ready := testkit.DoJSON(t, app.Engine, "GET", "/readyz", nil)
	if ready.Status != 200 {
		t.Fatalf("/readyz: %d %s", ready.Status, ready.Body)
	}
}

func TestSeedIsIdempotentAndRestartKeepsInFlightCounter(t *testing.T) {
	f := newFixture(t)
	cfg := devConfig(t, map[string]string{"CK_SEED_DEMO": "true"})
	if _, err := coupon.Build(context.Background(), cfg, f.res, quietLogger()); err != nil {
		t.Fatal(err)
	}
	f.mr.Set(keys.Stock(1001), "1337") // 秒杀进行中，order 已预扣

	if _, err := coupon.Build(context.Background(), cfg, f.res, quietLogger()); err != nil {
		t.Fatalf("second Build (restart): %v", err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM cpn.coupons`); n != 2 {
		t.Fatalf("coupons = %d after restart, want 2", n)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM cpn.coupon_stock`); n != 16 {
		t.Fatalf("buckets = %d after restart, want 16", n)
	}
	if got, _ := f.mr.Get(keys.Stock(1001)); got != "1337" {
		t.Fatalf("a restart must not reset the in-flight counter, got %q", got)
	}
}

func TestWithoutSeedNothingIsCreatedOrPreheated(t *testing.T) {
	f := newFixture(t)
	if _, err := coupon.Build(context.Background(), devConfig(t, nil), f.res, quietLogger()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM cpn.coupons`); n != 0 {
		t.Fatalf("coupons = %d, want 0", n)
	}
	if keys := f.mr.Keys(); len(keys) != 0 {
		t.Fatalf("redis keys = %v, want none", keys)
	}
}

func TestStartupPreheatsPreexistingSeckillCouponsAndSkipsDisabled(t *testing.T) {
	f := newFixture(t)
	cfg := devConfig(t, map[string]string{"CK_SEED_DEMO": "true"})
	if _, err := coupon.Build(context.Background(), cfg, f.res, quietLogger()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE cpn.coupons SET status = 0 WHERE id = 1001`); err != nil {
		t.Fatal(err)
	}
	f.mr.Del(keys.Stock(1001))

	if _, err := coupon.Build(context.Background(), cfg, f.res, quietLogger()); err != nil {
		t.Fatal(err)
	}
	if f.mr.Exists(keys.Stock(1001)) {
		t.Fatal("a disabled seckill coupon must not be preheated")
	}
}

func TestStartupSurvivesRedisOutage(t *testing.T) {
	f := newFixture(t)
	f.mr.Close()
	cfg := devConfig(t, map[string]string{"CK_SEED_DEMO": "true"})
	app, err := coupon.Build(context.Background(), cfg, f.res, quietLogger())
	if err != nil {
		t.Fatalf("Build must keep going when only the preheat fails: %v", err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM cpn.coupons`); n != 2 {
		t.Fatalf("coupons = %d", n)
	}
	ready := testkit.DoJSON(t, app.Engine, "GET", "/readyz", nil)
	if ready.Status != 503 {
		t.Fatalf("/readyz must report the Redis outage, got %d", ready.Status)
	}
}

func TestConfigValidation(t *testing.T) {
	if err := devConfig(t, map[string]string{"CK_SEED_DEMO": "true"}).Validate(); err != nil {
		t.Fatalf("seed in dev is allowed: %v", err)
	}
	prod := devConfig(t, map[string]string{
		"CK_ENV": "prod", "CK_SEED_DEMO": "true",
		"CK_PG_DSN": "postgres://u:p@db:5432/couponkill", "CK_INTERNAL_TOKEN": "a-production-internal-token-0123456789",
		"CK_GATEWAY_SIGN_KEY": "a-production-gateway-sign-key-0123456789abcdef",
	})
	if err := prod.Validate(); err == nil {
		t.Fatal("CK_SEED_DEMO must be refused in prod")
	}
	if err := devConfig(t, map[string]string{"CK_COUPON_INPUT_TZ": "Mars/Olympus"}).Validate(); err == nil {
		t.Fatal("an unknown timezone must be refused")
	}
}

func TestConfigDefaultsAndBounds(t *testing.T) {
	cfg := devConfig(t, nil)
	if cfg.StockBuckets != 8 || cfg.HTTPPort != 8080 || cfg.SeedDemo || cfg.InputTZ != "Asia/Shanghai" {
		t.Fatalf("defaults: %+v", cfg)
	}
	var bad coupon.Config
	if err := config.Load(&bad, config.WithMap(map[string]string{"CK_COUPON_STOCK_BUCKETS": "0"})); err == nil {
		t.Fatal("0 buckets must be rejected")
	}
	if err := config.Load(&bad, config.WithMap(map[string]string{"CK_COUPON_STOCK_BUCKETS": "65"})); err == nil {
		t.Fatal("65 buckets must be rejected")
	}
}
