package connector_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func envMap(extra map[string]string) config.Option {
	m := map[string]string{"CK_ENV": "dev"}
	for k, v := range extra {
		m[k] = v
	}
	return config.WithMap(m)
}

func TestLoadConfigDefaultsAndPort(t *testing.T) {
	cfg, err := connector.LoadConfig(envMap(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPPort != 8085 || !cfg.SyncEnabled || cfg.SyncInterval != time.Minute || cfg.SyncBudget != 30*time.Second ||
		cfg.SyncLockTTL != 60*time.Second || cfg.MockStock != 500 {
		t.Fatalf("defaults = %+v", cfg)
	}
	if cfg.JDAppKey != "" || cfg.TBAppKey != "" || cfg.PDDAppKey != "" {
		t.Fatal("平台凭证不能有默认值（缺凭证即禁用）")
	}
}

func TestLoadConfigProdDoesNotDemandPlatformCredentials(t *testing.T) {
	_, err := connector.LoadConfig(envMap(map[string]string{
		"CK_ENV": "prod", "CK_JWT_SECRET": strings.Repeat("j", 40), "CK_INTERNAL_TOKEN": strings.Repeat("i", 24),
		"CK_GATEWAY_SIGN_KEY": strings.Repeat("g", 40), "CK_PG_DSN": "postgres://u:p@h/db",
	}))
	if err != nil {
		t.Fatalf("prod without platform credentials must start (platforms just stay DISABLED): %v", err)
	}
}

func TestLoadConfigRejectsBudgetNotBelowLockTTL(t *testing.T) {
	_, err := connector.LoadConfig(envMap(map[string]string{"CK_CONNECTOR_SYNC_BUDGET": "60s", "CK_CONNECTOR_SYNC_LOCK_TTL": "60s"}))
	if err == nil || !strings.Contains(err.Error(), "CK_CONNECTOR_SYNC_BUDGET") {
		t.Fatalf("err = %v", err)
	}
	if _, err := connector.LoadConfig(envMap(map[string]string{"CK_CONNECTOR_SYNC_LOCK_TTL": "5s"})); err == nil {
		t.Fatal("lock ttl below 10s must be rejected")
	}
}

func TestConfigNeverPrintsPlatformSecrets(t *testing.T) {
	cfg, err := connector.LoadConfig(envMap(map[string]string{
		"CK_CONNECTOR_JD_APP_KEY": "JDKEY-visible?", "CK_CONNECTOR_JD_APP_SECRET": "JDSECRET-xyz", "CK_CONNECTOR_JD_ACCESS_TOKEN": "JDTOKEN-xyz",
		"CK_CONNECTOR_TB_APP_SECRET": "TBSECRET-xyz", "CK_CONNECTOR_PDD_APP_SECRET": "PDDSECRET-xyz",
	}))
	if err != nil {
		t.Fatal(err)
	}
	summary := config.Summary(&cfg)
	for _, secret := range []string{"JDKEY-visible", "JDSECRET", "JDTOKEN", "TBSECRET", "PDDSECRET"} {
		for k, v := range summary {
			if strings.Contains(v, secret) {
				t.Fatalf("config summary leaks %s via %s=%s", secret, k, v)
			}
		}
	}
}

// Build 的端到端装配：迁移 cnx schema、起得来、/readyz 检查 PG+Redis、定时任务默认挂上，并且可被配置关闭。
func TestBuildWiresTheServiceAgainstRealPostgresAndRedis(t *testing.T) {
	dsn := dbxtest.NewDSN(t)
	_, mr := testkit.Redis(t)
	cfg, err := connector.LoadConfig(envMap(map[string]string{"CK_PG_DSN": dsn, "CK_REDIS_ADDR": mr.Addr()}))
	if err != nil {
		t.Fatal(err)
	}

	rt, err := connector.Build(context.Background(), cfg, discardLogger())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() {
		for i := len(rt.OnClose) - 1; i >= 0; i-- {
			_ = rt.OnClose[i](context.Background())
		}
	})

	if len(rt.Workers) != 1 || rt.Workers[0].Name != "stock-sync" {
		t.Fatalf("workers = %+v", rt.Workers)
	}
	if res := testkit.DoJSON(t, rt.Engine, "GET", "/readyz", nil); res.Status != 200 {
		t.Fatalf("/readyz = %d %s", res.Status, res.Body)
	}
	hs := testkit.DoJSON(t, rt.Engine, "GET", connector.BasePath+"/health", nil)
	if hs.Status != 200 || !strings.Contains(string(hs.Body), `"platform":"JD","status":"DISABLED"`) {
		t.Fatalf("health = %d %s: 缺凭证的 JD 必须是 DISABLED", hs.Status, hs.Body)
	}
	// 迁移确实跑过：cnx.platform_sku_bindings 存在（再迁移一次是幂等的）。
	pool, err := dbx.Open(context.Background(), dsn, dbx.Options{SlowQueryThreshold: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM cnx.platform_sku_bindings`).Scan(&n); err != nil {
		t.Fatalf("cnx schema not migrated: %v", err)
	}

	cfg.SyncEnabled = false
	rt2, err := connector.Build(context.Background(), cfg, discardLogger())
	if err != nil {
		t.Fatalf("second Build (migration must be idempotent): %v", err)
	}
	defer func() {
		for i := len(rt2.OnClose) - 1; i >= 0; i-- {
			_ = rt2.OnClose[i](context.Background())
		}
	}()
	if len(rt2.Workers) != 0 {
		t.Fatalf("workers with sync disabled = %+v", rt2.Workers)
	}
}

func TestBuildFailsCleanlyWhenPostgresIsUnreachable(t *testing.T) {
	_, mr := testkit.Redis(t)
	cfg, err := connector.LoadConfig(envMap(map[string]string{
		"CK_PG_DSN": "postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1", "CK_REDIS_ADDR": mr.Addr(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connector.Build(context.Background(), cfg, discardLogger()); err == nil {
		t.Fatal("Build must fail when PG is unreachable")
	}
}
