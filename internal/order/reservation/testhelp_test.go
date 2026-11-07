package reservation

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/order/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

// seqIDs 是测试用的单调递增 ID 生成器（从 1000 起，避开手写 ID）。
type seqIDs struct{ n atomic.Int64 }

func (s *seqIDs) Next() int64 { return 1000 + s.n.Add(1) }

// env 是一组测试夹具：隔离数据库、假时钟、假券查询。
type env struct {
	pool    *pgxpool.Pool
	clock   *testkit.Clock
	coupons *fakeCoupons
	svc     *Service
}

// newEnv 建库、迁移并装配 Service（时钟固定在“现在”，由测试推进）。
func newEnv(t *testing.T) *env {
	t.Helper()
	pool := dbxtest.New(t)
	if err := dbx.Migrate(context.Background(), pool, "ord", migrations.FS); err != nil {
		t.Fatalf("迁移 ord schema: %v", err)
	}
	clock := testkit.NewClock(time.Now().UTC().Truncate(time.Millisecond))
	coupons := &fakeCoupons{items: map[int64]Coupon{}}
	svc := NewService(pool, &seqIDs{}, coupons, WithClock(clock.Now))
	return &env{pool: pool, clock: clock, coupons: coupons, svc: svc}
}

// fakeCoupons 是 CouponLookup 的假实现。
type fakeCoupons struct {
	mu    sync.Mutex
	items map[int64]Coupon
	err   error
}

func (f *fakeCoupons) put(c Coupon) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[c.ID] = c
}

func (f *fakeCoupons) Coupon(_ context.Context, id int64) (Coupon, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return Coupon{}, f.err
	}
	c, ok := f.items[id]
	if !ok {
		return Coupon{}, apperr.CouponNotFound
	}
	return c, nil
}

// seckillCoupon 构造一张启用中的秒杀券，开售时间在 startIn 之后，持续 length。
func seckillCoupon(id int64, now time.Time, startIn, length time.Duration) Coupon {
	start, end := now.Add(startIn), now.Add(startIn+length)
	return Coupon{ID: id, Type: CouponTypeSeckill, Status: CouponStatusEnabled, SeckillStartAt: &start, SeckillEndAt: &end}
}

// row 描述要直接写入数据库的预约行（绕过 Service，用来制造任意起始状态）。
type row struct {
	id, userID, couponID int64
	status               Status
	triggerAt            *time.Time
	firedAt              *time.Time
	requestID            *string
	retryCount           int
	updatedAt            time.Time
}

// insertRow 直接写一行预约；PENDING 必须有 trigger_at（表约束）。
func insertRow(t *testing.T, pool *pgxpool.Pool, r row) {
	t.Helper()
	if r.updatedAt.IsZero() {
		r.updatedAt = time.Now().UTC()
	}
	_, err := pool.Exec(context.Background(),
		`INSERT INTO ord.reservations (id, user_id, coupon_id, status, trigger_at, fired_at, request_id, retry_count, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
		r.id, r.userID, r.couponID, string(r.status), r.triggerAt, r.firedAt, r.requestID, r.retryCount, r.updatedAt)
	if err != nil {
		t.Fatalf("插入预约行 %+v: %v", r, err)
	}
}

// load 读取一行预约（不经 Service），用来断言数据库里的真实状态。
func load(t *testing.T, pool *pgxpool.Pool, id int64) Reservation {
	t.Helper()
	got, err := getByID(context.Background(), pool, id, 0)
	if err != nil {
		t.Fatalf("读取预约 %d: %v", id, err)
	}
	return got
}

// notificationsOf 返回某用户全部通知的 (type, refId) 列表，按创建顺序。
func notificationsOf(t *testing.T, pool *pgxpool.Pool, userID int64) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT type FROM ord.notifications WHERE user_id = $1 ORDER BY id`, userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			t.Fatal(err)
		}
		out = append(out, typ)
	}
	return out
}

// fakeSeckiller 是 Seckiller 的假实现：记录每个 (用户,券) 的调用次数，行为由 fn 决定。
type fakeSeckiller struct {
	mu    sync.Mutex
	calls map[[2]int64]int
	total int
	fn    func(ctx context.Context, userID, couponID int64, nth int) (string, error)
}

func newSeckiller(fn func(ctx context.Context, userID, couponID int64, nth int) (string, error)) *fakeSeckiller {
	return &fakeSeckiller{calls: map[[2]int64]int{}, fn: fn}
}

// okSeckiller 总是成功并返回形如 req-<user>-<coupon> 的请求号。
func okSeckiller() *fakeSeckiller {
	return newSeckiller(func(_ context.Context, userID, couponID int64, _ int) (string, error) {
		return requestIDFor(userID, couponID), nil
	})
}

func requestIDFor(userID, couponID int64) string {
	return "req-" + itoa(userID) + "-" + itoa(couponID)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for ; n > 0; n /= 10 {
		i--
		b[i] = byte('0' + n%10)
	}
	return string(b[i:])
}

func (f *fakeSeckiller) Enter(ctx context.Context, userID, couponID int64) (string, error) {
	f.mu.Lock()
	k := [2]int64{userID, couponID}
	f.calls[k]++
	nth := f.calls[k]
	f.total++
	f.mu.Unlock()
	return f.fn(ctx, userID, couponID, nth)
}

func (f *fakeSeckiller) callsFor(userID, couponID int64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[[2]int64{userID, couponID}]
}

func (f *fakeSeckiller) totalCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.total
}

// errBoom 是测试用的非业务错误（例如网络故障）。
var errBoom = errors.New("boom")
