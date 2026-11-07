package user_test

import (
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user"
)

func load(t *testing.T, env map[string]string) (user.Config, error) {
	t.Helper()
	var cfg user.Config
	err := config.Load(&cfg, config.WithDefaults(map[string]string{"CK_HTTP_PORT": user.DefaultPort}), config.WithMap(env))
	return cfg, err
}

func TestConfigDefaultsAndSeedSwitches(t *testing.T) {
	cfg, err := load(t, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPPort != 8081 || cfg.WantsSeedDemo() || cfg.IsProd() {
		t.Fatalf("defaults wrong: port=%d seed=%v prod=%v", cfg.HTTPPort, cfg.WantsSeedDemo(), cfg.IsProd())
	}
	for _, key := range []string{"CK_USER_SEED_DEMO", "CK_SEED_DEMO"} {
		cfg, err := load(t, map[string]string{key: "true"})
		if err != nil || !cfg.WantsSeedDemo() {
			t.Fatalf("%s=true: seed=%v err=%v", key, cfg.WantsSeedDemo(), err)
		}
	}
}

func TestConfigProdRejectsDevDefaultSecrets(t *testing.T) {
	if _, err := load(t, map[string]string{"CK_ENV": "prod"}); err == nil {
		t.Fatal("CK_ENV=prod 且未设置密钥时必须启动失败")
	}
	full := map[string]string{
		"CK_ENV":              "prod",
		"CK_PG_DSN":           "postgres://u:p@db:5432/couponkill",
		"CK_JWT_SECRET":       "prod-jwt-secret-0123456789-abcdefghijklmnop",
		"CK_INTERNAL_TOKEN":   "prod-internal-token-0123456789",
		"CK_GATEWAY_SIGN_KEY": "prod-gateway-sign-key-0123456789-abcdefghijkl",
	}
	if _, err := load(t, full); err != nil {
		t.Fatalf("显式设置全部密钥后应通过: %v", err)
	}
}
