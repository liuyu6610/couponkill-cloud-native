package reservation

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

const base = "/api/v1/order/reservations"

// newRouter 构造与生产一致的路由：网关身份中间件 + 本包路由，并预置一张一小时后开售的秒杀券 1001。
func newRouter(t *testing.T) (*env, *httpx.Engine) {
	t.Helper()
	e := newEnv(t)
	e.coupons.put(seckillCoupon(1001, e.clock.Now(), time.Hour, time.Hour))
	e.coupons.put(seckillCoupon(1002, e.clock.Now(), time.Hour, time.Hour))
	eng := httpx.NewEngine(httpx.Options{Service: "order-test"})
	RegisterRoutes(eng.Group("", httpx.RequireGatewayIdentity(testkit.Signer(t))), e.svc)
	return e, eng
}

func idStr(id int64) string { return strconv.FormatInt(id, 10) }

func TestRoutesRequireIdentity(t *testing.T) {
	_, h := newRouter(t)
	for _, rt := range []struct {
		method, path string
		body         any
	}{
		{"POST", base, map[string]any{"couponId": "1001"}},
		{"DELETE", base + "/1", nil},
		{"GET", base + "/mine", nil},
		{"GET", base + "/1", nil},
	} {
		res := testkit.DoJSON(t, h, rt.method, rt.path, rt.body)
		if res.Status != http.StatusUnauthorized {
			t.Errorf("%s %s 无身份应 401, 得到 %d", rt.method, rt.path, res.Status)
		}
	}
}

func TestRoutesFailClosedWithoutIdentityMiddleware(t *testing.T) {
	e := newEnv(t)
	eng := httpx.NewEngine(httpx.Options{Service: "order-test"})
	RegisterRoutes(eng, e.svc) // 漏挂身份中间件
	res := testkit.DoJSON(t, eng, "GET", base+"/mine", nil, testkit.User(1, "user"))
	if res.Status != http.StatusUnauthorized {
		t.Fatalf("没有挂身份中间件时应拒绝而不是放行, 得到 %d", res.Status)
	}
}

func TestCreateRouteReturnsFrontendShapedReservation(t *testing.T) {
	e, h := newRouter(t)
	res := testkit.DoJSON(t, h, "POST", base, map[string]any{"couponId": "1001"}, testkit.User(7, "user"))
	if res.Status != http.StatusOK || res.Code != 200 {
		t.Fatalf("status=%d code=%d msg=%s", res.Status, res.Code, res.Message)
	}
	var got map[string]any
	res.Decode(&got)
	for _, key := range []string{"id", "userId", "couponId", "status", "reserveAt", "triggerAt", "firedAt",
		"requestId", "orderId", "failCode", "failReason", "retryCount", "createdAt", "updatedAt"} {
		if _, ok := got[key]; !ok {
			t.Errorf("响应缺少前端类型要求的字段 %q: %v", key, got)
		}
	}
	if got["userId"] != "7" || got["couponId"] != "1001" || got["status"] != "PENDING" || got["retryCount"] != float64(0) {
		t.Errorf("字段取值不符: %v", got)
	}
	if _, isStr := got["id"].(string); !isStr {
		t.Errorf("id 必须是字符串: %T", got["id"])
	}
	for _, key := range []string{"firedAt", "requestId", "orderId", "failCode", "failReason"} {
		if got[key] != nil {
			t.Errorf("新预约的 %s 应为 null: %v", key, got[key])
		}
	}
	trigger, err := time.Parse(time.RFC3339, got["triggerAt"].(string))
	if err != nil || !trigger.Equal(*e.coupons.items[1001].SeckillStartAt) {
		t.Errorf("triggerAt 应为 RFC3339 的开售时间: %v err=%v", got["triggerAt"], err)
	}
}

func TestCreateRouteAcceptsNumericCouponID(t *testing.T) {
	_, h := newRouter(t)
	res := testkit.DoJSON(t, h, "POST", base, `{"couponId":1002}`, testkit.User(7, "user"))
	if res.Status != http.StatusOK {
		t.Fatalf("数字形式的 couponId 也应被接受: %d %s", res.Status, res.Body)
	}
}

func TestCreateRouteErrorMapping(t *testing.T) {
	e, h := newRouter(t)
	user := testkit.User(7, "user")
	e.coupons.put(Coupon{ID: 2001, Type: CouponTypeNormal, Status: CouponStatusEnabled})

	cases := []struct {
		name         string
		body         any
		status, code int
	}{
		{"缺少 couponId", `{}`, 400, 10004},
		{"couponId 为 0", `{"couponId":"0"}`, 400, 10004},
		{"couponId 为负数", `{"couponId":"-5"}`, 400, 10004},
		{"couponId 不是数字", `{"couponId":"abc"}`, 400, 10004},
		{"非法 JSON", `{"couponId":`, 400, 10004},
		{"空请求体", nil, 400, 10004},
		{"券不存在", map[string]any{"couponId": "999"}, 404, 10008},
		{"不是秒杀券", map[string]any{"couponId": "2001"}, 400, 10004},
	}
	for _, tc := range cases {
		res := testkit.DoJSON(t, h, "POST", base, tc.body, user)
		if res.Status != tc.status || res.Code != tc.code || !res.DataIsNull() || res.RequestID == "" {
			t.Errorf("%s: 期望 %d/%d, 得到 %d/%d body=%s", tc.name, tc.status, tc.code, res.Status, res.Code, res.Body)
		}
	}

	// 重复预约 409/10010
	if res := testkit.DoJSON(t, h, "POST", base, map[string]any{"couponId": "1001"}, user); res.Status != 200 {
		t.Fatalf("首次创建应成功: %s", res.Body)
	}
	res := testkit.DoJSON(t, h, "POST", base, map[string]any{"couponId": "1001"}, user)
	if res.Status != http.StatusConflict || res.Code != 10010 {
		t.Errorf("重复预约应 409/10010, 得到 %d/%d", res.Status, res.Code)
	}
}

func TestCancelRoute(t *testing.T) {
	e, h := newRouter(t)
	owner, other := testkit.User(7, "user"), testkit.User(8, "user")
	var created Reservation
	testkit.DoJSON(t, h, "POST", base, map[string]any{"couponId": "1001"}, owner).Decode(&created)
	path := base + "/" + idStr(created.ID)

	// 别人取消：404；不存在：404；非数字：400
	if res := testkit.DoJSON(t, h, "DELETE", path, nil, other); res.Status != http.StatusNotFound {
		t.Errorf("他人取消应 404, 得到 %d", res.Status)
	}
	if res := testkit.DoJSON(t, h, "DELETE", base+"/424242", nil, owner); res.Status != http.StatusNotFound {
		t.Errorf("不存在应 404, 得到 %d", res.Status)
	}
	if res := testkit.DoJSON(t, h, "DELETE", base+"/abc", nil, owner); res.Status != http.StatusBadRequest {
		t.Errorf("非数字 id 应 400, 得到 %d", res.Status)
	}

	res := testkit.DoJSON(t, h, "DELETE", path, nil, owner)
	var ok bool
	res.Decode(&ok)
	if res.Status != 200 || !ok {
		t.Fatalf("属主取消应 200 且 data=true: %d %s", res.Status, res.Body)
	}
	if st := load(t, e.pool, created.ID).Status; st != StatusCancelled {
		t.Fatalf("状态应为 CANCELLED, 得到 %s", st)
	}
	// 终态再取消：409
	if res := testkit.DoJSON(t, h, "DELETE", path, nil, owner); res.Status != http.StatusConflict || res.Code != 10004 {
		t.Errorf("重复取消应 409/10004, 得到 %d/%d", res.Status, res.Code)
	}
}

func TestGetRoute(t *testing.T) {
	_, h := newRouter(t)
	owner, other := testkit.User(7, "user"), testkit.User(8, "user")
	var created Reservation
	testkit.DoJSON(t, h, "POST", base, map[string]any{"couponId": "1001"}, owner).Decode(&created)
	path := base + "/" + idStr(created.ID)

	res := testkit.DoJSON(t, h, "GET", path, nil, owner)
	var got Reservation
	res.Decode(&got)
	if res.Status != 200 || got.ID != created.ID || got.Status != StatusPending {
		t.Fatalf("属主读取: %d %+v", res.Status, got)
	}
	for name, r := range map[string]*testkit.Response{
		"他人的":  testkit.DoJSON(t, h, "GET", path, nil, other),
		"不存在的": testkit.DoJSON(t, h, "GET", base+"/424242", nil, owner),
	} {
		if r.Status != http.StatusNotFound || !r.DataIsNull() {
			t.Errorf("%s预约应 404 信封: %d %s", name, r.Status, r.Body)
		}
	}
	if res := testkit.DoJSON(t, h, "GET", base+"/abc", nil, owner); res.Status != http.StatusBadRequest {
		t.Errorf("非数字 id 应 400, 得到 %d", res.Status)
	}
}

type pageBody struct {
	Items []Reservation `json:"items"`
	Total int64         `json:"total"`
	Page  int           `json:"page"`
	Size  int           `json:"size"`
}

func TestMineRoutePagedNewestFirst(t *testing.T) {
	e, h := newRouter(t)
	owner := testkit.User(7, "user")
	var ids []int64
	for _, coupon := range []string{"1001", "1002"} {
		e.clock.Advance(time.Second)
		var r Reservation
		testkit.DoJSON(t, h, "POST", base, map[string]any{"couponId": coupon}, owner).Decode(&r)
		ids = append(ids, r.ID)
	}
	testkit.DoJSON(t, h, "POST", base, map[string]any{"couponId": "1001"}, testkit.User(8, "user"))

	var page pageBody
	res := testkit.DoJSON(t, h, "GET", base+"/mine", nil, owner)
	res.Decode(&page)
	if res.Status != 200 || page.Total != 2 || page.Page != 1 || page.Size != 100 || len(page.Items) != 2 ||
		page.Items[0].ID != ids[1] || page.Items[1].ID != ids[0] {
		t.Fatalf("默认分页/排序不符: %d %+v", res.Status, page)
	}
	page = pageBody{}
	testkit.DoJSON(t, h, "GET", base+"/mine?page=2&size=1", nil, owner).Decode(&page)
	if len(page.Items) != 1 || page.Items[0].ID != ids[0] || page.Page != 2 || page.Size != 1 {
		t.Fatalf("第 2 页应是最老一条: %+v", page)
	}
	// 没有预约的用户：items 是 []，不是 null
	res = testkit.DoJSON(t, h, "GET", base+"/mine", nil, testkit.User(99, "user"))
	var raw struct {
		Items json.RawMessage `json:"items"`
	}
	res.Decode(&raw)
	if string(raw.Items) != "[]" {
		t.Fatalf("空列表应输出 [], 得到 %s", raw.Items)
	}
	for _, q := range []string{"?size=101", "?page=0", "?size=0", "?page=x"} {
		if res := testkit.DoJSON(t, h, "GET", base+"/mine"+q, nil, owner); res.Status != 400 || res.Code != 10004 {
			t.Errorf("%s 应 400/10004, 得到 %d/%d", q, res.Status, res.Code)
		}
	}
}

func TestRouteSystemErrorDoesNotLeakInternals(t *testing.T) {
	e, h := newRouter(t)
	e.pool.Close() // 制造数据库故障
	res := testkit.DoJSON(t, h, "GET", base+"/mine", nil, testkit.User(7, "user"))
	if res.Status != http.StatusInternalServerError || res.Code != 50000 || res.Message != "系统错误" {
		t.Fatalf("数据库故障应 500/50000「系统错误」, 得到 %d/%d %q", res.Status, res.Code, res.Message)
	}
}

func TestHandlerAndSchedulerEndToEnd(t *testing.T) {
	e, h := newRouter(t)
	user := testkit.User(7, "user")
	var created Reservation
	testkit.DoJSON(t, h, "POST", base, map[string]any{"couponId": "1001"}, user).Decode(&created)

	sk := okSeckiller()
	s := newScheduler(t, e, sk)
	runOnce(t, s) // 未开售：不触发
	if sk.totalCalls() != 0 {
		t.Fatal("开售前不得触发")
	}
	e.clock.Advance(time.Hour + time.Second) // 到点
	runOnce(t, s)

	var got Reservation
	testkit.DoJSON(t, h, "GET", base+"/"+idStr(created.ID), nil, user).Decode(&got)
	if got.Status != StatusQueued || got.RequestID == nil {
		t.Fatalf("到点后接口应看到 QUEUED: %+v", got)
	}
	if err := e.svc.OnSeckillResult(context.Background(),
		ResultEvent{RequestID: *got.RequestID, UserID: 7, CouponID: 1001, Success: true, OrderID: 31415}); err != nil {
		t.Fatal(err)
	}
	testkit.DoJSON(t, h, "GET", base+"/"+idStr(created.ID), nil, user).Decode(&got)
	if got.Status != StatusSuccess || got.OrderID == nil || *got.OrderID != 31415 {
		t.Fatalf("结果事件后接口应看到 SUCCESS: %+v", got)
	}
	var raw map[string]any
	testkit.DoJSON(t, h, "GET", base+"/"+idStr(created.ID), nil, user).Decode(&raw)
	if raw["orderId"] != "31415" {
		t.Fatalf("orderId 应输出为字符串: %v", raw["orderId"])
	}
}
