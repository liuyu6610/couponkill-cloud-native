package logx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

func records(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line is not JSON: %q (%v)", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestNewWritesJSONWithServiceAndLevelFilter(t *testing.T) {
	var buf bytes.Buffer
	l := logx.New(logx.Options{Service: "order", Level: "warn", Writer: &buf})
	l.Info("hidden")
	l.Warn("shown", "k", "v")
	recs := records(t, &buf)
	if len(recs) != 1 {
		t.Fatalf("want 1 record (info filtered), got %d: %s", len(recs), buf.String())
	}
	r := recs[0]
	if r["msg"] != "shown" || r["service"] != "order" || r["k"] != "v" || r["level"] != "WARN" {
		t.Fatalf("unexpected record: %v", r)
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{"debug": slog.LevelDebug, "INFO": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError, "": slog.LevelInfo}
	for in, want := range cases {
		got, err := logx.ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := logx.ParseLevel("loud"); err == nil {
		t.Error("ParseLevel must reject unknown levels")
	}
}

func TestFromCarriesRequestIDAndFields(t *testing.T) {
	var buf bytes.Buffer
	l := logx.New(logx.Options{Service: "svc", Writer: &buf})
	ctx := logx.NewContext(context.Background(), l)
	ctx = logx.WithRequestID(ctx, "req-1")
	ctx = logx.With(ctx, "coupon_id", "1001")
	logx.From(ctx).Info("deduct")

	r := records(t, &buf)[0]
	if r["request_id"] != "req-1" || r["coupon_id"] != "1001" {
		t.Fatalf("ctx logger lost fields: %v", r)
	}
	if got := logx.RequestID(ctx); got != "req-1" {
		t.Fatalf("RequestID = %q", got)
	}
}

func TestNewContextKeepsExistingRequestID(t *testing.T) {
	var buf bytes.Buffer
	ctx := logx.WithRequestID(context.Background(), "req-2")
	ctx = logx.NewContext(ctx, logx.New(logx.Options{Writer: &buf}))
	logx.From(ctx).Info("x")
	if r := records(t, &buf)[0]; r["request_id"] != "req-2" {
		t.Fatalf("replacing the logger must not drop request_id: %v", r)
	}
}

func TestFromFallsBackToDefaultLogger(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })
	slog.SetDefault(logx.New(logx.Options{Writer: &buf}))

	logx.From(context.Background()).Info("plain")
	logx.From(logx.WithRequestID(context.Background(), "r3")).Info("with-id")
	recs := records(t, &buf)
	if len(recs) != 2 || recs[1]["request_id"] != "r3" {
		t.Fatalf("fallback logger wrong: %v", recs)
	}
	if logx.RequestID(context.Background()) != "" {
		t.Error("RequestID of a bare ctx must be empty")
	}
}

func TestSetupInstallsDefault(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })
	logx.Setup(logx.Options{Service: "gw", Writer: &buf})
	slog.Info("via default")
	if r := records(t, &buf); len(r) != 1 || r[0]["service"] != "gw" {
		t.Fatalf("Setup must install the logger as slog default: %v", r)
	}
}

func TestSensitiveKeysAreRedacted(t *testing.T) {
	var buf bytes.Buffer
	l := logx.New(logx.Options{Writer: &buf})
	l.Info("login",
		"password", "hunter2",
		"jwt_secret", "s3cr3t",
		"X-Internal-Token", "tok",
		"Authorization", "Bearer abc",
		"gateway_sign_key", "k",
		"user", "demo",
		"cfg", config.Secret("raw-secret"),
		slog.Group("req", slog.String("access_token", "t0k")),
	)
	out := buf.String()
	for _, leaked := range []string{"hunter2", "s3cr3t", "Bearer abc", "raw-secret", "t0k", `"tok"`} {
		if strings.Contains(out, leaked) {
			t.Errorf("log leaked %q: %s", leaked, out)
		}
	}
	if !strings.Contains(out, `"user":"demo"`) {
		t.Errorf("non-sensitive keys must be kept: %s", out)
	}
}

func TestNewRequestIDIsUniqueAndSafe(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := logx.NewRequestID()
		if len(id) != 32 || seen[id] {
			t.Fatalf("bad or duplicate id %q", id)
		}
		seen[id] = true
	}
}
