package kafkax

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// ProducerConfig 配置 Kafka 生产者。
type ProducerConfig struct {
	// Brokers 种子 broker 地址（必填）。
	Brokers []string
	// ClientID 出现在 broker 日志里，便于排查。
	ClientID string
	// DeliveryTimeout 单条记录从入队到确认的总时限，默认 5s；调用方 ctx 更短时以 ctx 为准。
	DeliveryTimeout time.Duration
	// AutoCreateTopics 是否允许自动创建主题（仅开发环境使用；生产主题由部署预建）。
	AutoCreateTopics bool
}

// KafkaProducer 是基于 franz-go 的 Producer：幂等写入、acks=all。
type KafkaProducer struct {
	cl *kgo.Client
}

var _ Producer = (*KafkaProducer)(nil)

// NewProducer 创建生产者（不会立刻连接 broker）。
func NewProducer(cfg ProducerConfig) (*KafkaProducer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, errors.New("kafkax: producer needs at least one broker")
	}
	if cfg.DeliveryTimeout <= 0 {
		cfg.DeliveryTimeout = 5 * time.Second
	}
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()), // 幂等写入要求 acks=all
		kgo.RecordDeliveryTimeout(cfg.DeliveryTimeout),
		kgo.ProducerLinger(time.Millisecond), // 1ms 攒批：几乎不增加延迟，却能显著提升吞吐
	}
	if cfg.ClientID != "" {
		opts = append(opts, kgo.ClientID(cfg.ClientID))
	}
	if cfg.AutoCreateTopics {
		opts = append(opts, kgo.AllowAutoTopicCreation())
	}
	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafkax: new producer: %w", err)
	}
	return &KafkaProducer{cl: cl}, nil
}

// Publish 实现 Producer：同步等待确认。
func (p *KafkaProducer) Publish(ctx context.Context, topic string, key, value []byte, opts ...PublishOption) error {
	o := ApplyPublishOptions(opts)
	rec := &kgo.Record{Topic: topic, Key: key, Value: value}
	for _, h := range o.Headers {
		rec.Headers = append(rec.Headers, kgo.RecordHeader{Key: h.Key, Value: h.Value})
	}
	if err := p.cl.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("kafkax: publish to %s: %w", topic, err)
	}
	return nil
}

// Ping 检查与 broker 的连通性，适合挂到 /readyz。
func (p *KafkaProducer) Ping(ctx context.Context) error {
	if err := p.cl.Ping(ctx); err != nil {
		return fmt.Errorf("kafka ping: %w", err)
	}
	return nil
}

// Close 关闭生产者；未确认的记录会被丢弃，调用前应先停止发布。
func (p *KafkaProducer) Close() error {
	p.cl.Close()
	return nil
}
