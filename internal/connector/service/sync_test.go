package service_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

func status(b domain.Binding) string {
	if b.LastSyncStatus == nil {
		return ""
	}
	return *b.LastSyncStatus
}

func TestScheduledRoundPushesAbsoluteStockAndRecordsSuccess(t *testing.T) {
	h := newHarness(t, nil)
	b := h.bind(1)
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		return qtySnapshot(domain.Mock, sku, 37), nil
	}

	res, ran, err := h.svc.RunScheduled(context.Background())
	if err != nil || !ran {
		t.Fatalf("RunScheduled = %+v, ran=%v, err=%v", res, ran, err)
	}
	if res.SyncedOK != 1 || res.Failed != 0 || res.Skipped != 0 {
		t.Fatalf("result = %+v", res)
	}
	reqs := h.coupon.requests()
	if len(reqs) != 1 || reqs[0].CouponID != 1001 || reqs[0].Available != 37 || reqs[0].Source != "connector" {
		t.Fatalf("coupon requests = %+v", reqs)
	}
	got := h.get(b)
	if status(got) != domain.SyncSuccess || got.LastStock == nil || *got.LastStock != 37 || got.LastError != nil ||
		got.LastSyncAt == nil || !got.LastSyncAt.Equal(h.clk.Now()) {
		t.Fatalf("binding = %+v", got)
	}
	if h.mr.Exists(lockKey) {
		t.Fatal("同步锁必须在一轮结束后释放")
	}
}

func TestPlatformWithoutExactQuantityIsSkippedNeverInflated(t *testing.T) {
	h := newHarness(t, nil)
	b := h.bind(1)
	avail := true
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		return domain.StockSnapshot{Platform: domain.Mock, ExternalSkuID: sku, StockAvailable: &avail}, nil // 有货但无精确数
	}

	res, _, err := h.svc.RunScheduled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 || res.SyncedOK != 0 {
		t.Fatalf("result = %+v, want skipped=1", res)
	}
	if len(h.coupon.requests()) != 0 {
		t.Fatal("没有精确库存时禁止向 coupon 推送（防虚增）")
	}
	got := h.get(b)
	if status(got) != domain.SyncSkip || got.LastError == nil || got.LastSyncAt == nil {
		t.Fatalf("binding = %+v, want SKIP recorded with last_sync_at", got)
	}
}

func TestUnavailableWithoutQuantityPushesZero(t *testing.T) {
	h := newHarness(t, nil)
	h.bind(1)
	none := false
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		return domain.StockSnapshot{Platform: domain.Mock, ExternalSkuID: sku, StockAvailable: &none}, nil
	}
	if _, _, err := h.svc.RunScheduled(context.Background()); err != nil {
		t.Fatal(err)
	}
	reqs := h.coupon.requests()
	if len(reqs) != 1 || reqs[0].Available != 0 {
		t.Fatalf("requests = %+v, want one push of available=0（无货即清零）", reqs)
	}
}

func TestBudgetCutsRoundAndStalestBindingsRotateToFront(t *testing.T) {
	// 10 个绑定，每个绑定“耗时”100ms（推进假时钟），预算 250ms：每轮只来得及处理 3 个。
	h := newHarness(t, func(c *service.SyncConfig) { c.Budget = 250 * time.Millisecond })
	for i := 1; i <= 10; i++ {
		h.bind(i)
	}
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		h.clk.Advance(100 * time.Millisecond)
		return qtySnapshot(domain.Mock, sku, 5), nil
	}

	var results []domain.SyncBatchResult
	for range 4 {
		res, ran, err := h.svc.RunScheduled(context.Background())
		if err != nil || !ran {
			t.Fatalf("round: ran=%v err=%v", ran, err)
		}
		results = append(results, res)
	}

	want := []string{
		"sku-1", "sku-2", "sku-3", // 第 1 轮：全部从未同步，按 id
		"sku-4", "sku-5", "sku-6", // 第 2 轮：1-3 刚同步过，4-6 是最久未同步（NULL）
		"sku-7", "sku-8", "sku-9", // 第 3 轮
		"sku-10", "sku-1", "sku-2", // 第 4 轮：10 仍是 NULL 最先，然后是最老的 1、2
	}
	if got := h.adapter.calls(); !slices.Equal(got, want) {
		t.Fatalf("processing order = %v\nwant              = %v", got, want)
	}
	if results[0].SyncedOK != 3 || results[0].Skipped != 7 {
		t.Fatalf("round 1 = %+v, want ok=3 skipped(deferred)=7", results[0])
	}
	if results[3].SyncedOK != 3 || results[3].Skipped != 7 {
		t.Fatalf("round 4 = %+v", results[3])
	}
}

func TestZeroBudgetDefersEverythingWithoutCallingOut(t *testing.T) {
	h := newHarness(t, func(c *service.SyncConfig) { c.Budget = time.Nanosecond })
	h.bind(1)
	h.bind(2)
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		h.clk.Advance(time.Second)
		return qtySnapshot(domain.Mock, sku, 5), nil
	}
	res, _, err := h.svc.RunScheduled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// 第一条在预算内开始并跑完（预算只在每条开始前检查），第二条被顺延。
	if res.SyncedOK != 1 || res.Skipped != 1 {
		t.Fatalf("result = %+v", res)
	}
}

func TestFailingBindingIsRecordedAndDoesNotStarveTheRest(t *testing.T) {
	h := newHarness(t, func(c *service.SyncConfig) { c.Budget = 50 * time.Millisecond })
	b1 := h.bind(1)
	h.bind(2)
	h.bind(3)
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		h.clk.Advance(100 * time.Millisecond)
		if sku == "sku-1" {
			return domain.StockSnapshot{}, errors.New("JOS timeout") // 持续失败的慢绑定
		}
		return qtySnapshot(domain.Mock, sku, 5), nil
	}

	first, _, err := h.svc.RunScheduled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	h.svc.RunScheduled(context.Background())
	h.svc.RunScheduled(context.Background())

	// 失败也刷新 last_sync_at：否则 sku-1 每轮都排第一并耗尽预算，sku-2/3 会被饿死。
	if got, want := h.adapter.calls(), []string{"sku-1", "sku-2", "sku-3"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if first.Failed != 1 {
		t.Fatalf("first round = %+v, want failed=1", first)
	}
	got := h.get(b1)
	if status(got) != domain.SyncFail || got.LastError == nil || got.LastSyncAt == nil || got.LastStock != nil {
		t.Fatalf("failed binding = %+v", got)
	}
	if len(h.coupon.requests()) != 2 {
		t.Fatalf("coupon pushes = %d, want 2 (sku-2, sku-3)", len(h.coupon.requests()))
	}
}

func TestDisabledPlatformFailsOnlyItsOwnBindings(t *testing.T) {
	h := newHarness(t, nil)
	ok := h.bind(1)
	// 另一个平台的绑定：适配器 DISABLED（缺凭证），注册表据此拒绝。
	jd := &fakeAdapter{platform: domain.JD, status: domain.StatusDisabled}
	svc := h.withAdapters(jd)
	bad, err := h.store.UpsertBinding(context.Background(), domain.BindingCommand{Platform: domain.JD, ExternalSkuID: "jd-1", CouponID: 2002}, true)
	if err != nil {
		t.Fatal(err)
	}

	res, _, err := svc.RunScheduled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.SyncedOK != 1 || res.Failed != 1 {
		t.Fatalf("result = %+v", res)
	}
	if got := h.get(bad); status(got) != domain.SyncFail || got.LastError == nil {
		t.Fatalf("disabled binding = %+v", got)
	}
	if got := h.get(ok); status(got) != domain.SyncSuccess {
		t.Fatalf("mock binding = %+v", got)
	}
	if len(jd.calls()) != 0 {
		t.Fatal("DISABLED 平台不得被调用")
	}
}

// 平台库存读数可能是恒定值（mock 恒为 500）或过期值：自动同步与普通手工同步必须以「只降不升」模式推送，
// 否则每一轮都会把已卖出的库存重置回去造成超卖。只有管理员显式 force 才允许抬高。
func TestSyncPushesMinModeUnlessForced(t *testing.T) {
	h := newHarness(t, nil)
	h.bind(1)
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		return qtySnapshot(domain.Mock, sku, 500), nil
	}

	if _, _, err := h.svc.RunScheduled(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SyncAll(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SyncAll(context.Background(), true); err != nil {
		t.Fatal(err)
	}

	reqs := h.coupon.requests()
	if len(reqs) != 3 {
		t.Fatalf("pushes = %d, want 3 (scheduled, manual, forced)", len(reqs))
	}
	for i, want := range []string{internalapi.SyncModeMin, internalapi.SyncModeMin, internalapi.SyncModeSet} {
		if reqs[i].Mode != want || reqs[i].Available != 500 {
			t.Errorf("push %d = %+v, want mode %q with the platform value 500", i, reqs[i], want)
		}
	}
}

func TestSyncOneForcedPushesSetMode(t *testing.T) {
	h := newHarness(t, nil)
	b := h.bind(1)
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		return qtySnapshot(domain.Mock, sku, 80), nil
	}
	if _, err := h.svc.SyncOne(context.Background(), int64(b.ID), false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SyncOne(context.Background(), int64(b.ID), true); err != nil {
		t.Fatal(err)
	}
	reqs := h.coupon.requests()
	if len(reqs) != 2 || reqs[0].Mode != internalapi.SyncModeMin || reqs[1].Mode != internalapi.SyncModeSet {
		t.Fatalf("pushes = %+v, want [min, set]", reqs)
	}
}

func TestSyncStockFailureIsNotRetriedInRoundAndRetriedNextRound(t *testing.T) {
	h := newHarness(t, nil)
	b := h.bind(1)
	var failing = true
	var mu sync.Mutex
	h.coupon.err = func(internalapi.SyncRequest) error {
		mu.Lock()
		defer mu.Unlock()
		if failing {
			return internalapi.ErrTransient
		}
		return nil
	}
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		return qtySnapshot(domain.Mock, sku, 40), nil
	}

	first, _, err := h.svc.RunScheduled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Failed != 1 || len(h.coupon.requests()) != 1 {
		t.Fatalf("first round = %+v, pushes = %d; want failed=1 and exactly one push（循环自己不重试）", first, len(h.coupon.requests()))
	}
	got := h.get(b)
	if status(got) != domain.SyncFail || got.LastStock != nil || got.LastError == nil {
		t.Fatalf("after failure = %+v (last_stock 不能被未确认的推送改动)", got)
	}

	mu.Lock()
	failing = false
	mu.Unlock()
	h.clk.Advance(time.Minute)
	second, _, err := h.svc.RunScheduled(context.Background())
	if err != nil || second.SyncedOK != 1 {
		t.Fatalf("second round = %+v, %v", second, err)
	}
	reqs := h.coupon.requests()
	if len(reqs) != 2 || reqs[0] != reqs[1] {
		t.Fatalf("requests = %+v, want two identical absolute sets（绝对值设置，重放不会叠加）", reqs)
	}
	if got := h.get(b); status(got) != domain.SyncSuccess || *got.LastStock != 40 || got.LastError != nil {
		t.Fatalf("after recovery = %+v", got)
	}
}

func TestSyncStockBusinessRejectionIsRecordedAsFailure(t *testing.T) {
	h := newHarness(t, nil)
	b := h.bind(1)
	h.coupon.err = func(internalapi.SyncRequest) error { return apperr.CouponNotFound }

	res, _, err := h.svc.RunScheduled(context.Background())
	if err != nil || res.Failed != 1 {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if got := h.get(b); status(got) != domain.SyncFail || got.LastError == nil {
		t.Fatalf("binding = %+v", got)
	}
}

func TestScheduledRoundSkipsWhenLockHeldElsewhere(t *testing.T) {
	h := newHarness(t, nil)
	h.bind(1)
	h.mr.Set(lockKey, "someone-else")

	res, ran, err := h.svc.RunScheduled(context.Background())
	if err != nil || ran || res != (domain.SyncBatchResult{}) {
		t.Fatalf("RunScheduled = %+v, ran=%v, err=%v; want skipped silently", res, ran, err)
	}
	if len(h.adapter.calls()) != 0 {
		t.Fatal("拿不到锁必须完全不动作")
	}
	if v, _ := h.mr.Get(lockKey); v != "someone-else" {
		t.Fatalf("lock value = %q: 不得动别人的锁", v)
	}
}

func TestScheduledRoundReturnsErrorWhenRedisIsDown(t *testing.T) {
	h := newHarness(t, nil)
	h.bind(1)
	h.mr.Close()

	_, ran, err := h.svc.RunScheduled(context.Background())
	if err == nil || ran {
		t.Fatalf("ran=%v err=%v, want an error and no work", ran, err)
	}
	if len(h.adapter.calls()) != 0 {
		t.Fatal("Redis 不可用时（拿不到锁）不得同步")
	}
}

func TestManualAndScheduledSyncExcludeEachOther(t *testing.T) {
	h := newHarness(t, nil)
	b := h.bind(1)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		once.Do(func() { close(entered) })
		<-release
		return qtySnapshot(domain.Mock, sku, 5), nil
	}

	done := make(chan error, 1)
	go func() {
		_, _, err := h.svc.RunScheduled(context.Background())
		done <- err
	}()
	<-entered // 定时批次已持锁并停在平台调用里

	if _, err := h.svc.SyncOne(context.Background(), int64(b.ID), false); !domain.IsSyncBusy(err) {
		t.Fatalf("manual SyncOne err = %v, want 409 busy", err)
	}
	if _, err := h.svc.SyncAll(context.Background(), false); !domain.IsSyncBusy(err) {
		t.Fatalf("manual SyncAll err = %v, want 409 busy", err)
	}
	if got := len(h.adapter.calls()); got != 1 {
		t.Fatalf("platform calls = %d: 手工同步不得与定时批次在同一券上交错", got)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SyncOne(context.Background(), int64(b.ID), false); err != nil {
		t.Fatalf("manual SyncOne after batch finished: %v", err)
	}
}

func TestTwoConcurrentManualSyncsOfSameBindingRunExactlyOnce(t *testing.T) {
	h := newHarness(t, nil)
	b := h.bind(1)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		once.Do(func() { close(entered) })
		<-release
		return qtySnapshot(domain.Mock, sku, 5), nil
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, errs[0] = h.svc.SyncOne(context.Background(), int64(b.ID), false)
	}()
	<-entered
	_, errs[1] = h.svc.SyncOne(context.Background(), int64(b.ID), false)
	close(release)
	wg.Wait()

	if errs[0] != nil || !domain.IsSyncBusy(errs[1]) {
		t.Fatalf("errs = %v, %v; want first ok and second busy", errs[0], errs[1])
	}
	if got := len(h.coupon.requests()); got != 1 {
		t.Fatalf("pushes = %d, want exactly 1", got)
	}
}

func TestLockLostDuringPlatformCallNeverWritesStockAndStopsRound(t *testing.T) {
	h := newHarness(t, nil)
	b1, b2, b3 := h.bind(1), h.bind(2), h.bind(3)
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		if sku == "sku-2" {
			h.mr.Set(lockKey, "stolen-by-another-instance") // 平台调用期间锁过期并被别人拿走
		}
		return qtySnapshot(domain.Mock, sku, 5), nil
	}

	res, _, err := h.svc.RunScheduled(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	reqs := h.coupon.requests()
	if len(reqs) != 1 || reqs[0].CouponID != 1001 {
		t.Fatalf("pushes = %+v: 丢锁后一律不得再写 coupon 库存", reqs)
	}
	if res.SyncedOK != 1 || res.Skipped != 2 || res.Failed != 0 {
		t.Fatalf("result = %+v, want ok=1 deferred=2 failed=0", res)
	}
	if status(h.get(b1)) != domain.SyncSuccess {
		t.Fatalf("binding 1 = %+v", h.get(b1))
	}
	for _, b := range []domain.Binding{b2, b3} {
		if got := h.get(b); got.LastSyncAt != nil || status(got) != domain.SyncNew {
			t.Fatalf("binding %d = %+v: 丢锁顺延的绑定必须原样保留，下一轮排在最前", b.ID, got)
		}
	}
	if got := h.adapter.calls(); !slices.Equal(got, []string{"sku-1", "sku-2"}) {
		t.Fatalf("platform calls = %v, want sku-3 never touched", got)
	}
	if v, _ := h.mr.Get(lockKey); v != "stolen-by-another-instance" {
		t.Fatalf("lock = %q: 释放时不得删掉别人的锁", v)
	}
}

func TestLockLostBetweenBindingsStopsBeforeNextBinding(t *testing.T) {
	h := newHarness(t, nil)
	h.bind(1)
	h.bind(2)
	h.coupon.after = func(req internalapi.SyncRequest) {
		if req.CouponID == 1001 {
			h.mr.Del(lockKey) // 第一条写完后锁过期
		}
	}

	res, _, err := h.svc.RunScheduled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.SyncedOK != 1 || res.Skipped != 1 {
		t.Fatalf("result = %+v", res)
	}
	if got := h.adapter.calls(); !slices.Equal(got, []string{"sku-1"}) {
		t.Fatalf("platform calls = %v: 每条开始前必须先续租校验", got)
	}
}

func TestEachBindingRenewsTheLease(t *testing.T) {
	h := newHarness(t, func(c *service.SyncConfig) { c.LockTTL = 60 * time.Second })
	h.bind(1)
	h.bind(2)
	ttls := []time.Duration{}
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		h.mr.FastForward(40 * time.Second) // 每条“耗时”40s：不续租的话第二条开始前锁已过期
		ttls = append(ttls, h.mr.TTL(lockKey))
		return qtySnapshot(domain.Mock, sku, 5), nil
	}

	res, _, err := h.svc.RunScheduled(context.Background())
	if err != nil || res.SyncedOK != 2 {
		t.Fatalf("result = %+v, %v; 逐条续租后两条都应成功", res, err)
	}
	for i, ttl := range ttls {
		if ttl < 15*time.Second {
			t.Fatalf("ttl before binding %d = %v, want the lease renewed to ~60s then 40s consumed", i+1, ttl)
		}
	}
}

func TestManualSyncOneReturnsBindingWithOutcome(t *testing.T) {
	h := newHarness(t, nil)
	b := h.bind(1)
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		return qtySnapshot(domain.Mock, sku, 12), nil
	}

	got, err := h.svc.SyncOne(context.Background(), int64(b.ID), true)
	if err != nil {
		t.Fatal(err)
	}
	if status(got) != domain.SyncSuccess || *got.LastStock != 12 {
		t.Fatalf("binding = %+v", got)
	}
	if h.mr.Exists(lockKey) {
		t.Fatal("手工同步结束后必须释放锁")
	}

	// 平台失败：返回带 FAIL 状态的绑定（前端据此展示），锁照样释放。
	h.adapter.stockFn = func(context.Context, string) (domain.StockSnapshot, error) {
		return domain.StockSnapshot{}, errors.New("boom")
	}
	got, err = h.svc.SyncOne(context.Background(), int64(b.ID), false)
	if err != nil || status(got) != domain.SyncFail || got.LastError == nil {
		t.Fatalf("failed manual sync = %+v, %v", got, err)
	}
	if h.mr.Exists(lockKey) {
		t.Fatal("失败后也必须释放锁")
	}
}

func TestManualSyncOneMissingBindingIs404WithoutTakingTheLock(t *testing.T) {
	h := newHarness(t, nil)
	h.mr.Set(lockKey, "other")
	_, err := h.svc.SyncOne(context.Background(), 424242, false)
	if !domain.IsBindingNotFound(err) {
		t.Fatalf("err = %v, want binding not found (404 先于 409)", err)
	}
}

func TestManualSyncOneLockLostBeforeWriteIsConflict(t *testing.T) {
	h := newHarness(t, nil)
	b := h.bind(1)
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		h.mr.Del(lockKey)
		return qtySnapshot(domain.Mock, sku, 5), nil
	}
	_, err := h.svc.SyncOne(context.Background(), int64(b.ID), false)
	if !domain.IsSyncBusy(err) {
		t.Fatalf("err = %v, want 409 lock-lost", err)
	}
	if len(h.coupon.requests()) != 0 {
		t.Fatal("丢锁后不得写库存")
	}
}

func TestManualSyncAllIsUnboundedAndEchoesForce(t *testing.T) {
	h := newHarness(t, func(c *service.SyncConfig) { c.Budget = time.Nanosecond })
	for i := 1; i <= 3; i++ {
		h.bind(i)
	}
	h.adapter.stockFn = func(_ context.Context, sku string) (domain.StockSnapshot, error) {
		h.clk.Advance(time.Minute)
		return qtySnapshot(domain.Mock, sku, 5), nil
	}
	res, err := h.svc.SyncAll(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if res.SyncedOK != 3 || res.Skipped != 0 || !res.Force {
		t.Fatalf("result = %+v: 手工全量同步不受定时预算限制", res)
	}
}

func TestCancelledContextStopsRoundWithoutRecordingFailures(t *testing.T) {
	h := newHarness(t, nil)
	b1 := h.bind(1)
	b2 := h.bind(2)
	ctx, cancel := context.WithCancel(context.Background())
	h.adapter.stockFn = func(ctx context.Context, sku string) (domain.StockSnapshot, error) {
		cancel() // 服务关闭：调用方 ctx 被取消
		return domain.StockSnapshot{}, ctx.Err()
	}

	res, _, err := h.svc.RunScheduled(ctx)
	if err != nil {
		t.Fatalf("shutdown is not an error: %v", err)
	}
	if res.Failed != 0 || res.SyncedOK != 0 {
		t.Fatalf("result = %+v", res)
	}
	for _, b := range []domain.Binding{b1, b2} {
		if got := h.get(b); got.LastSyncAt != nil {
			t.Fatalf("binding %d recorded during shutdown: %+v（ctx 取消不是平台故障，不能记 FAIL）", b.ID, got)
		}
	}
	if h.mr.Exists(lockKey) {
		t.Fatal("取消后也必须释放锁")
	}
}
