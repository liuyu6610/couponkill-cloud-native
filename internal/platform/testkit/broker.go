package testkit

import (
	"context"
	"errors"
	"hash/fnv"
	"sync"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/kafkax"
)

// 故障注入产生的错误。
var (
	// ErrInjectedCrash 模拟“处理完但提交位点前进程崩溃”：Source.Commit 返回它，位点保持不变。
	ErrInjectedCrash = errors.New("testkit: injected crash before offset commit")
	// ErrInjectedPublish 模拟 Kafka 发布失败（超时/不可用）。
	ErrInjectedPublish = errors.New("testkit: injected publish failure")
)

type topicPartition struct {
	topic     string
	partition int32
}

type groupPartition struct {
	group string
	tp    topicPartition
}

// FakeBroker 是内存版 Kafka：按主题/分区保存记录，消费组位点单独维护。
// 它实现与生产一致的 kafkax.Producer / kafkax.Source 接口，因此服务可以不依赖 Kafka 测试
// 生产者与消费者，包括重复投递、崩溃-提交前、重放、发布失败。
//
// 限制：同一个消费组同一时刻只应有一个活跃 Source（不模拟分区分配）。
type FakeBroker struct {
	partitions int

	mu         sync.Mutex
	log        map[string][]kafkax.Message // 主题 → 按发布顺序的记录
	nextOffset map[topicPartition]int64
	committed  map[groupPartition]int64 // 下一条要读的偏移 = 已提交偏移
	rr         int                      // 无 key 记录的轮询计数
	wake       chan struct{}            // 每次发布后关闭并重建，唤醒阻塞的 Poll

	crash       map[string]int // 消费组 → 还要崩溃几次
	failPublish int
	publishErr  error
	published   int
}

// BrokerOption 调整 FakeBroker。
type BrokerOption func(*FakeBroker)

// WithPartitions 设置每个主题的分区数（默认 1）。
func WithPartitions(n int) BrokerOption {
	return func(b *FakeBroker) {
		if n > 0 {
			b.partitions = n
		}
	}
}

// NewFakeBroker 创建空的内存 broker。
func NewFakeBroker(opts ...BrokerOption) *FakeBroker {
	b := &FakeBroker{
		partitions: 1,
		log:        map[string][]kafkax.Message{},
		nextOffset: map[topicPartition]int64{},
		committed:  map[groupPartition]int64{},
		wake:       make(chan struct{}),
		crash:      map[string]int{},
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

// Producer 返回绑定到本 broker 的生产者。
func (b *FakeBroker) Producer() kafkax.Producer { return fakeProducer{b} }

// ConsumerConfig 返回可直接传给 kafkax.Consume 的配置：已带 Source、死信生产者与 1ms 级退避。
// 每次调用创建一个新的 Source，起点是该消费组已提交的位点（因此“重启消费者”即再调用一次）。
func (b *FakeBroker) ConsumerConfig(group string, topics ...string) kafkax.ConsumerConfig {
	return kafkax.ConsumerConfig{
		Group:       group,
		Topics:      topics,
		DLT:         b.Producer(),
		Source:      b.newSource(group, topics),
		BackoffBase: time.Millisecond,
		BackoffMax:  5 * time.Millisecond,
	}
}

// FailPublishes 让接下来的 n 次 Publish 失败（返回 ErrInjectedPublish）。
func (b *FakeBroker) FailPublishes(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failPublish = n
	b.publishErr = ErrInjectedPublish
}

// FailPublishesWith 同 FailPublishes，但使用自定义错误。
func (b *FakeBroker) FailPublishesWith(n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failPublish = n
	b.publishErr = err
}

// CrashBeforeCommit 让该消费组接下来的 n 个消费者实例各自在第一次提交时“崩溃”：
// 提交返回 ErrInjectedCrash（位点不动），且该实例此后的提交都会失败，像进程死亡一样。
// kafkax.Consume 因此返回错误；再起一个 Consume（新实例）即可观察到重投。
func (b *FakeBroker) CrashBeforeCommit(group string, n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.crash[group] = n
}

// ResetGroup 把消费组位点重置到起点，模拟重放。
func (b *FakeBroker) ResetGroup(group string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for k := range b.committed {
		if k.group == group {
			delete(b.committed, k)
		}
	}
}

// Messages 返回主题里已发布的全部记录（按发布顺序的拷贝）。
func (b *FakeBroker) Messages(topic string) []kafkax.Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]kafkax.Message, len(b.log[topic]))
	for i, m := range b.log[topic] {
		out[i] = cloneMessage(m)
	}
	return out
}

func cloneMessage(m kafkax.Message) kafkax.Message {
	m.Key = append([]byte(nil), m.Key...)
	m.Value = append([]byte(nil), m.Value...)
	hs := make([]kafkax.Header, len(m.Headers))
	for i, h := range m.Headers {
		hs[i] = kafkax.Header{Key: h.Key, Value: append([]byte(nil), h.Value...)}
	}
	m.Headers = hs
	return m
}

// Committed 返回消费组在某分区已提交的位点（= 已确认处理的记录条数）。
func (b *FakeBroker) Committed(group, topic string, partition int32) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.committed[groupPartition{group, topicPartition{topic, partition}}]
}

// PublishCalls 返回 Publish 被调用（含失败）的总次数。
func (b *FakeBroker) PublishCalls() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.published
}

// WaitForMessages 等待主题累计至少 n 条记录，超时则让测试失败。
func (b *FakeBroker) WaitForMessages(t testing.TB, topic string, n int, timeout time.Duration) []kafkax.Message {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if msgs := b.Messages(topic); len(msgs) >= n {
			return msgs
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d messages on %q, have %d", n, topic, len(b.Messages(topic)))
		}
		time.Sleep(time.Millisecond)
	}
}

// WaitForCommit 等待消费组在某分区的位点至少达到 offset，超时则让测试失败。
func (b *FakeBroker) WaitForCommit(t testing.TB, group, topic string, partition int32, offset int64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for b.Committed(group, topic, partition) < offset {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s/%s[%d] commit >= %d, have %d", group, topic, partition, offset, b.Committed(group, topic, partition))
		}
		time.Sleep(time.Millisecond)
	}
}

// ---------- 生产者 ----------

type fakeProducer struct{ b *FakeBroker }

func (p fakeProducer) Publish(ctx context.Context, topic string, key, value []byte, opts ...kafkax.PublishOption) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	o := kafkax.ApplyPublishOptions(opts)
	b := p.b
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published++
	if b.failPublish > 0 {
		b.failPublish--
		return b.publishErr
	}
	var part int32
	if len(key) > 0 {
		h := fnv.New32a()
		_, _ = h.Write(key)
		part = int32(h.Sum32() % uint32(b.partitions))
	} else {
		part = int32(b.rr % b.partitions)
		b.rr++
	}
	tp := topicPartition{topic, part}
	off := b.nextOffset[tp]
	b.nextOffset[tp] = off + 1
	hs := append([]kafkax.Header(nil), o.Headers...)
	for i := range hs {
		hs[i].Value = append([]byte(nil), hs[i].Value...)
	}
	b.log[topic] = append(b.log[topic], kafkax.Message{
		Topic: topic, Partition: part, Offset: off,
		Key: append([]byte(nil), key...), Value: append([]byte(nil), value...),
		Headers: hs, Timestamp: time.Now(),
	})
	close(b.wake)
	b.wake = make(chan struct{})
	return nil
}

func (p fakeProducer) Close() error { return nil }

// ---------- 消费端 Source ----------

type fakeSource struct {
	b       *FakeBroker
	group   string
	topics  []string
	pos     map[topicPartition]int64 // 已投递到的位置（下一条要投递的偏移）
	max     int
	crashed bool // 已被注入崩溃：像进程死亡一样，之后的提交一律失败
}

func (b *FakeBroker) newSource(group string, topics []string) *fakeSource {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := &fakeSource{b: b, group: group, topics: topics, pos: map[topicPartition]int64{}, max: 100}
	for k, off := range b.committed {
		if k.group == group {
			s.pos[k.tp] = off
		}
	}
	return s
}

// Poll 实现 kafkax.Source：返回尚未投递给本 Source 的记录，没有则阻塞。
func (s *fakeSource) Poll(ctx context.Context) ([]kafkax.Message, error) {
	for {
		s.b.mu.Lock()
		var out []kafkax.Message
		for _, topic := range s.topics {
			for _, m := range s.b.log[topic] {
				if len(out) >= s.max {
					break
				}
				tp := topicPartition{m.Topic, m.Partition}
				if m.Offset >= s.pos[tp] {
					out = append(out, cloneMessage(m))
				}
			}
		}
		// 一个分区内偏移连续：按分区推进已投递位置。
		for _, m := range out {
			tp := topicPartition{m.Topic, m.Partition}
			if m.Offset+1 > s.pos[tp] {
				s.pos[tp] = m.Offset + 1
			}
		}
		wake := s.b.wake
		s.b.mu.Unlock()
		if len(out) > 0 {
			return out, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wake:
		}
	}
}

// Commit 实现 kafkax.Source。
func (s *fakeSource) Commit(_ context.Context, msgs ...kafkax.Message) error {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	if !s.crashed {
		if n := s.b.crash[s.group]; n > 0 {
			s.b.crash[s.group] = n - 1
			s.crashed = true
		}
	}
	if s.crashed {
		return ErrInjectedCrash
	}
	for _, m := range msgs {
		k := groupPartition{s.group, topicPartition{m.Topic, m.Partition}}
		if m.Offset+1 > s.b.committed[k] {
			s.b.committed[k] = m.Offset + 1
		}
	}
	return nil
}

// Close 实现 kafkax.Source。
func (s *fakeSource) Close() {}
