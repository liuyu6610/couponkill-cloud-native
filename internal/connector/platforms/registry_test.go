package platforms

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

// fakeAdapter 是只用于注册表测试的适配器：健康状态可切换，并统计 Health 被调用的次数。
type fakeAdapter struct {
	platform domain.Platform
	status   atomic.Value // string
	checks   atomic.Int32
}

func newFake(p domain.Platform, status string) *fakeAdapter {
	f := &fakeAdapter{platform: p}
	f.status.Store(status)
	return f
}

func (f *fakeAdapter) Platform() domain.Platform { return f.platform }
func (f *fakeAdapter) Product(context.Context, string) (domain.ProductSnapshot, error) {
	return domain.ProductSnapshot{}, nil
}
func (f *fakeAdapter) Stock(context.Context, string) (domain.StockSnapshot, error) {
	return domain.StockSnapshot{}, nil
}
func (f *fakeAdapter) Health(context.Context) domain.Health {
	f.checks.Add(1)
	return domain.Health{Platform: f.platform, Status: f.status.Load().(string), Message: "m"}
}

func TestRegistryNeverChecksHealthAtConstruction(t *testing.T) {
	a := newFake(domain.JD, domain.StatusDisabled)
	if _, err := NewRegistry(RegistryConfig{HealthTTL: time.Minute}, a); err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if a.checks.Load() != 0 {
		t.Fatalf("Health 在构造时被调用了 %d 次，必须惰性检查", a.checks.Load())
	}
}

func TestRegistryRejectsDuplicatePlatforms(t *testing.T) {
	_, err := NewRegistry(RegistryConfig{}, newFake(domain.JD, domain.StatusUp), newFake(domain.JD, domain.StatusUp))
	if err == nil {
		t.Fatal("同一平台注册两个适配器必须失败，否则会静默覆盖")
	}
}

func TestRegistryGetGatesOnHealthWithLazyTTL(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	a := newFake(domain.JD, domain.StatusDisabled)
	reg, err := NewRegistry(RegistryConfig{HealthTTL: 30 * time.Second, Now: clk.Now}, a)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// DISABLED：不可用，错误可被 errors.Is 识别。
	if _, err := reg.Get(ctx, domain.JD); !errors.Is(err, ErrDisabled) {
		t.Fatalf("Get(DISABLED) err = %v, want ErrDisabled", err)
	}
	// TTL 内再次调用不重新检查。
	a.status.Store(domain.StatusUp)
	if _, err := reg.Get(ctx, domain.JD); !errors.Is(err, ErrDisabled) {
		t.Fatalf("TTL 内应沿用缓存状态, err = %v", err)
	}
	if got := a.checks.Load(); got != 1 {
		t.Fatalf("checks = %d, want 1 (TTL 内不复查)", got)
	}

	// TTL 过后惰性复查，DISABLED -> UP。
	clk.Advance(31 * time.Second)
	got, err := reg.Get(ctx, domain.JD)
	if err != nil || got != PlatformAdapter(a) {
		t.Fatalf("Get after TTL = %v, %v; want adapter", got, err)
	}

	// UP -> DOWN。
	a.status.Store(domain.StatusDown)
	clk.Advance(31 * time.Second)
	if _, err := reg.Get(ctx, domain.JD); !errors.Is(err, ErrDown) {
		t.Fatalf("Get(DOWN) err = %v, want ErrDown", err)
	}
	// DOWN -> UP 恢复后自动重新参与调度。
	a.status.Store(domain.StatusUp)
	clk.Advance(31 * time.Second)
	if _, err := reg.Get(ctx, domain.JD); err != nil {
		t.Fatalf("Get after recovery: %v", err)
	}
}

func TestRegistryUnknownPlatform(t *testing.T) {
	reg, _ := NewRegistry(RegistryConfig{}, newFake(domain.Mock, domain.StatusUp))
	if _, err := reg.Get(context.Background(), domain.TB); !errors.Is(err, ErrUnknownPlatform) {
		t.Fatalf("err = %v, want ErrUnknownPlatform", err)
	}
}

func TestRegistryHealthListsAllInDisplayOrderUsingCache(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	jd := newFake(domain.JD, domain.StatusDisabled)
	mock := newFake(domain.Mock, domain.StatusUp)
	reg, _ := NewRegistry(RegistryConfig{HealthTTL: time.Minute, Now: clk.Now}, jd, mock)

	hs := reg.Health(context.Background())
	if len(hs) != 2 || hs[0].Platform != domain.Mock || hs[1].Platform != domain.JD {
		t.Fatalf("health = %+v, want MOCK then JD (展示顺序)", hs)
	}
	reg.Health(context.Background())
	if jd.checks.Load() != 1 || mock.checks.Load() != 1 {
		t.Fatalf("checks jd=%d mock=%d, want 1/1 (缓存)", jd.checks.Load(), mock.checks.Load())
	}
}

func TestRegistryTreatsUnknownStatusAsUnavailable(t *testing.T) {
	reg, _ := NewRegistry(RegistryConfig{}, newFake(domain.Mock, "CONFIGURED"))
	if _, err := reg.Get(context.Background(), domain.Mock); err == nil {
		t.Fatal("只有 UP 才可用；未知状态不得被当作可用")
	}
}
