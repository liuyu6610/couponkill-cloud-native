package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/platforms"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

func statusOfErr(err error) int { return apperr.StatusOf(err) }

func TestBindingServiceValidation(t *testing.T) {
	svc := service.NewBindingService(newStore(t))
	ctx := context.Background()
	ok := domain.BindingCommand{Platform: domain.Mock, ExternalSkuID: " sku-1 ", CouponID: 5}

	cases := map[string]func(c *domain.BindingCommand){
		"missing platform": func(c *domain.BindingCommand) { c.Platform = "" },
		"unknown platform": func(c *domain.BindingCommand) { c.Platform = "AMAZON" },
		"blank sku":        func(c *domain.BindingCommand) { c.ExternalSkuID = "   " },
		"long sku":         func(c *domain.BindingCommand) { c.ExternalSkuID = strings.Repeat("x", 129) },
		"zero coupon":      func(c *domain.BindingCommand) { c.CouponID = 0 },
	}
	for name, mutate := range cases {
		c := ok
		mutate(&c)
		if _, err := svc.Upsert(ctx, c); statusOfErr(err) != http.StatusBadRequest {
			t.Errorf("%s: err = %v, want 400", name, err)
		}
	}

	b, err := svc.Upsert(ctx, ok)
	if err != nil {
		t.Fatal(err)
	}
	if b.ExternalSkuID != "sku-1" || !b.SyncEnabled {
		t.Fatalf("binding = %+v, want trimmed sku and sync enabled by default", b)
	}
	off := false
	ok.SyncEnabled = &off
	if b, err = svc.Upsert(ctx, ok); err != nil || b.SyncEnabled {
		t.Fatalf("explicit syncEnabled=false: %+v, %v", b, err)
	}
	if _, err := svc.Upsert(ctx, domain.BindingCommand{Platform: domain.JD, ExternalSkuID: "other", CouponID: 5}); statusOfErr(err) != http.StatusConflict {
		t.Fatalf("coupon already bound: err = %v, want 409", err)
	}
	got, err := svc.ByCoupon(ctx, 5)
	if err != nil || got == nil || got.ID != b.ID {
		t.Fatalf("ByCoupon = %+v, %v", got, err)
	}
	if none, err := svc.ByCoupon(ctx, 99); err != nil || none != nil {
		t.Fatalf("ByCoupon missing = %+v, %v; want nil", none, err)
	}
	if _, err := svc.ByCoupon(ctx, 0); statusOfErr(err) != http.StatusBadRequest {
		t.Fatalf("ByCoupon(0) err = %v, want 400", err)
	}
	if list, err := svc.List(ctx); err != nil || len(list) != 1 {
		t.Fatalf("List = %+v, %v", list, err)
	}
}

func newPriceService(t *testing.T, clk *testkit.Clock, adapters ...platforms.PlatformAdapter) (*service.PriceService, *service.BindingService) {
	t.Helper()
	store := newStore(t)
	reg, err := platforms.NewRegistry(platforms.RegistryConfig{HealthTTL: time.Hour, Now: clk.Now}, adapters...)
	if err != nil {
		t.Fatal(err)
	}
	return service.NewPriceService(store, reg, service.PriceConfig{CacheTTL: 30 * time.Second, Now: clk.Now}), service.NewBindingService(store)
}

func num(s string) *json.Number { n := json.Number(s); return &n }

func TestPriceMapUpsertValidationAndDefaults(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	svc, _ := newPriceService(t, clk, platforms.NewMock(platforms.MockConfig{}))
	ctx := context.Background()
	base := domain.PriceMapCommand{CouponID: 1, Platform: domain.JD, ExternalSkuID: "jd-1", ManualPrice: num("88.8")}

	bad := map[string]func(c *domain.PriceMapCommand){
		"no platform":      func(c *domain.PriceMapCommand) { c.Platform = "" },
		"unknown platform": func(c *domain.PriceMapCommand) { c.Platform = "X" },
		"no coupon":        func(c *domain.PriceMapCommand) { c.CouponID = 0 },
		"blank sku":        func(c *domain.PriceMapCommand) { c.ExternalSkuID = " " },
		"negative price":   func(c *domain.PriceMapCommand) { c.ManualPrice = num("-1") },
		"three decimals":   func(c *domain.PriceMapCommand) { c.ManualPrice = num("1.234") },
		"exponent price":   func(c *domain.PriceMapCommand) { c.ManualPrice = num("1e3") },
		"bad currency":     func(c *domain.PriceMapCommand) { c.Currency = "RMB1" },
		"overlong title":   func(c *domain.PriceMapCommand) { s := strings.Repeat("题", 201); c.Title = &s },
		"price too big":    func(c *domain.PriceMapCommand) { c.ManualPrice = num("12345678901") },
	}
	for name, mutate := range bad {
		c := base
		mutate(&c)
		if _, err := svc.Upsert(ctx, c); statusOfErr(err) != http.StatusBadRequest {
			t.Errorf("%s: err = %v, want 400", name, err)
		}
	}

	pm, err := svc.Upsert(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if pm.Currency != "CNY" || !pm.Enabled || pm.ManualPrice == nil || pm.ManualPrice.String() != "88.80" {
		t.Fatalf("pm = %+v, want defaults CNY/enabled and numeric(12,2) price", pm)
	}
	lower := base
	lower.Currency = " usd "
	if pm, err = svc.Upsert(ctx, lower); err != nil || pm.Currency != "USD" {
		t.Fatalf("currency normalization: %+v, %v", pm, err)
	}
	list, err := svc.List(ctx, 1)
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %+v, %v (唯一 (coupon, platform))", list, err)
	}
	if _, err := svc.List(ctx, 0); statusOfErr(err) != http.StatusBadRequest {
		t.Fatalf("List(0) err = %v", err)
	}
	if err := svc.Delete(ctx, int64(pm.ID)); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, int64(pm.ID)); err != nil {
		t.Fatalf("second delete should be a no-op: %v", err)
	}
	if err := svc.Delete(ctx, 0); statusOfErr(err) != http.StatusBadRequest {
		t.Fatalf("Delete(0) err = %v", err)
	}
}

func TestCompareCombinesBindingProbeAndManualMapsWithoutDuplicates(t *testing.T) {
	clk := testkit.NewClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	jd := &fakeAdapter{platform: domain.JD, product: func(_ context.Context, sku string) (domain.ProductSnapshot, error) {
		return domain.ProductSnapshot{Platform: domain.JD, ExternalSkuID: sku, Title: "京东款", Price: num("99.50")}, nil
	}}
	svc, bindings := newPriceService(t, clk, platforms.NewMock(platforms.MockConfig{}), jd)
	ctx := context.Background()

	b, err := bindings.Upsert(ctx, domain.BindingCommand{Platform: domain.JD, ExternalSkuID: "jd-1", CouponID: 7})
	if err != nil {
		t.Fatal(err)
	}
	title := "手工款"
	mustUpsert := func(c domain.PriceMapCommand) {
		t.Helper()
		if _, err := svc.Upsert(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	mustUpsert(domain.PriceMapCommand{CouponID: 7, Platform: domain.JD, ExternalSkuID: "jd-1", ManualPrice: num("1")}) // 与主 SKU 重复 → 忽略
	mustUpsert(domain.PriceMapCommand{CouponID: 7, Platform: domain.TB, ExternalSkuID: "tb-1", Title: &title, ManualPrice: num("79.9")})
	mustUpsert(domain.PriceMapCommand{CouponID: 7, Platform: domain.Mock, ExternalSkuID: "m-1"}) // 无手工价 → 探测
	off := false
	mustUpsert(domain.PriceMapCommand{CouponID: 7, Platform: domain.PDD, ExternalSkuID: "p-1", ManualPrice: num("5"), Enabled: &off}) // 停用 → 不出现

	res, err := svc.Compare(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if res.CouponID != 7 || res.BindingID == nil || *res.BindingID != b.ID || len(res.Items) != 3 {
		t.Fatalf("result = %+v", res)
	}
	primary, manual, probed := res.Items[0], res.Items[1], res.Items[2]
	if primary.Source != "PROBE" || primary.Confidence != "HIGH" || primary.Price.String() != "99.50" || *primary.Title != "京东款" || primary.Currency != "CNY" {
		t.Fatalf("primary = %+v", primary)
	}
	if manual.Platform != domain.TB || manual.Source != "MANUAL" || manual.Confidence != "MEDIUM" || manual.Price.String() != "79.90" || !manual.FetchedAt.Equal(clk.Now()) {
		t.Fatalf("manual = %+v", manual)
	}
	if probed.Platform != domain.Mock || probed.Source != "PROBE" || probed.Confidence != "MEDIUM" || probed.Price.String() != "99.00" {
		t.Fatalf("probed = %+v", probed)
	}
}

func TestCompareWithoutBindingReturnsOnlyManualRowsAndNullBindingID(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	svc, _ := newPriceService(t, clk, platforms.NewMock(platforms.MockConfig{}))
	ctx := context.Background()
	if _, err := svc.Upsert(ctx, domain.PriceMapCommand{CouponID: 3, Platform: domain.TB, ExternalSkuID: "t", ManualPrice: num("2")}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Compare(ctx, 3)
	if err != nil || res.BindingID != nil || len(res.Items) != 1 {
		t.Fatalf("result = %+v, %v", res, err)
	}
	empty, err := svc.Compare(ctx, 4)
	if err != nil || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatalf("empty = %+v, %v; want items=[] not null", empty, err)
	}
	if _, err := svc.Compare(ctx, 0); statusOfErr(err) != http.StatusBadRequest {
		t.Fatalf("Compare(0) err = %v", err)
	}
}

func TestCompareProbeFailuresDegradeToLowConfidenceWithoutLeakingDetails(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	secretDetail := "dial tcp 10.0.0.1: app_key=SUPERSECRET"
	jd := &fakeAdapter{platform: domain.JD, product: func(context.Context, string) (domain.ProductSnapshot, error) {
		return domain.ProductSnapshot{}, &platforms.UpstreamError{Platform: "JD", Err: errors.New(secretDetail)}
	}}
	tb := &fakeAdapter{platform: domain.TB, status: domain.StatusDisabled}
	svc, bindings := newPriceService(t, clk, jd, tb)
	ctx := context.Background()
	if _, err := bindings.Upsert(ctx, domain.BindingCommand{Platform: domain.JD, ExternalSkuID: "jd-1", CouponID: 9}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Upsert(ctx, domain.PriceMapCommand{CouponID: 9, Platform: domain.TB, ExternalSkuID: "tb-1"}); err != nil { // 无手工价且平台禁用
		t.Fatal(err)
	}

	res, err := svc.Compare(ctx, 9)
	if err != nil || len(res.Items) != 2 {
		t.Fatalf("result = %+v, %v; 探测失败不能让接口失败", res, err)
	}
	for _, it := range res.Items {
		if it.Confidence != "LOW" || it.Price != nil || it.Message == nil || !strings.HasPrefix(*it.Message, "探测失败") {
			t.Fatalf("item = %+v, want LOW with failure note", it)
		}
		if strings.Contains(*it.Message, "SUPERSECRET") || strings.Contains(*it.Message, "10.0.0.1") {
			t.Fatalf("message leaks internals: %q", *it.Message)
		}
	}
}

func TestCompareResultIsCachedUntilTTLOrWrite(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	calls := 0
	jd := &fakeAdapter{platform: domain.JD, product: func(_ context.Context, sku string) (domain.ProductSnapshot, error) {
		calls++
		return domain.ProductSnapshot{Platform: domain.JD, ExternalSkuID: sku, Price: num("1")}, nil
	}}
	svc, bindings := newPriceService(t, clk, jd)
	ctx := context.Background()
	if _, err := bindings.Upsert(ctx, domain.BindingCommand{Platform: domain.JD, ExternalSkuID: "j", CouponID: 1}); err != nil {
		t.Fatal(err)
	}

	for range 3 {
		if _, err := svc.Compare(ctx, 1); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("probe calls = %d, want 1: 匿名接口的重复请求必须命中缓存而不是打第三方", calls)
	}
	clk.Advance(31 * time.Second)
	svc.Compare(ctx, 1)
	if calls != 2 {
		t.Fatalf("after TTL calls = %d, want 2", calls)
	}
	if _, err := svc.Upsert(ctx, domain.PriceMapCommand{CouponID: 1, Platform: domain.TB, ExternalSkuID: "t", ManualPrice: num("3")}); err != nil {
		t.Fatal(err)
	}
	res, _ := svc.Compare(ctx, 1)
	if calls != 3 || len(res.Items) != 2 {
		t.Fatalf("after write: calls=%d items=%d; 写入后缓存必须失效", calls, len(res.Items))
	}
}

func TestProbeServiceErrorMapping(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	ok := &fakeAdapter{platform: domain.Mock}
	disabled := &fakeAdapter{platform: domain.JD, status: domain.StatusDisabled}
	down := &fakeAdapter{platform: domain.TB, status: domain.StatusDown}
	pdd := &fakeAdapter{platform: domain.PDD}
	reg, err := platforms.NewRegistry(platforms.RegistryConfig{Now: clk.Now}, ok, disabled, down, pdd)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewProbeService(reg)
	ctx := context.Background()

	if snap, err := svc.Product(ctx, " mock ", " s1 "); err != nil || snap.ExternalSkuID != "s1" {
		t.Fatalf("lowercase platform + trimmed sku: %+v, %v", snap, err)
	}
	if snap, err := svc.Stock(ctx, "MOCK", "s1"); err != nil || snap.StockQty == nil {
		t.Fatalf("stock: %+v, %v", snap, err)
	}
	expect := func(name string, err error, status int) {
		t.Helper()
		if statusOfErr(err) != status {
			t.Errorf("%s: err = %v, status %d, want %d", name, err, statusOfErr(err), status)
		}
	}
	_, err = svc.Product(ctx, "AMAZON", "x")
	expect("unknown platform", err, http.StatusBadRequest)
	_, err = svc.Product(ctx, "", "x")
	expect("empty platform", err, http.StatusBadRequest)
	_, err = svc.Product(ctx, "MOCK", "  ")
	expect("blank sku", err, http.StatusBadRequest)
	_, err = svc.Stock(ctx, "JD", "x")
	expect("disabled", err, http.StatusServiceUnavailable)
	_, err = svc.Stock(ctx, "TB", "x")
	expect("down", err, http.StatusServiceUnavailable)

	pdd.stockFn = func(context.Context, string) (domain.StockSnapshot, error) {
		return domain.StockSnapshot{}, platforms.ErrNotFound
	}
	_, err = svc.Stock(ctx, "PDD", "x")
	expect("sku missing", err, http.StatusNotFound)
	pdd.stockFn = func(context.Context, string) (domain.StockSnapshot, error) {
		return domain.StockSnapshot{}, &platforms.RejectedError{Platform: "PDD", Code: "1", Message: "bad token"}
	}
	_, err = svc.Stock(ctx, "PDD", "x")
	expect("rejected", err, http.StatusBadGateway)
	pdd.stockFn = func(context.Context, string) (domain.StockSnapshot, error) {
		return domain.StockSnapshot{}, &platforms.UpstreamError{Platform: "PDD", Status: 503, Err: errors.New("x")}
	}
	_, err = svc.Stock(ctx, "PDD", "x")
	expect("upstream", err, http.StatusServiceUnavailable)
	pdd.stockFn = func(context.Context, string) (domain.StockSnapshot, error) {
		return domain.StockSnapshot{}, platforms.ErrCircuitOpen
	}
	_, err = svc.Stock(ctx, "PDD", "x")
	expect("breaker open", err, http.StatusServiceUnavailable)

	if hs := svc.Health(ctx); len(hs) != 4 || hs[0].Platform != domain.Mock {
		t.Fatalf("health = %+v", hs)
	}
}
