package notification

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

const base = "/api/v1/order/notifications"

// newRouter 构造与生产一致的路由：网关身份中间件 + 本包路由。
func newRouter(t *testing.T) (http.Handler, *Service, *seqIDs, func(userID int64, n int) []int64) {
	t.Helper()
	svc, pool := newService(t)
	eng := httpx.NewEngine(httpx.Options{Service: "order-test"})
	RegisterRoutes(eng.Group("", httpx.RequireGatewayIdentity(testkit.Signer(t))), svc)
	ids := &seqIDs{}
	return eng, svc, ids, func(userID int64, n int) []int64 { return seed(t, pool, userID, ids, n) }
}

type pageBody struct {
	Items []map[string]any `json:"items"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Size  int              `json:"size"`
}

func TestRoutesRequireIdentity(t *testing.T) {
	h, _, _, _ := newRouter(t)
	for _, rt := range []struct{ method, path string }{
		{"GET", base + "/mine"},
		{"GET", base + "/unread-count"},
		{"POST", base + "/1/read"},
		{"POST", base + "/read-all"},
	} {
		res := testkit.DoJSON(t, h, rt.method, rt.path, nil)
		if res.Status != http.StatusUnauthorized {
			t.Errorf("%s %s 无身份应 401, 得到 %d", rt.method, rt.path, res.Status)
		}
	}
}

func TestRoutesFailClosedWithoutIdentityMiddleware(t *testing.T) {
	eng := httpx.NewEngine(httpx.Options{Service: "order-test"})
	svc, _ := newService(t)
	RegisterRoutes(eng, svc) // 漏挂身份中间件
	res := testkit.DoJSON(t, eng, "GET", base+"/unread-count", nil, testkit.User(1, "user"))
	if res.Status != http.StatusUnauthorized {
		t.Fatalf("没有挂身份中间件时应拒绝而不是放行, 得到 %d", res.Status)
	}
}

func TestMineReturnsPagedEnvelopeWithFrontendFieldNames(t *testing.T) {
	h, _, _, seedFor := newRouter(t)
	ids := seedFor(1, 3)
	seedFor(2, 2)

	res := testkit.DoJSON(t, h, "GET", base+"/mine?page=1&size=2", nil, testkit.User(1, "user"))
	if res.Status != http.StatusOK || res.Code != 200 {
		t.Fatalf("status=%d code=%d msg=%s", res.Status, res.Code, res.Message)
	}
	var page pageBody
	res.Decode(&page)
	if page.Total != 3 || page.Page != 1 || page.Size != 2 || len(page.Items) != 2 {
		t.Fatalf("分页信封不符: %+v", page)
	}
	first := page.Items[0]
	if first["id"] != strconv.FormatInt(ids[2], 10) {
		t.Fatalf("最新的应排第一, id=%v 期望 %d", first["id"], ids[2])
	}
	for _, key := range []string{"id", "userId", "type", "title", "content", "refId", "readFlag", "createdAt"} {
		if _, ok := first[key]; !ok {
			t.Errorf("响应缺少前端类型要求的字段 %q: %v", key, first)
		}
	}
	if first["userId"] != "1" || first["readFlag"] != false || first["refId"] != nil {
		t.Errorf("字段取值不符: %v", first)
	}
}

func TestMineDefaultsLegacyLimitAndValidation(t *testing.T) {
	h, _, _, seedFor := newRouter(t)
	seedFor(1, 5)
	user := testkit.User(1, "user")

	var page pageBody
	testkit.DoJSON(t, h, "GET", base+"/mine", nil, user).Decode(&page)
	if page.Page != 1 || page.Size != 20 || len(page.Items) != 5 {
		t.Fatalf("默认应为第 1 页每页 20: %+v", page)
	}
	page = pageBody{}
	testkit.DoJSON(t, h, "GET", base+"/mine?limit=2", nil, user).Decode(&page)
	if page.Size != 2 || len(page.Items) != 2 {
		t.Fatalf("旧参数 limit 应等价于 size: %+v", page)
	}
	for _, q := range []string{"?size=51", "?page=0", "?size=-1", "?page=abc"} {
		res := testkit.DoJSON(t, h, "GET", base+"/mine"+q, nil, user)
		if res.Status != http.StatusBadRequest || res.Code != 10004 {
			t.Errorf("%s 应 400/10004, 得到 %d/%d", q, res.Status, res.Code)
		}
	}
}

func TestUnreadCountEnvelope(t *testing.T) {
	h, _, _, seedFor := newRouter(t)
	seedFor(1, 3)
	res := testkit.DoJSON(t, h, "GET", base+"/unread-count", nil, testkit.User(1, "user"))
	var out map[string]int64
	res.Decode(&out)
	if res.Status != 200 || out["count"] != 3 {
		t.Fatalf("未读数 = %v (status %d)", out, res.Status)
	}
	res = testkit.DoJSON(t, h, "GET", base+"/unread-count", nil, testkit.User(99, "user"))
	res.Decode(&out)
	if out["count"] != 0 {
		t.Fatalf("无通知的用户未读数应为 0: %v", out)
	}
}

func TestMarkReadRoute(t *testing.T) {
	h, _, _, seedFor := newRouter(t)
	ids := seedFor(1, 2)
	other := seedFor(2, 1)
	user := testkit.User(1, "user")

	res := testkit.DoJSON(t, h, "POST", base+"/"+strconv.FormatInt(ids[0], 10)+"/read", nil, user)
	var ok bool
	res.Decode(&ok)
	if res.Status != 200 || !ok {
		t.Fatalf("标记已读应返回 data=true: status=%d body=%s", res.Status, res.Body)
	}
	// 幂等
	if res = testkit.DoJSON(t, h, "POST", base+"/"+strconv.FormatInt(ids[0], 10)+"/read", nil, user); res.Status != 200 {
		t.Fatalf("重复已读应 200, 得到 %d", res.Status)
	}
	// 别人的、不存在的：404 且信封 data 为 null
	for name, id := range map[string]int64{"别人的": other[0], "不存在的": 424242} {
		res = testkit.DoJSON(t, h, "POST", base+"/"+strconv.FormatInt(id, 10)+"/read", nil, user)
		if res.Status != http.StatusNotFound || !res.DataIsNull() || res.RequestID == "" {
			t.Errorf("%s通知应 404 信封: status=%d body=%s", name, res.Status, res.Body)
		}
	}
	// 非数字 ID
	if res = testkit.DoJSON(t, h, "POST", base+"/abc/read", nil, user); res.Status != http.StatusBadRequest {
		t.Errorf("非数字 id 应 400, 得到 %d", res.Status)
	}
}

func TestReadAllRoute(t *testing.T) {
	h, svc, _, seedFor := newRouter(t)
	seedFor(1, 3)
	seedFor(2, 2)
	user := testkit.User(1, "user")

	res := testkit.DoJSON(t, h, "POST", base+"/read-all", nil, user)
	var n int64
	res.Decode(&n)
	if res.Status != 200 || n != 3 {
		t.Fatalf("read-all 应返回 data=3: status=%d body=%s", res.Status, res.Body)
	}
	res = testkit.DoJSON(t, h, "POST", base+"/read-all", nil, user)
	res.Decode(&n)
	if n != 0 {
		t.Fatalf("二次 read-all 应返回 0, 得到 %d", n)
	}
	if left, _ := svc.UnreadCount(t.Context(), 2); left != 2 {
		t.Fatalf("用户 2 的未读不应受影响, 剩 %d", left)
	}
}

func TestServiceErrorsAreSystemErrorsWithoutLeaking(t *testing.T) {
	svc, pool := newService(t)
	eng := httpx.NewEngine(httpx.Options{Service: "order-test"})
	RegisterRoutes(eng.Group("", httpx.RequireGatewayIdentity(testkit.Signer(t))), svc)
	pool.Close() // 制造数据库故障

	res := testkit.DoJSON(t, eng, "GET", base+"/unread-count", nil, testkit.User(1, "user"))
	if res.Status != http.StatusInternalServerError || res.Code != 50000 {
		t.Fatalf("数据库故障应 500/50000, 得到 %d/%d", res.Status, res.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(res.Body, &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["message"]) != `"系统错误"` {
		t.Fatalf("响应体不应泄露内部错误: %s", res.Body)
	}
}
