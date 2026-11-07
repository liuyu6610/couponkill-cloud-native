package gateway_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/gateway"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
)

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := gateway.LoadConfig(config.WithMap(map[string]string{}))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.HTTPPort != 8088 {
		t.Errorf("HTTPPort = %d, want 8088", cfg.HTTPPort)
	}
	if cfg.UpstreamTimeout != 3*time.Second || cfg.SeckillTimeout != 2*time.Second {
		t.Errorf("timeouts = %v / %v, want 3s / 2s", cfg.UpstreamTimeout, cfg.SeckillTimeout)
	}
	if cfg.RedisTimeout != 50*time.Millisecond {
		t.Errorf("RedisTimeout = %v, want 50ms", cfg.RedisTimeout)
	}
	if !cfg.RevocationEnabled || !cfg.RevocationFailOpen {
		t.Errorf("revocation enabled/failOpen = %v/%v, want true/true", cfg.RevocationEnabled, cfg.RevocationFailOpen)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Errorf("TrustedProxies = %v, want none by default", cfg.TrustedProxies)
	}
	if cfg.MaxBodyBytes != 1<<20 {
		t.Errorf("MaxBodyBytes = %d, want 1MiB", cfg.MaxBodyBytes)
	}
}

func TestLoadConfig_Overrides(t *testing.T) {
	cfg, err := gateway.LoadConfig(config.WithMap(map[string]string{
		"CK_GATEWAY_CORS_ORIGINS":         "https://a.example.com, https://b.example.com:8443",
		"CK_GATEWAY_REVOCATION_ENABLED":   "false",
		"CK_GATEWAY_REVOCATION_FAIL_OPEN": "false",
		"CK_GATEWAY_TRUSTED_PROXIES":      "10.0.0.0/8,192.168.1.1",
		"CK_GATEWAY_SECKILL_RPS":          "0.5",
		"CK_HTTP_PORT":                    "9000",
	}))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := strings.Join(cfg.CORSOrigins, "|"); got != "https://a.example.com|https://b.example.com:8443" {
		t.Errorf("CORSOrigins = %q", got)
	}
	if cfg.RevocationEnabled || cfg.RevocationFailOpen {
		t.Errorf("revocation flags not overridden: %v/%v", cfg.RevocationEnabled, cfg.RevocationFailOpen)
	}
	if cfg.SeckillRate != 0.5 || cfg.HTTPPort != 9000 {
		t.Errorf("seckill rate %v port %d", cfg.SeckillRate, cfg.HTTPPort)
	}
}

func TestLoadConfig_RejectsBadValues(t *testing.T) {
	cases := map[string]map[string]string{
		"wildcard origin":      {"CK_GATEWAY_CORS_ORIGINS": "*"},
		"origin with path":     {"CK_GATEWAY_CORS_ORIGINS": "https://a.example.com/app"},
		"origin without host":  {"CK_GATEWAY_CORS_ORIGINS": "https://"},
		"bad trusted proxy":    {"CK_GATEWAY_TRUSTED_PROXIES": "not-an-ip"},
		"bad upstream url":     {"CK_ORDER_URL": "order-service:8082"},
		"upstream url scheme":  {"CK_USER_URL": "ftp://user:21"},
		"zero seckill burst":   {"CK_GATEWAY_SECKILL_BURST": "0"},
		"tiny body limit":      {"CK_GATEWAY_MAX_BODY_BYTES": "10"},
		"non numeric rate":     {"CK_GATEWAY_IP_RPS": "fast"},
		"prod without secrets": {"CK_ENV": "prod"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := gateway.LoadConfig(config.WithMap(env))
			if err == nil {
				t.Fatal("expected an error")
			}
			var ve *config.ValidationError
			if !errors.As(err, &ve) || len(ve.Problems) == 0 {
				t.Fatalf("want *config.ValidationError with problems, got %T %v", err, err)
			}
		})
	}
}

func TestLoadConfig_ReportsAllProblemsAtOnce(t *testing.T) {
	_, err := gateway.LoadConfig(config.WithMap(map[string]string{
		"CK_GATEWAY_CORS_ORIGINS":    "*",
		"CK_GATEWAY_TRUSTED_PROXIES": "nope",
		"CK_ORDER_URL":               "bad",
	}))
	var ve *config.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	if len(ve.Problems) < 3 {
		t.Errorf("want at least 3 problems reported together, got %v", ve.Problems)
	}
}
