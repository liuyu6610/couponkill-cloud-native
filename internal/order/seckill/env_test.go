package seckill_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders/orderstest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/seckill"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/kafkax"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

const couponID = int64(1001)

// env 是一整套秒杀测试环境：真实 PG（dbxtest）、miniredis（含 Lua）、假 Kafka、假 coupon 服务。
type env struct {
	t      *testing.T
	pool   *pgxpool.Pool
	rdb    *redis.Client
	mr     *miniredis.Miniredis
	broker *testkit.FakeBroker
	coupon *orderstest.FakeCoupon
	clock  *testkit.Clock
	hook   *recordingHook
	svc    *seckill.Service
}

// recordingHook 记录所有 ResultHook 回调。
type recordingHook struct {
	mu     sync.Mutex
	events []seckill.ResultEvent
	err    error
	panics bool
}

func (h *recordingHook) OnSeckillResult(_ context.Context, ev seckill.ResultEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, ev)
	if h.panics {
		panic("hook exploded")
	}
	return h.err
}

func (h *recordingHook) all() []seckill.ResultEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]seckill.ResultEvent(nil), h.events...)
}

func (h *recordingHook) forRequest(id string) []seckill.ResultEvent {
	var out []seckill.ResultEvent
	for _, ev := range h.all() {
		if ev.RequestID == id {
			out = append(out, ev)
		}
	}
	return out
}

func newEnv(t *testing.T, mutate ...func(*seckill.Options)) *env {
	t.Helper()
	return newEnvWithBroker(t, testkit.NewFakeBroker(), mutate...)
}

func newEnvWithBroker(t *testing.T, broker *testkit.FakeBroker, mutate ...func(*seckill.Options)) *env {
	t.Helper()
	rdb, mr := testkit.Redis(t)
	e := &env{
		t: t, pool: orderstest.NewPool(t), rdb: rdb, mr: mr, broker: broker,
		coupon: orderstest.NewFakeCoupon(t), clock: testkit.NewClock(time.Now().Truncate(time.Millisecond)), hook: &recordingHook{},
	}
	ids, err := idgen.New(1)
	if err != nil {
		t.Fatal(err)
	}
	opts := seckill.Options{Now: e.clock.Now, EnqueueTimeout: time.Second}
	for _, m := range mutate {
		m(&opts)
	}
	e.svc, err = seckill.New(seckill.Deps{
		DB: e.pool, Redis: rdb, Producer: e.broker.Producer(), Coupon: e.coupon.Client(t), IDs: ids,
	}, opts, e.hook)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// seed 登记一张秒杀券：假 coupon 里有 stock 件库存，Redis 计数器同样预热为 stock。
func (e *env) seed(id int64, stock int) {
	e.t.Helper()
	e.coupon.AddCoupon(orderstest.SeckillCoupon(id), stock)
	e.mr.Set(keys.Stock(id), strconv.Itoa(stock))
}

// counter 返回 Redis 预扣计数；键不存在时第二个返回值为 false。
func (e *env) counter(id int64) (int, bool) {
	e.t.Helper()
	if !e.mr.Exists(keys.Stock(id)) {
		return 0, false
	}
	v, err := e.mr.Get(keys.Stock(id))
	if err != nil {
		e.t.Fatalf("read counter: %v", err)
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		e.t.Fatalf("counter %q: %v", v, err)
	}
	return n, true
}

// reqField 读取请求哈希的字段。
func (e *env) reqField(requestID, field string) string {
	e.t.Helper()
	return e.mr.HGet(keys.SeckillRequest(requestID), field)
}

func (e *env) inflight() []string {
	e.t.Helper()
	if !e.mr.Exists(keys.SeckillInflight()) {
		return nil
	}
	members, err := e.mr.ZMembers(keys.SeckillInflight())
	if err != nil {
		e.t.Fatalf("read inflight: %v", err)
	}
	return members
}

// orderCount 返回券的订单数（可限定状态，0 = 不限）。
func (e *env) orderCount(couponID int64, status int) int {
	e.t.Helper()
	sql, args := `SELECT count(*) FROM ord.orders WHERE coupon_id = $1`, []any{couponID}
	if status > 0 {
		sql += ` AND status = $2`
		args = append(args, status)
	}
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		e.t.Fatalf("count orders: %v", err)
	}
	return n
}

func (e *env) ordersOfRequest(requestID string) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM ord.orders WHERE request_id = $1`, requestID).Scan(&n); err != nil {
		e.t.Fatalf("count orders by request: %v", err)
	}
	return n
}

func (e *env) notificationCount(userID int64) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM ord.notifications WHERE user_id = $1`, userID).Scan(&n); err != nil {
		e.t.Fatalf("count notifications: %v", err)
	}
	return n
}

// enter 以 userID 秒杀 couponID 并断言受理成功，返回 requestId。
func (e *env) enter(userID, couponID int64) string {
	e.t.Helper()
	res, err := e.svc.Enter(context.Background(), userID, couponID)
	if err != nil {
		e.t.Fatalf("Enter(user %d): %v", userID, err)
	}
	return res.RequestID
}

// consumeAll 用一个新的消费者实例处理 topic 里的全部消息直到提交位点追平，然后停止。
func (e *env) consumeAll() {
	e.t.Helper()
	e.consumeAllWith(e.svc)
}

func (e *env) consumeAllWith(svc *seckill.Service) {
	e.t.Helper()
	msgs := e.broker.Messages(events.TopicSeckillOrderCreate)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- svc.Consume(ctx, e.consumerConfig())
	}()
	e.waitCommitted(len(msgs))
	cancel()
	if err := <-done; err != nil {
		e.t.Fatalf("Consume: %v", err)
	}
}

func (e *env) consumerConfig() kafkax.ConsumerConfig {
	return e.broker.ConsumerConfig(events.GroupSeckillOrderCreate, events.TopicSeckillOrderCreate)
}

func (e *env) waitCommitted(n int) {
	e.t.Helper()
	if n == 0 {
		return
	}
	e.broker.WaitForCommit(e.t, events.GroupSeckillOrderCreate, events.TopicSeckillOrderCreate, 0, int64(n), 20*time.Second)
}

// message 构造发布到主题的建单命令载荷。
func command(requestID string, userID, couponID int64, at time.Time) []byte {
	raw, err := events.MarshalSeckillOrderCreate(events.NewSeckillOrderCreate(requestID, userID, couponID, at))
	if err != nil {
		panic(err)
	}
	return raw
}

// blockingProducer 的 Publish 一直阻塞到 ctx 结束，模拟一个不再回 ack 的 broker。
type blockingProducer struct {
	mu  sync.Mutex
	err error
}

func (b *blockingProducer) Publish(ctx context.Context, _ string, _, _ []byte, _ ...kafkax.PublishOption) error {
	<-ctx.Done()
	b.mu.Lock()
	b.err = ctx.Err()
	b.mu.Unlock()
	return ctx.Err()
}

func (b *blockingProducer) Close() error { return nil }

// newServiceWithProducer 复用 env 的其余依赖，换一个生产者。
func newServiceWithProducer(t *testing.T, e *env, p kafkax.Producer, hooks ...seckill.ResultHook) *seckill.Service {
	t.Helper()
	ids, err := idgen.New(2)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := seckill.New(seckill.Deps{DB: e.pool, Redis: e.rdb, Producer: p, Coupon: e.coupon.Client(t), IDs: ids},
		seckill.Options{Now: e.clock.Now, EnqueueTimeout: 40 * time.Millisecond}, hooks...)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func deductRequest(requestID string, userID, couponID int64) internalapi.DeductRequest {
	return internalapi.DeductRequest{RequestID: requestID, CouponID: couponID, UserID: userID}
}
