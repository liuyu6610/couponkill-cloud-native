package order_test

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders/orderstest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/seckill"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

func TestConfigDefaultsMatchTheDocumentedValues(t *testing.T) {
	var cfg order.Config
	if err := config.Load(&cfg, config.WithMap(map[string]string{}), config.WithDefaults(map[string]string{"CK_HTTP_PORT": "8082"})); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPPort != 8082 || cfg.EnqueueTimeout != 500*time.Millisecond || cfg.SweepInterval != 30*time.Second ||
		cfg.SweepStale != 120*time.Second || cfg.SweepBatch != 100 || cfg.SeckillCooldown != 0 ||
		!cfg.SweepEnabled || !cfg.ConsumeEnabled || cfg.MaxActiveCoupons != 15 || cfg.MaxActiveSeckill != 5 ||
		cfg.MaxPageSize != 100 || cfg.AdminMaxOffset != 10000 {
		t.Fatalf("defaults drifted from the documented values: %+v", cfg)
	}
	err := config.Load(&cfg, config.WithMap(map[string]string{"CK_ORDER_SWEEP_STALE": "1s", "CK_ORDER_ENQUEUE_TIMEOUT": "0s"}))
	if err == nil {
		t.Fatal("a 1s stale threshold and a 0s enqueue timeout must be rejected")
	}
}

type hookCounter struct {
	mu     sync.Mutex
	events []seckill.ResultEvent
}

func (h *hookCounter) OnSeckillResult(_ context.Context, ev seckill.ResultEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, ev)
	return nil
}

func (h *hookCounter) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.events)
}

// 整个服务经 HTTP 走一遍：秒杀 → 轮询到 SUCCESS → 取消（库存与占位归还）→ 再次秒杀成功。
// 同时验证 WithResultHook / WithRoutes / Workers 的组装。
func TestServerEndToEndSeckillCancelAndSeckillAgain(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	pool := orderstest.NewPool(t)
	coupon := orderstest.NewFakeCoupon(t)
	broker := testkit.NewFakeBroker()
	ids, err := idgen.New(1)
	if err != nil {
		t.Fatal(err)
	}
	cfg := order.Config{
		ConsumeEnabled: true, ConsumeConcurrency: 1, ConsumeMaxRetries: 3, SweepEnabled: true,
		SweepInterval: time.Second, SweepStale: 120 * time.Second, SweepBatch: 100,
		EnqueueTimeout: time.Second, MaxPageSize: 100, AdminMaxOffset: 10000, MaxActiveCoupons: 15, MaxActiveSeckill: 5,
	}
	hook := &hookCounter{}
	srv, err := order.New(order.Deps{
		Config: cfg, DB: pool, Redis: rdb, Producer: broker.Producer(), Coupon: coupon.Client(t), IDs: ids,
		Signer: testkit.Signer(t), InternalToken: testkit.InternalToken,
		Consumer: broker.ConsumerConfig(events.GroupSeckillOrderCreate, events.TopicSeckillOrderCreate),
	}, order.WithResultHook(hook), order.WithRoutes(func(r gin.IRouter) {
		r.GET("/api/v1/order/ping-aux", func(c *gin.Context) {
			id, ok := httpx.IdentityFrom(c)
			if !ok {
				httpx.Fail(c, nil)
				return
			}
			httpx.OK(c, gin.H{"userId": id.UserID})
		})
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(srv.Workers()); got != 2 {
		t.Fatalf("workers = %d, want the consumer and the sweeper", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for _, w := range srv.Workers() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Run(ctx); err != nil {
				t.Errorf("worker %s: %v", w.Name, err)
			}
		}()
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})

	const cid = int64(1001)
	coupon.AddCoupon(orderstest.SeckillCoupon(cid), 2)
	mr.Set(keys.Stock(cid), "2")
	user := testkit.User(42, "user")
	do := func(method, path string, body any, opts ...testkit.ReqOption) *testkit.Response {
		return testkit.DoJSON(t, srv.Handler(), method, path, body, opts...)
	}
	seckillAndWait := func() (string, events.SeckillResult) {
		res := do("POST", "/api/v1/order/seckill", map[string]string{"couponId": strconv.FormatInt(cid, 10)}, user)
		if res.Status != http.StatusAccepted {
			t.Fatalf("seckill = %d %s", res.Status, res.Body)
		}
		var acc events.SeckillAccepted
		res.Decode(&acc)
		deadline := time.Now().Add(20 * time.Second)
		for {
			var r events.SeckillResult
			do("GET", "/api/v1/order/seckill/result?requestId="+acc.RequestID, nil, user).Decode(&r)
			if r.Status.Terminal() {
				return acc.RequestID, r
			}
			if time.Now().After(deadline) {
				t.Fatalf("request %s never reached a terminal state (last %+v)", acc.RequestID, r)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	rid1, r1 := seckillAndWait()
	if r1.Status != events.StatusSuccess || r1.OrderID == 0 {
		t.Fatalf("first seckill = %+v", r1)
	}
	if v, _ := mr.Get(keys.Stock(cid)); v != "1" || coupon.Stock(cid) != 1 {
		t.Fatalf("after the first order: counter %s DB %d, want 1 and 1", v, coupon.Stock(cid))
	}
	// 持有期间再抢是重复。
	if res := do("POST", "/api/v1/order/seckill", map[string]string{"couponId": strconv.FormatInt(cid, 10)}, user); res.Status != http.StatusConflict || res.Code != 10010 {
		t.Fatalf("second seckill while holding = %d code %d", res.Status, res.Code)
	}

	// 取消：库存与占位全部归还。
	cancelRes := do("POST", "/api/v1/order/cancel", map[string]string{"orderId": strconv.FormatInt(r1.OrderID, 10)}, user)
	var cancelled bool
	cancelRes.Decode(&cancelled)
	if cancelRes.Status != http.StatusOK || !cancelled {
		t.Fatalf("cancel = %d %s", cancelRes.Status, cancelRes.Body)
	}
	if v, _ := mr.Get(keys.Stock(cid)); v != "2" || coupon.Stock(cid) != 2 {
		t.Fatalf("after the cancel: counter %s DB %d, want 2 and 2", v, coupon.Stock(cid))
	}
	if mr.Exists(keys.SeckillUser(cid, 42)) {
		t.Fatal("the marker must be removed by the cancel so the user can grab again")
	}

	rid2, r2 := seckillAndWait()
	if r2.Status != events.StatusSuccess || r2.OrderID == r1.OrderID || rid2 == rid1 {
		t.Fatalf("seckill after cancel = %+v (request %s vs %s)", r2, rid2, rid1)
	}
	if v, _ := mr.Get(keys.Stock(cid)); v != "1" || coupon.Stock(cid) != 1 {
		t.Fatalf("after the second order: counter %s DB %d, want 1 and 1", v, coupon.Stock(cid))
	}
	if got := hook.count(); got != 2 {
		t.Fatalf("hook saw %d results, want 2", got)
	}

	// 列表只含该用户的两张订单（一取消一有效），按创建时间倒序。
	var page struct {
		Items []orders.Order `json:"items"`
		Total int64          `json:"total"`
	}
	do("GET", "/api/v1/order/user/me?page=1&size=10", nil, user).Decode(&page)
	if page.Total != 2 || len(page.Items) != 2 || page.Items[0].ID != r2.OrderID || page.Items[1].Status != orders.StatusCancelled {
		t.Fatalf("my orders = %+v", page)
	}

	// WithRoutes 的路由在网关身份之后；内部路由在内部令牌之后；健康检查可用。
	if res := do("GET", "/api/v1/order/ping-aux", nil, user); res.Status != http.StatusOK {
		t.Fatalf("aux route = %d", res.Status)
	}
	if res := do("GET", "/api/v1/order/ping-aux", nil); res.Status != http.StatusUnauthorized {
		t.Fatalf("aux route without identity = %d", res.Status)
	}
	if res := do("POST", "/internal/v1/orders/grant", `{"requestId":"g1","userId":"7","couponId":"1001"}`); res.Status != http.StatusUnauthorized {
		t.Fatalf("grant without the internal token = %d", res.Status)
	}
	if res := do("GET", "/healthz", nil); res.Status != http.StatusOK {
		t.Fatalf("healthz = %d", res.Status)
	}
}
