package repo_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/repo"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
)

// newStore 在一次性数据库里跑完 cnx 迁移，返回连到它的 Store。
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

func bindingCmd(p domain.Platform, sku string, coupon int64) domain.BindingCommand {
	return domain.BindingCommand{Platform: p, ExternalSkuID: sku, CouponID: idgen.ID(coupon)}
}

func TestUpsertBindingCreatesThenUpdatesInPlace(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	created, err := s.UpsertBinding(ctx, bindingCmd(domain.Mock, "sku-1", 1001), true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 || created.Platform != domain.Mock || created.CouponID != 1001 || !created.SyncEnabled ||
		created.LastSyncStatus == nil || *created.LastSyncStatus != domain.SyncNew || created.LastSyncAt != nil {
		t.Fatalf("created = %+v", created)
	}

	// 同平台同 SKU 再次提交：原地更新（同一个 id），只改启用开关。
	updated, err := s.UpsertBinding(ctx, bindingCmd(domain.Mock, "sku-1", 1001), false)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.ID != created.ID || updated.SyncEnabled {
		t.Fatalf("updated = %+v, want same id and sync disabled", updated)
	}
}

func TestUpsertBindingRejectsConflicts(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.UpsertBinding(ctx, bindingCmd(domain.Mock, "sku-1", 1001), true); err != nil {
		t.Fatal(err)
	}

	// 一张券只能绑一个平台 SKU（避免多源覆盖库存）。
	if _, err := s.UpsertBinding(ctx, bindingCmd(domain.JD, "other", 1001), true); !domain.IsBindingConflict(err) {
		t.Fatalf("same coupon, other sku: err = %v, want ErrBindingConflict", err)
	}
	// 另一张券想绑已被占用的 SKU：改绑的目标券已有别的绑定时同样冲突。
	if _, err := s.UpsertBinding(ctx, bindingCmd(domain.JD, "jd-1", 2002), true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertBinding(ctx, bindingCmd(domain.JD, "jd-1", 1001), true); !domain.IsBindingConflict(err) {
		t.Fatalf("rebind sku onto taken coupon: err = %v, want ErrBindingConflict", err)
	}
	// 失败的更新不得改动原绑定。
	b, err := s.FindBindingByCoupon(ctx, 2002)
	if err != nil || b == nil || b.ExternalSkuID != "jd-1" {
		t.Fatalf("binding for 2002 = %+v, %v", b, err)
	}
}

func TestConcurrentUpsertsForSameCouponYieldExactlyOneWinner(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.UpsertBinding(ctx, bindingCmd(domain.Mock, "sku-"+string(rune('a'+i)), 7), true)
		}()
	}
	wg.Wait()

	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case !domain.IsBindingConflict(err):
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("wins = %d, want exactly 1（并发创建不得产生两条同券绑定，也不得出现 500）", wins)
	}
}

func TestListAndLookups(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	a, _ := s.UpsertBinding(ctx, bindingCmd(domain.Mock, "a", 1), true)
	b, _ := s.UpsertBinding(ctx, bindingCmd(domain.Mock, "b", 2), true)

	all, err := s.ListBindings(ctx)
	if err != nil || len(all) != 2 || all[0].ID != b.ID || all[1].ID != a.ID {
		t.Fatalf("ListBindings = %+v, %v; want newest id first", all, err)
	}
	if got, err := s.GetBinding(ctx, int64(a.ID)); err != nil || got.ID != a.ID {
		t.Fatalf("GetBinding = %+v, %v", got, err)
	}
	if _, err := s.GetBinding(ctx, 424242); !domain.IsBindingNotFound(err) {
		t.Fatalf("GetBinding missing err = %v, want ErrBindingNotFound", err)
	}
	if got, err := s.FindBindingByCoupon(ctx, 999); err != nil || got != nil {
		t.Fatalf("FindBindingByCoupon missing = %+v, %v; want nil,nil", got, err)
	}
}

func TestRecordSyncKeepsLastStockOnFailureAndStampsTime(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	b, _ := s.UpsertBinding(ctx, bindingCmd(domain.Mock, "a", 1), true)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	stock := int64(120)
	ok, err := s.RecordSync(ctx, int64(b.ID), domain.SyncOutcome{Status: domain.SyncSuccess, LastStock: &stock, At: t0})
	if err != nil {
		t.Fatal(err)
	}
	if ok.LastStock == nil || *ok.LastStock != 120 || *ok.LastSyncStatus != domain.SyncSuccess || ok.LastError != nil ||
		ok.LastSyncAt == nil || !ok.LastSyncAt.Equal(t0) {
		t.Fatalf("after success = %+v", ok)
	}

	msg := "platform timeout"
	failed, err := s.RecordSync(ctx, int64(b.ID), domain.SyncOutcome{Status: domain.SyncFail, Error: &msg, At: t0.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if failed.LastStock == nil || *failed.LastStock != 120 {
		t.Fatalf("failure must not erase last_stock, got %v", failed.LastStock)
	}
	if *failed.LastSyncStatus != domain.SyncFail || failed.LastError == nil || *failed.LastError != msg || !failed.LastSyncAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("after failure = %+v (失败也必须刷新 last_sync_at)", failed)
	}

	if _, err := s.RecordSync(ctx, 424242, domain.SyncOutcome{Status: domain.SyncFail, At: t0}); !domain.IsBindingNotFound(err) {
		t.Fatalf("RecordSync missing err = %v, want ErrBindingNotFound", err)
	}
}

func TestListSyncCandidatesIsStalestFirstAndSkipsDisabled(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	mk := func(sku string, coupon int64, enabled bool) domain.Binding {
		b, err := s.UpsertBinding(ctx, bindingCmd(domain.Mock, sku, coupon), enabled)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	recent, old, never, tie, disabled := mk("recent", 1, true), mk("old", 2, true), mk("never", 3, true), mk("tie", 4, true), mk("off", 5, false)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for id, at := range map[idgen.ID]time.Time{recent.ID: base.Add(10 * time.Minute), old.ID: base, tie.ID: base, disabled.ID: base.Add(-time.Hour)} {
		if _, err := s.RecordSync(ctx, int64(id), domain.SyncOutcome{Status: domain.SyncSuccess, At: at}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListSyncCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ids []idgen.ID
	for _, b := range got {
		ids = append(ids, b.ID)
	}
	// 从未同步的最先，其后按 last_sync_at 升序，同时间按 id；停用的不出现。
	want := []idgen.ID{never.ID, old.ID, tie.ID, recent.ID}
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	}
}

func priceCmd(coupon int64, p domain.Platform, sku string, price string) domain.PriceMapCommand {
	cmd := domain.PriceMapCommand{CouponID: idgen.ID(coupon), Platform: p, ExternalSkuID: sku}
	if price != "" {
		n := json.Number(price)
		cmd.ManualPrice = &n
	}
	return cmd
}

func TestPriceMapUpsertIsUniquePerCouponAndPlatform(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	title := "同款"

	first := priceCmd(1, domain.JD, "jd-1", "99.5")
	first.Title = &title
	created, err := s.UpsertPriceMap(ctx, first, "CNY", true)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.ManualPrice == nil || created.ManualPrice.String() != "99.50" || created.Currency != "CNY" || !created.Enabled {
		t.Fatalf("created = %+v (numeric(12,2) 应回读为 99.50)", created)
	}

	// 同 (coupon, platform) 再次提交：同一行被更新，不会产生第二行。
	updated, err := s.UpsertPriceMap(ctx, priceCmd(1, domain.JD, "jd-2", ""), "USD", false)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.ExternalSkuID != "jd-2" || updated.ManualPrice != nil || updated.Currency != "USD" || updated.Enabled || updated.Title != nil {
		t.Fatalf("updated = %+v", updated)
	}
	if _, err := s.UpsertPriceMap(ctx, priceCmd(1, domain.TB, "tb-1", "10"), "CNY", true); err != nil {
		t.Fatal(err)
	}

	all, _ := s.ListPriceMaps(ctx, 1, false)
	enabled, _ := s.ListPriceMaps(ctx, 1, true)
	if len(all) != 2 || len(enabled) != 1 || enabled[0].Platform != domain.TB {
		t.Fatalf("all=%d enabled=%+v", len(all), enabled)
	}
	if other, _ := s.ListPriceMaps(ctx, 2, false); len(other) != 0 {
		t.Fatalf("other coupon rows = %+v", other)
	}
}

func TestPriceMapDeleteIsIdempotent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	pm, err := s.UpsertPriceMap(ctx, priceCmd(1, domain.JD, "jd-1", "1"), "CNY", true)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.DeletePriceMap(ctx, int64(pm.ID)); err != nil {
			t.Fatalf("delete: %v", err)
		}
	}
	if rows, _ := s.ListPriceMaps(ctx, 1, false); len(rows) != 0 {
		t.Fatalf("rows after delete = %+v", rows)
	}
}
