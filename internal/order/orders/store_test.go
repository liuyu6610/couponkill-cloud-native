package orders_test

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders/orderstest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
)

var nextID atomic.Int64

func newOrder(user, coupon int64, request string) orders.NewOrder {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return orders.NewOrder{
		ID: 1_000_000 + nextID.Add(1), UserID: user, CouponID: coupon, RequestID: request,
		CouponName: "券" + strconv.FormatInt(coupon, 10), CouponType: orders.CouponTypeSeckill,
		FaceValue: "50.00", MinSpend: "100.00", Source: orders.SourceSeckill,
		Now: now, ExpireAt: now.Add(24 * time.Hour),
	}
}

func mustInsert(t *testing.T, pool *pgxpool.Pool, o orders.NewOrder) {
	t.Helper()
	if _, inserted, err := orders.Insert(context.Background(), pool, o); err != nil || !inserted {
		t.Fatalf("Insert(%s) = inserted %v err %v", o.RequestID, inserted, err)
	}
}

func rowCount(t *testing.T, pool *pgxpool.Pool, where string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM ord.orders WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// 去掉 ON CONFLICT (user_id, request_id) DO NOTHING，同一请求号的重放会变成唯一冲突错误，本测试变红。
func TestInsertAbsorbsReplayOfSameRequest(t *testing.T) {
	pool := orderstest.NewPool(t)
	ctx := context.Background()
	o := newOrder(42, 1001, "req-1")
	mustInsert(t, pool, o)

	replay := o
	replay.ID = o.ID + 777
	id, inserted, err := orders.Insert(ctx, pool, replay)
	if err != nil {
		t.Fatalf("replay must be absorbed without error, got %v", err)
	}
	if inserted || id != 0 {
		t.Fatalf("replay reported inserted=%v id=%d, want a no-op", inserted, id)
	}
	if n := rowCount(t, pool, "user_id = 42"); n != 1 {
		t.Fatalf("rows = %d, want exactly 1", n)
	}
	got, ok, err := orders.FindByRequest(ctx, pool, 42, "req-1")
	if err != nil || !ok || got.ID != o.ID || string(got.FaceValue) != "50.00" || got.Status != orders.StatusCreated {
		t.Fatalf("FindByRequest = %+v ok=%v err=%v", got, ok, err)
	}
}

// 不同请求号、同一用户同一张券：命中部分唯一索引。错误必须能被 IsUniqueViolation(err, "") 识别
// （分区表上约束名是分区索引名，不能按名字匹配）；取消后同一张券可以再领。
func TestInsertSecondActiveOrderOfSameCouponIsUniqueViolationUntilCancelled(t *testing.T) {
	pool := orderstest.NewPool(t)
	ctx := context.Background()
	first := newOrder(42, 1001, "req-a")
	mustInsert(t, pool, first)

	_, _, err := orders.Insert(ctx, pool, newOrder(42, 1001, "req-b"))
	if !dbx.IsUniqueViolation(err, "") {
		t.Fatalf("second active order err = %v, want a unique violation", err)
	}
	if dbx.IsUniqueViolation(err, "uk_orders_user_coupon_active") {
		t.Fatalf("partition index names leak the parent name now; the 'never match on constraint name' note needs revisiting")
	}

	if ok, err := orders.Cancel(ctx, pool, 42, first.ID, time.Now()); err != nil || !ok {
		t.Fatalf("Cancel = %v, %v", ok, err)
	}
	mustInsert(t, pool, newOrder(42, 1001, "req-c"))
	if has, err := orders.HasActive(ctx, pool, 42, 1001); err != nil || !has {
		t.Fatalf("HasActive after re-grab = %v, %v", has, err)
	}
}

// 去掉 `AND status = 1`，第二次取消也会「成功」，调用方就会重复回补库存。
func TestCancelOnlyMovesCreatedOrdersAndExactlyOneConcurrentCancelWins(t *testing.T) {
	pool := orderstest.NewPool(t)
	ctx := context.Background()
	o := newOrder(7, 1001, "req-cancel")
	mustInsert(t, pool, o)

	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := orders.Cancel(ctx, pool, 7, o.ID, time.Now())
			if err != nil {
				t.Errorf("Cancel: %v", err)
				return
			}
			if ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("%d concurrent cancels succeeded, want exactly 1", wins.Load())
	}
	got, _, _ := orders.FindByID(ctx, pool, 7, o.ID)
	if got.Status != orders.StatusCancelled || got.CancelTime == nil {
		t.Fatalf("order after cancel = %+v", got)
	}

	// 已使用的订单不能取消。
	used := newOrder(7, 1002, "req-used")
	mustInsert(t, pool, used)
	if _, err := pool.Exec(ctx, `UPDATE ord.orders SET status = 2, use_time = now() WHERE user_id = 7 AND id = $1`, used.ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := orders.Cancel(ctx, pool, 7, used.ID, time.Now()); err != nil || ok {
		t.Fatalf("cancelling a used order = %v, %v; want false, nil", ok, err)
	}
}

func TestCountsListAndAdminFilters(t *testing.T) {
	pool := orderstest.NewPool(t)
	ctx := context.Background()
	for i, spec := range []struct {
		user, coupon int64
		typ          int
	}{{5, 1, orders.CouponTypeSeckill}, {5, 2, orders.CouponTypeNormal}, {5, 3, orders.CouponTypeNormal}, {6, 1, orders.CouponTypeSeckill}} {
		o := newOrder(spec.user, spec.coupon, "req-list-"+strconv.Itoa(i))
		o.CouponType = spec.typ
		o.Now = o.Now.Add(time.Duration(i) * time.Second)
		mustInsert(t, pool, o)
	}
	if _, err := pool.Exec(ctx, `UPDATE ord.orders SET status = 3 WHERE user_id = 5 AND coupon_id = 3`); err != nil {
		t.Fatal(err)
	}

	c, err := orders.CountsByUser(ctx, pool, 5)
	if err != nil || c != (orders.Counts{Total: 2, Seckill: 1, Normal: 1, Expired: 1}) {
		t.Fatalf("CountsByUser = %+v, %v", c, err)
	}

	items, total, err := orders.ListByUser(ctx, pool, 5, 1, 1)
	if err != nil || total != 3 || len(items) != 1 {
		t.Fatalf("ListByUser page 2 = %d items total %d err %v", len(items), total, err)
	}

	items, total, err = orders.ListAll(ctx, pool, orders.AdminFilter{CouponID: 1}, 0, 10)
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("ListAll coupon=1 = %d items total %d err %v", len(items), total, err)
	}
	items, total, err = orders.ListAll(ctx, pool, orders.AdminFilter{UserID: 5, Status: orders.StatusExpired}, 0, 10)
	if err != nil || total != 1 || len(items) != 1 || items[0].CouponID != 3 {
		t.Fatalf("ListAll user=5 status=3 = %+v total %d err %v", items, total, err)
	}
}
