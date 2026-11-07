package config_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
)

type svcConfig struct {
	config.Common
	config.Redis
	Name     string        `env:"CK_T_NAME" default:"svc"`
	Enabled  bool          `env:"CK_T_ENABLED" default:"false"`
	Workers  int           `env:"CK_T_WORKERS" default:"4" min:"1" max:"64"`
	Ratio    float64       `env:"CK_T_RATIO" default:"0.5"`
	Timeout  time.Duration `env:"CK_T_TIMEOUT" default:"3s" min:"1ms"`
	Origins  []string      `env:"CK_T_ORIGINS" default:"a, b ,c"`
	Limit    uint          `env:"CK_T_LIMIT" default:"7"`
	Token    config.Secret `env:"CK_T_TOKEN" default:"dev-token-dev-token-dev-token-0000" min:"32"`
	Required string        `env:"CK_T_REQUIRED" required:"true"`
	Mode     string        `env:"CK_T_MODE" default:"fast" oneof:"fast,slow"`
}

func load(t *testing.T, env map[string]string, opts ...config.Option) (svcConfig, error) {
	t.Helper()
	var c svcConfig
	opts = append([]config.Option{config.WithMap(env)}, opts...)
	err := config.Load(&c, opts...)
	return c, err
}

func TestLoadAppliesDefaultsAndTypedParsing(t *testing.T) {
	c, err := load(t, map[string]string{"CK_T_REQUIRED": "x"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Name != "svc" || c.Enabled || c.Workers != 4 || c.Ratio != 0.5 || c.Limit != 7 {
		t.Fatalf("defaults wrong: %+v", c)
	}
	if c.Timeout != 3*time.Second {
		t.Fatalf("duration default = %v", c.Timeout)
	}
	if got := strings.Join(c.Origins, "|"); got != "a|b|c" {
		t.Fatalf("list default = %q (items must be trimmed)", got)
	}
	if c.Env != "dev" || c.HTTPPort != 8080 || c.LogLevel != "info" || c.WorkerID != -1 {
		t.Fatalf("Common defaults wrong: %+v", c.Common)
	}
	if c.RedisAddr != "127.0.0.1:6379" || c.RedisDB != 0 {
		t.Fatalf("Redis defaults wrong: %+v", c.Redis)
	}
}

func TestLoadOverridesFromEnvironment(t *testing.T) {
	c, err := load(t, map[string]string{
		"CK_T_REQUIRED": "x", "CK_T_NAME": "order", "CK_T_ENABLED": "true", "CK_T_WORKERS": "8",
		"CK_T_TIMEOUT": "250ms", "CK_T_ORIGINS": "https://a.example,https://b.example", "CK_HTTP_PORT": "9000",
		"CK_T_MODE": "slow", "CK_T_TOKEN": "an-explicit-token-an-explicit-token-1",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Name != "order" || !c.Enabled || c.Workers != 8 || c.Timeout != 250*time.Millisecond || c.HTTPPort != 9000 || c.Mode != "slow" {
		t.Fatalf("overrides not applied: %+v", c)
	}
	if len(c.Origins) != 2 || c.Token.Reveal() != "an-explicit-token-an-explicit-token-1" {
		t.Fatalf("list/secret wrong: %+v", c)
	}
}

func TestWithDefaultsOverridesTagDefault(t *testing.T) {
	c, err := load(t, map[string]string{"CK_T_REQUIRED": "x"}, config.WithDefaults(map[string]string{"CK_HTTP_PORT": "8082"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HTTPPort != 8082 {
		t.Fatalf("HTTPPort = %d, want 8082", c.HTTPPort)
	}
	c, err = load(t, map[string]string{"CK_T_REQUIRED": "x", "CK_HTTP_PORT": "1234"}, config.WithDefaults(map[string]string{"CK_HTTP_PORT": "8082"}))
	if err != nil || c.HTTPPort != 1234 {
		t.Fatalf("explicit env must beat WithDefaults, got %d err=%v", c.HTTPPort, err)
	}
}

func TestLoadReportsAllProblemsAtOnce(t *testing.T) {
	_, err := load(t, map[string]string{
		"CK_T_WORKERS": "0", "CK_T_TIMEOUT": "forever", "CK_T_MODE": "weird", "CK_T_ENABLED": "maybe", "CK_ENV": "staging",
	})
	var ve *config.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *ValidationError, got %T %v", err, err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{"CK_T_REQUIRED", "CK_T_WORKERS", "CK_T_TIMEOUT", "CK_T_MODE", "CK_T_ENABLED", "CK_ENV"} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems should mention %s, got:\n%s", want, joined)
		}
	}
	if len(ve.Problems) != 6 {
		t.Errorf("want exactly 6 problems, got %d:\n%s", len(ve.Problems), joined)
	}
}

func TestLoadRejectsNonStructPointer(t *testing.T) {
	var s string
	if err := config.Load(&s, config.WithMap(nil)); err == nil {
		t.Fatal("Load into *string must fail")
	}
	if err := config.Load(svcConfig{}, config.WithMap(nil)); err == nil {
		t.Fatal("Load into a non-pointer must fail")
	}
}

func TestProdRejectsMissingDefaultAndShortSecrets(t *testing.T) {
	base := map[string]string{"CK_ENV": "prod", "CK_T_REQUIRED": "x"}

	// 默认密钥（未设置）在 prod 下必须失败，且 JWT / 内部令牌等都要报出来。
	var full struct {
		config.Common
		config.Postgres
		config.JWT
		config.Internal
		config.GatewaySign
	}
	err := config.Load(&full, config.WithMap(map[string]string{"CK_ENV": "prod"}))
	var ve *config.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("prod with defaults must fail, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{"CK_PG_DSN", "CK_JWT_SECRET", "CK_INTERNAL_TOKEN", "CK_GATEWAY_SIGN_KEY"} {
		if !strings.Contains(joined, want) {
			t.Errorf("prod problems should mention %s, got:\n%s", want, joined)
		}
	}
	// 报错信息里绝不能出现默认密钥本身。
	if strings.Contains(joined, "dev-only") {
		t.Errorf("problems must not echo secret values:\n%s", joined)
	}

	// 显式设置成默认值也不行。
	var c svcConfig
	env := map[string]string{"CK_ENV": "prod", "CK_T_REQUIRED": "x", "CK_T_TOKEN": "dev-token-dev-token-dev-token-0000"}
	if err := config.Load(&c, config.WithMap(env)); err == nil {
		t.Fatal("prod must reject a secret equal to its dev default")
	}

	// 太短也不行。
	env["CK_T_TOKEN"] = "short"
	if err := config.Load(&c, config.WithMap(env)); err == nil {
		t.Fatal("prod must reject a short secret")
	}

	// 正确设置则通过。
	base["CK_T_TOKEN"] = "a-strong-production-token-0123456789"
	if err := config.Load(&c, config.WithMap(base)); err != nil {
		t.Fatalf("prod with strong secret must pass: %v", err)
	}
	if !c.IsProd() {
		t.Fatal("IsProd must be true")
	}
}

func TestDevModeAllowsDefaultsButWarns(t *testing.T) {
	c, err := load(t, map[string]string{"CK_T_REQUIRED": "x"})
	if err != nil {
		t.Fatalf("dev must accept defaults: %v", err)
	}
	if c.IsProd() {
		t.Fatal("IsProd must be false in dev")
	}
	w := config.Warnings(&c)
	if len(w) != 1 || !strings.Contains(w[0], "CK_T_TOKEN") {
		t.Fatalf("want one warning for CK_T_TOKEN, got %v", w)
	}
	if strings.Contains(w[0], "dev-token-dev-token") {
		t.Fatalf("warning must not echo the secret: %q", w[0])
	}
	c2, _ := load(t, map[string]string{"CK_T_REQUIRED": "x", "CK_T_TOKEN": "an-explicit-token-an-explicit-token-1"})
	if got := config.Warnings(&c2); len(got) != 0 {
		t.Fatalf("explicit secret must not warn: %v", got)
	}
}

func TestShortSecretRejectedEvenInDev(t *testing.T) {
	_, err := load(t, map[string]string{"CK_T_REQUIRED": "x", "CK_T_TOKEN": "short"})
	if err == nil {
		t.Fatal("a too-short explicit secret must fail validation in dev as well")
	}
}

func TestSecretNeverPrints(t *testing.T) {
	s := config.Secret("super-secret-value")
	for name, got := range map[string]string{
		"String":   s.String(),
		"%v":       fmt.Sprintf("%v", s),
		"%s":       fmt.Sprintf("%s", s),
		"%+v":      fmt.Sprintf("%+v", s),
		"%#v":      fmt.Sprintf("%#v", s),
		"LogValue": s.LogValue().String(),
	} {
		if strings.Contains(got, "super-secret") {
			t.Errorf("%s leaked the secret: %q", name, got)
		}
	}
	b, err := json.Marshal(struct{ S config.Secret }{s})
	if err != nil || strings.Contains(string(b), "super-secret") {
		t.Errorf("json leaked secret: %s err=%v", b, err)
	}
	if s.Reveal() != "super-secret-value" {
		t.Error("Reveal must return the raw value")
	}
	if config.Secret("").String() != "" {
		t.Error("an empty secret should render empty so that 'unset' stays visible")
	}
	if slog.AnyValue(s).Kind() != slog.KindLogValuer {
		t.Error("Secret must implement slog.LogValuer")
	}
}

func TestSummaryIsRedacted(t *testing.T) {
	c, err := load(t, map[string]string{"CK_T_REQUIRED": "x", "CK_T_TOKEN": "an-explicit-token-an-explicit-token-1", "CK_PG_DSN": "x"})
	if err != nil {
		t.Fatal(err)
	}
	m := config.Summary(&c)
	if m["CK_T_NAME"] != "svc" || m["CK_HTTP_PORT"] != "8080" || m["CK_T_ORIGINS"] != "a,b,c" {
		t.Fatalf("summary content wrong: %v", m)
	}
	if m["CK_T_TOKEN"] != "[REDACTED]" {
		t.Fatalf("secret must be redacted in summary, got %q", m["CK_T_TOKEN"])
	}
}

func TestReportLogsRedactedSummaryAndWarnings(t *testing.T) {
	c, err := load(t, map[string]string{"CK_T_REQUIRED": "x"})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	config.Report(slog.New(slog.NewJSONHandler(&buf, nil)), &c)
	out := buf.String()
	if !strings.Contains(out, "configuration loaded") || !strings.Contains(out, "CK_HTTP_PORT") {
		t.Fatalf("summary not logged: %s", out)
	}
	if !strings.Contains(out, `"level":"WARN"`) || !strings.Contains(out, "CK_T_TOKEN") {
		t.Fatalf("dev-default secret must produce a warning: %s", out)
	}
	if strings.Contains(out, "dev-token-dev-token") {
		t.Fatalf("report must never print secrets: %s", out)
	}
}

func TestCommonLevelValidation(t *testing.T) {
	if _, err := load(t, map[string]string{"CK_T_REQUIRED": "x", "CK_LOG_LEVEL": "loud"}); err == nil {
		t.Fatal("invalid log level must fail")
	}
	if _, err := load(t, map[string]string{"CK_T_REQUIRED": "x", "CK_HTTP_PORT": "70000"}); err == nil {
		t.Fatal("port out of range must fail")
	}
}
