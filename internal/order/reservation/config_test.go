package reservation

import (
	"strings"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
)

func TestConfigLoadsDefaultsFromEnvTags(t *testing.T) {
	var cfg struct{ Reservation Config }
	if err := config.Load(&cfg, config.WithMap(map[string]string{})); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Reservation != DefaultConfig() {
		t.Fatalf("env tag 默认值应与 DefaultConfig 一致:\n got  %+v\n want %+v", cfg.Reservation, DefaultConfig())
	}
	if err := cfg.Reservation.Validate(); err != nil {
		t.Fatalf("默认配置应合法: %v", err)
	}
}

func TestConfigLoadsOverrides(t *testing.T) {
	var cfg struct{ Reservation Config }
	err := config.Load(&cfg, config.WithMap(map[string]string{
		"CK_ORDER_RESERVATION_ENABLED":              "false",
		"CK_ORDER_RESERVATION_SCAN_INTERVAL":        "250ms",
		"CK_ORDER_RESERVATION_BATCH_SIZE":           "200",
		"CK_ORDER_RESERVATION_CONCURRENCY":          "8",
		"CK_ORDER_RESERVATION_MAX_RETRY":            "5",
		"CK_ORDER_RESERVATION_RETRY_BACKOFF":        "500ms",
		"CK_ORDER_RESERVATION_ENTER_TIMEOUT":        "3s",
		"CK_ORDER_RESERVATION_STALE_FIRING_TIMEOUT": "30s",
		"CK_ORDER_RESERVATION_QUEUED_SYNC_AFTER":    "2m",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{Enabled: false, ScanInterval: 250 * time.Millisecond, BatchSize: 200, Concurrency: 8, MaxRetry: 5,
		RetryBackoff: 500 * time.Millisecond, EnterTimeout: 3 * time.Second, StaleFiringTimeout: 30 * time.Second,
		QueuedSyncAfter: 2 * time.Minute}
	if cfg.Reservation != want {
		t.Fatalf("覆盖值不符:\n got  %+v\n want %+v", cfg.Reservation, want)
	}
}

func TestConfigLoadRejectsOutOfRangeValues(t *testing.T) {
	var cfg struct{ Reservation Config }
	err := config.Load(&cfg, config.WithMap(map[string]string{
		"CK_ORDER_RESERVATION_MAX_RETRY":  "0",
		"CK_ORDER_RESERVATION_BATCH_SIZE": "100000",
	}))
	if err == nil {
		t.Fatal("越界取值应被 config.Load 拒绝")
	}
	for _, key := range []string{"CK_ORDER_RESERVATION_MAX_RETRY", "CK_ORDER_RESERVATION_BATCH_SIZE"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("错误应指出 %s: %v", key, err)
		}
	}
}

func TestConfigValidateCrossFieldConstraint(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StaleFiringTimeout = cfg.EnterTimeout
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "StaleFiringTimeout") {
		t.Fatalf("回收超时不大于入口超时应被拒绝: %v", err)
	}
}
