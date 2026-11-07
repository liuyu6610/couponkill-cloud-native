package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

var (
	winStart = time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	winEnd   = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
)

func seckillCmd() service.CreateCommand {
	return service.CreateCommand{
		Name: "秒杀券", Type: service.TypeSeckill, FaceValue: "50", MinSpend: "100.5",
		ValidDays: 7, PerUserLimit: 1, TotalStock: 50,
		SeckillStartAt: &winStart, SeckillEndAt: &winEnd,
	}
}

func normalCmd() service.CreateCommand {
	return service.CreateCommand{
		Name: "常驻券", Type: service.TypeNormal, FaceValue: "10", MinSpend: "0",
		ValidDays: 15, PerUserLimit: 2, TotalStock: 20,
	}
}

func wantCode(t *testing.T, err error, want *apperr.Error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want code %d (%s)", err, want.Code, want.Message)
	}
}

func TestCreateSeckillSplitsStockIntoBucketsAndWarmsRedis(t *testing.T) {
	h := newHarness(t, service.Config{StockBuckets: 8})
	c, err := h.svc.Create(context.Background(), seckillCmd())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.TotalStock != 50 || c.Remaining != 50 {
		t.Fatalf("totalStock is the seckill stock: %+v", c)
	}
	if c.FaceValue != "50.00" || c.MinSpend != "100.50" {
		t.Fatalf("money must be normalised to 2 decimals: %q %q", c.FaceValue, c.MinSpend)
	}
	got := h.repo.buckets[c.ID]
	if len(got) != 8 {
		t.Fatalf("buckets = %v, want 8 buckets", got)
	}
	var sum int64
	for _, b := range got {
		sum += b
	}
	if sum != 50 || got[0] != 7 || got[7] != 6 {
		t.Fatalf("remainder must be spread over the first buckets: %v", got)
	}
	if v, ok := h.cache.stockOf(c.ID); !ok || v != 50 {
		t.Fatalf("redis counter = %d,%v want 50,true", v, ok)
	}
}

func TestCreateNormalCouponHasNoRedisCounter(t *testing.T) {
	h := newHarness(t, service.Config{})
	c, err := h.svc.Create(context.Background(), normalCmd())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, ok := h.cache.stockOf(c.ID); ok {
		t.Fatal("normal coupons must not get a seckill counter")
	}
	if len(h.repo.buckets[c.ID]) != service.DefaultStockBuckets {
		t.Fatalf("default bucket count = %d, want %d", len(h.repo.buckets[c.ID]), service.DefaultStockBuckets)
	}
}

func TestCreateGeneratesIDAndKeepsExplicitID(t *testing.T) {
	h := newHarness(t, service.Config{})
	auto, err := h.svc.Create(context.Background(), normalCmd())
	if err != nil || auto.ID <= 9000 {
		t.Fatalf("auto id: %+v %v", auto, err)
	}
	cmd := normalCmd()
	cmd.ID = 4242
	got, err := h.svc.Create(context.Background(), cmd)
	if err != nil || got.ID != 4242 {
		t.Fatalf("explicit id: %+v %v", got, err)
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	bad := map[string]func(c *service.CreateCommand){
		"empty name":          func(c *service.CreateCommand) { c.Name = "   " },
		"name too long":       func(c *service.CreateCommand) { c.Name = string(make([]rune, 101)) + "x" },
		"unknown type":        func(c *service.CreateCommand) { c.Type = 3 },
		"negative face":       func(c *service.CreateCommand) { c.FaceValue = "-1" },
		"3 decimals":          func(c *service.CreateCommand) { c.FaceValue = "1.234" },
		"scientific notation": func(c *service.CreateCommand) { c.MinSpend = "1e3" },
		"zero valid days":     func(c *service.CreateCommand) { c.ValidDays = 0 },
		"zero per user":       func(c *service.CreateCommand) { c.PerUserLimit = 0 },
		"negative stock":      func(c *service.CreateCommand) { c.TotalStock = -1 },
		"stock over cap":      func(c *service.CreateCommand) { c.TotalStock = service.MaxStock + 1 },
		"no window":           func(c *service.CreateCommand) { c.SeckillStartAt, c.SeckillEndAt = nil, nil },
		"half window":         func(c *service.CreateCommand) { c.SeckillEndAt = nil },
		"end before start":    func(c *service.CreateCommand) { c.SeckillStartAt, c.SeckillEndAt = &winEnd, &winStart },
		"start equals end":    func(c *service.CreateCommand) { c.SeckillEndAt = c.SeckillStartAt },
		"negative id":         func(c *service.CreateCommand) { c.ID = -5 },
	}
	for name, mut := range bad {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, service.Config{})
			cmd := seckillCmd()
			mut(&cmd)
			_, err := h.svc.Create(context.Background(), cmd)
			wantCode(t, err, apperr.InvalidRequest)
			if len(h.repo.created) != 0 {
				t.Fatal("an invalid command must not reach the repo")
			}
		})
	}

	t.Run("normal coupon with window", func(t *testing.T) {
		h := newHarness(t, service.Config{})
		cmd := normalCmd()
		cmd.SeckillStartAt, cmd.SeckillEndAt = &winStart, &winEnd
		_, err := h.svc.Create(context.Background(), cmd)
		wantCode(t, err, apperr.InvalidRequest)
	})
}

func TestCreateReportsSavedButCacheFailed(t *testing.T) {
	h := newHarness(t, service.Config{})
	h.cache.err = errBoom
	_, err := h.svc.Create(context.Background(), seckillCmd())
	wantCode(t, err, apperr.SystemBusy)
	if len(h.repo.created) != 1 {
		t.Fatal("the coupon must stay persisted when only the cache refresh failed")
	}
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Message == apperr.SystemBusy.Message {
		t.Fatalf("message must tell the admin it was saved: %v", err)
	}
}

func TestCreateDuplicateIDIsConflict(t *testing.T) {
	h := newHarness(t, service.Config{})
	dup := apperr.New(apperr.CodeInvalidRequest, 409, "优惠券ID已存在")
	h.repo.createErr = dup
	_, err := h.svc.Create(context.Background(), seckillCmd())
	if !errors.Is(err, dup) {
		t.Fatalf("err = %v", err)
	}
	if apperr.StatusOf(err) != 409 {
		t.Fatalf("status = %d", apperr.StatusOf(err))
	}
}

func TestStatusDisableDropsCounterAndEnableRestoresIt(t *testing.T) {
	h := newHarness(t, service.Config{})
	c, err := h.svc.Create(context.Background(), seckillCmd())
	if err != nil {
		t.Fatal(err)
	}
	if n, err := h.svc.SetStatus(context.Background(), c.ID, service.StatusDisabled); err != nil || n != 1 {
		t.Fatalf("disable: %d %v", n, err)
	}
	if _, ok := h.cache.stockOf(c.ID); ok {
		t.Fatal("a disabled coupon must have no stock counter")
	}
	if _, err := h.svc.SetStatus(context.Background(), c.ID, service.StatusEnabled); err != nil {
		t.Fatal(err)
	}
	if v, ok := h.cache.stockOf(c.ID); !ok || v != 50 {
		t.Fatalf("re-enabled counter = %d,%v want 50,true", v, ok)
	}
}

func TestStatusValidation(t *testing.T) {
	h := newHarness(t, service.Config{})
	_, err := h.svc.SetStatus(context.Background(), 1, 2)
	wantCode(t, err, apperr.InvalidRequest)
	_, err = h.svc.SetStatus(context.Background(), 0, 1)
	wantCode(t, err, apperr.InvalidRequest)
	_, err = h.svc.SetStatus(context.Background(), 99, 1)
	wantCode(t, err, apperr.CouponNotFound)
}

func TestSeckillWindowRules(t *testing.T) {
	h := newHarness(t, service.Config{})
	sk, _ := h.svc.Create(context.Background(), seckillCmd())
	nm, _ := h.svc.Create(context.Background(), normalCmd())
	ctx := context.Background()

	_, err := h.svc.SetSeckillWindow(ctx, sk.ID, winEnd, winStart)
	wantCode(t, err, apperr.InvalidRequest)
	_, err = h.svc.SetSeckillWindow(ctx, nm.ID, winStart, winEnd)
	wantCode(t, err, apperr.InvalidRequest)
	_, err = h.svc.SetSeckillWindow(ctx, 12345, winStart, winEnd)
	wantCode(t, err, apperr.CouponNotFound)

	newEnd := winEnd.Add(time.Hour)
	got, err := h.svc.SetSeckillWindow(ctx, sk.ID, winStart, newEnd)
	if err != nil {
		t.Fatalf("SetSeckillWindow: %v", err)
	}
	if got.SeckillEndAt == nil || !got.SeckillEndAt.Equal(newEnd) {
		t.Fatalf("window not updated: %+v", got)
	}
}

func TestDeleteIsIdempotentAndClearsRedis(t *testing.T) {
	h := newHarness(t, service.Config{})
	c, _ := h.svc.Create(context.Background(), seckillCmd())
	n, err := h.svc.Delete(context.Background(), c.ID)
	if err != nil || n != 1 {
		t.Fatalf("delete: %d %v", n, err)
	}
	if _, ok := h.cache.stockOf(c.ID); ok {
		t.Fatal("deleting a coupon must remove its counter")
	}
	n, err = h.svc.Delete(context.Background(), c.ID)
	if err != nil || n != 0 {
		t.Fatalf("second delete must be an idempotent 0: %d %v", n, err)
	}
}

func TestGetHidesDisabledCouponsFromPublic(t *testing.T) {
	h := newHarness(t, service.Config{})
	c, _ := h.svc.Create(context.Background(), normalCmd())
	if _, err := h.svc.SetStatus(context.Background(), c.ID, service.StatusDisabled); err != nil {
		t.Fatal(err)
	}
	_, err := h.svc.Get(context.Background(), c.ID, false)
	wantCode(t, err, apperr.CouponNotFound)
	if _, err := h.svc.Get(context.Background(), c.ID, true); err != nil {
		t.Fatalf("admin must see disabled coupons: %v", err)
	}
}

func TestListPagingBounds(t *testing.T) {
	h := newHarness(t, service.Config{})
	_, err := h.svc.List(context.Background(), 1, service.MaxPageSize+1, false)
	wantCode(t, err, apperr.InvalidRequest)
	p, err := h.svc.List(context.Background(), 0, 0, false)
	if err != nil || p.Page != 1 || p.Size != service.DefaultPageSize {
		t.Fatalf("defaults: %+v %v", p, err)
	}
}

// 缺省模式（调用方没传 mode）必须只降不升：漏传的调用方不能把库存抬高造成超卖。
func TestSyncDefaultsToMinModeAndNeverRaisesStock(t *testing.T) {
	h := newHarness(t, service.Config{})
	c, _ := h.svc.Create(context.Background(), seckillCmd()) // 库存 50

	resp, err := h.svc.Sync(context.Background(), syncReqMode(c.ID, 500, ""))
	if err != nil || resp.Available != 50 {
		t.Fatalf("default mode with a higher target: %+v %v, want the unchanged 50", resp, err)
	}
	if v, _ := h.cache.stockOf(c.ID); v != 50 {
		t.Fatalf("redis = %d, want 50 (must equal the real DB remaining, not the platform number)", v)
	}
	resp, err = h.svc.Sync(context.Background(), syncReqMode(c.ID, 30, internalapi.SyncModeMin))
	if err != nil || resp.Available != 30 {
		t.Fatalf("min with a lower target: %+v %v, want 30", resp, err)
	}
	if v, _ := h.cache.stockOf(c.ID); v != 30 {
		t.Fatalf("redis = %d, want 30", v)
	}
	if got := h.repo.setAvailMode; len(got) != 2 || got[0] != internalapi.SyncModeMin || got[1] != internalapi.SyncModeMin {
		t.Fatalf("the repo must receive the effective mode (empty -> min), got %v", got)
	}
	_, err = h.svc.Sync(context.Background(), syncReqMode(c.ID, 5, "max"))
	wantCode(t, err, apperr.InvalidRequest)
}

func TestSyncSetsAbsoluteAndRefreshesRedis(t *testing.T) {
	h := newHarness(t, service.Config{})
	c, _ := h.svc.Create(context.Background(), seckillCmd())
	resp, err := h.svc.Sync(context.Background(), syncReq(c.ID, 12))
	if err != nil || resp.Available != 12 {
		t.Fatalf("Sync: %+v %v", resp, err)
	}
	if v, _ := h.cache.stockOf(c.ID); v != 12 {
		t.Fatalf("redis = %d want 12 (= DB remaining)", v)
	}
	_, err = h.svc.Sync(context.Background(), syncReq(c.ID, service.MaxStock+1))
	wantCode(t, err, apperr.InvalidRequest)
	_, err = h.svc.Sync(context.Background(), syncReq(999, 1))
	wantCode(t, err, apperr.CouponNotFound)
}

func TestPreheatOneRules(t *testing.T) {
	h := newHarness(t, service.Config{})
	sk, _ := h.svc.Create(context.Background(), seckillCmd())
	nm, _ := h.svc.Create(context.Background(), normalCmd())
	ctx := context.Background()

	h.cache.mu.Lock()
	h.cache.stock[sk.ID] = 3 // 人为漂移
	h.cache.mu.Unlock()
	res, err := h.svc.PreheatOne(ctx, sk.ID)
	if err != nil || res.Stock != 50 {
		t.Fatalf("PreheatOne: %+v %v", res, err)
	}
	if v, _ := h.cache.stockOf(sk.ID); v != 50 {
		t.Fatalf("admin preheat must overwrite drift, got %d", v)
	}

	_, err = h.svc.PreheatOne(ctx, nm.ID)
	wantCode(t, err, apperr.InvalidRequest)
	_, err = h.svc.PreheatOne(ctx, 424242)
	wantCode(t, err, apperr.CouponNotFound)
	if _, err := h.svc.SetStatus(ctx, sk.ID, service.StatusDisabled); err != nil {
		t.Fatal(err)
	}
	_, err = h.svc.PreheatOne(ctx, sk.ID)
	wantCode(t, err, apperr.CouponDisabled)
}

func TestPreheatAllStartupDoesNotClobberInFlightCounter(t *testing.T) {
	h := newHarness(t, service.Config{PreheatConcurrency: 3})
	ctx := context.Background()
	var ids []int64
	for range 5 {
		c, err := h.svc.Create(ctx, seckillCmd())
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	nm, _ := h.svc.Create(ctx, normalCmd())
	// 模拟重启：两张券的计数器已被在途秒杀扣减，一张缺失。
	h.cache.mu.Lock()
	h.cache.stock[ids[0]] = 41
	h.cache.stock[ids[1]] = 7
	delete(h.cache.stock, ids[2])
	h.cache.mu.Unlock()

	sum, err := h.svc.PreheatAll(ctx, false)
	if err != nil || sum.Total != 5 || sum.Preheated != 5 || sum.Failed != 0 {
		t.Fatalf("PreheatAll(startup): %+v %v", sum, err)
	}
	if v, _ := h.cache.stockOf(ids[0]); v != 41 {
		t.Fatalf("startup preheat must keep the in-flight counter, got %d", v)
	}
	if v, ok := h.cache.stockOf(ids[2]); !ok || v != 50 {
		t.Fatalf("missing counter must be written, got %d,%v", v, ok)
	}
	if _, ok := h.cache.stockOf(nm.ID); ok {
		t.Fatal("normal coupons are never preheated")
	}

	sum, err = h.svc.PreheatAll(ctx, true)
	if err != nil || sum.Preheated != 5 {
		t.Fatalf("PreheatAll(force): %+v %v", sum, err)
	}
	if v, _ := h.cache.stockOf(ids[0]); v != 50 {
		t.Fatalf("forced preheat must realign to DB remaining, got %d", v)
	}
}

func TestPreheatAllReportsFailuresWithoutStoppingOthers(t *testing.T) {
	h := newHarness(t, service.Config{})
	ctx := context.Background()
	for range 3 {
		if _, err := h.svc.Create(ctx, seckillCmd()); err != nil {
			t.Fatal(err)
		}
	}
	h.cache.err = errBoom
	sum, err := h.svc.PreheatAll(ctx, true)
	if err == nil || sum.Failed != 3 || sum.Preheated != 0 {
		t.Fatalf("all three must fail loudly: %+v %v", sum, err)
	}
}

func TestPreheatAllStopsFeedingWhenContextCancelled(t *testing.T) {
	h := newHarness(t, service.Config{PreheatConcurrency: 1})
	for range 4 {
		if _, err := h.svc.Create(context.Background(), seckillCmd()); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sum, err := h.svc.PreheatAll(ctx, true)
	if err == nil {
		t.Fatal("a cancelled preheat must report an error")
	}
	if sum.Preheated+sum.Failed != sum.Total {
		t.Fatalf("every coupon must be accounted for: %+v", sum)
	}
}
