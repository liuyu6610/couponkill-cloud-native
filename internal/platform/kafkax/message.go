// Package kafkax 封装 franz-go：幂等生产者，以及“处理成功才提交位点、失败重试、超限进死信”的消费组运行器。
//
// 语义是至少一次（at-least-once）：位点只在 handler 成功（或消息已安全进入死信主题）之后提交，
// 因此崩溃、重平衡、重放都会导致重复投递，handler 必须幂等。
package kafkax

import (
	"context"
	"errors"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Header 是 Kafka 记录头。
type Header struct {
	Key   string
	Value []byte
}

// Message 是一条已消费的记录。
type Message struct {
	Topic     string
	Partition int32
	Offset    int64
	Key       []byte
	Value     []byte
	Headers   []Header
	Timestamp time.Time

	raw *kgo.Record // 仅由 kafka Source 设置，用于精确提交
}

// Header 返回第一个同名记录头的值（字符串），不存在返回空串。
func (m Message) Header(key string) string {
	for _, h := range m.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

// Producer 是服务依赖的发布接口。生产实现见 NewProducer，测试实现见 testkit.FakeBroker.Producer。
type Producer interface {
	// Publish 同步发布并等待 broker 确认。ctx 超时/取消即返回错误——此时记录“可能已送达也可能没有”，
	// 调用方必须按“结果未知”处理（例如用幂等键让消费端去重）。
	Publish(ctx context.Context, topic string, key, value []byte, opts ...PublishOption) error
	// Close 释放资源，之后不得再 Publish。
	Close() error
}

// PublishOptions 是单次发布的附加参数。
type PublishOptions struct {
	Headers []Header
}

// PublishOption 修改单次发布参数。
type PublishOption func(*PublishOptions)

// WithHeader 附加一个记录头。
func WithHeader(key, value string) PublishOption {
	return func(o *PublishOptions) { o.Headers = append(o.Headers, Header{Key: key, Value: []byte(value)}) }
}

// WithHeaders 附加多个记录头（保持原样，用于转发/死信）。
func WithHeaders(hs ...Header) PublishOption {
	return func(o *PublishOptions) { o.Headers = append(o.Headers, hs...) }
}

// ApplyPublishOptions 汇总一组 PublishOption（供 Producer 的各个实现使用）。
func ApplyPublishOptions(opts []PublishOption) PublishOptions {
	var o PublishOptions
	for _, f := range opts {
		f(&o)
	}
	return o
}

// DLTSuffix 是死信主题后缀：原主题 X 的死信主题为 X_dlt。
const DLTSuffix = "_dlt"

// 死信记录里追加的头。
const (
	HeaderOriginalTopic     = "x-original-topic"
	HeaderOriginalPartition = "x-original-partition"
	HeaderOriginalOffset    = "x-original-offset"
	HeaderError             = "x-error"
	HeaderAttempts          = "x-attempts"
	HeaderConsumerGroup     = "x-consumer-group"
	HeaderFailedAt          = "x-failed-at"
)

type permanentError struct{ err error }

func (p permanentError) Error() string { return p.err.Error() }
func (p permanentError) Unwrap() error { return p.err }

// Permanent 标记一个错误为不可重试（如载荷无法解析）：消费者跳过重试，直接送入死信。
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

// IsPermanent 报告错误链里是否含 Permanent 标记。
func IsPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}
