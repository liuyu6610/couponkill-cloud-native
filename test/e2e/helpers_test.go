// Package e2e 是对整套系统的端到端测试：真实的 gateway、user、coupon、order、connector 进程，
// 真实的 PostgreSQL，Redis 与 Kafka 可以是真实的也可以是 cmd/devinfra 的替身。
//
// 没有设置 CK_E2E_BASE_URL（指向网关，例如 http://127.0.0.1:8088）时整个包跳过；
// CK_REQUIRE_E2E=true 时改为失败，CI 必须这样设置，否则跳过会让流水线虚假地绿。
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// envelope 是所有响应的统一信封。
type envelope struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data"`
	RequestID string          `json:"requestId"`
}

// resp 是一次 HTTP 调用的结果：HTTP 状态 + 信封。
type resp struct {
	Status int
	envelope
	Raw string
}

// decode 把 data 解到 v，失败即终止测试。
func (r resp) decode(t testing.TB, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Data, v); err != nil {
		t.Fatalf("decode data %s: %v (status=%d body=%s)", r.Data, err, r.Status, r.Raw)
	}
}

type client struct {
	base string
	http *http.Client
}

var (
	baseOnce sync.Once
	cli      *client
)

// gw 返回指向网关的客户端；没有配置时跳过（或在 CK_REQUIRE_E2E=true 时失败）。
func gw(t testing.TB) *client {
	t.Helper()
	baseOnce.Do(func() {
		if base := strings.TrimRight(os.Getenv("CK_E2E_BASE_URL"), "/"); base != "" {
			cli = &client{base: base, http: &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 256}}}
		}
	})
	if cli == nil {
		if os.Getenv("CK_REQUIRE_E2E") == "true" {
			t.Fatal("CK_REQUIRE_E2E=true 但没有设置 CK_E2E_BASE_URL")
		}
		t.Skip("设置 CK_E2E_BASE_URL（网关地址）后才运行端到端测试")
	}
	return cli
}

// do 发一次 JSON 请求。token 为空表示匿名；body 为 nil 表示无请求体。
func (c *client) do(t testing.TB, method, path, token string, body any, headers ...string) resp {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.base+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	out := resp{Status: res.StatusCode, Raw: string(raw)}
	_ = json.Unmarshal(raw, &out.envelope)
	return out
}

// user 是一个已登录的测试用户。
type user struct {
	Name, ID, Token string
}

func (c *client) login(t testing.TB, name, password string) user {
	t.Helper()
	r := c.do(t, "POST", "/api/v1/user/login", "", map[string]string{"username": name, "password": password})
	if r.Status != 200 {
		t.Fatalf("login %s: status=%d body=%s", name, r.Status, r.Raw)
	}
	var d struct {
		Token  string `json:"token"`
		UserID string `json:"userId"`
	}
	r.decode(t, &d)
	if d.Token == "" || d.UserID == "" {
		t.Fatalf("login %s: empty token or userId: %s", name, r.Raw)
	}
	return user{Name: name, ID: d.UserID, Token: d.Token}
}

var (
	adminOnce sync.Once
	admin     user
)

// adminUser 返回种子管理员（admin/admin1234，仅 CK_SEED_DEMO=true 的开发/测试环境存在）。
func adminUser(t testing.TB) user {
	t.Helper()
	c := gw(t)
	adminOnce.Do(func() { admin = c.login(t, "admin", "admin1234") })
	if admin.Token == "" {
		t.Fatal("admin login failed in an earlier test")
	}
	return admin
}

const testPassword = "E2ePassw0rd!x"

var prefixSeq int

var prefixMu sync.Mutex

// uniquePrefix 返回不会与历史数据冲突的用户名前缀。
func uniquePrefix(tag string) string {
	prefixMu.Lock()
	defer prefixMu.Unlock()
	prefixSeq++
	return fmt.Sprintf("e2e%s%d%d", tag, time.Now().UnixNano()%1_000_000, prefixSeq)
}

// users 用管理员批量造用户并并发登录。
func (c *client) users(t testing.TB, tag string, n int) []user {
	t.Helper()
	adm := adminUser(t)
	prefix := uniquePrefix(tag)
	r := c.do(t, "POST", "/api/v1/user/batch/generate", adm.Token,
		map[string]any{"prefix": prefix, "start": 1, "count": n, "password": testPassword})
	if r.Status != 200 {
		t.Fatalf("batch generate: status=%d body=%s", r.Status, r.Raw)
	}
	out := make([]user, n)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for i := range n {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = c.login(t, prefix+strconv.Itoa(i+1), testPassword)
		}()
	}
	wg.Wait()
	return out
}

// newCoupon 用管理员创建秒杀券（type=2），时间窗相对当前时间。
func (c *client) newSeckill(t testing.TB, stock int, start, end time.Duration) string {
	t.Helper()
	now := time.Now()
	return c.createCoupon(t, map[string]any{
		"name": fmt.Sprintf("e2e秒杀券-%d", now.UnixNano()), "description": "e2e", "type": 2,
		"faceValue": 10.00, "minSpend": 0.00, "validDays": 7, "perUserLimit": 1, "totalStock": stock,
		"seckillStartAt": now.Add(start).Format(time.RFC3339Nano), "seckillEndAt": now.Add(end).Format(time.RFC3339Nano),
	})
}

func (c *client) createCoupon(t testing.TB, body map[string]any) string {
	t.Helper()
	r := c.do(t, "POST", "/api/v1/coupon/create", adminUser(t).Token, body)
	if r.Status != 200 {
		t.Fatalf("create coupon: status=%d body=%s", r.Status, r.Raw)
	}
	var d struct {
		ID string `json:"id"`
	}
	r.decode(t, &d)
	if d.ID == "" {
		t.Fatalf("create coupon returned no id: %s", r.Raw)
	}
	return d.ID
}

// remaining 读券的剩余库存（各库存桶之和）。
func (c *client) remaining(t testing.TB, couponID string) int {
	t.Helper()
	r := c.do(t, "GET", "/api/v1/coupon/"+couponID, adminUser(t).Token, nil)
	if r.Status != 200 {
		t.Fatalf("get coupon %s: status=%d body=%s", couponID, r.Status, r.Raw)
	}
	var d struct {
		Remaining int `json:"remainingStock"`
	}
	r.decode(t, &d)
	return d.Remaining
}

// seckill 发起一次秒杀，返回原始结果（调用方判断 202 / 409 ...）。
func (c *client) seckill(t testing.TB, u user, couponID string) resp {
	t.Helper()
	return c.do(t, "POST", "/api/v1/order/seckill", u.Token, map[string]string{"couponId": couponID})
}

// accepted 取出 202 响应里的 requestId。
func accepted(t testing.TB, r resp) string {
	t.Helper()
	if r.Status != http.StatusAccepted {
		t.Fatalf("seckill not accepted: status=%d body=%s", r.Status, r.Raw)
	}
	var d struct {
		RequestID string `json:"requestId"`
		Status    string `json:"status"`
	}
	r.decode(t, &d)
	if d.RequestID == "" || d.Status != "QUEUED" {
		t.Fatalf("unexpected accepted payload: %s", r.Raw)
	}
	return d.RequestID
}

type seckillResult struct {
	Status  string `json:"status"`
	OrderID string `json:"orderId"`
	Reason  string `json:"reason"`
}

// waitResult 轮询秒杀结果直到终态（SUCCESS/FAILED）。
func (c *client) waitResult(t testing.TB, u user, requestID string, timeout time.Duration) seckillResult {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		r := c.do(t, "GET", "/api/v1/order/seckill/result?requestId="+requestID, u.Token, nil)
		if r.Status != 200 {
			t.Fatalf("poll result %s: status=%d body=%s", requestID, r.Status, r.Raw)
		}
		var res seckillResult
		r.decode(t, &res)
		if res.Status == "SUCCESS" || res.Status == "FAILED" {
			return res
		}
		if time.Now().After(deadline) {
			t.Fatalf("request %s did not reach a terminal state within %s (last=%s)", requestID, timeout, res.Status)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// eventually 反复执行 cond 直到返回 true 或超时。
func eventually(t testing.TB, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for: %s", timeout, what)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

type page struct {
	Items []json.RawMessage `json:"items"`
	Total int               `json:"total"`
}

// myOrders 返回当前用户订单列表的原始项。
func (c *client) myOrders(t testing.TB, u user) []map[string]any {
	t.Helper()
	r := c.do(t, "GET", "/api/v1/order/user/me?page=1&size=50", u.Token, nil)
	if r.Status != 200 {
		t.Fatalf("my orders: status=%d body=%s", r.Status, r.Raw)
	}
	var p page
	r.decode(t, &p)
	out := make([]map[string]any, 0, len(p.Items))
	for _, it := range p.Items {
		var m map[string]any
		if err := json.Unmarshal(it, &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

// unread 返回未读通知数。
func (c *client) unread(t testing.TB, u user) int {
	t.Helper()
	r := c.do(t, "GET", "/api/v1/order/notifications/unread-count", u.Token, nil)
	if r.Status != 200 {
		t.Fatalf("unread-count: status=%d body=%s", r.Status, r.Raw)
	}
	var d struct {
		Count int `json:"count"`
	}
	r.decode(t, &d)
	return d.Count
}
