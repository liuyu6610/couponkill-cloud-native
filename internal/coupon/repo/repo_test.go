package repo_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/repo"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
)

type env struct {
	pool *pgxpool.Pool
	repo *repo.Repo
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := dbxtest.New(t)
	if err := dbx.Migrate(context.Background(), pool, "cpn", migrations.FS); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return &env{pool: pool, repo: repo.New(pool)}
}

// addCoupon 创建一张启用的券，库存按 buckets 个桶均分。
func (e *env) addCoupon(t *testing.T, id int64, typ int, stock int64, buckets int) {
	t.Helper()
	nc := service.NewCoupon{
		ID: id, Name: fmt.Sprintf("券%d", id), Type: typ, FaceValue: "10.00", MinSpend: "0.00",
		ValidDays: 7, PerUserLimit: 1, TotalStock: stock,
	}
	if typ == service.TypeSeckill {
		s, en := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
		nc.SeckillStartAt, nc.SeckillEndAt = &s, &en
	}
	if _, err := e.repo.Create(context.Background(), nc, service.Distribute(stock, buckets)); err != nil {
		t.Fatalf("Create(%d): %v", id, err)
	}
}

func (e *env) scalar(t *testing.T, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func (e *env) remaining(t *testing.T, id int64) int64 {
	t.Helper()
	return e.scalar(t, `SELECT COALESCE(sum(remaining),0) FROM cpn.coupon_stock WHERE coupon_id=$1`, id)
}

func (e *env) minBucket(t *testing.T, id int64) int64 {
	t.Helper()
	return e.scalar(t, `SELECT COALESCE(min(remaining),0) FROM cpn.coupon_stock WHERE coupon_id=$1`, id)
}

func (e *env) ledger(t *testing.T, id int64, state string) int64 {
	t.Helper()
	return e.scalar(t, `SELECT count(*) FROM cpn.stock_ledger WHERE coupon_id=$1 AND state=$2`, id, state)
}

func deduct(r *repo.Repo, rid string, coupon, user int64) (service.DeductResult, error) {
	return r.Deduct(context.Background(), internalapi.DeductRequest{RequestID: rid, CouponID: coupon, UserID: user})
}

func release(r *repo.Repo, rid string, coupon int64) (service.ReleaseResult, error) {
	return r.Release(context.Background(), internalapi.ReleaseRequest{RequestID: rid, CouponID: coupon})
}

// 不变式：每个 requestId 至多扣一次；200 个并发请求抢 50 件，恰好 50 个成功。
func TestConcurrentDeductNeverOversellsAndLedgerMatches(t *testing.T) {
	e := newEnv(t)
	const coupon = 1001
	e.addCoupon(t, coupon, service.TypeSeckill, 50, 8)

	var ok, soldOut, other atomic.Int32
	var wg sync.WaitGroup
	for i := range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := deduct(e.repo, fmt.Sprintf("req-%d", i), coupon, int64(10_000+i))
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, apperr.OutOfStock):
				soldOut.Add(1)
			default:
				other.Add(1)
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if ok.Load() != 50 || soldOut.Load() != 150 || other.Load() != 0 {
		t.Fatalf("ok=%d soldOut=%d other=%d, want 50/150/0", ok.Load(), soldOut.Load(), other.Load())
	}
	if rem := e.remaining(t, coupon); rem != 0 {
		t.Fatalf("remaining = %d, want 0", rem)
	}
	if m := e.minBucket(t, coupon); m < 0 {
		t.Fatalf("a bucket went negative: %d", m)
	}
	if n := e.ledger(t, coupon, "deducted"); n != 50 {
		t.Fatalf("ledger deducted rows = %d, want 50", n)
	}
	if n := e.scalar(t, `SELECT count(*) FROM cpn.stock_ledger WHERE coupon_id=$1`, coupon); n != 50 {
		t.Fatalf("failed attempts must leave no ledger rows, total rows = %d", n)
	}
}

// 同一个 requestId 并发 20 次：恰好 1 次生效，所有响应指向同一个桶。
func TestSameRequestIDDeductedConcurrentlyHasExactlyOneEffect(t *testing.T) {
	e := newEnv(t)
	const coupon = 1002
	e.addCoupon(t, coupon, service.TypeSeckill, 100, 8)

	results := make([]service.DeductResult, 20)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := deduct(e.repo, "dup-req", coupon, 77)
			if err != nil {
				t.Errorf("deduct %d: %v", i, err)
			}
			results[i] = r
		}()
	}
	wg.Wait()

	fresh := 0
	for _, r := range results {
		if !r.Already {
			fresh++
		}
		if r.Bucket != results[0].Bucket {
			t.Fatalf("replays must report the first bucket: %+v vs %+v", r, results[0])
		}
	}
	if fresh != 1 {
		t.Fatalf("fresh deductions = %d, want exactly 1", fresh)
	}
	if rem := e.remaining(t, coupon); rem != 99 {
		t.Fatalf("remaining = %d, want 99 (one unit taken)", rem)
	}
	if n := e.ledger(t, coupon, "deducted"); n != 1 {
		t.Fatalf("ledger rows = %d, want 1", n)
	}
}

func TestReleaseRestoresStockAndLaterDeductOfSameRequestIsRejected(t *testing.T) {
	e := newEnv(t)
	const coupon = 1003
	e.addCoupon(t, coupon, service.TypeSeckill, 10, 4)

	first, err := deduct(e.repo, "r1", coupon, 5)
	if err != nil || first.Already {
		t.Fatalf("deduct: %+v %v", first, err)
	}
	if rem := e.remaining(t, coupon); rem != 9 {
		t.Fatalf("remaining after deduct = %d", rem)
	}
	rel, err := release(e.repo, "r1", coupon)
	if err != nil || !rel.Released || rel.Already {
		t.Fatalf("release: %+v %v", rel, err)
	}
	if rem := e.remaining(t, coupon); rem != 10 {
		t.Fatalf("remaining after release = %d, want 10", rem)
	}
	if _, err := deduct(e.repo, "r1", coupon, 5); !errors.Is(err, apperr.OutOfStock) {
		t.Fatalf("re-deduct of a released request must be OutOfStock, got %v", err)
	}
	if rem := e.remaining(t, coupon); rem != 10 {
		t.Fatalf("the rejected re-deduct must not take stock, remaining = %d", rem)
	}
}

func TestReleaseWithoutDeductWritesTombstoneThatBlocksLateDeduct(t *testing.T) {
	e := newEnv(t)
	const coupon = 1004
	e.addCoupon(t, coupon, service.TypeSeckill, 10, 4)

	rel, err := release(e.repo, "ghost", coupon)
	if err != nil || rel.Released || rel.Already {
		t.Fatalf("tombstone release must report neither released nor already: %+v %v", rel, err)
	}
	if n := e.ledger(t, coupon, "released"); n != 1 {
		t.Fatalf("tombstone rows = %d, want 1", n)
	}
	if _, err := deduct(e.repo, "ghost", coupon, 5); !errors.Is(err, apperr.OutOfStock) {
		t.Fatalf("a late deduct after the tombstone must be OutOfStock, got %v", err)
	}
	if rem := e.remaining(t, coupon); rem != 10 {
		t.Fatalf("stock must be untouched, remaining = %d", rem)
	}
}

func TestDuplicateReleaseIsNoOp(t *testing.T) {
	e := newEnv(t)
	const coupon = 1005
	e.addCoupon(t, coupon, service.TypeSeckill, 10, 4)
	if _, err := deduct(e.repo, "r1", coupon, 5); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		rel, err := release(e.repo, "r1", coupon)
		if err != nil {
			t.Fatalf("release %d: %v", i, err)
		}
		if i == 0 && !rel.Released {
			t.Fatalf("first release must release: %+v", rel)
		}
		if i > 0 && (rel.Released || !rel.Already) {
			t.Fatalf("replayed release must be alreadyReleased only: %+v", rel)
		}
	}
	if rem := e.remaining(t, coupon); rem != 10 {
		t.Fatalf("remaining = %d, want 10 (restored exactly once)", rem)
	}
}

func TestRequestIDIsBoundToItsCoupon(t *testing.T) {
	e := newEnv(t)
	e.addCoupon(t, 1006, service.TypeSeckill, 10, 4)
	e.addCoupon(t, 1007, service.TypeSeckill, 10, 4)
	if _, err := deduct(e.repo, "shared", 1006, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := deduct(e.repo, "shared", 1007, 5); !errors.Is(err, apperr.InvalidRequest) {
		t.Fatalf("reusing a requestId for another coupon must be rejected: %v", err)
	}
	if _, err := release(e.repo, "shared", 1007); !errors.Is(err, apperr.InvalidRequest) {
		t.Fatalf("releasing against the wrong coupon must be rejected: %v", err)
	}
	if e.remaining(t, 1006) != 9 || e.remaining(t, 1007) != 10 {
		t.Fatal("a rejected call must not move stock")
	}
}

// 崩溃注入：桶已更新、事务未提交时进程「崩溃」，什么都不会留下；重试后恰好生效一次。
func TestCrashAfterBucketUpdateRollsEverythingBack(t *testing.T) {
	e := newEnv(t)
	const coupon = 1008
	e.addCoupon(t, coupon, service.TypeSeckill, 10, 4)

	crash := errors.New("injected crash before commit")
	e.repo.SetCrashHook(func() error { return crash })
	if _, err := deduct(e.repo, "crashy", coupon, 5); !errors.Is(err, crash) {
		t.Fatalf("err = %v, want the injected crash", err)
	}
	if rem := e.remaining(t, coupon); rem != 10 {
		t.Fatalf("remaining = %d, the bucket update must have rolled back", rem)
	}
	if n := e.scalar(t, `SELECT count(*) FROM cpn.stock_ledger`); n != 0 {
		t.Fatalf("ledger rows = %d, nothing may be persisted", n)
	}

	e.repo.SetCrashHook(nil)
	res, err := deduct(e.repo, "crashy", coupon, 5)
	if err != nil || res.Already {
		t.Fatalf("retry after the crash must be a fresh deduction: %+v %v", res, err)
	}
	if rem := e.remaining(t, coupon); rem != 9 {
		t.Fatalf("remaining = %d, want 9", rem)
	}
	if n := e.ledger(t, coupon, "deducted"); n != 1 {
		t.Fatalf("ledger rows = %d, want 1", n)
	}
}

func TestDeductRejectsMissingAndDisabledCouponsWithoutLeavingLedgerRows(t *testing.T) {
	e := newEnv(t)
	const coupon = 1009
	e.addCoupon(t, coupon, service.TypeSeckill, 10, 4)

	if _, err := deduct(e.repo, "x", 424242, 5); !errors.Is(err, apperr.CouponNotFound) {
		t.Fatalf("missing coupon: %v", err)
	}
	if err := e.repo.UpdateStatus(context.Background(), coupon, service.StatusDisabled); err != nil {
		t.Fatal(err)
	}
	if _, err := deduct(e.repo, "y", coupon, 5); !errors.Is(err, apperr.CouponDisabled) {
		t.Fatalf("disabled coupon: %v", err)
	}
	if n := e.scalar(t, `SELECT count(*) FROM cpn.stock_ledger`); n != 0 {
		t.Fatalf("rejections must not leave ledger rows, got %d", n)
	}
	// 重新启用后，同一个 requestId 可以正常扣（之前的拒绝没有占位）。
	if err := e.repo.UpdateStatus(context.Background(), coupon, service.StatusEnabled); err != nil {
		t.Fatal(err)
	}
	if _, err := deduct(e.repo, "y", coupon, 5); err != nil {
		t.Fatalf("deduct after re-enable: %v", err)
	}
}

// 售罄 ≠ 某个桶空：库存只剩最后一个桶时，任何用户都必须能扣到，直到全桶为空。
func TestDeductScansAllBucketsAndWrapsAround(t *testing.T) {
	e := newEnv(t)
	const coupon = 1010
	e.addCoupon(t, coupon, service.TypeSeckill, 8, 8)
	if _, err := e.pool.Exec(context.Background(), `UPDATE cpn.coupon_stock SET remaining = CASE WHEN bucket = 5 THEN 5 ELSE 0 END WHERE coupon_id = $1`, coupon); err != nil {
		t.Fatal(err)
	}
	for u := range 5 {
		res, err := deduct(e.repo, fmt.Sprintf("w-%d", u), coupon, int64(1000+u*7919))
		if err != nil || res.Bucket != 5 {
			t.Fatalf("user %d: %+v %v, want bucket 5", u, res, err)
		}
	}
	if _, err := deduct(e.repo, "w-last", coupon, 31); !errors.Is(err, apperr.OutOfStock) {
		t.Fatalf("every bucket empty must be OutOfStock: %v", err)
	}
}

func TestStartBucketIsInRangeAndSpreadsConsecutiveIDs(t *testing.T) {
	const n = 8
	hits := make([]int, n)
	base := int64(7_000_000_000_000_000_000)
	for i := range int64(8000) {
		b := repo.StartBucket(base+i, n)
		if b < 0 || b >= n {
			t.Fatalf("bucket %d out of range", b)
		}
		hits[b]++
	}
	for b, h := range hits {
		if h < 800 || h > 1200 {
			t.Fatalf("bucket %d got %d of 8000 consecutive snowflake-like ids; hashing is not spreading: %v", b, h, hits)
		}
	}
}

// 混合负载不变式：remaining + 已扣流水数 == 初始库存，无论重放/归还/并发如何交错。
func TestMixedDeductReleaseWorkloadConservesStock(t *testing.T) {
	e := newEnv(t)
	const coupon, total = 1011, 40
	e.addCoupon(t, coupon, service.TypeSeckill, total, 8)

	var wg sync.WaitGroup
	for g := range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 25 {
				rid := fmt.Sprintf("mix-%d", (g*7+i)%60) // 大量 requestId 被多个 goroutine 重复使用
				uid := int64(100 + (g*7+i)%60)
				switch (g + i) % 3 {
				case 0, 1:
					if _, err := deduct(e.repo, rid, coupon, uid); err != nil && !errors.Is(err, apperr.OutOfStock) {
						t.Errorf("deduct: %v", err)
					}
				default:
					if _, err := release(e.repo, rid, coupon); err != nil {
						t.Errorf("release: %v", err)
					}
				}
			}
		}()
	}
	wg.Wait()

	rem, deducted := e.remaining(t, coupon), e.ledger(t, coupon, "deducted")
	if rem+deducted != total {
		t.Fatalf("remaining(%d) + deducted(%d) = %d, want %d", rem, deducted, rem+deducted, total)
	}
	if m := e.minBucket(t, coupon); m < 0 {
		t.Fatalf("negative bucket: %d", m)
	}
}

// 归还与扣减在同一 requestId 上竞速：不论谁先到，终态都一致（要么已扣已还，要么墓碑拦住扣减）。
func TestReleaseRacingDeductForSameRequestConverges(t *testing.T) {
	e := newEnv(t)
	const coupon = 1012
	e.addCoupon(t, coupon, service.TypeSeckill, 100, 8)
	for i := range 30 {
		rid := fmt.Sprintf("race-%d", i)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = deduct(e.repo, rid, coupon, 9) }()
		go func() { defer wg.Done(); _, _ = release(e.repo, rid, coupon) }()
		wg.Wait()
		// 再各来一次，消除调度先后的差异
		_, _ = deduct(e.repo, rid, coupon, 9)
		_, _ = release(e.repo, rid, coupon)
	}
	if rem := e.remaining(t, coupon); rem != 100 {
		t.Fatalf("every raced request ends released, remaining = %d, want 100", rem)
	}
	if n := e.ledger(t, coupon, "deducted"); n != 0 {
		t.Fatalf("deducted rows left = %d", n)
	}
}

func TestSetAvailableRedistributesKeepingBucketCountAndRaisesTotal(t *testing.T) {
	e := newEnv(t)
	const coupon = 1013
	e.addCoupon(t, coupon, service.TypeSeckill, 16, 4)
	if _, err := deduct(e.repo, "s1", coupon, 5); err != nil {
		t.Fatal(err)
	}

	if applied, err := e.repo.SetAvailable(context.Background(), coupon, 103, 8, internalapi.SyncModeSet); err != nil || applied != 103 {
		t.Fatalf("SetAvailable: applied=%d err=%v", applied, err)
	}
	if rem := e.remaining(t, coupon); rem != 103 {
		t.Fatalf("remaining = %d, want absolute 103", rem)
	}
	if n := e.scalar(t, `SELECT count(*) FROM cpn.coupon_stock WHERE coupon_id=$1`, coupon); n != 4 {
		t.Fatalf("an existing coupon keeps its own bucket count (4), got %d", n)
	}
	if hi, lo := e.scalar(t, `SELECT max(remaining) FROM cpn.coupon_stock WHERE coupon_id=$1`, coupon),
		e.scalar(t, `SELECT min(remaining) FROM cpn.coupon_stock WHERE coupon_id=$1`, coupon); hi-lo > 1 {
		t.Fatalf("buckets must be balanced, max-min = %d", hi-lo)
	}
	if tot := e.scalar(t, `SELECT total_stock FROM cpn.coupons WHERE id=$1`, coupon); tot != 103 {
		t.Fatalf("total_stock = %d, must be raised to cover the available amount", tot)
	}
	if _, err := e.repo.SetAvailable(context.Background(), coupon, 0, 8, internalapi.SyncModeSet); err != nil {
		t.Fatal(err)
	}
	if rem := e.remaining(t, coupon); rem != 0 {
		t.Fatalf("remaining = %d, want 0", rem)
	}
	if _, err := deduct(e.repo, "s2", coupon, 5); !errors.Is(err, apperr.OutOfStock) {
		t.Fatalf("zero available means sold out: %v", err)
	}
	if _, err := e.repo.SetAvailable(context.Background(), 555555, 5, 8, internalapi.SyncModeSet); !errors.Is(err, apperr.CouponNotFound) {
		t.Fatalf("missing coupon: %v", err)
	}
}

// 自动同步（min 模式）永远不能把库存抬高：外部平台读数常常是恒定值（mock 恒为 500）或过期值，
// 按它「设置」会把已卖出的库存重置回去造成超卖。
func TestSetAvailableMinModeNeverRaisesAndOnlyLowers(t *testing.T) {
	e := newEnv(t)
	const coupon = 1014
	ctx := context.Background()
	e.addCoupon(t, coupon, service.TypeSeckill, 100, 4)
	for i := range 50 {
		if _, err := deduct(e.repo, fmt.Sprintf("m%d", i), coupon, int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if rem := e.remaining(t, coupon); rem != 50 {
		t.Fatalf("precondition: remaining = %d, want 50", rem)
	}

	// 平台说 500（高于本地剩余）：不能抬高
	applied, err := e.repo.SetAvailable(ctx, coupon, 500, 8, internalapi.SyncModeMin)
	if err != nil || applied != 50 {
		t.Fatalf("min with a higher target: applied=%d err=%v, want 50 (unchanged)", applied, err)
	}
	if rem := e.remaining(t, coupon); rem != 50 {
		t.Fatalf("min must not raise: remaining = %d, want 50", rem)
	}
	if tot := e.scalar(t, `SELECT total_stock FROM cpn.coupons WHERE id=$1`, coupon); tot != 100 {
		t.Fatalf("min with a higher target must not touch total_stock, got %d", tot)
	}

	// 平台说 20（低于本地剩余）：降到 20，且桶仍然均衡
	applied, err = e.repo.SetAvailable(ctx, coupon, 20, 8, internalapi.SyncModeMin)
	if err != nil || applied != 20 {
		t.Fatalf("min with a lower target: applied=%d err=%v, want 20", applied, err)
	}
	if rem := e.remaining(t, coupon); rem != 20 {
		t.Fatalf("remaining = %d, want 20", rem)
	}
	if hi, lo := e.scalar(t, `SELECT max(remaining) FROM cpn.coupon_stock WHERE coupon_id=$1`, coupon),
		e.scalar(t, `SELECT min(remaining) FROM cpn.coupon_stock WHERE coupon_id=$1`, coupon); hi-lo > 1 {
		t.Fatalf("buckets must stay balanced after lowering, max-min = %d", hi-lo)
	}

	// 相等：无变化；之后明确的 set 才能抬高
	if applied, err = e.repo.SetAvailable(ctx, coupon, 20, 8, internalapi.SyncModeMin); err != nil || applied != 20 {
		t.Fatalf("min with an equal target: applied=%d err=%v", applied, err)
	}
	if applied, err = e.repo.SetAvailable(ctx, coupon, 500, 8, internalapi.SyncModeSet); err != nil || applied != 500 {
		t.Fatalf("set must be able to raise: applied=%d err=%v", applied, err)
	}
	if rem := e.remaining(t, coupon); rem != 500 {
		t.Fatalf("after set remaining = %d, want 500", rem)
	}
}

// 券还没有任何库存桶（从未初始化）时没有「当前值」可以保护，min 模式按首次设置处理，否则永远建不出库存。
func TestSetAvailableMinModeInitialisesACouponWithoutBuckets(t *testing.T) {
	e := newEnv(t)
	const coupon = 1015
	e.addCoupon(t, coupon, service.TypeSeckill, 10, 1)
	// Create 本身要求至少一个桶，这里直接删掉桶行来模拟「从未初始化」的券
	if _, err := e.pool.Exec(context.Background(), `DELETE FROM cpn.coupon_stock WHERE coupon_id=$1`, coupon); err != nil {
		t.Fatal(err)
	}
	if n := e.scalar(t, `SELECT count(*) FROM cpn.coupon_stock WHERE coupon_id=$1`, coupon); n != 0 {
		t.Fatalf("precondition: coupon must have no buckets, got %d", n)
	}
	applied, err := e.repo.SetAvailable(context.Background(), coupon, 30, 4, internalapi.SyncModeMin)
	if err != nil || applied != 30 {
		t.Fatalf("applied=%d err=%v, want 30", applied, err)
	}
	if rem := e.remaining(t, coupon); rem != 30 {
		t.Fatalf("remaining = %d, want 30", rem)
	}
}

// 不变式：并发扣减 + 反复用「高于当前值」的目标做 min 同步，剩余量 + 已扣量恒等于初始量（绝不超卖）。
func TestSetAvailableMinModeRacingDeductsNeverOversells(t *testing.T) {
	e := newEnv(t)
	const coupon, initial = 1016, 120
	e.addCoupon(t, coupon, service.TypeSeckill, initial, 8)
	var wg sync.WaitGroup
	for g := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 10 {
				if _, err := deduct(e.repo, fmt.Sprintf("race-%d-%d", g, i), coupon, int64(g*100+i+1)); err != nil && !errors.Is(err, apperr.OutOfStock) {
					t.Errorf("deduct: %v", err)
				}
				if _, err := e.repo.SetAvailable(context.Background(), coupon, 500, 8, internalapi.SyncModeMin); err != nil {
					t.Errorf("min sync: %v", err)
				}
			}
		}()
	}
	wg.Wait()
	if got := e.remaining(t, coupon) + e.ledger(t, coupon, "deducted"); got != initial {
		t.Fatalf("remaining + deducted = %d, want %d: a min sync raised the stock", got, initial)
	}
}

func TestSetAvailableRacingDeductsStaysConsistent(t *testing.T) {
	e := newEnv(t)
	const coupon = 1014
	e.addCoupon(t, coupon, service.TypeSeckill, 50, 8)
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := deduct(e.repo, fmt.Sprintf("sd-%d", i), coupon, int64(i+1)); err != nil && !errors.Is(err, apperr.OutOfStock) {
				t.Errorf("deduct: %v", err)
			}
		}()
	}
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.repo.SetAvailable(context.Background(), coupon, 200, 8, internalapi.SyncModeSet); err != nil {
				t.Errorf("SetAvailable: %v", err)
			}
		}()
	}
	wg.Wait()
	if m := e.minBucket(t, coupon); m < 0 {
		t.Fatalf("negative bucket %d", m)
	}
	if rem := e.remaining(t, coupon); rem < 160 || rem > 200 {
		t.Fatalf("remaining %d outside what 40 deducts around five absolute sets can produce", rem)
	}
}

func TestCreateGetAndDuplicateID(t *testing.T) {
	e := newEnv(t)
	e.addCoupon(t, 2001, service.TypeNormal, 20, 8)
	c, err := e.repo.Get(context.Background(), 2001)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if c.TotalStock != 20 || c.Remaining != 20 || c.FaceValue != "10.00" || c.Status != service.StatusEnabled || c.CreatedAt.IsZero() {
		t.Fatalf("coupon: %+v", c)
	}
	if n := e.scalar(t, `SELECT count(*) FROM cpn.coupon_stock WHERE coupon_id=2001`); n != 8 {
		t.Fatalf("bucket rows = %d", n)
	}
	_, err = e.repo.Create(context.Background(), service.NewCoupon{ID: 2001, Name: "dup", Type: 1, FaceValue: "1.00", MinSpend: "0.00", ValidDays: 1, PerUserLimit: 1}, []int64{0})
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.HTTPStatus != 409 {
		t.Fatalf("duplicate id must be 409, got %v", err)
	}
	if n := e.scalar(t, `SELECT count(*) FROM cpn.coupon_stock WHERE coupon_id=2001`); n != 8 {
		t.Fatalf("a failed create must not touch the existing coupon's buckets, got %d", n)
	}
	if _, err := e.repo.Get(context.Background(), 999); !errors.Is(err, apperr.CouponNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestAvailableAndListSemantics(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()
	e.addCoupon(t, 3001, service.TypeNormal, 5, 2)  // 常驻，启用
	e.addCoupon(t, 3002, service.TypeSeckill, 5, 2) // 秒杀，进行中（窗口 ±1h）
	e.addCoupon(t, 3003, service.TypeNormal, 5, 2)  // 将被停用
	e.addCoupon(t, 3004, service.TypeSeckill, 5, 2) // 将设为已结束
	e.addCoupon(t, 3005, service.TypeSeckill, 5, 2) // 将设为即将开始
	if err := e.repo.UpdateStatus(ctx, 3003, service.StatusDisabled); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.UpdateWindow(ctx, 3004, now.Add(-3*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.UpdateWindow(ctx, 3005, now.Add(time.Hour), now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, err := e.repo.Available(ctx, now, 100)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[int64]bool{}
	for _, c := range got {
		ids[c.ID] = true
		if c.Remaining != 5 {
			t.Fatalf("remaining must be the bucket sum: %+v", c)
		}
	}
	for id, want := range map[int64]bool{3001: true, 3002: true, 3003: false, 3004: false, 3005: true} {
		if ids[id] != want {
			t.Errorf("Available contains %d = %v, want %v", id, ids[id], want)
		}
	}

	all, total, err := e.repo.List(ctx, 0, 2, false)
	if err != nil || total != 5 || len(all) != 2 {
		t.Fatalf("List(all): %d items total %d err %v", len(all), total, err)
	}
	enabled, total, err := e.repo.List(ctx, 0, 100, true)
	if err != nil || total != 4 || len(enabled) != 4 {
		t.Fatalf("List(enabled): %d items total %d err %v", len(enabled), total, err)
	}
	page2, _, err := e.repo.List(ctx, 4, 2, false)
	if err != nil || len(page2) != 1 {
		t.Fatalf("offset paging: %d %v", len(page2), err)
	}
}

func TestDeleteCascadesBucketsButKeepsLedger(t *testing.T) {
	e := newEnv(t)
	const coupon = 4001
	e.addCoupon(t, coupon, service.TypeSeckill, 10, 4)
	if _, err := deduct(e.repo, "d1", coupon, 5); err != nil {
		t.Fatal(err)
	}
	n, err := e.repo.Delete(context.Background(), coupon)
	if err != nil || n != 1 {
		t.Fatalf("Delete: %d %v", n, err)
	}
	if c := e.scalar(t, `SELECT count(*) FROM cpn.coupon_stock WHERE coupon_id=$1`, coupon); c != 0 {
		t.Fatalf("buckets must cascade, left %d", c)
	}
	if c := e.ledger(t, coupon, "deducted"); c != 1 {
		t.Fatalf("ledger must stay for audit, got %d", c)
	}
	if n, err := e.repo.Delete(context.Background(), coupon); err != nil || n != 0 {
		t.Fatalf("second delete: %d %v", n, err)
	}
	// 券删除后迟到的归还不应报错（流水保留，桶已不在）。
	if _, err := release(e.repo, "d1", coupon); err != nil {
		t.Fatalf("release after delete: %v", err)
	}
}

func TestStockStateAndEnabledSeckillIDs(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.addCoupon(t, 5001, service.TypeSeckill, 12, 4)
	e.addCoupon(t, 5002, service.TypeSeckill, 3, 4)
	e.addCoupon(t, 5003, service.TypeNormal, 3, 4)
	if err := e.repo.UpdateStatus(ctx, 5002, service.StatusDisabled); err != nil {
		t.Fatal(err)
	}
	st, err := e.repo.StockState(ctx, 5001)
	if err != nil || st.Type != service.TypeSeckill || st.Status != service.StatusEnabled || st.Remaining != 12 {
		t.Fatalf("StockState: %+v %v", st, err)
	}
	if _, err := e.repo.StockState(ctx, 9); !errors.Is(err, apperr.CouponNotFound) {
		t.Fatalf("missing: %v", err)
	}
	ids, err := e.repo.EnabledSeckillIDs(ctx)
	if err != nil || len(ids) != 1 || ids[0] != 5001 {
		t.Fatalf("EnabledSeckillIDs = %v %v, want [5001]", ids, err)
	}
	if err := e.repo.UpdateStatus(ctx, 9, 1); !errors.Is(err, apperr.CouponNotFound) {
		t.Fatalf("UpdateStatus on missing: %v", err)
	}
	if err := e.repo.UpdateWindow(ctx, 9, time.Now(), time.Now().Add(time.Hour)); !errors.Is(err, apperr.CouponNotFound) {
		t.Fatalf("UpdateWindow on missing: %v", err)
	}
}
