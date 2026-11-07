package cache_test

import (
	"context"
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/cache"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

func TestPutStockOverwritesAndHasNoTTL(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	c := cache.New(rdb)
	ctx := context.Background()
	if err := c.PutStock(ctx, 1001, 50); err != nil {
		t.Fatal(err)
	}
	if err := c.PutStock(ctx, 1001, 33); err != nil {
		t.Fatal(err)
	}
	got, err := mr.Get(keys.Stock(1001))
	if err != nil || got != "33" {
		t.Fatalf("ck:stock:1001 = %q,%v want 33", got, err)
	}
	if ttl := mr.TTL(keys.Stock(1001)); ttl != 0 {
		t.Fatalf("the counter is a ledger and must not expire, ttl=%v", ttl)
	}
}

func TestPutStockIfAbsentKeepsExistingCounter(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	c := cache.New(rdb)
	ctx := context.Background()
	written, err := c.PutStockIfAbsent(ctx, 7, 10)
	if err != nil || !written {
		t.Fatalf("first write: %v %v", written, err)
	}
	if err := rdb.Set(ctx, keys.Stock(7), 4, 0).Err(); err != nil { // order 服务扣减了
		t.Fatal(err)
	}
	written, err = c.PutStockIfAbsent(ctx, 7, 10)
	if err != nil || written {
		t.Fatalf("second write must be skipped: %v %v", written, err)
	}
	if got, _ := mr.Get(keys.Stock(7)); got != "4" {
		t.Fatalf("in-flight counter clobbered: %q", got)
	}
}

func TestDropStockAndMetaTolerateMissingKeys(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	c := cache.New(rdb)
	ctx := context.Background()
	if err := c.DropStock(ctx, 9); err != nil {
		t.Fatalf("DropStock on a missing key: %v", err)
	}
	if err := c.DropMeta(ctx, 9); err != nil {
		t.Fatalf("DropMeta on a missing key: %v", err)
	}
	mr.Set(keys.Stock(9), "3")
	mr.Set(keys.CouponMeta(9), `{"status":1}`)
	if err := c.DropStock(ctx, 9); err != nil {
		t.Fatal(err)
	}
	if err := c.DropMeta(ctx, 9); err != nil {
		t.Fatal(err)
	}
	if mr.Exists(keys.Stock(9)) || mr.Exists(keys.CouponMeta(9)) {
		t.Fatal("keys must be deleted")
	}
}

func TestTakeStockDecrementsOnlyPositiveExistingCounters(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	c := cache.New(rdb)
	ctx := context.Background()

	if err := c.TakeStock(ctx, 5); err != nil {
		t.Fatalf("TakeStock on a missing counter must be a no-op: %v", err)
	}
	if mr.Exists(keys.Stock(5)) {
		t.Fatal("TakeStock must never create a counter (a bare DECR would create -1)")
	}

	mr.Set(keys.Stock(5), "2")
	for range 4 {
		if err := c.TakeStock(ctx, 5); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := mr.Get(keys.Stock(5)); got != "0" {
		t.Fatalf("counter = %q, want 0 (never negative)", got)
	}
}
