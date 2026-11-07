package connector_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/platforms"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/repo"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

const lockKey = "ck:lock:connector-sync"

// stubAdapter 是可编程的平台适配器（替身只放在系统边界：外部平台）。
type stubAdapter struct {
	platform domain.Platform
	status   string
	stock    int64
}

func (s *stubAdapter) Platform() domain.Platform { return s.platform }
func (s *stubAdapter) Product(_ context.Context, sku string) (domain.ProductSnapshot, error) {
	p := json.Number("12.50")
	on := true
	return domain.ProductSnapshot{Platform: s.platform, ExternalSkuID: sku, Title: "stub-" + sku, Price: &p, OnSale: &on, RawStatus: "1"}, nil
}
func (s *stubAdapter) Stock(_ context.Context, sku string) (domain.StockSnapshot, error) {
	q := s.stock
	a := q > 0
	return domain.StockSnapshot{Platform: s.platform, ExternalSkuID: sku, StockQty: &q, StockAvailable: &a, StockStateDesc: "STUB", Area: "x"}, nil
}
func (s *stubAdapter) Health(context.Context) domain.Health {
	st := s.status
	if st == "" {
		st = domain.StatusUp
	}
	return domain.Health{Platform: s.platform, Status: st, Message: "stub"}
}

type stubCoupon struct{ reqs []internalapi.SyncRequest }

func (c *stubCoupon) SyncStock(_ context.Context, r internalapi.SyncRequest) (internalapi.SyncResponse, error) {
	c.reqs = append(c.reqs, r)
	return internalapi.SyncResponse{CouponID: r.CouponID, Available: r.Available}, nil
}

type stubJD struct{}

func (stubJD) CredentialsConfigured() bool { return true }
func (stubJD) ServerURL() string           { return "https://api.jd.example/routerjson" }
func (stubJD) DefaultArea() string         { return "1_72_2799_0" }
func (stubJD) AppKeyMasked() string        { return "ab****yz" }

type env struct {
	t      *testing.T
	eng    *httpx.Engine
	mr     *miniredis.Miniredis
	coupon *stubCoupon
	store  *repo.Store
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := dbxtest.New(t)
	if err := dbx.Migrate(context.Background(), pool, "cnx", migrations.FS); err != nil {
		t.Fatal(err)
	}
	gen, _ := idgen.New(1)
	store := repo.NewStore(pool, gen)
	rdb, mr := testkit.Redis(t)
	reg, err := platforms.NewRegistry(platforms.RegistryConfig{HealthTTL: time.Hour},
		platforms.NewMock(platforms.MockConfig{}),
		&stubAdapter{platform: domain.JD, stock: 77},
		&stubAdapter{platform: domain.TB, status: domain.StatusDisabled},
		&stubAdapter{platform: domain.PDD, status: domain.StatusDown})
	if err != nil {
		t.Fatal(err)
	}
	coupon := &stubCoupon{}
	eng := connector.NewEngine(connector.Deps{
		Bindings: service.NewBindingService(store),
		Prices:   service.NewPriceService(store, reg, service.PriceConfig{CacheTTL: -1}),
		Probe:    service.NewProbeService(reg),
		Sync:     service.NewSyncService(store, reg, coupon, service.NewRedisLocker(rdb, time.Minute), service.SyncConfig{LockTTL: time.Minute}),
		Signer:   testkit.Signer(t),
		JD:       stubJD{},
	})
	return &env{t: t, eng: eng, mr: mr, coupon: coupon, store: store}
}

var (
	anon   = []testkit.ReqOption{}
	user   = []testkit.ReqOption{testkit.User(42, "user")}
	admin  = []testkit.ReqOption{testkit.User(1, "admin")}
	forged = []testkit.ReqOption{testkit.Header("X-User-Id", "1"), testkit.Header("X-User-Roles", "admin")}
)

func (e *env) do(method, path string, body any, who []testkit.ReqOption) *testkit.Response {
	e.t.Helper()
	return testkit.DoJSON(e.t, e.eng, method, BasePath+path, body, who...)
}

const BasePath = connector.BasePath

func TestAuthorizationMatrixForEveryRoute(t *testing.T) {
	e := newEnv(t)
	type route struct {
		method, path string
		body         any
		public       bool // C 端只读，匿名可访问
	}
	routes := []route{
		{"GET", "/health", nil, true},
		{"GET", "/bindings/by-coupon/1001", nil, true},
		{"GET", "/price-compare?couponId=1001", nil, true},
		{"POST", "/bindings", map[string]any{"platform": "MOCK", "externalSkuId": "s", "couponId": "1"}, false},
		{"GET", "/bindings", nil, false},
		{"POST", "/price-maps", map[string]any{"couponId": "1", "platform": "TB", "externalSkuId": "t", "manualPrice": 1}, false},
		{"GET", "/price-maps?couponId=1", nil, false},
		{"DELETE", "/price-maps/1", nil, false},
		{"POST", "/sync/1", nil, false},
		{"POST", "/sync", nil, false},
		{"GET", "/platforms", nil, false},
		{"GET", "/probe/MOCK/product?skuId=s", nil, false},
		{"GET", "/probe/MOCK/stock?skuId=s", nil, false},
	}
	if len(routes) != 13 {
		t.Fatalf("routes = %d, 规格要求 13 个端点", len(routes))
	}
	for _, r := range routes {
		name := r.method + " " + r.path

		got := e.do(r.method, r.path, r.body, anon)
		if r.public && got.Status == http.StatusUnauthorized || r.public && got.Status == http.StatusForbidden {
			t.Errorf("%s anonymous = %d, want public read to be allowed", name, got.Status)
		}
		if !r.public && got.Status != http.StatusUnauthorized {
			t.Errorf("%s anonymous = %d, want 401", name, got.Status)
		}

		got = e.do(r.method, r.path, r.body, user)
		if r.public && got.Status >= 400 && got.Status != http.StatusNotFound {
			t.Errorf("%s user = %d, want allowed", name, got.Status)
		}
		if !r.public && got.Status != http.StatusForbidden {
			t.Errorf("%s user = %d, want 403", name, got.Status)
		}

		got = e.do(r.method, r.path, r.body, forged)
		if r.path != "/health" && got.Status != http.StatusUnauthorized { // /health 完全不看身份头
			t.Errorf("%s forged identity headers = %d, want 401（没有网关签名的身份头一律不信）", name, got.Status)
		}

		if got = e.do(r.method, r.path, r.body, admin); got.Status == http.StatusUnauthorized || got.Status == http.StatusForbidden {
			t.Errorf("%s admin = %d, want allowed", name, got.Status)
		}
	}
}

func TestBindingEndpointsRoundTripAndJSONShape(t *testing.T) {
	e := newEnv(t)

	// couponId 同时接受字符串与数字；platform 大小写不敏感。
	res := e.do("POST", "/bindings", `{"platform":"mock","externalSkuId":" sku-1 ","couponId":"1001","syncEnabled":true}`, admin)
	if res.Status != 200 {
		t.Fatalf("create = %d %s", res.Status, res.Body)
	}
	var raw map[string]any
	res.Decode(&raw)
	for k, want := range map[string]any{
		"platform": "MOCK", "externalSkuId": "sku-1", "couponId": "1001", "syncEnabled": true, "lastSyncStatus": "NEW",
		"lastStock": nil, "lastSyncAt": nil, "lastError": nil,
	} {
		if got, ok := raw[k]; !ok || got != want {
			t.Errorf("field %s = %v (present=%v), want %v", k, got, ok, want)
		}
	}
	if id, ok := raw["id"].(string); !ok || id == "" {
		t.Errorf("id = %v, want a string", raw["id"])
	}
	for _, k := range []string{"createdAt", "updatedAt"} {
		s, _ := raw[k].(string)
		if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
			t.Errorf("%s = %q is not RFC3339", k, s)
		}
	}

	if res = e.do("POST", "/bindings", `{"platform":"JD","externalSkuId":"jd-9","couponId":1002}`, admin); res.Status != 200 {
		t.Fatalf("numeric couponId = %d %s", res.Status, res.Body)
	}
	var list []domain.Binding
	e.do("GET", "/bindings", nil, admin).Decode(&list)
	if len(list) != 2 || list[0].Platform != domain.JD {
		t.Fatalf("list = %+v, want newest first", list)
	}

	var byCoupon *domain.Binding
	if res = e.do("GET", "/bindings/by-coupon/1001", nil, anon); res.Status != 200 {
		t.Fatalf("by-coupon = %d", res.Status)
	}
	res.Decode(&byCoupon)
	if byCoupon == nil || byCoupon.ExternalSkuID != "sku-1" {
		t.Fatalf("by-coupon = %+v", byCoupon)
	}
	if res = e.do("GET", "/bindings/by-coupon/9999", nil, anon); res.Status != 200 || !res.DataIsNull() {
		t.Fatalf("unbound coupon = %d %s, want 200 with data null", res.Status, res.Body)
	}
}

func TestBindingErrorPaths(t *testing.T) {
	e := newEnv(t)
	e.do("POST", "/bindings", `{"platform":"MOCK","externalSkuId":"a","couponId":"5"}`, admin)

	cases := []struct {
		name   string
		body   any
		status int
		msg    string
	}{
		{"malformed json", `{"platform":`, 400, "JSON"},
		{"empty body", ``, 400, ""},
		{"unknown platform", `{"platform":"AMAZON","externalSkuId":"a","couponId":"1"}`, 400, "未知平台"},
		{"missing platform", `{"externalSkuId":"a","couponId":"1"}`, 400, "platform"},
		{"blank sku", `{"platform":"MOCK","externalSkuId":" ","couponId":"1"}`, 400, "externalSkuId"},
		{"bad coupon id", `{"platform":"MOCK","externalSkuId":"a","couponId":"abc"}`, 400, ""},
		{"zero coupon", `{"platform":"MOCK","externalSkuId":"a","couponId":0}`, 400, "couponId"},
		{"coupon already bound", `{"platform":"JD","externalSkuId":"zzz","couponId":"5"}`, 409, "已绑定"},
	}
	for _, c := range cases {
		res := e.do("POST", "/bindings", c.body, admin)
		if res.Status != c.status || !strings.Contains(res.Message, c.msg) {
			t.Errorf("%s: %d %q, want %d containing %q", c.name, res.Status, res.Message, c.status, c.msg)
		}
		if res.Code == 200 {
			t.Errorf("%s: envelope code must not be 200", c.name)
		}
	}
	if res := e.do("GET", "/bindings/by-coupon/abc", nil, anon); res.Status != 400 {
		t.Errorf("by-coupon non-numeric = %d, want 400", res.Status)
	}
	if res := e.do("GET", "/bindings/by-coupon/-3", nil, anon); res.Status != 400 {
		t.Errorf("by-coupon negative = %d, want 400", res.Status)
	}
}

func TestPriceMapEndpointsAndCompare(t *testing.T) {
	e := newEnv(t)
	e.do("POST", "/bindings", `{"platform":"JD","externalSkuId":"jd-1","couponId":"7"}`, admin)

	res := e.do("POST", "/price-maps", `{"couponId":"7","platform":"TB","externalSkuId":"tb-1","title":"同款","manualPrice":79.9}`, admin)
	if res.Status != 200 {
		t.Fatalf("upsert = %d %s", res.Status, res.Body)
	}
	if !strings.Contains(string(res.Body), `"manualPrice":79.90`) {
		t.Fatalf("body = %s, want manualPrice as a two-decimal JSON number", res.Body)
	}
	var pm domain.PriceMap
	res.Decode(&pm)
	if pm.Currency != "CNY" || !pm.Enabled || pm.CouponID != 7 {
		t.Fatalf("price map = %+v", pm)
	}

	var maps []domain.PriceMap
	e.do("GET", "/price-maps?couponId=7", nil, admin).Decode(&maps)
	if len(maps) != 1 {
		t.Fatalf("maps = %+v", maps)
	}

	var cmp domain.PriceCompareResult
	res = e.do("GET", "/price-compare?couponId=7", nil, anon)
	if res.Status != 200 {
		t.Fatalf("compare = %d %s", res.Status, res.Body)
	}
	res.Decode(&cmp)
	if len(cmp.Items) != 2 || cmp.Items[0].Source != "PROBE" || cmp.Items[1].Source != "MANUAL" || cmp.BindingID == nil {
		t.Fatalf("compare = %+v", cmp)
	}
	if !strings.Contains(string(res.Body), `"price":12.50`) || !strings.Contains(string(res.Body), `"fetchedAt":"20`) {
		t.Fatalf("body = %s, want two-decimal price and RFC3339 fetchedAt", res.Body)
	}

	if res = e.do("DELETE", "/price-maps/"+pm.ID.String(), nil, admin); res.Status != 200 || !res.DataIsNull() {
		t.Fatalf("delete = %d %s", res.Status, res.Body)
	}
	if res = e.do("DELETE", "/price-maps/"+pm.ID.String(), nil, admin); res.Status != 200 {
		t.Fatalf("second delete = %d, want idempotent 200", res.Status)
	}
}

func TestPriceMapAndCompareErrorPaths(t *testing.T) {
	e := newEnv(t)
	for name, tc := range map[string]struct {
		method, path string
		body         any
		who          []testkit.ReqOption
		status       int
	}{
		"compare without couponId": {"GET", "/price-compare", nil, anon, 400},
		"compare bad couponId":     {"GET", "/price-compare?couponId=x", nil, anon, 400},
		"list without couponId":    {"GET", "/price-maps", nil, admin, 400},
		"list bad couponId":        {"GET", "/price-maps?couponId=0", nil, admin, 400},
		"delete bad id":            {"DELETE", "/price-maps/abc", nil, admin, 400},
		"upsert malformed":         {"POST", "/price-maps", `[`, admin, 400},
		"upsert negative price":    {"POST", "/price-maps", `{"couponId":"1","platform":"JD","externalSkuId":"a","manualPrice":-1}`, admin, 400},
		"upsert three decimals":    {"POST", "/price-maps", `{"couponId":"1","platform":"JD","externalSkuId":"a","manualPrice":1.234}`, admin, 400},
		"upsert bad currency":      {"POST", "/price-maps", `{"couponId":"1","platform":"JD","externalSkuId":"a","currency":"yuan"}`, admin, 400},
		"upsert price wrong type":  {"POST", "/price-maps", `{"couponId":"1","platform":"JD","externalSkuId":"a","manualPrice":"cheap"}`, admin, 400},
		"upsert unknown platform":  {"POST", "/price-maps", `{"couponId":"1","platform":"X","externalSkuId":"a"}`, admin, 400},
	} {
		if res := e.do(tc.method, tc.path, tc.body, tc.who); res.Status != tc.status || res.Code == 200 {
			t.Errorf("%s: status=%d code=%d msg=%q, want %d", name, res.Status, res.Code, res.Message, tc.status)
		}
	}
}

func TestSyncEndpoints(t *testing.T) {
	e := newEnv(t)
	var b domain.Binding
	e.do("POST", "/bindings", `{"platform":"MOCK","externalSkuId":"mock-sku:30","couponId":"1001"}`, admin).Decode(&b)

	res := e.do("POST", "/sync/"+b.ID.String()+"?force=true", nil, admin)
	if res.Status != 200 {
		t.Fatalf("sync one = %d %s", res.Status, res.Body)
	}
	var synced domain.Binding
	res.Decode(&synced)
	if synced.LastSyncStatus == nil || *synced.LastSyncStatus != domain.SyncSuccess || synced.LastStock == nil || *synced.LastStock != 30 {
		t.Fatalf("synced = %+v", synced)
	}
	if len(e.coupon.reqs) != 1 || e.coupon.reqs[0].Available != 30 || e.coupon.reqs[0].CouponID != 1001 {
		t.Fatalf("coupon requests = %+v", e.coupon.reqs)
	}

	res = e.do("POST", "/sync?force=true", nil, admin)
	if res.Status != 200 || !strings.Contains(string(res.Body), `"syncedOk":1`) || !strings.Contains(string(res.Body), `"force":true`) {
		t.Fatalf("sync all = %d %s", res.Status, res.Body)
	}
	if res = e.do("POST", "/sync", nil, admin); !strings.Contains(string(res.Body), `"force":false`) {
		t.Fatalf("sync all default force = %s", res.Body)
	}

	// 错误路径。
	if res = e.do("POST", "/sync/abc", nil, admin); res.Status != 400 {
		t.Errorf("bad id = %d", res.Status)
	}
	if res = e.do("POST", "/sync/"+b.ID.String()+"?force=maybe", nil, admin); res.Status != 400 {
		t.Errorf("bad force = %d, want 400", res.Status)
	}
	if res = e.do("POST", "/sync/424242", nil, admin); res.Status != 404 {
		t.Errorf("unknown binding = %d, want 404", res.Status)
	}
	e.mr.Set(lockKey, "scheduled-batch-running")
	for _, p := range []string{"/sync/" + b.ID.String(), "/sync"} {
		if res = e.do("POST", p, nil, admin); res.Status != 409 {
			t.Errorf("POST %s while lock held = %d %s, want 409", p, res.Status, res.Body)
		}
	}
	if v, _ := e.mr.Get(lockKey); v != "scheduled-batch-running" {
		t.Errorf("lock = %q: 被拒绝的手工同步不得动别人的锁", v)
	}
}

func TestHealthAndPlatforms(t *testing.T) {
	e := newEnv(t)

	var hs []domain.Health
	e.do("GET", "/health", nil, anon).Decode(&hs)
	if len(hs) != 4 || hs[0].Platform != domain.Mock || hs[0].Status != "UP" ||
		hs[2].Platform != domain.TB || hs[2].Status != "DISABLED" || hs[3].Status != "DOWN" {
		t.Fatalf("health = %+v", hs)
	}

	res := e.do("GET", "/platforms", nil, admin)
	var ps []map[string]any
	res.Decode(&ps)
	if len(ps) != 4 {
		t.Fatalf("platforms = %s", res.Body)
	}
	jd := ps[1]
	for k, want := range map[string]any{
		"platform": "JD", "status": "UP", "jdEnabled": true, "jdCredentialsConfigured": true,
		"jdServerUrl": "https://api.jd.example/routerjson", "jdDefaultArea": "1_72_2799_0", "jdAppKeyMasked": "ab****yz",
	} {
		if jd[k] != want {
			t.Errorf("jd[%s] = %v, want %v", k, jd[k], want)
		}
	}
	if _, has := ps[0]["jdEnabled"]; has {
		t.Error("non-JD platforms must not carry jd* fields")
	}
	if strings.Contains(string(res.Body), "secret") || strings.Contains(string(res.Body), "Secret") {
		t.Error("platforms must never expose secrets")
	}
}

func TestProbeEndpoints(t *testing.T) {
	e := newEnv(t)

	var stock domain.StockSnapshot
	res := e.do("GET", "/probe/jd/stock?skuId=%20123%20", nil, admin)
	if res.Status != 200 {
		t.Fatalf("probe stock = %d %s", res.Status, res.Body)
	}
	res.Decode(&stock)
	if stock.Platform != domain.JD || stock.ExternalSkuID != "123" || stock.StockQty == nil || *stock.StockQty != 77 {
		t.Fatalf("stock = %+v (lowercase platform and trimmed sku)", stock)
	}
	var prod domain.ProductSnapshot
	e.do("GET", "/probe/MOCK/product?skuId=s1", nil, admin).Decode(&prod)
	if prod.Title != "Mock商品-s1" || prod.Price == nil || prod.Price.String() != "99.00" {
		t.Fatalf("product = %+v", prod)
	}

	for name, tc := range map[string]struct {
		path   string
		status int
	}{
		"unknown platform": {"/probe/AMAZON/stock?skuId=1", 400},
		"missing skuId":    {"/probe/MOCK/stock", 400},
		"blank skuId":      {"/probe/MOCK/product?skuId=%20", 400},
		"disabled":         {"/probe/TB/stock?skuId=1", 503},
		"down":             {"/probe/PDD/product?skuId=1", 503},
	} {
		if res := e.do("GET", tc.path, nil, admin); res.Status != tc.status || res.Code == 200 {
			t.Errorf("%s: status=%d msg=%q, want %d", name, res.Status, res.Message, tc.status)
		}
	}
}

func TestUnknownRouteAndMethodReturnEnvelope(t *testing.T) {
	e := newEnv(t)
	if res := e.do("GET", "/nope", nil, admin); res.Status != 404 || res.Code == 0 {
		t.Errorf("unknown route = %d code=%d", res.Status, res.Code)
	}
	if res := e.do("PUT", "/bindings", nil, admin); res.Status != 404 && res.Status != 405 {
		t.Errorf("wrong method = %d", res.Status)
	}
}
