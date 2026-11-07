package kafkax_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/kafkax"
)

// 这组测试走真实的 franz-go 生产者/消费组代码，broker 由进程内的 kfake 提供（无需 Docker）。

const realWait = 30 * time.Second

func newCluster(t *testing.T, topics ...string) []string {
	t.Helper()
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, topics...))
	if err != nil {
		t.Fatalf("kfake: %v", err)
	}
	t.Cleanup(c.Close)
	return c.ListenAddrs()
}

func newProducer(t *testing.T, brokers []string) *kafkax.KafkaProducer {
	t.Helper()
	p, err := kafkax.NewProducer(kafkax.ProducerConfig{Brokers: brokers, ClientID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestNewProducerValidatesBrokers(t *testing.T) {
	if _, err := kafkax.NewProducer(kafkax.ProducerConfig{}); err == nil {
		t.Fatal("a producer without brokers must be rejected")
	}
}

func TestRealProducerAndConsumerGroupRoundTrip(t *testing.T) {
	brokers := newCluster(t, topic, topic+"_dlt")
	p := newProducer(t, brokers)

	ctx, cancel := context.WithTimeout(context.Background(), realWait)
	defer cancel()
	if err := p.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	for _, v := range []string{"one", "two", "three"} {
		if err := p.Publish(ctx, topic, []byte("user-1"), []byte(v), kafkax.WithHeader("trace", "t-"+v)); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	var (
		mu   sync.Mutex
		got  []kafkax.Message
		done = make(chan struct{})
	)
	r := start(t, kafkax.ConsumerConfig{Brokers: brokers, Group: group, Topics: []string{topic}, DLT: p}, func(_ context.Context, m kafkax.Message) error {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, m)
		if len(got) == 3 {
			close(done)
		}
		return nil
	})
	select {
	case <-done:
	case <-time.After(realWait):
		t.Fatalf("consumer received %d/3 messages", len(got))
	}
	mu.Lock()
	for i, want := range []string{"one", "two", "three"} {
		m := got[i]
		if string(m.Value) != want || string(m.Key) != "user-1" || m.Header("trace") != "t-"+want || m.Topic != topic || m.Offset != int64(i) {
			t.Errorf("message %d wrong: %+v", i, m)
		}
	}
	mu.Unlock()
	// 等位点提交落地再停：停止本身会提交已完成进度。
	if err := r.stop(t); err != nil {
		t.Fatalf("graceful stop: %v", err)
	}

	// 同一消费组再起一个消费者：已提交的消息不应再被投递；新消息正常到达。
	if err := p.Publish(ctx, topic, []byte("user-1"), []byte("four")); err != nil {
		t.Fatal(err)
	}
	next := make(chan string, 4)
	start(t, kafkax.ConsumerConfig{Brokers: brokers, Group: group, Topics: []string{topic}, DLT: p}, func(_ context.Context, m kafkax.Message) error {
		next <- string(m.Value)
		return nil
	})
	select {
	case v := <-next:
		if v != "four" {
			t.Fatalf("restarted consumer must resume after committed offsets, first message was %q", v)
		}
	case <-time.After(realWait):
		t.Fatal("restarted consumer received nothing")
	}
}

func TestRealConsumerDeadLettersAfterRetries(t *testing.T) {
	brokers := newCluster(t, topic, topic+"_dlt")
	p := newProducer(t, brokers)
	ctx, cancel := context.WithTimeout(context.Background(), realWait)
	defer cancel()

	if err := p.Publish(ctx, topic, []byte("k"), []byte("poison"), kafkax.WithHeader("trace", "x")); err != nil {
		t.Fatal(err)
	}
	start(t, kafkax.ConsumerConfig{
		Brokers: brokers, Group: group, Topics: []string{topic}, DLT: p,
		MaxRetries: 1, BackoffBase: time.Millisecond, BackoffMax: 2 * time.Millisecond,
	}, func(context.Context, kafkax.Message) error { return errors.New("cannot process") })

	dltSeen := make(chan kafkax.Message, 1)
	start(t, kafkax.ConsumerConfig{Brokers: brokers, Group: "dlt-reader", Topics: []string{topic + "_dlt"}, DLT: p}, func(_ context.Context, m kafkax.Message) error {
		dltSeen <- m
		return nil
	})
	select {
	case m := <-dltSeen:
		if string(m.Value) != "poison" || string(m.Key) != "k" || m.Header("trace") != "x" ||
			m.Header(kafkax.HeaderOriginalTopic) != topic || m.Header(kafkax.HeaderAttempts) != "2" {
			t.Fatalf("DLT record wrong: %+v headers=%+v", m, m.Headers)
		}
	case <-time.After(realWait):
		t.Fatal("no record arrived in the DLT topic")
	}
}

func TestRealProducerPublishHonoursContextDeadline(t *testing.T) {
	// 指向一个没有任何 broker 监听的地址：Publish 必须在 ctx 超时内返回错误，而不是无限阻塞。
	p, err := kafkax.NewProducer(kafkax.ProducerConfig{Brokers: []string{"127.0.0.1:1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := p.Publish(ctx, topic, nil, []byte("x")); err == nil {
		t.Fatal("publish to an unreachable broker must fail")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("Publish ignored the context deadline (%v)", d)
	}
}
