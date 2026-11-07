package seckill_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/seckill"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

func (e *env) engine() *httpx.Engine {
	eng := httpx.NewEngine(httpx.Options{Service: "order-test"})
	seckill.NewHandler(e.svc).Register(eng.Group("/api/v1/order", httpx.RequireGatewayIdentity(testkit.Signer(e.t))))
	return eng
}

func TestSeckillEndpointAcceptsWithHTTP202AndPollsToSuccess(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	eng := e.engine()

	res := testkit.DoJSON(t, eng, "POST", "/api/v1/order/seckill", map[string]string{"couponId": strconv.FormatInt(couponID, 10)}, testkit.User(42, "user"))
	if res.Status != http.StatusAccepted || res.Code != 200 {
		t.Fatalf("seckill = %d code %d (%s)", res.Status, res.Code, res.Body)
	}
	var acc events.SeckillAccepted
	res.Decode(&acc)
	if acc.Status != events.StatusQueued || acc.RequestID == "" {
		t.Fatalf("accepted = %+v", acc)
	}

	res = testkit.DoJSON(t, eng, "GET", "/api/v1/order/seckill/result?requestId="+acc.RequestID, nil, testkit.User(42, "user"))
	var r events.SeckillResult
	res.Decode(&r)
	if res.Status != http.StatusOK || r.Status != events.StatusQueued {
		t.Fatalf("result before consumption = %d %+v", res.Status, r)
	}

	e.consumeAll()
	res = testkit.DoJSON(t, eng, "GET", "/api/v1/order/seckill/result?requestId="+acc.RequestID, nil, testkit.User(42, "user"))
	r = events.SeckillResult{}
	res.Decode(&r)
	order := e.mustOrder(42, acc.RequestID)
	if r.Status != events.StatusSuccess || r.OrderID != order.ID {
		t.Fatalf("result after consumption = %+v, want SUCCESS with order %d", r, order.ID)
	}
	// 订单号以字符串编码（snowflake 超过 2^53）。
	if want := `"orderId":"` + strconv.FormatInt(order.ID, 10) + `"`; !strings.Contains(string(res.Body), want) {
		t.Fatalf("body %s does not carry %s", res.Body, want)
	}
}

func TestSeckillEndpointAcceptsNumericAndQueryCouponIdsAndRejectsBadOnes(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 5)
	eng := e.engine()
	user := testkit.User(42, "user")

	if res := testkit.DoJSON(t, eng, "POST", "/api/v1/order/seckill", `{"couponId":1001}`, user); res.Status != http.StatusAccepted {
		t.Fatalf("numeric couponId = %d (%s)", res.Status, res.Body)
	}
	if res := testkit.DoJSON(t, eng, "POST", "/api/v1/order/seckill?couponId=1001", nil, testkit.User(43, "user")); res.Status != http.StatusAccepted {
		t.Fatalf("query couponId = %d (%s)", res.Status, res.Body)
	}
	for name, body := range map[string]string{"missing": `{}`, "not a number": `{"couponId":"abc"}`, "zero": `{"couponId":"0"}`, "broken json": `{`} {
		res := testkit.DoJSON(t, eng, "POST", "/api/v1/order/seckill", body, testkit.User(44, "user"))
		if res.Status != http.StatusBadRequest || res.Code != apperr.CodeInvalidRequest {
			t.Fatalf("%s: %d code %d (%s), want 400 invalid request", name, res.Status, res.Code, res.Body)
		}
	}
	if res := testkit.DoJSON(t, eng, "POST", "/api/v1/order/seckill", nil, testkit.User(44, "user")); res.Status != http.StatusBadRequest {
		t.Fatalf("no couponId anywhere = %d", res.Status)
	}
}

func TestSeckillEndpointMapsBusinessErrorsToRealStatusCodes(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 1)
	eng := e.engine()
	body := map[string]string{"couponId": "1001"}

	if res := testkit.DoJSON(t, eng, "POST", "/api/v1/order/seckill", body, testkit.User(1, "user")); res.Status != http.StatusAccepted {
		t.Fatalf("first = %d", res.Status)
	}
	res := testkit.DoJSON(t, eng, "POST", "/api/v1/order/seckill", body, testkit.User(1, "user"))
	if res.Status != http.StatusConflict || res.Code != apperr.CodeRepeatSeckill || !res.DataIsNull() {
		t.Fatalf("repeat = %d code %d", res.Status, res.Code)
	}
	res = testkit.DoJSON(t, eng, "POST", "/api/v1/order/seckill", body, testkit.User(2, "user"))
	if res.Status != http.StatusConflict || res.Code != apperr.CodeOutOfStock {
		t.Fatalf("sold out = %d code %d", res.Status, res.Code)
	}
	res = testkit.DoJSON(t, eng, "POST", "/api/v1/order/seckill", map[string]string{"couponId": "5555"}, testkit.User(3, "user"))
	if res.Status != http.StatusNotFound || res.Code != apperr.CodeCouponNotFound {
		t.Fatalf("unknown coupon = %d code %d", res.Status, res.Code)
	}
	e.broker.FailPublishes(1)
	e.seed(1002, 3)
	res = testkit.DoJSON(t, eng, "POST", "/api/v1/order/seckill", map[string]string{"couponId": "1002"}, testkit.User(4, "user"))
	if res.Status != http.StatusServiceUnavailable || res.Code != apperr.CodeSystemBusy {
		t.Fatalf("enqueue failure = %d code %d", res.Status, res.Code)
	}
}

func TestSeckillEndpointsRequireAGatewayIdentity(t *testing.T) {
	e := newEnv(t)
	eng := e.engine()
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/v1/order/seckill"}, {"GET", "/api/v1/order/seckill/result?requestId=abc"},
	} {
		if res := testkit.DoJSON(t, eng, tc.method, tc.path, nil); res.Status != http.StatusUnauthorized {
			t.Fatalf("%s %s without identity = %d, want 401", tc.method, tc.path, res.Status)
		}
		forged := testkit.DoJSON(t, eng, tc.method, tc.path, nil, testkit.Header("X-User-Id", "42"))
		if forged.Status != http.StatusUnauthorized {
			t.Fatalf("%s %s with a forged X-User-Id = %d, want 401", tc.method, tc.path, forged.Status)
		}
	}
}

// 去掉属主校验，拿到他人 requestId 就能看到别人的订单号；「不存在」与「不是你的」必须无法区分。
func TestSeckillResultIsOnlyVisibleToTheOwner(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	eng := e.engine()
	rid := e.enter(42, couponID)
	e.consumeAll()

	owner := testkit.DoJSON(t, eng, "GET", "/api/v1/order/seckill/result?requestId="+rid, nil, testkit.User(42, "user"))
	if owner.Status != http.StatusOK {
		t.Fatalf("owner = %d", owner.Status)
	}
	other := testkit.DoJSON(t, eng, "GET", "/api/v1/order/seckill/result?requestId="+rid, nil, testkit.User(43, "user"))
	admin := testkit.DoJSON(t, eng, "GET", "/api/v1/order/seckill/result?requestId="+rid, nil, testkit.User(1, "admin"))
	unknown := testkit.DoJSON(t, eng, "GET", "/api/v1/order/seckill/result?requestId=does-not-exist", nil, testkit.User(43, "user"))
	for name, res := range map[string]*testkit.Response{"other user": other, "admin": admin, "unknown request": unknown} {
		if res.Status != http.StatusNotFound || res.Code != apperr.CodeOrderNotFound || !res.DataIsNull() {
			t.Fatalf("%s = %d code %d (%s), want an indistinguishable 404", name, res.Status, res.Code, res.Body)
		}
	}
	if other.Message != unknown.Message {
		t.Fatalf("messages differ (%q vs %q): that leaks whether the request exists", other.Message, unknown.Message)
	}

	if res := testkit.DoJSON(t, eng, "GET", "/api/v1/order/seckill/result", nil, testkit.User(42, "user")); res.Status != http.StatusBadRequest {
		t.Fatalf("missing requestId = %d", res.Status)
	}
	if res := testkit.DoJSON(t, eng, "GET", "/api/v1/order/seckill/result?requestId=bad%20id%7Cx", nil, testkit.User(42, "user")); res.Status != http.StatusBadRequest {
		t.Fatalf("malformed requestId = %d, want 400", res.Status)
	}
}

func TestSeckillResultReportsFailureReasons(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 0)
	e.mr.Set("ck:stock:1001", "1")
	eng := e.engine()
	rid := e.enter(42, couponID)
	e.consumeAll()

	res := testkit.DoJSON(t, eng, "GET", "/api/v1/order/seckill/result?requestId="+rid, nil, testkit.User(42, "user"))
	var r events.SeckillResult
	res.Decode(&r)
	if r.Status != events.StatusFailed || r.Reason != events.ReasonOutOfStock || r.OrderID != 0 {
		t.Fatalf("result = %+v, body %s", r, res.Body)
	}
}
