package kafkax

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// kafkaSource 是基于 franz-go 消费组的 Source：关闭自动提交，拉取期间阻塞再平衡。
type kafkaSource struct {
	cl  *kgo.Client
	max int
}

func newKafkaSource(cfg ConsumerConfig) (Source, error) {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(cfg.Group),
		kgo.ConsumeTopics(cfg.Topics...),
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		return nil, fmt.Errorf("kafkax: new consumer %q: %w", cfg.Group, err)
	}
	return &kafkaSource{cl: cl, max: cfg.PollMax}, nil
}

// Poll 实现 Source。每次调用开头先放开再平衡：上一批已经处理并提交完毕。
func (s *kafkaSource) Poll(ctx context.Context) ([]Message, error) {
	s.cl.AllowRebalance()
	for {
		fetches := s.cl.PollRecords(ctx, s.max)
		if fetches.IsClientClosed() {
			return nil, ErrClosed
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		failed := false
		fetches.EachError(func(topic string, partition int32, err error) {
			failed = true
			logx.From(ctx).Warn("kafka fetch error", "topic", topic, "partition", partition, "err", err)
		})
		var out []Message
		fetches.EachRecord(func(r *kgo.Record) { out = append(out, fromRecord(r)) })
		if len(out) > 0 {
			return out, nil
		}
		if failed && !sleep(ctx, 200*time.Millisecond) { // 只有错误：franz-go 会自行重试，避免空转
			return nil, ctx.Err()
		}
	}
}

// Commit 实现 Source。
func (s *kafkaSource) Commit(ctx context.Context, msgs ...Message) error {
	recs := make([]*kgo.Record, 0, len(msgs))
	for _, m := range msgs {
		r := m.raw
		if r == nil {
			r = &kgo.Record{Topic: m.Topic, Partition: m.Partition, Offset: m.Offset}
		}
		recs = append(recs, r)
	}
	if err := s.cl.CommitRecords(ctx, recs...); err != nil {
		return fmt.Errorf("kafkax: commit records: %w", err)
	}
	return nil
}

// Close 实现 Source。BlockRebalanceOnPoll 下必须用 CloseAllowingRebalance，否则会死锁等待再平衡。
func (s *kafkaSource) Close() { s.cl.CloseAllowingRebalance() }

func fromRecord(r *kgo.Record) Message {
	m := Message{Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Key: r.Key, Value: r.Value, Timestamp: r.Timestamp, raw: r}
	for _, h := range r.Headers {
		m.Headers = append(m.Headers, Header{Key: h.Key, Value: h.Value})
	}
	return m
}
