package service_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/platforms"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/repo"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

// lockKey 是同步锁在 Redis 里的键（设计 §8：ck:lock:{name}）。
const lockKey = "ck:lock:connector-sync"

// fakeAdapter 是可编程的平台适配器：记录 Stock 调用顺序，行为由 stock 回调决定。
type fakeAdapter struct {
	platform domain.Platform
	mu       sync.Mutex
	stockFn  func(ctx context.Context, sku string) (domain.StockSnapshot, error)
	product  func(ctx context.Context, sku string) (domain.ProductSnapshot, error)
	order    []string
	status   string
}

func (f *fakeAdapter) Platform() domain.Platform { return f.platform }

func (f *fakeAdapter) Product(ctx context.Context, sku string) (domain.ProductSnapshot, error) {
	if f.product == nil {
		return domain.ProductSnapshot{Platform: f.platform, ExternalSkuID: sku, Title: "fake-" + sku}, nil
	}
	return f.product(ctx, sku)
}

func (f *fakeAdapter) Stock(ctx context.Context, sku string) (domain.StockSnapshot, error) {
	f.mu.Lock()
	f.order = append(f.order, sku)
	fn := f.stockFn
	f.mu.Unlock()
	if fn == nil {
		return qtySnapshot(f.platform, sku, 10), nil
	}
	return fn(ctx, sku)
}

func (f *fakeAdapter) Health(context.Context) domain.Health {
	status := f.status
	if status == "" {
		status = domain.StatusUp
	}
	return domain.Health{Platform: f.platform, Status: status, Message: "fake"}
}

func (f *fakeAdapter) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.order...)
}

func qtySnapshot(p domain.Platform, sku string, qty int64) domain.StockSnapshot {
	avail := qty > 0
	return domain.StockSnapshot{Platform: p, ExternalSkuID: sku, StockQty: &qty, StockAvailable: &avail}
}

// fakeCoupon 是 coupon 服务的内部 API 替身：记录每次 SyncStock，可注入钩子与错误。
type fakeCoupon struct {
	mu    sync.Mutex
	reqs  []internalapi.SyncRequest
	err   func(req internalapi.SyncRequest) error
	after func(req internalapi.SyncRequest)
}

func (c *fakeCoupon) SyncStock(_ context.Context, req internalapi.SyncRequest) (internalapi.SyncResponse, error) {
	c.mu.Lock()
	c.reqs = append(c.reqs, req)
	errFn, after := c.err, c.after
	c.mu.Unlock()
	if after != nil {
		after(req)
	}
	if errFn != nil {
		if err := errFn(req); err != nil {
			return internalapi.SyncResponse{}, err
		}
	}
	return internalapi.SyncResponse{CouponID: req.CouponID, Available: req.Available}, nil
}

func (c *fakeCoupon) requests() []internalapi.SyncRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]internalapi.SyncRequest(nil), c.reqs...)
}

// harness 把真实 PG（dbxtest）、miniredis、假平台与假 coupon 装配成一个 SyncService。
type harness struct {
	t       *testing.T
	store   *repo.Store
	rdb     *redis.Client
	mr      *miniredis.Miniredis
	clk     *testkit.Clock
	adapter *fakeAdapter
	coupon  *fakeCoupon
	svc     *service.SyncService
	locker  *service.RedisLocker
}

func newStore(t *testing.T) *repo.Store {
	t.Helper()
	pool := dbxtest.New(t)
	if err := dbx.Migrate(context.Background(), pool, "cnx", migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	gen, err := idgen.New(1)
	if err != nil {
		t.Fatal(err)
	}
	return repo.NewStore(pool, gen)
}

func newHarness(t *testing.T, mutate func(*service.SyncConfig)) *harness {
	t.Helper()
	rdb, mr := testkit.Redis(t)
	h := &harness{
		t: t, store: newStore(t), rdb: rdb, mr: mr,
		clk:     testkit.NewClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		adapter: &fakeAdapter{platform: domain.Mock},
		coupon:  &fakeCoupon{},
	}
	reg, err := platforms.NewRegistry(platforms.RegistryConfig{HealthTTL: time.Hour, Now: h.clk.Now}, h.adapter)
	if err != nil {
		t.Fatal(err)
	}
	cfg := service.SyncConfig{LockTTL: 60 * time.Second, Now: h.clk.Now}
	if mutate != nil {
		mutate(&cfg)
	}
	h.locker = service.NewRedisLocker(rdb, cfg.LockTTL)
	h.svc = service.NewSyncService(h.store, reg, h.coupon, h.locker, cfg)
	return h
}

// withAdapters 基于同一个仓储/锁/coupon 替身，另建一个注册了额外适配器的 SyncService。
func (h *harness) withAdapters(extra ...platforms.PlatformAdapter) *service.SyncService {
	h.t.Helper()
	reg, err := platforms.NewRegistry(platforms.RegistryConfig{HealthTTL: time.Hour, Now: h.clk.Now},
		append([]platforms.PlatformAdapter{h.adapter}, extra...)...)
	if err != nil {
		h.t.Fatal(err)
	}
	return service.NewSyncService(h.store, reg, h.coupon, h.locker, service.SyncConfig{LockTTL: 60 * time.Second, Now: h.clk.Now})
}

// bind 创建 MOCK 平台的绑定，sku 名为 sku-N，券号 1000+N。
func (h *harness) bind(n int) domain.Binding {
	h.t.Helper()
	b, err := h.store.UpsertBinding(context.Background(), domain.BindingCommand{
		Platform: domain.Mock, ExternalSkuID: skuName(n), CouponID: idgen.ID(1000 + n),
	}, true)
	if err != nil {
		h.t.Fatalf("bind %d: %v", n, err)
	}
	return b
}

func skuName(n int) string { return "sku-" + strconv.Itoa(n) }

func (h *harness) get(b domain.Binding) domain.Binding {
	h.t.Helper()
	got, err := h.store.GetBinding(context.Background(), int64(b.ID))
	if err != nil {
		h.t.Fatalf("get binding: %v", err)
	}
	return got
}
