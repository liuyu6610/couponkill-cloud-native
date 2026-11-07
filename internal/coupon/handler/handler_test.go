package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/cache"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/handler"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/repo"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

// stubOrders 是 order 内部发券接口的替身。
type stubOrders struct {
	err   error
	calls atomic.Int32
}

func (o *stubOrders) GrantOrder(_ context.Context, req internalapi.GrantRequest) (internalapi.GrantResponse, error) {
	o.calls.Add(1)
	if o.err != nil {
		return internalapi.GrantResponse{}, o.err
	}
	return internalapi.GrantResponse{OrderID: 9_000_000_000_000_000_001}, nil
}

type rig struct {
	h      http.Handler
	pool   *pgxpool.Pool
	mr     *miniredis.Miniredis
	orders *stubOrders
	svc    *service.Service
}

func newRig(t *testing.T) *rig {
	t.Helper()
	pool := dbxtest.New(t)
	if err := dbx.Migrate(context.Background(), pool, "cpn", migrations.FS); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	rdb, mr := testkit.Redis(t)
	gen, err := idgen.New(1)
	if err != nil {
		t.Fatal(err)
	}
	orders := &stubOrders{}
	svc, err := service.New(service.Deps{
		Repo: repo.New(pool), Cache: cache.New(rdb), Orders: orders, IDs: gen,
		Config: service.Config{StockBuckets: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	eng := httpx.NewEngine(httpx.Options{Service: "coupon"})
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	if err := handler.Mount(eng, handler.Options{
		Service: svc, Signer: testkit.Signer(t), InternalToken: testkit.InternalToken, InputLocation: shanghai,
	}); err != nil {
		t.Fatal(err)
	}
	return &rig{h: eng, pool: pool, mr: mr, orders: orders, svc: svc}
}

var (
	admin = testkit.User(1, "admin")
	user  = testkit.User(42, "user")
)

func seckillBody() map[string]any {
	return map[string]any{
		"name": "秒杀券", "description": "d", "type": 2, "faceValue": 50, "minSpend": 100.5,
		"validDays": 7, "perUserLimit": 1, "totalStock": 40,
		"seckillStartAt": "2030-03-01T10:00:00Z", "seckillEndAt": "2030-03-01T12:00:00Z",
	}
}

func (r *rig) createSeckill(t *testing.T) string {
	t.Helper()
	res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/create", seckillBody(), admin)
	if res.Status != 200 {
		t.Fatalf("create: %d %s", res.Status, res.Body)
	}
	var out map[string]any
	res.Decode(&out)
	return out["id"].(string)
}

func (r *rig) stockKey(id string) string {
	n, _ := strconv.ParseInt(id, 10, 64)
	return keys.Stock(n)
}

// --- 公共接口：JSON 形状必须与前端 types/api.ts 一致 ---

func TestCreatedSeckillCouponMatchesFrontendShape(t *testing.T) {
	r := newRig(t)
	res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/create", seckillBody(), admin)
	if res.Status != http.StatusOK || res.Code != 200 {
		t.Fatalf("create: %d %s", res.Status, res.Body)
	}
	var c map[string]any
	res.Decode(&c)
	id, isString := c["id"].(string)
	if !isString || id == "" {
		t.Fatalf("id must be a JSON string: %#v", c["id"])
	}
	want := map[string]any{
		"name": "秒杀券", "description": "d", "type": float64(2), "faceValue": float64(50), "minSpend": 100.5,
		"validDays": float64(7), "perUserLimit": float64(1), "status": float64(1),
		"totalStock": float64(40), "remainingStock": float64(40),
		"seckillStartAt": "2030-03-01T10:00:00Z", "seckillEndAt": "2030-03-01T12:00:00Z",
	}
	for k, v := range want {
		if c[k] != v {
			t.Errorf("%s = %#v, want %#v", k, c[k], v)
		}
	}
	for _, k := range []string{"createdAt", "updatedAt"} {
		if s, ok := c[k].(string); !ok || s == "" {
			t.Errorf("%s must be a timestamp string: %#v", k, c[k])
		}
	}

	// 创建后：计数器已预热，元数据缓存键已删除。
	if got, err := r.mr.Get(r.stockKey(id)); err != nil || got != "40" {
		t.Fatalf("ck:stock = %q,%v want 40", got, err)
	}
}

func TestNormalCouponUsesRegularStockFields(t *testing.T) {
	r := newRig(t)
	res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/create", map[string]any{
		"name": "常驻券", "type": 1, "faceValue": "10", "minSpend": 0, "validDays": 15, "perUserLimit": 2, "totalStock": 20,
	}, admin)
	if res.Status != 200 {
		t.Fatalf("create: %d %s", res.Status, res.Body)
	}
	var c map[string]any
	res.Decode(&c)
	if c["totalStock"] != float64(20) || c["remainingStock"] != float64(20) {
		t.Fatalf("normal coupon stock fields: %v", c)
	}
	for _, gone := range []string{"seckillTotalStock", "seckillRemainingStock", "createTime", "updateTime", "shardIndex", "version"} {
		if _, present := c[gone]; present {
			t.Fatalf("legacy field %q must not be emitted: %v", gone, c)
		}
	}
	if c["seckillStartAt"] != nil || c["seckillEndAt"] != nil {
		t.Fatalf("no window must serialise as null: %v", c)
	}
	if r.mr.Exists(r.stockKey(c["id"].(string))) {
		t.Fatal("a normal coupon must not get a seckill counter")
	}
}

func TestAvailableIsAnArrayAndListIsPaged(t *testing.T) {
	r := newRig(t)
	for range 3 {
		r.createSeckill(t)
	}
	res := testkit.DoJSON(t, r.h, "GET", "/api/v1/coupon/available", nil)
	if res.Status != 200 {
		t.Fatalf("available: %d %s", res.Status, res.Body)
	}
	var arr []map[string]any
	res.Decode(&arr)
	if len(arr) != 3 {
		t.Fatalf("available = %d items, want 3", len(arr))
	}

	res = testkit.DoJSON(t, r.h, "GET", "/api/v1/coupon/list?page=2&size=2", nil)
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
		Page  int              `json:"page"`
		Size  int              `json:"size"`
	}
	res.Decode(&page)
	if res.Status != 200 || page.Total != 3 || page.Page != 2 || page.Size != 2 || len(page.Items) != 1 {
		t.Fatalf("paged list: %d %+v", res.Status, page)
	}

	for _, bad := range []string{"size=101", "page=0&size=5", "page=abc"} {
		res = testkit.DoJSON(t, r.h, "GET", "/api/v1/coupon/list?"+bad, nil)
		if res.Status != 400 || res.Code != apperr.CodeInvalidRequest {
			t.Errorf("list?%s: %d %d", bad, res.Status, res.Code)
		}
	}
}

func TestEmptyAvailableIsEmptyArrayNotNull(t *testing.T) {
	r := newRig(t)
	res := testkit.DoJSON(t, r.h, "GET", "/api/v1/coupon/available", nil)
	if res.Status != 200 || string(res.Body) == "" {
		t.Fatal("bad response")
	}
	var arr []any
	res.Decode(&arr)
	if arr == nil || len(arr) != 0 {
		t.Fatalf("must be [] not null: %s", res.Body)
	}
}

func TestGetCouponByID(t *testing.T) {
	r := newRig(t)
	id := r.createSeckill(t)
	res := testkit.DoJSON(t, r.h, "GET", "/api/v1/coupon/"+id, nil)
	var c map[string]any
	res.Decode(&c)
	if res.Status != 200 || c["id"] != id {
		t.Fatalf("get: %d %s", res.Status, res.Body)
	}
	if res := testkit.DoJSON(t, r.h, "GET", "/api/v1/coupon/987654321", nil); res.Status != 404 || res.Code != apperr.CodeCouponNotFound {
		t.Fatalf("missing coupon: %d %d", res.Status, res.Code)
	}
	if res := testkit.DoJSON(t, r.h, "GET", "/api/v1/coupon/not-a-number", nil); res.Status != 400 {
		t.Fatalf("non numeric id: %d", res.Status)
	}
	if res := testkit.DoJSON(t, r.h, "GET", "/api/v1/coupon/-5", nil); res.Status != 400 {
		t.Fatalf("negative id: %d", res.Status)
	}
}

// --- 权限矩阵 ---

func TestAdminEndpointsRequireAdminRole(t *testing.T) {
	r := newRig(t)
	id := r.createSeckill(t)
	routes := []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/v1/coupon/create", seckillBody()},
		{"POST", "/api/v1/coupon/" + id + "/seckill-window", map[string]any{"seckillStartAt": "2030-04-01T10:00:00Z", "seckillEndAt": "2030-04-01T11:00:00Z"}},
		{"POST", "/api/v1/coupon/" + id + "/status", map[string]any{"status": 1}},
		{"DELETE", "/api/v1/coupon/" + id, nil},
		{"POST", "/api/v1/coupon/admin/grant", map[string]any{"couponId": id, "userId": "42"}},
		{"POST", "/api/v1/coupon/preheat-stock", nil},
		{"POST", "/api/v1/coupon/preheat-stock/" + id, nil},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			anon := testkit.DoJSON(t, r.h, rt.method, rt.path, rt.body)
			if anon.Status != 401 || anon.Code != apperr.CodeAuthFail {
				t.Errorf("anonymous: %d/%d, want 401/%d", anon.Status, anon.Code, apperr.CodeAuthFail)
			}
			forged := testkit.DoJSON(t, r.h, rt.method, rt.path, rt.body,
				testkit.Header("X-User-Id", "1"), testkit.Header("X-User-Roles", "admin"), testkit.Header("X-Gateway-Sig", "deadbeef"))
			if forged.Status != 401 {
				t.Errorf("forged identity headers: %d, want 401", forged.Status)
			}
			plain := testkit.DoJSON(t, r.h, rt.method, rt.path, rt.body, user)
			if plain.Status != 403 || plain.Code != apperr.CodeForbidden {
				t.Errorf("plain user: %d/%d, want 403/%d", plain.Status, plain.Code, apperr.CodeForbidden)
			}
		})
	}
	// 以上拒绝都不得产生副作用。
	if got, err := r.mr.Get(r.stockKey(id)); err != nil || got != "40" {
		t.Fatalf("rejected admin calls must not touch the counter: %q %v", got, err)
	}
}

func TestInternalEndpointsRequireInternalToken(t *testing.T) {
	r := newRig(t)
	id := r.createSeckill(t)
	deduct := map[string]any{"requestId": "r-1", "couponId": id, "userId": "42"}
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{"POST", internalapi.PathStockDeduct, deduct},
		{"POST", internalapi.PathStockRelease, map[string]any{"requestId": "r-1", "couponId": id}},
		{"POST", internalapi.PathStockSync, map[string]any{"couponId": id, "available": 5, "source": "test"}},
		{"GET", internalapi.CouponPath(mustID(t, id)), nil},
	} {
		t.Run(tc.path, func(t *testing.T) {
			none := testkit.DoJSON(t, r.h, tc.method, tc.path, tc.body)
			if none.Status != 401 {
				t.Errorf("no token: %d", none.Status)
			}
			wrong := testkit.DoJSON(t, r.h, tc.method, tc.path, tc.body, testkit.Header("X-Internal-Token", "wrong-token"))
			if wrong.Status != 401 {
				t.Errorf("wrong token: %d", wrong.Status)
			}
			asAdmin := testkit.DoJSON(t, r.h, tc.method, tc.path, tc.body, admin)
			if asAdmin.Status != 401 {
				t.Errorf("an admin identity must not open internal endpoints: %d", asAdmin.Status)
			}
		})
	}
	if res := testkit.DoJSON(t, r.h, "POST", internalapi.PathStockDeduct, deduct, testkit.Internal()); res.Status != 200 {
		t.Fatalf("with the right token: %d %s", res.Status, res.Body)
	}
}

func mustID(t *testing.T, s string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// --- 公共接口对停用券的可见性 ---

func TestDisabledCouponsAreHiddenFromEveryoneButAdmins(t *testing.T) {
	r := newRig(t)
	id := r.createSeckill(t)
	if res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/"+id+"/status", map[string]any{"status": 0}, admin); res.Status != 200 {
		t.Fatalf("disable: %d %s", res.Status, res.Body)
	}
	type page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	list := func(opts ...testkit.ReqOption) page {
		res := testkit.DoJSON(t, r.h, "GET", "/api/v1/coupon/list", nil, opts...)
		var p page
		res.Decode(&p)
		return p
	}
	if p := list(); p.Total != 0 {
		t.Fatalf("anonymous list must not expose disabled coupons: %+v", p)
	}
	if p := list(user); p.Total != 0 {
		t.Fatalf("user list must not expose disabled coupons: %+v", p)
	}
	if p := list(testkit.Header("X-User-Id", "1"), testkit.Header("X-User-Roles", "admin"), testkit.Header("X-Gateway-Sig", "bad")); p.Total != 0 {
		t.Fatalf("forged admin headers must be treated as anonymous: %+v", p)
	}
	if p := list(admin); p.Total != 1 {
		t.Fatalf("admin list must include disabled coupons: %+v", p)
	}
	if res := testkit.DoJSON(t, r.h, "GET", "/api/v1/coupon/"+id, nil, user); res.Status != 404 {
		t.Fatalf("user get of disabled coupon: %d", res.Status)
	}
	if res := testkit.DoJSON(t, r.h, "GET", "/api/v1/coupon/"+id, nil, admin); res.Status != 200 {
		t.Fatalf("admin get of disabled coupon: %d", res.Status)
	}
	res := testkit.DoJSON(t, r.h, "GET", "/api/v1/coupon/available", nil)
	var arr []any
	res.Decode(&arr)
	if len(arr) != 0 {
		t.Fatalf("available must exclude disabled coupons: %s", res.Body)
	}
}

// --- 创建：两种入参方式、时间解析、校验 ---

func TestCreateAcceptsQueryStringAndLegacyTimeFormat(t *testing.T) {
	r := newRig(t)
	q := url.Values{
		"name": {"query券"}, "type": {"2"}, "faceValue": {"10"}, "minSpend": {"0"}, "validDays": {"7"},
		"perUserLimit": {"1"}, "totalStock": {"32"},
		"seckillStartAt": {"2030-03-01 10:00:00"}, "seckillEndAt": {"2030-03-01 12:00:00"},
	}
	res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/create?"+q.Encode(), nil, admin)
	if res.Status != 200 {
		t.Fatalf("create via query: %d %s", res.Status, res.Body)
	}
	var c map[string]any
	res.Decode(&c)
	if c["seckillStartAt"] != "2030-03-01T02:00:00Z" || c["seckillEndAt"] != "2030-03-01T04:00:00Z" {
		t.Fatalf("zone-less input is Asia/Shanghai (+08:00): %v %v", c["seckillStartAt"], c["seckillEndAt"])
	}
}

func TestCreateHonoursExplicitIDAndRejectsDuplicates(t *testing.T) {
	r := newRig(t)
	body := seckillBody()
	body["id"] = "880000000000000001"
	res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/create", body, admin)
	var c map[string]any
	res.Decode(&c)
	if res.Status != 200 || c["id"] != "880000000000000001" {
		t.Fatalf("explicit id: %d %s", res.Status, res.Body)
	}
	dup := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/create", body, admin)
	if dup.Status != 409 {
		t.Fatalf("duplicate id: %d %s", dup.Status, dup.Body)
	}
}

func TestCreateValidationErrors(t *testing.T) {
	r := newRig(t)
	mut := func(f func(m map[string]any)) map[string]any {
		m := seckillBody()
		f(m)
		return m
	}
	cases := map[string]any{
		"malformed json":      `{"name":`,
		"empty body":          "",
		"missing name":        mut(func(m map[string]any) { delete(m, "name") }),
		"bad type":            mut(func(m map[string]any) { m["type"] = 5 }),
		"missing face value":  mut(func(m map[string]any) { delete(m, "faceValue") }),
		"negative face value": mut(func(m map[string]any) { m["faceValue"] = -1 }),
		"three decimals":      mut(func(m map[string]any) { m["minSpend"] = 1.234 }),
		"no stock":            mut(func(m map[string]any) { delete(m, "totalStock") }),
		"negative stock":      mut(func(m map[string]any) { m["totalStock"] = -3 }),
		"no window":           mut(func(m map[string]any) { delete(m, "seckillStartAt"); delete(m, "seckillEndAt") }),
		"end before start":    mut(func(m map[string]any) { m["seckillEndAt"] = "2030-03-01T09:00:00Z" }),
		"garbage time":        mut(func(m map[string]any) { m["seckillStartAt"] = "tomorrow" }),
		"wrong field type":    mut(func(m map[string]any) { m["validDays"] = "seven" }),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/create", body, admin)
			if res.Status != 400 || res.Code != apperr.CodeInvalidRequest {
				t.Fatalf("status/code = %d/%d, want 400/%d (%s)", res.Status, res.Code, apperr.CodeInvalidRequest, res.Body)
			}
			if !res.DataIsNull() || res.Message == "" {
				t.Fatalf("failure envelope must have data=null and a message: %s", res.Body)
			}
		})
	}
	var n int
	if err := r.pool.QueryRow(context.Background(), `SELECT count(*) FROM cpn.coupons`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("no invalid request may create a coupon: %d %v", n, err)
	}
}

// --- 状态机与 Redis ---

func TestStatusWindowAndDeleteKeepRedisConsistent(t *testing.T) {
	r := newRig(t)
	id := r.createSeckill(t)
	key := r.stockKey(id)
	metaKey := keys.CouponMeta(mustID(t, id))
	r.mr.Set(metaKey, `{"stale":true}`)

	// 停用：计数器删除、元数据缓存失效
	res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/"+id+"/status", map[string]any{"status": 0}, admin)
	var n int
	res.Decode(&n)
	if res.Status != 200 || n != 1 {
		t.Fatalf("disable: %d %s", res.Status, res.Body)
	}
	if r.mr.Exists(key) || r.mr.Exists(metaKey) {
		t.Fatal("disabling must drop both the stock counter and the meta cache")
	}

	// 启用：计数器恢复为 DB 剩余
	r.mr.Set(metaKey, `{"stale":true}`)
	if res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/"+id+"/status?status=1", nil, admin); res.Status != 200 {
		t.Fatalf("enable via query: %d %s", res.Status, res.Body)
	}
	if got, _ := r.mr.Get(key); got != "40" || r.mr.Exists(metaKey) {
		t.Fatalf("counter=%q meta exists=%v", got, r.mr.Exists(metaKey))
	}

	// 改时间窗：返回更新后的券，元数据缓存失效
	r.mr.Set(metaKey, `{"stale":true}`)
	res = testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/"+id+"/seckill-window",
		map[string]any{"seckillStartAt": "2031-01-01 08:00:00", "seckillEndAt": "2031-01-01 09:00:00"}, admin)
	var c map[string]any
	res.Decode(&c)
	if res.Status != 200 || c["seckillStartAt"] != "2031-01-01T00:00:00Z" || r.mr.Exists(metaKey) {
		t.Fatalf("window: %d %s meta=%v", res.Status, res.Body, r.mr.Exists(metaKey))
	}

	// 删除：幂等，计数器与元数据缓存都清掉
	r.mr.Set(metaKey, `{"stale":true}`)
	res = testkit.DoJSON(t, r.h, "DELETE", "/api/v1/coupon/"+id, nil, admin)
	res.Decode(&n)
	if res.Status != 200 || n != 1 || r.mr.Exists(key) || r.mr.Exists(metaKey) {
		t.Fatalf("delete: %d %s", res.Status, res.Body)
	}
	res = testkit.DoJSON(t, r.h, "DELETE", "/api/v1/coupon/"+id, nil, admin)
	res.Decode(&n)
	if res.Status != 200 || n != 0 {
		t.Fatalf("second delete must be an idempotent 0: %d %s", res.Status, res.Body)
	}
}

func TestStatusAndWindowErrors(t *testing.T) {
	r := newRig(t)
	id := r.createSeckill(t)
	cases := []struct {
		name, path string
		body       any
		status     int
		code       int
	}{
		{"status out of range", "/api/v1/coupon/" + id + "/status", map[string]any{"status": 2}, 400, apperr.CodeInvalidRequest},
		{"status missing", "/api/v1/coupon/" + id + "/status", map[string]any{}, 400, apperr.CodeInvalidRequest},
		{"status unknown coupon", "/api/v1/coupon/12345/status", map[string]any{"status": 1}, 404, apperr.CodeCouponNotFound},
		{"window reversed", "/api/v1/coupon/" + id + "/seckill-window", map[string]any{"seckillStartAt": "2031-01-01T10:00:00Z", "seckillEndAt": "2031-01-01T09:00:00Z"}, 400, apperr.CodeInvalidRequest},
		{"window unknown coupon", "/api/v1/coupon/12345/seckill-window", map[string]any{"seckillStartAt": "2031-01-01T08:00:00Z", "seckillEndAt": "2031-01-01T09:00:00Z"}, 404, apperr.CodeCouponNotFound},
		{"window missing end", "/api/v1/coupon/" + id + "/seckill-window", map[string]any{"seckillStartAt": "2031-01-01T08:00:00Z"}, 400, apperr.CodeInvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := testkit.DoJSON(t, r.h, "POST", tc.path, tc.body, admin)
			if res.Status != tc.status || res.Code != tc.code {
				t.Fatalf("got %d/%d (%s), want %d/%d", res.Status, res.Code, res.Body, tc.status, tc.code)
			}
		})
	}
}

func TestWindowRejectedForNormalCoupon(t *testing.T) {
	r := newRig(t)
	res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/create", map[string]any{
		"name": "常驻券", "type": 1, "faceValue": 10, "minSpend": 0, "validDays": 15, "perUserLimit": 1, "totalStock": 5,
	}, admin)
	var c map[string]any
	res.Decode(&c)
	res = testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/"+c["id"].(string)+"/seckill-window",
		map[string]any{"seckillStartAt": "2031-01-01T08:00:00Z", "seckillEndAt": "2031-01-01T09:00:00Z"}, admin)
	if res.Status != 400 {
		t.Fatalf("normal coupons have no seckill window: %d %s", res.Status, res.Body)
	}
}

// --- 预热 ---

func TestPreheatEndpointsAlignRedisWithDB(t *testing.T) {
	r := newRig(t)
	id := r.createSeckill(t)
	key := r.stockKey(id)

	r.mr.Set(key, "3") // 漂移
	res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/preheat-stock/"+id, nil, admin)
	var one map[string]any
	res.Decode(&one)
	if res.Status != 200 || one["couponId"] != id || one["stock"] != float64(40) {
		t.Fatalf("preheat one: %d %s", res.Status, res.Body)
	}
	if got, _ := r.mr.Get(key); got != "40" {
		t.Fatalf("counter = %q, want 40", got)
	}

	r.mr.Del(key)
	res = testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/preheat-stock", nil, admin)
	var sum map[string]any
	res.Decode(&sum)
	if res.Status != 200 || sum["total"] != float64(1) || sum["preheated"] != float64(1) || sum["failed"] != float64(0) {
		t.Fatalf("preheat all: %d %s", res.Status, res.Body)
	}
	if got, _ := r.mr.Get(key); got != "40" {
		t.Fatalf("counter = %q after preheat all", got)
	}

	if res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/preheat-stock/12345", nil, admin); res.Status != 404 {
		t.Fatalf("unknown coupon: %d", res.Status)
	}
}

func TestPreheatNeverWritesStockKeyForDisabledOrNormalCoupons(t *testing.T) {
	r := newRig(t)
	id := r.createSeckill(t)
	normal := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/create", map[string]any{
		"name": "常驻券", "type": 1, "faceValue": 10, "minSpend": 0, "validDays": 15, "perUserLimit": 1, "totalStock": 5,
	}, admin)
	var nc map[string]any
	normal.Decode(&nc)
	testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/"+id+"/status", map[string]any{"status": 0}, admin)

	if res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/preheat-stock/"+id, nil, admin); res.Status != 409 || res.Code != apperr.CodeCouponDisabled {
		t.Fatalf("disabled preheat: %d/%d", res.Status, res.Code)
	}
	if res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/preheat-stock/"+nc["id"].(string), nil, admin); res.Status != 400 {
		t.Fatalf("normal coupon preheat: %d", res.Status)
	}
	sum := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/preheat-stock", nil, admin)
	var s map[string]any
	sum.Decode(&s)
	if s["total"] != float64(0) {
		t.Fatalf("nothing is eligible for preheating: %s", sum.Body)
	}
	if r.mr.Exists(r.stockKey(id)) || r.mr.Exists(r.stockKey(nc["id"].(string))) {
		t.Fatal("no stock key may exist for a disabled seckill coupon or a normal coupon")
	}
}

func TestRedisOutageAfterCommitReportsSavedButUnwarmed(t *testing.T) {
	r := newRig(t)
	r.mr.Close()
	res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/create", seckillBody(), admin)
	if res.Status != 503 || res.Code != apperr.CodeSystemBusy {
		t.Fatalf("redis down: %d/%d %s", res.Status, res.Code, res.Body)
	}
	if res.Message == apperr.SystemBusy.Message {
		t.Fatalf("the message must explain the coupon was saved: %q", res.Message)
	}
	var n int
	if err := r.pool.QueryRow(context.Background(), `SELECT count(*) FROM cpn.coupons`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("the coupon must be persisted: %d %v", n, err)
	}
}

// --- 内部接口 ---

func TestInternalDeductReleaseSyncInfoContract(t *testing.T) {
	r := newRig(t)
	id := r.createSeckill(t)
	call := func(method, path string, body any) *testkit.Response {
		return testkit.DoJSON(t, r.h, method, path, body, testkit.Internal())
	}

	d := call("POST", internalapi.PathStockDeduct, map[string]any{"requestId": "req-1", "couponId": id, "userId": "42"})
	var dr map[string]any
	d.Decode(&dr)
	if d.Status != 200 || dr["couponId"] != id || dr["alreadyDeducted"] != false {
		t.Fatalf("deduct: %d %s", d.Status, d.Body)
	}
	if _, ok := dr["bucket"].(float64); !ok {
		t.Fatalf("bucket must be a number: %v", dr)
	}
	d2 := call("POST", internalapi.PathStockDeduct, map[string]any{"requestId": "req-1", "couponId": id, "userId": "42"})
	var dr2 map[string]any
	d2.Decode(&dr2)
	if dr2["alreadyDeducted"] != true || dr2["bucket"] != dr["bucket"] {
		t.Fatalf("replay must report alreadyDeducted with the same bucket: %v vs %v", dr2, dr)
	}

	rel := call("POST", internalapi.PathStockRelease, map[string]any{"requestId": "req-1", "couponId": id})
	var rr map[string]any
	rel.Decode(&rr)
	if rel.Status != 200 || rr["released"] != true || rr["alreadyReleased"] != false || rr["couponId"] != id {
		t.Fatalf("release: %d %s", rel.Status, rel.Body)
	}
	if again := call("POST", internalapi.PathStockDeduct, map[string]any{"requestId": "req-1", "couponId": id, "userId": "42"}); again.Status != 409 || again.Code != apperr.CodeOutOfStock {
		t.Fatalf("deduct after release: %d/%d", again.Status, again.Code)
	}

	sy := call("POST", internalapi.PathStockSync, map[string]any{"couponId": id, "available": 7, "source": "connector", "mode": "set"})
	var sr map[string]any
	sy.Decode(&sr)
	if sy.Status != 200 || sr["available"] != float64(7) || sr["couponId"] != id {
		t.Fatalf("sync: %d %s", sy.Status, sy.Body)
	}
	var dbRemaining int64
	if err := r.pool.QueryRow(context.Background(), `SELECT sum(remaining) FROM cpn.coupon_stock WHERE coupon_id=$1`, mustID(t, id)).Scan(&dbRemaining); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.mr.Get(r.stockKey(id)); dbRemaining != 7 || got != "7" {
		t.Fatalf("after sync DB=%d redis=%q, both must be 7", dbRemaining, got)
	}

	inf := call("GET", internalapi.CouponPath(mustID(t, id)), nil)
	var ci map[string]any
	inf.Decode(&ci)
	if inf.Status != 200 || ci["id"] != id || ci["type"] != float64(2) || ci["status"] != float64(1) ||
		ci["faceValue"] != float64(50) || ci["seckillStartAt"] == nil {
		t.Fatalf("info: %d %s", inf.Status, inf.Body)
	}
}

// 连接器不传 mode（或传 min）时，平台给出再大的库存读数也不能把本地库存抬高；只有显式 mode=set 才可以。
// 同时响应里的 available 必须是实际生效值，Redis 计数器必须等于 DB 剩余量。
func TestInternalSyncDefaultModeNeverRaisesStock(t *testing.T) {
	r := newRig(t)
	id := r.createSeckill(t)
	call := func(body map[string]any) (int, map[string]any) {
		res := testkit.DoJSON(t, r.h, "POST", internalapi.PathStockSync, body, testkit.Internal())
		var m map[string]any
		res.Decode(&m)
		return res.Status, m
	}
	dbRemaining := func() int64 {
		var n int64
		if err := r.pool.QueryRow(context.Background(), `SELECT sum(remaining) FROM cpn.coupon_stock WHERE coupon_id=$1`, mustID(t, id)).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := dbRemaining()
	if before <= 0 || before >= 100000 {
		t.Fatalf("unexpected seed stock %d", before)
	}

	for name, body := range map[string]map[string]any{
		"no mode":  {"couponId": id, "available": before + 1000, "source": "connector"},
		"mode min": {"couponId": id, "available": before + 1000, "source": "connector", "mode": "min"},
	} {
		status, m := call(body)
		if status != 200 || m["available"] != float64(before) {
			t.Fatalf("%s: status=%d body=%v, want 200 with the unchanged available %d", name, status, m, before)
		}
		if got := dbRemaining(); got != before {
			t.Fatalf("%s raised the stock: %d -> %d", name, before, got)
		}
	}
	status, m := call(map[string]any{"couponId": id, "available": before - 1, "source": "connector"})
	if status != 200 || m["available"] != float64(before-1) || dbRemaining() != before-1 {
		t.Fatalf("a lower target must lower the stock: %d %v db=%d", status, m, dbRemaining())
	}
	if status, _ := call(map[string]any{"couponId": id, "available": 5, "source": "connector", "mode": "max"}); status != 400 {
		t.Fatalf("an unknown mode must be rejected with 400, got %d", status)
	}
	status, m = call(map[string]any{"couponId": id, "available": before + 1000, "source": "admin", "mode": "set"})
	if status != 200 || m["available"] != float64(before+1000) || dbRemaining() != before+1000 {
		t.Fatalf("mode=set must be able to raise: %d %v db=%d", status, m, dbRemaining())
	}
}

func TestInternalDeductSoldOutAndErrors(t *testing.T) {
	r := newRig(t)
	body := seckillBody()
	body["totalStock"] = 2
	res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/create", body, admin)
	var c map[string]any
	res.Decode(&c)
	id := c["id"].(string)
	deduct := func(rid, coupon string) *testkit.Response {
		return testkit.DoJSON(t, r.h, "POST", internalapi.PathStockDeduct,
			map[string]any{"requestId": rid, "couponId": coupon, "userId": "7"}, testkit.Internal())
	}
	for i := range 2 {
		if res := deduct(fmt.Sprintf("s-%d", i), id); res.Status != 200 {
			t.Fatalf("deduct %d: %d %s", i, res.Status, res.Body)
		}
	}
	if res := deduct("s-3", id); res.Status != 409 || res.Code != apperr.CodeOutOfStock || !res.DataIsNull() {
		t.Fatalf("sold out: %d/%d %s", res.Status, res.Code, res.Body)
	}
	if res := deduct("s-4", "99999"); res.Status != 404 || res.Code != apperr.CodeCouponNotFound {
		t.Fatalf("unknown coupon: %d/%d", res.Status, res.Code)
	}
	testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/"+id+"/status", map[string]any{"status": 0}, admin)
	if res := deduct("s-5", id); res.Status != 409 || res.Code != apperr.CodeCouponDisabled {
		t.Fatalf("disabled: %d/%d", res.Status, res.Code)
	}
	for name, body := range map[string]any{
		"empty requestId":    map[string]any{"requestId": "", "couponId": id, "userId": "7"},
		"requestId bad char": map[string]any{"requestId": "a b", "couponId": id, "userId": "7"},
		"numeric id":         map[string]any{"requestId": "x", "couponId": 5, "userId": "7"},
		"zero user":          map[string]any{"requestId": "x", "couponId": id, "userId": "0"},
		"malformed":          `{"requestId":`,
	} {
		if res := testkit.DoJSON(t, r.h, "POST", internalapi.PathStockDeduct, body, testkit.Internal()); res.Status != 400 {
			t.Errorf("%s: status %d, want 400 (%s)", name, res.Status, res.Body)
		}
	}
	if res := testkit.DoJSON(t, r.h, "POST", internalapi.PathStockSync,
		map[string]any{"couponId": id, "available": -1, "source": "x"}, testkit.Internal()); res.Status != 400 {
		t.Errorf("negative sync: %d", res.Status)
	}
	if res := testkit.DoJSON(t, r.h, "GET", internalapi.CouponPath(31337), nil, testkit.Internal()); res.Status != 404 {
		t.Errorf("unknown coupon info: %d", res.Status)
	}
}

// --- 管理员发券 ---

func TestAdminGrantHappyPathAndRejection(t *testing.T) {
	r := newRig(t)
	id := r.createSeckill(t)
	key := r.stockKey(id)

	res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/admin/grant",
		map[string]any{"couponId": id, "userId": "42", "requestId": "adm-1"}, admin)
	var g map[string]any
	res.Decode(&g)
	if res.Status != 200 || g["requestId"] != "adm-1" || g["orderId"] != "9000000000000000001" || g["alreadyGranted"] != false {
		t.Fatalf("grant: %d %s", res.Status, res.Body)
	}
	if got, _ := r.mr.Get(key); got != "39" {
		t.Fatalf("redis must follow the DB deduction: %q", got)
	}

	// order 明确拒绝（用户已持有）：库存归还，计数器不变
	r.orders.err = apperr.RepeatSeckill
	res = testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/admin/grant",
		map[string]any{"couponId": id, "userId": "43", "requestId": "adm-2"}, admin)
	if res.Status != 409 || res.Code != apperr.CodeRepeatSeckill {
		t.Fatalf("rejected grant: %d/%d", res.Status, res.Code)
	}
	var dbRemaining int64
	if err := r.pool.QueryRow(context.Background(), `SELECT sum(remaining) FROM cpn.coupon_stock WHERE coupon_id=$1`, mustID(t, id)).Scan(&dbRemaining); err != nil {
		t.Fatal(err)
	}
	if dbRemaining != 39 {
		t.Fatalf("a rejected grant must give the stock back: %d", dbRemaining)
	}
	if got, _ := r.mr.Get(key); got != "39" {
		t.Fatalf("counter must be untouched by a rejected grant: %q", got)
	}

	for name, body := range map[string]any{
		"missing user":   map[string]any{"couponId": id},
		"zero coupon":    map[string]any{"couponId": "0", "userId": "42"},
		"bad request id": map[string]any{"couponId": id, "userId": "42", "requestId": "has space"},
	} {
		if res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/admin/grant", body, admin); res.Status != 400 {
			t.Errorf("%s: %d %s", name, res.Status, res.Body)
		}
	}
}

func TestAdminGrantUnknownOutcomeIs503WithRequestID(t *testing.T) {
	r := newRig(t)
	id := r.createSeckill(t)
	r.orders.err = fmt.Errorf("order down: %w", internalapi.ErrTransient)
	res := testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/admin/grant",
		map[string]any{"couponId": id, "userId": "42", "requestId": "adm-9"}, admin)
	if res.Status != 503 || res.Code != apperr.CodeSystemBusy {
		t.Fatalf("unknown outcome: %d/%d", res.Status, res.Code)
	}
	if !contains(res.Message, "adm-9") {
		t.Fatalf("message must carry the requestId for the retry: %q", res.Message)
	}
	var dbRemaining int64
	if err := r.pool.QueryRow(context.Background(), `SELECT sum(remaining) FROM cpn.coupon_stock WHERE coupon_id=$1`, mustID(t, id)).Scan(&dbRemaining); err != nil {
		t.Fatal(err)
	}
	if dbRemaining != 39 {
		t.Fatalf("the deduction must be kept until the retry resolves the outcome: %d", dbRemaining)
	}
	// 用同一个 requestId 重试，order 恢复后成功，且库存只被扣了一次。
	r.orders.err = nil
	res = testkit.DoJSON(t, r.h, "POST", "/api/v1/coupon/admin/grant",
		map[string]any{"couponId": id, "userId": "42", "requestId": "adm-9"}, admin)
	if res.Status != 200 {
		t.Fatalf("retry: %d %s", res.Status, res.Body)
	}
	if err := r.pool.QueryRow(context.Background(), `SELECT sum(remaining) FROM cpn.coupon_stock WHERE coupon_id=$1`, mustID(t, id)).Scan(&dbRemaining); err != nil || dbRemaining != 39 {
		t.Fatalf("retry with the same requestId must not deduct again: %d %v", dbRemaining, err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestUnknownRoutesUseEnvelope(t *testing.T) {
	r := newRig(t)
	res := testkit.DoJSON(t, r.h, "GET", "/api/v1/coupon/1/nothing", nil)
	if res.Status != 404 || res.Code == 0 {
		t.Fatalf("unknown route: %d %s", res.Status, res.Body)
	}
}
