package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

// 种子数据与匿名可读的目录：券 1001（秒杀）与 1002（常驻）必须在列表里并带库存。
func TestSeededLoginAndPublicCatalog(t *testing.T) {
	c := gw(t)
	demo := c.login(t, "demo", "demo1234")
	if demo.ID != "10000" {
		t.Fatalf("seeded demo user id = %s, want 10000", demo.ID)
	}

	r := c.do(t, "GET", "/api/v1/coupon/available", "", nil)
	if r.Status != 200 {
		t.Fatalf("anonymous available list: status=%d body=%s", r.Status, r.Raw)
	}
	var list []struct {
		ID        string `json:"id"`
		Type      int    `json:"type"`
		Remaining int    `json:"remainingStock"`
	}
	r.decode(t, &list)
	seen := map[string]int{}
	for _, it := range list {
		seen[it.ID] = it.Type
		if it.ID == "1001" || it.ID == "1002" {
			if it.Remaining <= 0 {
				t.Errorf("seed coupon %s has no remainingStock", it.ID)
			}
		}
	}
	if seen["1001"] != 2 || seen["1002"] != 1 {
		t.Fatalf("seed coupons missing or wrong type: %v", seen)
	}
}

// 最核心的不变式：库存 20、60 个用户并发抢 → 恰好 20 个受理、其余 409 售罄；
// 受理的请求全部落成订单且每人一单；数据库剩余库存恰好为 0（绝不超卖）；重复抢同一张券被拒。
func TestSeckillNeverOversellsUnderConcurrency(t *testing.T) {
	c := gw(t)
	const stock, nUsers = 20, 60
	coupon := c.newSeckill(t, stock, -time.Minute, time.Hour)
	users := c.users(t, "oversell", nUsers)

	results := make([]resp, nUsers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = c.seckill(t, users[i], coupon)
		}()
	}
	close(start)
	wg.Wait()

	var okUsers []user
	rejected := 0
	for i, r := range results {
		switch r.Status {
		case http.StatusAccepted:
			okUsers = append(okUsers, users[i])
		case http.StatusConflict:
			if r.Code != 10002 {
				t.Fatalf("user %d rejected with code %d, want 10002 (sold out): %s", i, r.Code, r.Raw)
			}
			rejected++
		default:
			t.Fatalf("user %d: unexpected status %d body=%s", i, r.Status, r.Raw)
		}
	}
	if len(okUsers) != stock || rejected != nUsers-stock {
		t.Fatalf("accepted=%d rejected=%d, want exactly %d accepted and %d sold-out", len(okUsers), rejected, stock, nUsers-stock)
	}

	for i, r := range results {
		if r.Status != http.StatusAccepted {
			continue
		}
		u := users[i]
		res := c.waitResult(t, u, accepted(t, r), 45*time.Second)
		if res.Status != "SUCCESS" || res.OrderID == "" {
			t.Fatalf("accepted request of %s ended as %+v, want SUCCESS with an orderId", u.Name, res)
		}
	}

	if got := c.remaining(t, coupon); got != 0 {
		t.Fatalf("remainingStock = %d after all %d accepted requests succeeded, want 0 (no oversell, no leak)", got, stock)
	}
	for _, u := range okUsers {
		orders := c.myOrders(t, u)
		n := 0
		for _, o := range orders {
			if o["couponId"] == coupon {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("%s has %d orders of the coupon, want exactly 1", u.Name, n)
		}
	}
	for _, u := range users {
		isOK := false
		for _, o := range okUsers {
			if o.ID == u.ID {
				isOK = true
			}
		}
		if !isOK && len(c.myOrders(t, u)) != 0 {
			t.Fatalf("a rejected user %s unexpectedly has orders", u.Name)
		}
	}

	// 成功的用户收到一条站内通知；重复抢同一张券被拒
	eventually(t, 10*time.Second, "success notification", func() bool { return c.unread(t, okUsers[0]) >= 1 })
	if r := c.seckill(t, okUsers[0], coupon); r.Status != http.StatusConflict {
		t.Fatalf("a second seckill by the same user: status=%d body=%s, want 409", r.Status, r.Raw)
	}
}

// 取消订单归还库存且幂等；取消后别人能抢到，重复取消不会重复归还。
func TestCancelRestoresStockOnceAndFreesTheCoupon(t *testing.T) {
	c := gw(t)
	coupon := c.newSeckill(t, 1, -time.Minute, time.Hour)
	us := c.users(t, "cancel", 2)
	a, b := us[0], us[1]

	res := c.waitResult(t, a, accepted(t, c.seckill(t, a, coupon)), 30*time.Second)
	if res.Status != "SUCCESS" {
		t.Fatalf("first user: %+v", res)
	}
	if r := c.seckill(t, b, coupon); r.Status != http.StatusConflict || r.Code != 10002 {
		t.Fatalf("second user must be sold out: status=%d body=%s", r.Status, r.Raw)
	}

	if r := c.do(t, "POST", "/api/v1/order/cancel", a.Token, map[string]string{"orderId": res.OrderID}); r.Status != 200 {
		t.Fatalf("cancel: status=%d body=%s", r.Status, r.Raw)
	}
	eventually(t, 10*time.Second, "stock restored after cancel", func() bool { return c.remaining(t, coupon) == 1 })

	// B 现在能抢到
	resB := c.waitResult(t, b, accepted(t, c.seckill(t, b, coupon)), 30*time.Second)
	if resB.Status != "SUCCESS" {
		t.Fatalf("second user after the cancel: %+v", resB)
	}
	if got := c.remaining(t, coupon); got != 0 {
		t.Fatalf("remaining = %d, want 0 after B took the restored unit", got)
	}

	// 重复取消既不报错也不能再次归还库存（否则就凭空多出库存）
	if r := c.do(t, "POST", "/api/v1/order/cancel", a.Token, map[string]string{"orderId": res.OrderID}); r.Status != 200 {
		t.Fatalf("second cancel must be harmless: status=%d body=%s", r.Status, r.Raw)
	}
	time.Sleep(500 * time.Millisecond)
	if got := c.remaining(t, coupon); got != 0 {
		t.Fatalf("remaining = %d after a repeated cancel, want 0 (stock must be restored exactly once)", got)
	}
	// A 取消之后可以再次参与（这里库存已被 B 占用，所以是售罄而不是「重复秒杀」）
	if r := c.seckill(t, a, coupon); r.Status != http.StatusConflict || r.Code != 10002 {
		t.Fatalf("A after cancelling: status=%d body=%s, want sold out (not 'repeat')", r.Status, r.Raw)
	}
}

// 预约帮抢：开售前预约 → 开售前手动抢被拒（未开始）→ 到点系统自动代抢 → 预约成功、产生订单。
func TestReservationFiresWhenTheWindowOpens(t *testing.T) {
	c := gw(t)
	coupon := c.newSeckill(t, 5, 5*time.Second, time.Hour)
	u := c.users(t, "resv", 1)[0]

	if r := c.seckill(t, u, coupon); r.Status != http.StatusConflict || r.Code != 10006 {
		t.Fatalf("seckill before the window opens: status=%d body=%s, want 409 / 10006", r.Status, r.Raw)
	}
	if r := c.do(t, "POST", "/api/v1/order/reservations", u.Token, map[string]string{"couponId": coupon}); r.Status != 200 {
		t.Fatalf("create reservation: status=%d body=%s", r.Status, r.Raw)
	}
	status := func() string {
		r := c.do(t, "GET", "/api/v1/order/reservations/mine", u.Token, nil)
		if r.Status != 200 {
			t.Fatalf("my reservations: status=%d body=%s", r.Status, r.Raw)
		}
		var p struct {
			Items []struct {
				CouponID string `json:"couponId"`
				Status   string `json:"status"`
			} `json:"items"`
		}
		r.decode(t, &p)
		for _, it := range p.Items {
			if it.CouponID == coupon {
				return it.Status
			}
		}
		return "MISSING"
	}
	if got := status(); got != "PENDING" {
		t.Fatalf("a reservation made before the window must be PENDING, got %s", got)
	}

	eventually(t, 60*time.Second, "reservation to reach SUCCESS", func() bool {
		s := status()
		if s == "FAILED" || s == "EXPIRED" || s == "CANCELLED" {
			t.Fatalf("reservation ended as %s", s)
		}
		return s == "SUCCESS"
	})
	orders := c.myOrders(t, u)
	if len(orders) != 1 || orders[0]["couponId"] != coupon {
		t.Fatalf("expected exactly one order from the reservation, got %v", orders)
	}
	if got := c.remaining(t, coupon); got != 4 {
		t.Fatalf("remaining = %d, want 4 after one reserved purchase out of 5", got)
	}
}

// 连接器同步的安全性：mock 平台恒返回 500，自动/普通同步绝不能把库存抬高（否则超卖），
// 只有管理员显式 force 才能设置为平台读数。
func TestConnectorSyncNeverRaisesStockUnlessForced(t *testing.T) {
	c := gw(t)
	adm := adminUser(t)
	coupon := c.newSeckill(t, 10, -time.Minute, time.Hour)

	r := c.do(t, "POST", "/api/v1/connector/bindings", adm.Token, map[string]any{
		"platform": "MOCK", "externalSkuId": "sku-" + coupon, "couponId": coupon, "syncEnabled": true,
	})
	if r.Status != 200 {
		t.Fatalf("create binding: status=%d body=%s", r.Status, r.Raw)
	}
	var b struct {
		ID string `json:"id"`
	}
	r.decode(t, &b)
	if b.ID == "" {
		t.Fatalf("binding without id: %s", r.Raw)
	}

	if r := c.do(t, "POST", "/api/v1/connector/sync/"+b.ID, adm.Token, nil); r.Status != 200 {
		t.Fatalf("sync: status=%d body=%s", r.Status, r.Raw)
	}
	if got := c.remaining(t, coupon); got != 10 {
		t.Fatalf("remaining = %d after a normal sync from a platform reporting 500, want 10 (min mode must never raise)", got)
	}
	if r := c.do(t, "POST", "/api/v1/connector/sync/"+b.ID+"?force=true", adm.Token, nil); r.Status != 200 {
		t.Fatalf("forced sync: status=%d body=%s", r.Status, r.Raw)
	}
	if got := c.remaining(t, coupon); got != 500 {
		t.Fatalf("remaining = %d after a forced sync, want the platform value 500", got)
	}

	// C 端只读的绑定查询匿名可用；管理接口普通用户不可用
	if r := c.do(t, "GET", "/api/v1/connector/bindings/by-coupon/"+coupon, "", nil); r.Status != 200 {
		t.Fatalf("anonymous by-coupon read: status=%d body=%s", r.Status, r.Raw)
	}
	u := c.users(t, "cnx", 1)[0]
	if r := c.do(t, "POST", "/api/v1/connector/bindings", u.Token, map[string]any{
		"platform": "MOCK", "externalSkuId": "x", "couponId": coupon,
	}); r.Status != http.StatusForbidden {
		t.Fatalf("a normal user creating a binding: status=%d body=%s, want 403", r.Status, r.Raw)
	}
}

// 整条链路上的权限边界。
func TestAuthBoundariesAcrossTheWholeStack(t *testing.T) {
	c := gw(t)
	u := c.users(t, "auth", 1)[0]

	if r := c.do(t, "POST", "/api/v1/order/seckill", "", map[string]string{"couponId": "1001"}); r.Status != http.StatusUnauthorized {
		t.Fatalf("anonymous seckill: status=%d, want 401", r.Status)
	}
	if r := c.do(t, "POST", "/api/v1/coupon/create", u.Token, map[string]any{"name": "x"}); r.Status != http.StatusForbidden {
		t.Fatalf("a normal user creating a coupon: status=%d body=%s, want 403", r.Status, r.Raw)
	}
	if r := c.do(t, "POST", "/internal/v1/stock/deduct", "", map[string]string{"requestId": "x"}); r.Status != http.StatusNotFound {
		t.Fatalf("/internal through the gateway: status=%d, want 404", r.Status)
	}
	if r := c.do(t, "POST", "/api/v1/auth/token/mock", "", map[string]any{"userId": 1, "roles": []string{"admin"}}); r.Status == http.StatusOK {
		t.Fatalf("the mock-token endpoint must not exist, got 200: %s", r.Raw)
	}
	// 伪造的身份头被网关剥掉：以普通用户身份带上管理员头，仍然不是管理员
	if r := c.do(t, "POST", "/api/v1/coupon/create", u.Token, map[string]any{"name": "x"},
		"X-User-Roles", "admin", "X-User-Id", "10001"); r.Status != http.StatusForbidden {
		t.Fatalf("forged admin headers: status=%d, want 403", r.Status)
	}
	// 绕过网关直连服务：没有网关签名的身份一律 401
	if direct := os.Getenv("CK_E2E_DIRECT_ORDER_URL"); direct != "" {
		dc := &client{base: direct, http: http.DefaultClient}
		if r := dc.do(t, "GET", "/api/v1/order/user/me", "", nil, "X-User-Id", u.ID, "X-User-Roles", "user"); r.Status != http.StatusUnauthorized {
			t.Fatalf("direct call to order with a forged unsigned identity: status=%d, want 401", r.Status)
		}
		if r := dc.do(t, "POST", "/internal/v1/orders/grant", "", map[string]string{"requestId": "x"}); r.Status != http.StatusUnauthorized {
			t.Fatalf("direct internal call without the token: status=%d, want 401", r.Status)
		}
	}

	// 登出之后旧令牌立刻失效（会话按令牌吊销）
	if r := c.do(t, "GET", "/api/v1/user/profile", u.Token, nil); r.Status != 200 {
		t.Fatalf("profile before logout: status=%d", r.Status)
	}
	if r := c.do(t, "POST", "/api/v1/user/logout", u.Token, nil); r.Status != 200 {
		t.Fatalf("logout: status=%d body=%s", r.Status, r.Raw)
	}
	if r := c.do(t, "GET", "/api/v1/user/profile", u.Token, nil); r.Status != http.StatusUnauthorized {
		t.Fatalf("profile after logout: status=%d, want 401", r.Status)
	}
}

// 错误响应的形状：真实 HTTP 状态 + 信封 + requestId，且不泄露内部细节。
func TestErrorEnvelopeShape(t *testing.T) {
	c := gw(t)
	r := c.do(t, "POST", "/api/v1/user/login", "", map[string]string{"username": "demo", "password": "wrong"})
	if r.Status != http.StatusUnauthorized || r.Code != 20005 || r.RequestID == "" || string(r.Data) != "null" {
		t.Fatalf("bad login: status=%d code=%d requestId=%q data=%s", r.Status, r.Code, r.RequestID, r.Data)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.Raw), &raw); err != nil {
		t.Fatalf("error body is not JSON: %v", err)
	}
	for _, k := range []string{"code", "message", "data", "requestId"} {
		if _, ok := raw[k]; !ok {
			t.Fatalf("error envelope lacks %q: %s", k, r.Raw)
		}
	}
}

// 前端用到的接口都能响应，关键字段齐全（前端 src/types/api.ts 与 src/services/contract.test.ts 假定的形状）。
func TestEndpointsUsedByTheFrontendRespondWithTheExpectedFields(t *testing.T) {
	c := gw(t)
	demo := c.login(t, "demo", "demo1234")
	adm := adminUser(t)

	hasKeys := func(what string, raw json.RawMessage, keys ...string) {
		t.Helper()
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("%s: data is not an object: %s", what, raw)
		}
		for _, k := range keys {
			if _, ok := m[k]; !ok {
				t.Errorf("%s lacks field %q: %s", what, k, raw)
			}
		}
	}
	get := func(path, token string) resp {
		t.Helper()
		r := c.do(t, "GET", path, token, nil)
		if r.Status != 200 {
			t.Fatalf("GET %s: status=%d body=%s", path, r.Status, r.Raw)
		}
		return r
	}

	hasKeys("profile", get("/api/v1/user/profile", demo.Token).Data, "id", "username", "createdAt")
	hasKeys("coupon 1001", get("/api/v1/coupon/1001", "").Data,
		"id", "name", "type", "faceValue", "minSpend", "validDays", "perUserLimit", "totalStock", "remainingStock", "status", "createdAt", "updatedAt")
	hasKeys("coupon list", get("/api/v1/coupon/list?page=1&size=10", "").Data, "items", "total")
	hasKeys("order list", get("/api/v1/order/user/me?page=1&size=10", demo.Token).Data, "items", "total")
	hasKeys("reservations", get("/api/v1/order/reservations/mine", demo.Token).Data, "items")
	hasKeys("notifications", get("/api/v1/order/notifications/mine?limit=20", demo.Token).Data, "items")
	hasKeys("unread count", get("/api/v1/order/notifications/unread-count", demo.Token).Data, "count")
	get("/api/v1/user/coupon/count", demo.Token)
	get("/api/v1/connector/health", "")
	get("/api/v1/connector/platforms", adm.Token)
	get("/api/v1/connector/bindings", adm.Token)

	// 前端从不带身份读这几个接口之外的东西：其余接口匿名一律 401
	for _, path := range []string{"/api/v1/user/profile", "/api/v1/order/user/me", "/api/v1/order/notifications/mine", "/api/v1/connector/bindings"} {
		if r := c.do(t, "GET", path, "", nil); r.Status != http.StatusUnauthorized {
			t.Errorf("anonymous GET %s: status=%d, want 401", path, r.Status)
		}
	}
}
