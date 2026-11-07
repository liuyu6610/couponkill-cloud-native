package orders_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders/orderstest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/seckill"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

const (
	normalCoupon  = int64(2001)
	seckillCoupon = int64(1001)
)

// fakeRestorer 记录 Restore 调用，并可注入失败。
type fakeRestorer struct {
	mu    sync.Mutex
	calls []restoreCall
	fail  int
}

type restoreCall struct {
	requestID      string
	couponID, user int64
}

func (r *fakeRestorer) Restore(_ context.Context, requestID string, couponID, userID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, restoreCall{requestID, couponID, userID})
	if r.fail > 0 {
		r.fail--
		return errors.New("redis down")
	}
	return nil
}

func (r *fakeRestorer) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// failingInsertDB 在开启时让订单 INSERT 失败，用来验证落单失败后的库存归还。
type failingInsertDB struct {
	*pgxpool.Pool
	failInsert atomic.Bool
}

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

func (d *failingInsertDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if d.failInsert.Load() && len(sql) > 20 && sql[1:20] == "INSERT INTO ord.ord" {
		return errRow{errors.New("disk full")}
	}
	return d.Pool.QueryRow(ctx, sql, args...)
}

type env struct {
	t        *testing.T
	db       *failingInsertDB
	coupon   *orderstest.FakeCoupon
	restorer *fakeRestorer
	svc      *orders.Service
	eng      *httpx.Engine
	clock    *testkit.Clock
}

func newEnv(t *testing.T, mutate ...func(*orders.Options)) *env {
	t.Helper()
	rdb, _ := testkit.Redis(t)
	e := &env{
		t: t, db: &failingInsertDB{Pool: orderstest.NewPool(t)}, coupon: orderstest.NewFakeCoupon(t),
		restorer: &fakeRestorer{}, clock: testkit.NewClock(time.Now().Truncate(time.Millisecond)),
	}
	client := e.coupon.Client(t)
	ids, err := idgen.New(3)
	if err != nil {
		t.Fatal(err)
	}
	opts := orders.Options{Now: e.clock.Now}
	for _, m := range mutate {
		m(&opts)
	}
	e.svc, err = orders.New(orders.Deps{
		DB: e.db, Coupons: seckill.NewCouponMeta(rdb, client), Stock: client, Restorer: e.restorer, IDs: ids,
	}, opts)
	if err != nil {
		t.Fatal(err)
	}
	signer := testkit.Signer(t)
	e.eng = httpx.NewEngine(httpx.Options{Service: "order-test"})
	h := orders.NewHandler(e.svc)
	h.Register(e.eng.Group("/api/v1/order", httpx.RequireGatewayIdentity(signer)))
	h.RegisterUser(e.eng.Group("/api/v1/user", httpx.RequireGatewayIdentity(signer)))
	h.RegisterInternal(e.eng.Group("", httpx.RequireInternalToken(testkit.InternalToken)))

	e.coupon.AddCoupon(orderstest.NormalCoupon(normalCoupon), 10)
	e.coupon.AddCoupon(orderstest.SeckillCoupon(seckillCoupon), 10)
	return e
}

func (e *env) do(method, path string, body any, opts ...testkit.ReqOption) *testkit.Response {
	e.t.Helper()
	return testkit.DoJSON(e.t, e.eng, method, path, body, opts...)
}

func (e *env) create(user, coupon int64) *testkit.Response {
	e.t.Helper()
	return e.do("POST", "/api/v1/order/create", map[string]string{"couponId": strconv.FormatInt(coupon, 10)}, testkit.User(user, "user"))
}

func (e *env) mustCreate(user, coupon int64) orders.Order {
	e.t.Helper()
	res := e.create(user, coupon)
	if res.Status != http.StatusOK {
		e.t.Fatalf("create(user %d coupon %d) = %d %s", user, coupon, res.Status, res.Body)
	}
	var o orders.Order
	res.Decode(&o)
	return o
}

func (e *env) cancel(user, orderID int64) *testkit.Response {
	e.t.Helper()
	return e.do("POST", "/api/v1/order/cancel", map[string]string{"orderId": strconv.FormatInt(orderID, 10)}, testkit.User(user, "user"))
}

func (e *env) insertSeckillOrder(user, coupon int64, requestID string) orders.Order {
	e.t.Helper()
	now := e.clock.Now()
	o := orders.NewOrder{ID: 5_000_000 + nextID.Add(1), UserID: user, CouponID: coupon, RequestID: requestID, CouponName: "s", CouponType: 2,
		Source: orders.SourceSeckill, Now: now, ExpireAt: now.Add(time.Hour)}
	if _, ok, err := orders.Insert(context.Background(), e.db.Pool, o); err != nil || !ok {
		e.t.Fatalf("seed seckill order: %v %v", ok, err)
	}
	got, _, _ := orders.FindByRequest(context.Background(), e.db.Pool, user, requestID)
	return got
}

func wantFailure(t *testing.T, res *testkit.Response, status int, want *apperr.Error) {
	t.Helper()
	if res.Status != status || res.Code != want.Code {
		t.Fatalf("response = %d code %d (%s), want %d code %d", res.Status, res.Code, res.Body, status, want.Code)
	}
}

func TestCreateGivesAUserANormalCouponAndTakesOneUnitOfStock(t *testing.T) {
	e := newEnv(t)

	o := e.mustCreate(42, normalCoupon)

	if o.UserID != 42 || o.CouponID != normalCoupon || o.Status != orders.StatusCreated || o.Source != orders.SourceNormal ||
		o.CouponName != "常驻券-2001" || string(o.FaceValue) != "50.00" || o.CouponType != orders.CouponTypeNormal {
		t.Fatalf("order = %+v", o)
	}
	if o.RequestID == "" || e.coupon.Effects(o.RequestID) != 1 || e.coupon.Stock(normalCoupon) != 9 {
		t.Fatalf("request %q effects %d stock %d", o.RequestID, e.coupon.Effects(o.RequestID), e.coupon.Stock(normalCoupon))
	}
	res := e.do("GET", "/api/v1/order/check/"+strconv.FormatInt(normalCoupon, 10), nil, testkit.User(42, "user"))
	var held bool
	res.Decode(&held)
	if !held {
		t.Fatal("check must report the coupon as held")
	}
}

func TestCreateRejectsWhatItMust(t *testing.T) {
	e := newEnv(t)
	e.coupon.AddCoupon(orderstest.NormalCoupon(2002), 0)
	disabled := orderstest.NormalCoupon(2003)
	disabled.Status = 0
	e.coupon.AddCoupon(disabled, 5)

	wantFailure(t, e.create(1, seckillCoupon), http.StatusBadRequest, apperr.SeckillUseDedicatedAPI)
	wantFailure(t, e.create(1, 2003), http.StatusConflict, apperr.CouponDisabled)
	wantFailure(t, e.create(1, 2002), http.StatusConflict, apperr.OutOfStock)
	wantFailure(t, e.create(1, 9999), http.StatusNotFound, apperr.CouponNotFound)
	if e.coupon.TotalEffects() != 0 {
		t.Fatalf("a rejected create consumed %d units of stock", e.coupon.TotalEffects())
	}
	for name, body := range map[string]any{"missing": map[string]string{}, "not an id": map[string]string{"couponId": "x"}} {
		res := e.do("POST", "/api/v1/order/create", body, testkit.User(1, "user"))
		if res.Status != http.StatusBadRequest {
			t.Fatalf("%s = %d", name, res.Status)
		}
	}
}

func TestCreateRepeatIsRejectedBeforeAnyStockIsTaken(t *testing.T) {
	e := newEnv(t)
	e.mustCreate(42, normalCoupon)

	wantFailure(t, e.create(42, normalCoupon), http.StatusConflict, apperr.RepeatSeckill)
	if e.coupon.DeductCalls() != 1 || e.coupon.Stock(normalCoupon) != 9 {
		t.Fatalf("deduct calls %d stock %d: the repeat must not touch stock", e.coupon.DeductCalls(), e.coupon.Stock(normalCoupon))
	}
}

// 同一用户并发领同一张券：恰好一个成功，输家要么被预检挡住，要么撞上部分唯一索引并归还库存。
// 去掉 IsUniqueViolation 分支的释放，或让输家不再归还，库存会被永久吞掉。
func TestConcurrentCreatesOfTheSameCouponLeaveExactlyOneOrderAndNoLeakedStock(t *testing.T) {
	e := newEnv(t)
	var wg sync.WaitGroup
	var ok, repeat atomic.Int32
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := e.create(42, normalCoupon)
			switch {
			case res.Status == http.StatusOK:
				ok.Add(1)
			case res.Code == apperr.CodeRepeatSeckill:
				repeat.Add(1)
			default:
				t.Errorf("unexpected response %d %s", res.Status, res.Body)
			}
		}()
	}
	wg.Wait()

	if ok.Load() != 1 || repeat.Load() != 11 {
		t.Fatalf("successes %d repeats %d, want 1 and 11", ok.Load(), repeat.Load())
	}
	if e.coupon.Stock(normalCoupon) != 9 || e.coupon.Held() != 1 {
		t.Fatalf("stock %d held ledger entries %d, want 9 and 1: every loser must give its unit back", e.coupon.Stock(normalCoupon), e.coupon.Held())
	}
}

func TestCreateGivesStockBackWhenThePersistenceStepFails(t *testing.T) {
	e := newEnv(t)
	e.db.failInsert.Store(true)

	res := e.create(42, normalCoupon)

	wantFailure(t, res, http.StatusInternalServerError, apperr.SysError)
	if e.coupon.Stock(normalCoupon) != 10 || e.coupon.Held() != 0 {
		t.Fatalf("stock %d held %d: the failed saga must release", e.coupon.Stock(normalCoupon), e.coupon.Held())
	}
	if e.coupon.ReleaseCalls() != 1 {
		t.Fatalf("release calls = %d", e.coupon.ReleaseCalls())
	}
}

// coupon 提交了扣减却丢了响应（transient）：结果未知，create 必须归还，否则库存悄悄少一件。
func TestCreateReleasesWhenTheDeductResponseWasLost(t *testing.T) {
	e := newEnv(t)
	e.coupon.FailDeductAfterCommit(1)

	res := e.create(42, normalCoupon)

	wantFailure(t, res, http.StatusServiceUnavailable, apperr.SystemBusy)
	if e.coupon.Stock(normalCoupon) != 10 || e.coupon.Held() != 0 {
		t.Fatalf("stock %d held %d: an unknown deduct outcome must be released", e.coupon.Stock(normalCoupon), e.coupon.Held())
	}
	// 之后用户能正常领取。
	e.mustCreate(42, normalCoupon)
}

func TestCreateEnforcesTheHoldingCap(t *testing.T) {
	e := newEnv(t, func(o *orders.Options) { o.MaxActiveCoupons = 1 })
	e.coupon.AddCoupon(orderstest.NormalCoupon(2002), 5)
	e.mustCreate(42, normalCoupon)

	wantFailure(t, e.create(42, 2002), http.StatusConflict, apperr.CouponLimitExceeded)
	if e.coupon.Stock(2002) != 5 {
		t.Fatal("a capped user must not consume stock")
	}
}

func TestCancelReturnsStockAndIsIdempotent(t *testing.T) {
	e := newEnv(t)
	o := e.mustCreate(42, normalCoupon)
	id, _ := strconv.ParseInt(strconv.FormatInt(o.ID, 10), 10, 64)

	for i := range 3 {
		res := e.cancel(42, id)
		var done bool
		res.Decode(&done)
		if res.Status != http.StatusOK || !done {
			t.Fatalf("cancel #%d = %d %s, want true", i, res.Status, res.Body)
		}
	}
	got, _, _ := orders.FindByID(context.Background(), e.db.Pool, 42, id)
	if got.Status != orders.StatusCancelled || got.CancelTime == nil {
		t.Fatalf("order = %+v", got)
	}
	if e.coupon.Stock(normalCoupon) != 10 || e.coupon.Held() != 0 {
		t.Fatalf("stock %d held %d, want everything back exactly once", e.coupon.Stock(normalCoupon), e.coupon.Held())
	}
	if e.restorer.count() != 0 {
		t.Fatal("a normal order must not touch the seckill restorer")
	}
	// 取消后可以再领。
	e.mustCreate(42, normalCoupon)
}

// 去掉「status = 1」条件之外的归属校验（user_id 过滤），别人就能取消你的订单。
func TestCancelOfSomeoneElsesOrOfAMissingOrderIsOrderNotFound(t *testing.T) {
	e := newEnv(t)
	o := e.mustCreate(42, normalCoupon)

	wantFailure(t, e.cancel(43, o.ID), http.StatusNotFound, apperr.OrderNotFound)
	wantFailure(t, e.cancel(42, 424242), http.StatusNotFound, apperr.OrderNotFound)
	got, _, _ := orders.FindByID(context.Background(), e.db.Pool, 42, o.ID)
	if got.Status != orders.StatusCreated {
		t.Fatalf("the victim's order was touched: %+v", got)
	}
	if e.coupon.Stock(normalCoupon) != 9 {
		t.Fatalf("stock = %d, a foreign cancel must not release", e.coupon.Stock(normalCoupon))
	}
}

func TestCancelRefusesUsedOrdersWithoutReleasingAnything(t *testing.T) {
	e := newEnv(t)
	o := e.mustCreate(42, normalCoupon)
	if _, err := e.db.Pool.Exec(context.Background(), `UPDATE ord.orders SET status = 2, use_time = now() WHERE user_id = 42 AND id = $1`, o.ID); err != nil {
		t.Fatal(err)
	}
	res := e.cancel(42, o.ID)
	var done bool
	res.Decode(&done)
	if res.Status != http.StatusOK || done {
		t.Fatalf("cancel of a used order = %d %s, want false", res.Status, res.Body)
	}
	if e.coupon.Stock(normalCoupon) != 9 || e.coupon.ReleaseCalls() != 0 {
		t.Fatal("a used order keeps its stock")
	}
}

// 并发取消：只有一个赢得 1→4，库存只回一次（对 coupon 的 release 本身幂等，这里验证终态与账本）。
func TestCancelRacingCancelsEndInOneCancelledOrderAndOneRelease(t *testing.T) {
	e := newEnv(t)
	o := e.mustCreate(42, normalCoupon)

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res := e.cancel(42, o.ID); res.Status != http.StatusOK {
				t.Errorf("cancel = %d %s", res.Status, res.Body)
			}
		}()
	}
	wg.Wait()

	got, _, _ := orders.FindByID(context.Background(), e.db.Pool, 42, o.ID)
	if got.Status != orders.StatusCancelled {
		t.Fatalf("status = %d", got.Status)
	}
	if e.coupon.Stock(normalCoupon) != 10 || e.coupon.Held() != 0 {
		t.Fatalf("stock %d held %d, want exactly one unit returned", e.coupon.Stock(normalCoupon), e.coupon.Held())
	}
}

// 取消秒杀订单走 Restorer（含 Redis 计数与占位）；归还失败时订单仍是已取消，重试同一个取消会重新归还（自愈）。
func TestCancelOfASeckillOrderRestoresThroughTheRestorerAndHealsOnRetry(t *testing.T) {
	e := newEnv(t)
	o := e.insertSeckillOrder(42, seckillCoupon, "rid-seckill-1")
	e.restorer.fail = 1

	res := e.cancel(42, o.ID)
	wantFailure(t, res, http.StatusInternalServerError, apperr.SysError)
	got, _, _ := orders.FindByID(context.Background(), e.db.Pool, 42, o.ID)
	if got.Status != orders.StatusCancelled {
		t.Fatalf("status = %d: the cancellation itself is committed even when the restore failed", got.Status)
	}

	res = e.cancel(42, o.ID)
	var done bool
	res.Decode(&done)
	if res.Status != http.StatusOK || !done {
		t.Fatalf("retry = %d %s", res.Status, res.Body)
	}
	if e.restorer.count() != 2 || e.restorer.calls[1] != (restoreCall{"rid-seckill-1", seckillCoupon, 42}) {
		t.Fatalf("restorer calls = %+v", e.restorer.calls)
	}
}

func TestListsCheckCountAndPaging(t *testing.T) {
	e := newEnv(t)
	for i := range 3 {
		id := int64(2010 + i)
		e.coupon.AddCoupon(orderstest.NormalCoupon(id), 5)
		e.clock.Advance(time.Second)
		e.mustCreate(42, id)
	}
	e.insertSeckillOrder(42, seckillCoupon, "rid-list")
	e.mustCreate(7, normalCoupon)

	res := e.do("GET", "/api/v1/order/user/me?page=2&size=3", nil, testkit.User(42, "user"))
	var page struct {
		Items []orders.Order `json:"items"`
		Total int64          `json:"total"`
		Page  int            `json:"page"`
		Size  int            `json:"size"`
	}
	res.Decode(&page)
	if res.Status != http.StatusOK || page.Total != 4 || page.Page != 2 || page.Size != 3 || len(page.Items) != 1 {
		t.Fatalf("page 2 = %d %+v", res.Status, page)
	}
	for _, it := range page.Items {
		if it.UserID != 42 {
			t.Fatalf("my orders leaked user %d's order", it.UserID)
		}
	}

	// 旧前端的 pageNum/pageSize 仍然可用；页大小被截断到上限。
	res = e.do("GET", "/api/v1/order/user/me?pageNum=1&pageSize=1000", nil, testkit.User(42, "user"))
	res.Decode(&page)
	if page.Size != 100 || len(page.Items) != 4 {
		t.Fatalf("legacy params: size %d items %d, want the cap 100 and 4 items", page.Size, len(page.Items))
	}

	res = e.do("GET", "/api/v1/user/coupon/count", nil, testkit.User(42, "user"))
	var raw map[string]any
	res.Decode(&raw)
	if raw["count"] != float64(4) {
		t.Fatalf("count = %v, the frontend contract reads {count}", raw["count"])
	}
	if raw["userId"] != "42" || raw["totalCount"] != float64(4) || raw["seckillCount"] != float64(1) || raw["normalCount"] != float64(3) || raw["expiredCount"] != float64(0) {
		t.Fatalf("counts = %v", raw)
	}

	res = e.do("GET", "/api/v1/order/check/"+strconv.FormatInt(seckillCoupon, 10), nil, testkit.User(42, "user"))
	var held bool
	res.Decode(&held)
	if !held {
		t.Fatal("seckill coupon should be held")
	}
	res = e.do("GET", "/api/v1/order/check/"+strconv.FormatInt(seckillCoupon, 10), nil, testkit.User(7, "user"))
	res.Decode(&held)
	if held {
		t.Fatal("user 7 does not hold the seckill coupon")
	}
	if res := e.do("GET", "/api/v1/order/check/abc", nil, testkit.User(7, "user")); res.Status != http.StatusBadRequest {
		t.Fatalf("check with a bad id = %d", res.Status)
	}
}

func TestOrderIDsAreSerialisedAsStrings(t *testing.T) {
	e := newEnv(t)
	o := e.mustCreate(42, normalCoupon)
	res := e.do("GET", "/api/v1/order/user/me", nil, testkit.User(42, "user"))
	var page struct {
		Items []map[string]any `json:"items"`
	}
	res.Decode(&page)
	if len(page.Items) != 1 {
		t.Fatalf("items = %d", len(page.Items))
	}
	for _, field := range []string{"id", "userId", "couponId"} {
		if _, isString := page.Items[0][field].(string); !isString {
			t.Fatalf("%s = %#v, want a JSON string (snowflake ids exceed 2^53)", field, page.Items[0][field])
		}
	}
	if page.Items[0]["id"] != strconv.FormatInt(o.ID, 10) {
		t.Fatalf("id = %v", page.Items[0]["id"])
	}
}

func TestAdminListingIsAdminOnlyFilteredAndBounded(t *testing.T) {
	e := newEnv(t)
	e.mustCreate(42, normalCoupon)
	e.mustCreate(43, normalCoupon)
	e.insertSeckillOrder(44, seckillCoupon, "rid-admin")
	admin := testkit.User(1, "admin")
	user := testkit.User(42, "user")

	wantFailure(t, e.do("GET", "/api/v1/order/admin", nil, user), http.StatusForbidden, apperr.Forbidden)
	wantFailure(t, e.do("GET", "/api/v1/order/user/43", nil, user), http.StatusForbidden, apperr.Forbidden)

	var page struct {
		Items []orders.Order `json:"items"`
		Total int64          `json:"total"`
		Size  int            `json:"size"`
	}
	res := e.do("GET", "/api/v1/order/admin?couponId="+strconv.FormatInt(normalCoupon, 10), nil, admin)
	res.Decode(&page)
	if res.Status != http.StatusOK || page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("filter by coupon = %d %+v", res.Status, page)
	}
	res = e.do("GET", "/api/v1/order/admin?userId=44&status=1", nil, admin)
	res.Decode(&page)
	if page.Total != 1 || page.Items[0].UserID != 44 {
		t.Fatalf("filter by user+status = %+v", page)
	}
	res = e.do("GET", "/api/v1/order/admin?orderId="+strconv.FormatInt(page.Items[0].ID, 10), nil, admin)
	res.Decode(&page)
	if page.Total != 1 {
		t.Fatalf("lookup by order id = %+v", page)
	}
	res = e.do("GET", "/api/v1/order/admin?startTime="+time.Now().Add(time.Hour).UTC().Format("2006-01-02T15:04:05Z07:00"), nil, admin)
	res.Decode(&page)
	if page.Total != 0 {
		t.Fatalf("a future startTime must match nothing, got %d", page.Total)
	}
	res = e.do("GET", "/api/v1/order/admin?size=100000", nil, admin)
	res.Decode(&page)
	if page.Size != 100 {
		t.Fatalf("size = %d, want it capped to 100", page.Size)
	}

	wantFailure(t, e.do("GET", "/api/v1/order/admin?page=102&size=100", nil, admin), http.StatusBadRequest, apperr.InvalidRequest)
	wantFailure(t, e.do("GET", "/api/v1/order/admin?startTime=yesterday", nil, admin), http.StatusBadRequest, apperr.InvalidRequest)
	wantFailure(t, e.do("GET", "/api/v1/order/admin?status=9", nil, admin), http.StatusBadRequest, apperr.InvalidRequest)
	if res := e.do("GET", "/api/v1/order/admin?page=101&size=100", nil, admin); res.Status != http.StatusOK {
		t.Fatalf("the last allowed offset (10000) = %d", res.Status)
	}

	res = e.do("GET", "/api/v1/order/user/43", nil, admin)
	res.Decode(&page)
	if res.Status != http.StatusOK || page.Total != 1 || page.Items[0].UserID != 43 {
		t.Fatalf("admin listing of user 43 = %d %+v", res.Status, page)
	}
}

func TestGrantIsIdempotentTouchesNoStockAndReportsRepeats(t *testing.T) {
	e := newEnv(t)
	grant := func(req internalapi.GrantRequest, opts ...testkit.ReqOption) *testkit.Response {
		return e.do("POST", internalapi.PathOrderGrant, req, append([]testkit.ReqOption{testkit.Internal()}, opts...)...)
	}
	req := internalapi.GrantRequest{RequestID: "grant-1", UserID: 42, CouponID: seckillCoupon}

	res := grant(req)
	var first internalapi.GrantResponse
	res.Decode(&first)
	if res.Status != http.StatusOK || first.OrderID == 0 || first.AlreadyGranted {
		t.Fatalf("first grant = %d %s", res.Status, res.Body)
	}
	o, ok, _ := orders.FindByRequest(context.Background(), e.db.Pool, 42, "grant-1")
	if !ok || o.ID != first.OrderID || o.Source != orders.SourceAdmin || o.Status != orders.StatusCreated {
		t.Fatalf("granted order = %+v ok=%v", o, ok)
	}

	res = grant(req)
	var replay internalapi.GrantResponse
	res.Decode(&replay)
	if replay.OrderID != first.OrderID || !replay.AlreadyGranted {
		t.Fatalf("replay = %+v, want the same order flagged AlreadyGranted", replay)
	}
	var n int
	_ = e.db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM ord.orders WHERE user_id = 42`).Scan(&n)
	if n != 1 {
		t.Fatalf("orders = %d after a replay, want 1", n)
	}

	// 同一用户同一张券换一个 requestId：已持有 → 409/10010。
	wantFailure(t, grant(internalapi.GrantRequest{RequestID: "grant-2", UserID: 42, CouponID: seckillCoupon}), http.StatusConflict, apperr.RepeatSeckill)
	wantFailure(t, grant(internalapi.GrantRequest{RequestID: "grant-3", UserID: 43, CouponID: 9999}), http.StatusNotFound, apperr.CouponNotFound)
	wantFailure(t, grant(internalapi.GrantRequest{RequestID: "bad id", UserID: 43, CouponID: seckillCoupon}), http.StatusBadRequest, apperr.InvalidRequest)
	if e.coupon.DeductCalls() != 0 || e.coupon.ReleaseCalls() != 0 {
		t.Fatalf("grant touched stock: deducts %d releases %d", e.coupon.DeductCalls(), e.coupon.ReleaseCalls())
	}

	// 内部端点必须带内部令牌；用户身份头不够。
	if res := e.do("POST", internalapi.PathOrderGrant, req); res.Status != http.StatusUnauthorized {
		t.Fatalf("grant without the internal token = %d", res.Status)
	}
	if res := e.do("POST", internalapi.PathOrderGrant, req, testkit.User(1, "admin")); res.Status != http.StatusUnauthorized {
		t.Fatalf("grant with a user identity = %d", res.Status)
	}
}

func TestConcurrentGrantsOfTheSameRequestProduceOneOrder(t *testing.T) {
	e := newEnv(t)
	req := internalapi.GrantRequest{RequestID: "grant-race", UserID: 42, CouponID: normalCoupon}
	var wg sync.WaitGroup
	var granted, replayed atomic.Int32
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := e.do("POST", internalapi.PathOrderGrant, req, testkit.Internal())
			var out internalapi.GrantResponse
			if res.Status != http.StatusOK {
				t.Errorf("grant = %d %s", res.Status, res.Body)
				return
			}
			res.Decode(&out)
			if out.AlreadyGranted {
				replayed.Add(1)
			} else {
				granted.Add(1)
			}
		}()
	}
	wg.Wait()
	if granted.Load() != 1 || replayed.Load() != 9 {
		t.Fatalf("granted %d replayed %d, want 1 and 9", granted.Load(), replayed.Load())
	}
}

func TestEndpointsRequireAGatewayIdentity(t *testing.T) {
	e := newEnv(t)
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/v1/order/create"}, {"POST", "/api/v1/order/cancel"}, {"GET", "/api/v1/order/check/1"},
		{"GET", "/api/v1/order/user/me"}, {"GET", "/api/v1/order/admin"}, {"GET", "/api/v1/user/coupon/count"},
	} {
		if res := e.do(tc.method, tc.path, nil); res.Status != http.StatusUnauthorized {
			t.Fatalf("%s %s = %d, want 401", tc.method, tc.path, res.Status)
		}
	}
}
