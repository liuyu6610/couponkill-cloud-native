// Package orderstest 是 order 服务各包测试共用的辅助：隔离的已迁移 PostgreSQL、
// 以及一个说内部 HTTP 协议的假 coupon 服务（带库存账本，行为遵守 internalapi 的 doc.go 语义）。
//
// 仅供测试使用，生产代码不得引用。
package orderstest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

// NewPool 返回一个已应用 ord 迁移的隔离数据库连接池；PostgreSQL 不可用时测试被跳过（见 dbxtest）。
func NewPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool := dbxtest.New(t)
	if err := dbx.Migrate(context.Background(), pool, "ord", migrations.FS); err != nil {
		t.Fatalf("migrate ord: %v", err)
	}
	return pool
}

// ledgerEntry 是假 coupon 的库存账本行。
type ledgerEntry struct {
	coupon   int64
	released bool
}

// FakeCoupon 是进程内的假 coupon 服务：真实的 internalapi 客户端通过 HTTP 调它，
// 因此重试、transient 映射、信封解析都走真实代码。
//
// 账本语义（与 internalapi/doc.go 一致）：deduct 对同一 requestId 幂等；已归还的 requestId 再 deduct 返回
// 409/OutOfStock；release 对从未扣减的 requestId 写墓碑；get 返回登记的券信息。
type FakeCoupon struct {
	// Server 是底层 httptest 服务器。
	Server *httptest.Server

	mu          sync.Mutex
	coupons     map[int64]*fakeCoupon
	ledger      map[string]*ledgerEntry
	deductCalls int
	getCalls    int
	releaseCall int
	effects     map[string]int // requestId → 真正改动库存的 deduct 次数
	// 故障注入：对 deduct 在状态变更之后返回 503（响应丢失），或在变更之前返回 503。
	failAfterCommit  int
	failBeforeCommit int
	failRelease      int
	failGet          int
	afterDeduct      func()
	beforeDeduct     func()
}

type fakeCoupon struct {
	info  internalapi.CouponInfo
	stock int
}

// NewFakeCoupon 启动假 coupon 服务，t.Cleanup 时关闭。
func NewFakeCoupon(t testing.TB) *FakeCoupon {
	t.Helper()
	f := &FakeCoupon{
		coupons: map[int64]*fakeCoupon{},
		ledger:  map[string]*ledgerEntry{},
		effects: map[string]int{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+internalapi.PathStockDeduct, f.handleDeduct)
	mux.HandleFunc("POST "+internalapi.PathStockRelease, f.handleRelease)
	mux.HandleFunc("GET /internal/v1/coupons/{id}", f.handleGet)
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Server.Close)
	return f
}

// Client 返回指向假服务的真实 internalapi 客户端；重试只尝试一次、不等待，让故障注入可预测。
func (f *FakeCoupon) Client(t testing.TB, opts ...internalapi.Option) *internalapi.CouponClient {
	t.Helper()
	all := append([]internalapi.Option{internalapi.WithRetryPolicy(internalapi.RetryPolicy{MaxAttempts: 1})}, opts...)
	c, err := internalapi.NewCouponClient(f.Server.URL, testkit.InternalToken, nil, all...)
	if err != nil {
		t.Fatalf("new coupon client: %v", err)
	}
	return c
}

// AddCoupon 登记一张券及其库存。
func (f *FakeCoupon) AddCoupon(info internalapi.CouponInfo, stock int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.coupons[info.ID] = &fakeCoupon{info: info, stock: stock}
}

// SetStatus 修改券状态（模拟停用）。
func (f *FakeCoupon) SetStatus(couponID int64, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c := f.coupons[couponID]; c != nil {
		c.info.Status = status
	}
}

// Stock 返回券当前的 DB 库存。
func (f *FakeCoupon) Stock(couponID int64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c := f.coupons[couponID]; c != nil {
		return c.stock
	}
	return 0
}

// Effects 返回 requestId 上真正改动过库存的 deduct 次数（幂等重放不计）。
func (f *FakeCoupon) Effects(requestID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.effects[requestID]
}

// TotalEffects 返回所有 requestId 的有效扣减次数之和。
func (f *FakeCoupon) TotalEffects() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, v := range f.effects {
		n += v
	}
	return n
}

// DeductCalls 返回收到的 deduct 请求总数（含幂等重放与注入的失败）。
func (f *FakeCoupon) DeductCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deductCalls
}

// GetCalls 返回收到的读券请求总数（含注入的失败）。
func (f *FakeCoupon) GetCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getCalls
}

// ReleaseCalls 返回收到的 release 请求总数。
func (f *FakeCoupon) ReleaseCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.releaseCall
}

// Held 返回账本里处于「已扣减未归还」状态的请求数。
func (f *FakeCoupon) Held() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, e := range f.ledger {
		if !e.released {
			n++
		}
	}
	return n
}

// Deducted 报告 requestId 当前是否持有库存（已扣且未归还）。
func (f *FakeCoupon) Deducted(requestID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.ledger[requestID]
	return e != nil && !e.released
}

// FailDeductAfterCommit 让接下来 n 次 deduct 先完成状态变更、再返回 503（模拟响应丢失/超时）。
func (f *FakeCoupon) FailDeductAfterCommit(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failAfterCommit = n
}

// FailDeductBeforeCommit 让接下来 n 次 deduct 不做任何变更直接返回 503。
func (f *FakeCoupon) FailDeductBeforeCommit(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failBeforeCommit = n
}

// OnAfterDeduct 登记一个回调：每次 deduct 在库存状态变更之后、写响应之前（不持有内部锁）被调用。
// 用来在「已扣库存、订单尚未落库」的窗口里注入其他并发动作（例如 sweeper 提前收尾）。
func (f *FakeCoupon) OnAfterDeduct(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.afterDeduct = fn
}

// OnBeforeDeduct 登记一个回调：每次 deduct 在读取账本之前（不持有内部锁）被调用，
// 用来在「消费者已领取、尚未扣库存」的窗口里注入其他并发动作。
func (f *FakeCoupon) OnBeforeDeduct(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.beforeDeduct = fn
}

// FailRelease 让接下来 n 次 release 不做任何变更直接返回 503。
func (f *FakeCoupon) FailRelease(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failRelease = n
}

// FailGet 让接下来 n 次读券返回 503。
func (f *FakeCoupon) FailGet(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failGet = n
}

func writeOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": data})
}

func writeErr(w http.ResponseWriter, e *apperr.Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPStatus)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": e.Code, "message": e.Message, "data": nil})
}

func (f *FakeCoupon) handleDeduct(w http.ResponseWriter, r *http.Request) {
	var req internalapi.DeductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Validate() != nil {
		writeErr(w, apperr.InvalidRequest)
		return
	}
	f.mu.Lock()
	before := f.beforeDeduct
	f.mu.Unlock()
	if before != nil {
		before()
	}
	f.mu.Lock()
	f.deductCalls++
	if f.failBeforeCommit > 0 {
		f.failBeforeCommit--
		f.mu.Unlock()
		writeErr(w, apperr.SystemBusy)
		return
	}
	resp, appErr := f.deductLocked(req)
	failAfter := false
	if appErr == nil && f.failAfterCommit > 0 {
		f.failAfterCommit--
		failAfter = true
	}
	hook := f.afterDeduct
	f.mu.Unlock()
	if hook != nil && appErr == nil {
		hook()
	}
	switch {
	case appErr != nil:
		writeErr(w, appErr)
	case failAfter:
		writeErr(w, apperr.SystemBusy)
	default:
		writeOK(w, resp)
	}
}

func (f *FakeCoupon) deductLocked(req internalapi.DeductRequest) (internalapi.DeductResponse, *apperr.Error) {
	c := f.coupons[req.CouponID]
	if c == nil {
		return internalapi.DeductResponse{}, apperr.CouponNotFound
	}
	if e := f.ledger[req.RequestID]; e != nil {
		if e.released {
			return internalapi.DeductResponse{}, apperr.OutOfStock
		}
		return internalapi.DeductResponse{CouponID: req.CouponID, Bucket: 0, AlreadyDeducted: true}, nil
	}
	if !c.info.Enabled() {
		return internalapi.DeductResponse{}, apperr.CouponDisabled
	}
	if c.stock <= 0 {
		return internalapi.DeductResponse{}, apperr.OutOfStock
	}
	c.stock--
	f.ledger[req.RequestID] = &ledgerEntry{coupon: req.CouponID}
	f.effects[req.RequestID]++
	return internalapi.DeductResponse{CouponID: req.CouponID, Bucket: 0}, nil
}

func (f *FakeCoupon) handleRelease(w http.ResponseWriter, r *http.Request) {
	var req internalapi.ReleaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Validate() != nil {
		writeErr(w, apperr.InvalidRequest)
		return
	}
	f.mu.Lock()
	f.releaseCall++
	if f.failRelease > 0 {
		f.failRelease--
		f.mu.Unlock()
		writeErr(w, apperr.SystemBusy)
		return
	}
	resp := internalapi.ReleaseResponse{CouponID: req.CouponID}
	switch e := f.ledger[req.RequestID]; {
	case e == nil:
		f.ledger[req.RequestID] = &ledgerEntry{coupon: req.CouponID, released: true}
	case e.released:
		resp.AlreadyReleased = true
	default:
		e.released = true
		if c := f.coupons[e.coupon]; c != nil {
			c.stock++
		}
		resp.Released = true
	}
	f.mu.Unlock()
	writeOK(w, resp)
}

func (f *FakeCoupon) handleGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("id")), 10, 64)
	if err != nil {
		writeErr(w, apperr.InvalidRequest)
		return
	}
	f.mu.Lock()
	f.getCalls++
	if f.failGet > 0 {
		f.failGet--
		f.mu.Unlock()
		writeErr(w, apperr.SystemBusy)
		return
	}
	c := f.coupons[id]
	var info internalapi.CouponInfo
	if c != nil {
		info = c.info
	}
	f.mu.Unlock()
	if c == nil {
		writeErr(w, apperr.CouponNotFound)
		return
	}
	writeOK(w, info)
}

// SeckillCoupon 构造一张启用中的秒杀券信息，时间窗为空（不限）。
func SeckillCoupon(id int64) internalapi.CouponInfo {
	return internalapi.CouponInfo{
		ID: id, Name: "秒杀券-" + strconv.FormatInt(id, 10), Type: internalapi.CouponTypeSeckill,
		Status: internalapi.CouponStatusEnabled, FaceValue: "50.00", MinSpend: "100.00", ValidDays: 7, PerUserLimit: 1,
	}
}

// NormalCoupon 构造一张启用中的常驻券信息。
func NormalCoupon(id int64) internalapi.CouponInfo {
	c := SeckillCoupon(id)
	c.Name = "常驻券-" + strconv.FormatInt(id, 10)
	c.Type = internalapi.CouponTypeNormal
	return c
}

// Window 返回时间窗指针，便于给 CouponInfo 设置 SeckillStartAt/EndAt。
func Window(t time.Time) *time.Time { return &t }
