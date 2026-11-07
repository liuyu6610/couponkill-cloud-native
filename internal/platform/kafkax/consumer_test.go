package kafkax_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/kafkax"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

const (
	topic = "seckill_order_create"
	group = "seckill-order-create-group"
	wait  = 5 * time.Second
)

// runner 在 goroutine 里跑 Consume，并保证测试结束时 goroutine 已退出。
type runner struct {
	cancel context.CancelFunc
	done   chan error
}

func start(t *testing.T, cfg kafkax.ConsumerConfig, h kafkax.Handler) *runner {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &runner{cancel: cancel, done: make(chan error, 1)}
	go func() { r.done <- kafkax.Consume(ctx, cfg, h) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-r.done:
		case <-time.After(wait):
			t.Error("Consume goroutine did not exit after ctx cancellation")
		}
	})
	return r
}

// stop 取消并返回 Consume 的返回值。
func (r *runner) stop(t *testing.T) error {
	t.Helper()
	r.cancel()
	select {
	case err := <-r.done:
		r.done <- err
		return err
	case <-time.After(wait):
		t.Fatal("Consume did not stop in time")
		return nil
	}
}

// exited 等待 Consume 自行返回（不取消）。
func (r *runner) exited(t *testing.T) error {
	t.Helper()
	select {
	case err := <-r.done:
		r.done <- err
		return err
	case <-time.After(wait):
		t.Fatal("Consume did not return by itself")
		return nil
	}
}

type calls struct {
	mu   sync.Mutex
	seen []string
}

func (c *calls) add(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = append(c.seen, s)
}
func (c *calls) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.seen...)
}
func (c *calls) waitLen(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for len(c.all()) < n {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d handler calls, got %v", n, c.all())
		}
		time.Sleep(time.Millisecond)
	}
}

func publish(t *testing.T, b *testkit.FakeBroker, key, value string, opts ...kafkax.PublishOption) {
	t.Helper()
	if err := b.Producer().Publish(context.Background(), topic, []byte(key), []byte(value), opts...); err != nil {
		t.Fatal(err)
	}
}

func TestConsumeHandlesInOrderAndCommitsAfterSuccess(t *testing.T) {
	b := testkit.NewFakeBroker()
	for i := 0; i < 5; i++ {
		publish(t, b, "u1", fmt.Sprintf("m%d", i), kafkax.WithHeader("trace", "abc"))
	}
	var c calls
	start(t, b.ConsumerConfig(group, topic), func(_ context.Context, m kafkax.Message) error {
		if m.Header("trace") != "abc" || string(m.Key) != "u1" || m.Topic != topic {
			t.Errorf("message fields lost: %+v", m)
		}
		c.add(string(m.Value))
		return nil
	})
	c.waitLen(t, 5)
	b.WaitForCommit(t, group, topic, 0, 5, wait)
	if got := strings.Join(c.all(), ","); got != "m0,m1,m2,m3,m4" {
		t.Fatalf("order violated: %s", got)
	}
}

func TestOffsetIsNotCommittedBeforeHandlerSucceeds(t *testing.T) {
	b := testkit.NewFakeBroker()
	publish(t, b, "k", "v")
	entered, release := make(chan struct{}), make(chan struct{})
	start(t, b.ConsumerConfig(group, topic), func(context.Context, kafkax.Message) error {
		close(entered)
		<-release
		return nil
	})
	<-entered
	time.Sleep(20 * time.Millisecond)
	if got := b.Committed(group, topic, 0); got != 0 {
		t.Fatalf("offset committed while the handler is still running: %d", got)
	}
	close(release)
	b.WaitForCommit(t, group, topic, 0, 1, wait)
}

func TestRetryThenSuccessDoesNotDeadLetter(t *testing.T) {
	b := testkit.NewFakeBroker()
	publish(t, b, "k", "v")
	var attempts atomic.Int32
	start(t, b.ConsumerConfig(group, topic), func(context.Context, kafkax.Message) error {
		if attempts.Add(1) < 3 {
			return errors.New("coupon service timeout")
		}
		return nil
	})
	b.WaitForCommit(t, group, topic, 0, 1, wait)
	if attempts.Load() != 3 {
		t.Fatalf("handler called %d times, want 3 (2 failures + 1 success)", attempts.Load())
	}
	if n := len(b.Messages(topic + "_dlt")); n != 0 {
		t.Fatalf("a message that eventually succeeded must not be dead-lettered (%d in DLT)", n)
	}
}

func TestExhaustedRetriesGoToDLTWithOriginalDataAndContext(t *testing.T) {
	b := testkit.NewFakeBroker()
	publish(t, b, "user-7", "payload-1", kafkax.WithHeader("trace", "t-1"))
	publish(t, b, "user-7", "payload-2")
	cfg := b.ConsumerConfig(group, topic)
	cfg.MaxRetries = 2
	var c calls
	start(t, cfg, func(_ context.Context, m kafkax.Message) error {
		c.add(string(m.Value))
		if string(m.Value) == "payload-1" {
			return errors.New("always broken")
		}
		return nil
	})
	dlt := b.WaitForMessages(t, topic+"_dlt", 1, wait)[0]
	b.WaitForCommit(t, group, topic, 0, 2, wait)

	if string(dlt.Key) != "user-7" || string(dlt.Value) != "payload-1" {
		t.Fatalf("DLT must preserve key and value: %q %q", dlt.Key, dlt.Value)
	}
	want := map[string]string{
		"trace":                        "t-1",
		kafkax.HeaderOriginalTopic:     topic,
		kafkax.HeaderOriginalOffset:    "0",
		kafkax.HeaderOriginalPartition: "0",
		kafkax.HeaderAttempts:          "3",
		kafkax.HeaderConsumerGroup:     group,
	}
	for k, v := range want {
		if got := dlt.Header(k); got != v {
			t.Errorf("DLT header %s = %q, want %q", k, got, v)
		}
	}
	if !strings.Contains(dlt.Header(kafkax.HeaderError), "always broken") {
		t.Errorf("DLT must carry the error: %q", dlt.Header(kafkax.HeaderError))
	}
	if _, err := time.Parse(time.RFC3339Nano, dlt.Header(kafkax.HeaderFailedAt)); err != nil {
		t.Errorf("x-failed-at must be RFC3339: %v", err)
	}
	// 一条毒消息不能卡住后面的消息。
	if got := c.all(); got[len(got)-1] != "payload-2" {
		t.Fatalf("message after the poison one must still be processed: %v", got)
	}
	if n := len(b.Messages(topic + "_dlt")); n != 1 {
		t.Fatalf("exactly one DLT record expected, got %d", n)
	}
}

func TestPermanentErrorSkipsRetries(t *testing.T) {
	b := testkit.NewFakeBroker()
	publish(t, b, "k", "not-json")
	var n atomic.Int32
	start(t, b.ConsumerConfig(group, topic), func(context.Context, kafkax.Message) error {
		n.Add(1)
		return kafkax.Permanent(errors.New("bad payload"))
	})
	dlt := b.WaitForMessages(t, topic+"_dlt", 1, wait)[0]
	if n.Load() != 1 || dlt.Header(kafkax.HeaderAttempts) != "1" {
		t.Fatalf("permanent errors must not be retried: calls=%d attempts=%s", n.Load(), dlt.Header(kafkax.HeaderAttempts))
	}
	if !kafkax.IsPermanent(fmt.Errorf("wrapped: %w", kafkax.Permanent(errors.New("x")))) || kafkax.IsPermanent(errors.New("x")) || kafkax.Permanent(nil) != nil {
		t.Fatal("Permanent/IsPermanent semantics wrong")
	}
}

func TestHandlerPanicIsTreatedAsFailure(t *testing.T) {
	b := testkit.NewFakeBroker()
	publish(t, b, "k", "boom")
	publish(t, b, "k", "fine")
	cfg := b.ConsumerConfig(group, topic)
	cfg.MaxRetries = 1
	var c calls
	start(t, cfg, func(_ context.Context, m kafkax.Message) error {
		c.add(string(m.Value))
		if string(m.Value) == "boom" {
			panic("handler bug")
		}
		return nil
	})
	dlt := b.WaitForMessages(t, topic+"_dlt", 1, wait)[0]
	if !strings.Contains(dlt.Header(kafkax.HeaderError), "panic") {
		t.Fatalf("panic must be reported in the DLT error header: %q", dlt.Header(kafkax.HeaderError))
	}
	b.WaitForCommit(t, group, topic, 0, 2, wait)
}

func TestDLTPublishFailureStopsConsumerWithoutCommitting(t *testing.T) {
	b := testkit.NewFakeBroker()
	publish(t, b, "k", "v")
	cfg := b.ConsumerConfig(group, topic)
	cfg.MaxRetries = -1 // 不重试，直接进死信
	b.FailPublishes(1)  // 死信发布失败
	r := start(t, cfg, func(context.Context, kafkax.Message) error { return errors.New("broken") })
	err := r.exited(t)
	if !errors.Is(err, testkit.ErrInjectedPublish) {
		t.Fatalf("Consume must surface the DLT failure, got %v", err)
	}
	if got := b.Committed(group, topic, 0); got != 0 {
		t.Fatalf("the message must NOT be committed when it could not be dead-lettered (committed=%d): that would lose it", got)
	}
	// 修复后重启：消息被重新投递并成功进入死信。
	start(t, b.ConsumerConfig(group, topic), func(context.Context, kafkax.Message) error { return kafkax.Permanent(errors.New("still broken")) })
	b.WaitForMessages(t, topic+"_dlt", 1, wait)
	b.WaitForCommit(t, group, topic, 0, 1, wait)
}

func TestCrashBeforeCommitCausesRedelivery(t *testing.T) {
	b := testkit.NewFakeBroker()
	publish(t, b, "k", "order-1")
	b.CrashBeforeCommit(group, 1)

	var c calls
	h := func(_ context.Context, m kafkax.Message) error { c.add(string(m.Value)); return nil }

	r := start(t, b.ConsumerConfig(group, topic), h)
	err := r.exited(t)
	if !errors.Is(err, testkit.ErrInjectedCrash) {
		t.Fatalf("crash must surface from Consume, got %v", err)
	}
	if b.Committed(group, topic, 0) != 0 {
		t.Fatal("crashed before commit: offset must not move")
	}
	// “进程重启”：新的消费者实例再次收到同一条消息 —— 至少一次语义，handler 必须幂等。
	start(t, b.ConsumerConfig(group, topic), h)
	b.WaitForCommit(t, group, topic, 0, 1, wait)
	if got := c.all(); len(got) != 2 || got[0] != "order-1" || got[1] != "order-1" {
		t.Fatalf("message must be delivered twice (crash then redelivery), got %v", got)
	}
}

func TestResetGroupReplaysEverything(t *testing.T) {
	b := testkit.NewFakeBroker()
	publish(t, b, "k", "a")
	publish(t, b, "k", "b")
	var c calls
	h := func(_ context.Context, m kafkax.Message) error { c.add(string(m.Value)); return nil }
	r := start(t, b.ConsumerConfig(group, topic), h)
	c.waitLen(t, 2)
	b.WaitForCommit(t, group, topic, 0, 2, wait)
	if err := r.stop(t); err != nil {
		t.Fatal(err)
	}
	b.ResetGroup(group)
	start(t, b.ConsumerConfig(group, topic), h)
	c.waitLen(t, 4)
	if got := strings.Join(c.all(), ""); got != "abab" {
		t.Fatalf("replay should redeliver everything in order, got %s", got)
	}
}

func TestCommittedMessagesAreNotRedeliveredOnRestart(t *testing.T) {
	b := testkit.NewFakeBroker()
	publish(t, b, "k", "a")
	var c calls
	h := func(_ context.Context, m kafkax.Message) error { c.add(string(m.Value)); return nil }
	r := start(t, b.ConsumerConfig(group, topic), h)
	b.WaitForCommit(t, group, topic, 0, 1, wait)
	_ = r.stop(t)
	publish(t, b, "k", "b")
	start(t, b.ConsumerConfig(group, topic), h)
	c.waitLen(t, 2)
	time.Sleep(20 * time.Millisecond)
	if got := strings.Join(c.all(), ""); got != "ab" {
		t.Fatalf("restart must resume after the committed offset, got %s", got)
	}
}

func TestContextCancellationStopsGracefullyAndKeepsInFlightMessageUncommitted(t *testing.T) {
	b := testkit.NewFakeBroker()
	publish(t, b, "k", "ok")
	publish(t, b, "k", "stuck")
	cfg := b.ConsumerConfig(group, topic)
	cfg.BackoffBase, cfg.BackoffMax = time.Hour, time.Hour // 退避期间被取消必须立刻响应
	var c calls
	r := start(t, cfg, func(_ context.Context, m kafkax.Message) error {
		c.add(string(m.Value))
		if string(m.Value) == "stuck" {
			return errors.New("transient")
		}
		return nil
	})
	c.waitLen(t, 2) // ok 与 stuck 的第一次尝试；之后进入长退避
	started := time.Now()
	if err := r.stop(t); err != nil {
		t.Fatalf("graceful stop must return nil, got %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancellation during backoff must be prompt")
	}
	if got := b.Committed(group, topic, 0); got != 1 {
		t.Fatalf("progress up to the last success must be committed, the in-flight message must not: committed=%d", got)
	}
	if n := len(b.Messages(topic + "_dlt")); n != 0 {
		t.Fatalf("shutdown must not dead-letter the in-flight message (%d)", n)
	}
	// 新实例接着处理 stuck（重新投递）。
	var after calls
	start(t, b.ConsumerConfig(group, topic), func(_ context.Context, m kafkax.Message) error { after.add(string(m.Value)); return nil })
	after.waitLen(t, 1)
	if got := after.all(); got[0] != "stuck" {
		t.Fatalf("expected redelivery of stuck, got %v", got)
	}
}

func TestHandlerErrorCausedByShutdownIsNotDeadLettered(t *testing.T) {
	b := testkit.NewFakeBroker()
	publish(t, b, "k", "v")
	entered := make(chan struct{})
	cfg := b.ConsumerConfig(group, topic)
	cfg.MaxRetries = -1
	r := start(t, cfg, func(ctx context.Context, _ kafkax.Message) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err() // handler 因关闭而失败
	})
	<-entered
	if err := r.stop(t); err != nil {
		t.Fatal(err)
	}
	if n := len(b.Messages(topic + "_dlt")); n != 0 || b.Committed(group, topic, 0) != 0 {
		t.Fatalf("shutdown errors must neither dead-letter nor commit (dlt=%d committed=%d)", n, b.Committed(group, topic, 0))
	}
}

func TestConcurrencyProcessesPartitionsInParallelButKeepsPartitionOrder(t *testing.T) {
	const partitions, perKey = 4, 25
	b := testkit.NewFakeBroker(testkit.WithPartitions(partitions))
	keys := []string{"u-a", "u-b", "u-c", "u-d", "u-e", "u-f"}
	for i := 0; i < perKey; i++ {
		for _, k := range keys {
			publish(t, b, k, fmt.Sprintf("%s:%03d", k, i))
		}
	}
	cfg := b.ConsumerConfig(group, topic)
	cfg.Concurrency = partitions

	var (
		mu       sync.Mutex
		perKeyOf = map[string][]string{}
		inFlight atomic.Int32
		peak     atomic.Int32
	)
	var total atomic.Int32
	start(t, cfg, func(_ context.Context, m kafkax.Message) error {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(200 * time.Microsecond)
		inFlight.Add(-1)
		mu.Lock()
		perKeyOf[string(m.Key)] = append(perKeyOf[string(m.Key)], string(m.Value))
		mu.Unlock()
		total.Add(1)
		return nil
	})
	deadline := time.Now().Add(10 * time.Second)
	for total.Load() < int32(len(keys)*perKey) {
		if time.Now().After(deadline) {
			t.Fatalf("processed only %d/%d", total.Load(), len(keys)*perKey)
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	for k, vals := range perKeyOf {
		for i, v := range vals {
			if want := fmt.Sprintf("%s:%03d", k, i); v != want {
				t.Fatalf("per-key order violated for %s at %d: %s", k, i, v)
			}
		}
	}
	if peak.Load() < 2 {
		t.Fatalf("expected parallel processing across partitions, peak concurrency %d", peak.Load())
	}
	if peak.Load() > partitions {
		t.Fatalf("concurrency limit exceeded: %d > %d", peak.Load(), partitions)
	}
}

func TestConsumeValidatesConfig(t *testing.T) {
	b := testkit.NewFakeBroker()
	ok := func(context.Context, kafkax.Message) error { return nil }
	good := b.ConsumerConfig(group, topic)

	noGroup := good
	noGroup.Group = ""
	noDLT := good
	noDLT.DLT = nil
	noSource := kafkax.ConsumerConfig{Group: group, DLT: b.Producer()}
	for name, cfg := range map[string]kafkax.ConsumerConfig{"no group": noGroup, "no DLT": noDLT, "no brokers/source": noSource} {
		if err := kafkax.Consume(context.Background(), cfg, ok); err == nil {
			t.Errorf("%s: Consume must reject the config", name)
		}
	}
	if err := kafkax.Consume(context.Background(), good, nil); err == nil {
		t.Error("nil handler must be rejected")
	}
}

func TestBackoffDelayIsExponentialAndCapped(t *testing.T) {
	base, max := 100*time.Millisecond, time.Second
	want := []time.Duration{100, 200, 400, 800, 1000, 1000}
	for i, w := range want {
		if got := kafkax.BackoffDelay(i, base, max); got != w*time.Millisecond {
			t.Errorf("attempt %d: %v, want %v", i, got, w*time.Millisecond)
		}
	}
}
