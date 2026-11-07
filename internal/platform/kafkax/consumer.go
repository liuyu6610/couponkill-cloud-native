package kafkax

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// ErrClosed 表示 Source 已被关闭。
var ErrClosed = errors.New("kafkax: source closed")

// errShutdown 是 handler 流程内部用来表示“因 ctx 取消而中止”的哨兵，不会返回给调用方。
var errShutdown = errors.New("kafkax: shutting down")

// Handler 处理一条消息。返回 nil 表示成功；返回错误则按配置重试，最终进入死信。
// handler 必须幂等：同一条消息可能被投递多次。
type Handler func(ctx context.Context, msg Message) error

// Source 是消费组的底层读取抽象：生产实现基于 franz-go，测试实现见 testkit.FakeBroker。
type Source interface {
	// Poll 阻塞直到有记录可处理；ctx 取消时返回 ctx.Err()。
	Poll(ctx context.Context) ([]Message, error)
	// Commit 提交位点：对每个分区，提交“所给消息中最大 Offset 之后”的位置。
	Commit(ctx context.Context, msgs ...Message) error
	// Close 释放资源（并退出消费组）。
	Close()
}

// ConsumerConfig 配置一个消费组运行器。
type ConsumerConfig struct {
	// Brokers、Group、Topics 描述要加入的消费组。Group 必填；使用 Source 时 Brokers/Topics 可省略。
	Brokers []string
	Group   string
	Topics  []string
	// DLT 用于发布死信记录（必填）。死信主题为 <原主题>_dlt。
	DLT Producer
	// MaxRetries 首次失败后的最大重试次数：0 取默认 5，负数表示不重试。
	MaxRetries int
	// BackoffBase / BackoffMax 重试退避的起点与上限（指数增长），默认 100ms / 5s。
	BackoffBase time.Duration
	BackoffMax  time.Duration
	// Concurrency 同时处理的分区数，默认 1。同一分区内始终严格按序处理。
	Concurrency int
	// PollMax 每次拉取的最大记录数，默认 100。
	PollMax int
	// Source 替换底层读取实现（测试用）；为空则连接 Brokers。
	Source Source
}

const (
	defaultMaxRetries  = 5
	defaultBackoffBase = 100 * time.Millisecond
	defaultBackoffMax  = 5 * time.Second
	defaultPollMax     = 100
	commitAttempts     = 3
	dltPublishTimeout  = 10 * time.Second
	maxErrorHeaderLen  = 1024
)

func (c ConsumerConfig) validated() (ConsumerConfig, error) {
	if c.Group == "" {
		return c, errors.New("kafkax: consumer group is required")
	}
	if c.DLT == nil {
		return c, errors.New("kafkax: dead-letter producer is required")
	}
	if c.Source == nil && (len(c.Brokers) == 0 || len(c.Topics) == 0) {
		return c, errors.New("kafkax: brokers and topics are required when no Source is given")
	}
	switch {
	case c.MaxRetries == 0:
		c.MaxRetries = defaultMaxRetries
	case c.MaxRetries < 0:
		c.MaxRetries = 0
	}
	if c.BackoffBase <= 0 {
		c.BackoffBase = defaultBackoffBase
	}
	if c.BackoffMax <= 0 {
		c.BackoffMax = defaultBackoffMax
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 1
	}
	if c.PollMax <= 0 {
		c.PollMax = defaultPollMax
	}
	return c, nil
}

// BackoffDelay 返回第 attempt 次失败（从 0 开始）之后的等待时间：base·2^attempt，封顶 max。
func BackoffDelay(attempt int, base, max time.Duration) time.Duration {
	d := base
	for i := 0; i < attempt && d < max; i++ {
		d *= 2
	}
	return min(d, max)
}

type consumer struct {
	cfg ConsumerConfig
	h   Handler
}

// Consume 加入消费组并持续处理消息，直到 ctx 取消（返回 nil）或遇到不可恢复的错误。
//
// 保证：
//   - 位点只在 handler 成功、或消息已成功发布到死信主题之后才提交；
//   - 死信发布失败、提交失败（重试后）会让 Consume 返回错误且不提交该消息，重启后重新投递；
//   - ctx 取消后不再启动新的 handler 调用，已成功的消息的位点会被提交，正在重试中的消息保持未提交。
func Consume(ctx context.Context, cfg ConsumerConfig, h Handler) error {
	cfg, err := cfg.validated()
	if err != nil {
		return err
	}
	if h == nil {
		return errors.New("kafkax: handler is required")
	}
	src := cfg.Source
	if src == nil {
		if src, err = newKafkaSource(cfg); err != nil {
			return err
		}
	}
	defer src.Close()

	c := &consumer{cfg: cfg, h: h}
	for {
		msgs, err := src.Poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("kafkax: poll: %w", err)
		}
		if len(msgs) == 0 {
			continue
		}
		if err := c.processBatch(ctx, src, msgs); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

type partitionKey struct {
	topic     string
	partition int32
}

type groupResult struct {
	last *Message // 本分区最后一条处理完成的消息
	err  error
}

// processBatch 把一批消息按分区分组、分区内顺序处理，然后提交各分区已完成的最后一条。
func (c *consumer) processBatch(ctx context.Context, src Source, msgs []Message) error {
	var order []partitionKey
	groups := map[partitionKey][]Message{}
	for _, m := range msgs {
		k := partitionKey{m.Topic, m.Partition}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], m)
	}

	results := make([]groupResult, len(order))
	if c.cfg.Concurrency <= 1 || len(order) == 1 {
		for i, k := range order {
			results[i] = c.runGroup(ctx, groups[k])
		}
	} else {
		sem := make(chan struct{}, c.cfg.Concurrency)
		var wg sync.WaitGroup
		for i, k := range order {
			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				results[i] = c.runGroup(ctx, groups[k])
			}()
		}
		wg.Wait()
	}

	var done []Message
	var firstErr error
	for _, r := range results {
		if r.last != nil {
			done = append(done, *r.last)
		}
		if r.err != nil && !errors.Is(r.err, errShutdown) && firstErr == nil {
			firstErr = r.err
		}
	}
	if len(done) > 0 {
		if err := c.commit(ctx, src, done); err != nil {
			return errors.Join(firstErr, err)
		}
	}
	return firstErr
}

func (c *consumer) runGroup(ctx context.Context, msgs []Message) groupResult {
	var res groupResult
	for i := range msgs {
		if err := c.handleOne(ctx, msgs[i]); err != nil {
			res.err = err
			return res
		}
		res.last = &msgs[i]
	}
	return res
}

// commit 提交位点；即使 ctx 已取消也要尽量提交已完成的进度。
func (c *consumer) commit(ctx context.Context, src Source, msgs []Message) error {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var err error
	for attempt := 1; attempt <= commitAttempts; attempt++ {
		if err = src.Commit(cctx, msgs...); err == nil {
			return nil
		}
		logx.From(ctx).Warn("kafka commit failed", "group", c.cfg.Group, "attempt", attempt, "err", err)
		select {
		case <-cctx.Done():
			return fmt.Errorf("kafkax: commit: %w", errors.Join(err, cctx.Err()))
		case <-time.After(time.Duration(attempt) * 50 * time.Millisecond):
		}
	}
	return fmt.Errorf("kafkax: commit after %d attempts: %w", commitAttempts, err)
}

// handleOne 处理单条消息：成功返回 nil；重试耗尽或遇到 Permanent 则送死信；ctx 取消返回 errShutdown。
func (c *consumer) handleOne(ctx context.Context, m Message) error {
	for attempt := 0; ; attempt++ {
		if ctx.Err() != nil {
			return errShutdown
		}
		err := c.invoke(ctx, m)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil { // 关闭过程中产生的错误不代表消息有问题，不进死信
			return errShutdown
		}
		if IsPermanent(err) || attempt >= c.cfg.MaxRetries {
			return c.deadLetter(ctx, m, err, attempt+1)
		}
		delay := BackoffDelay(attempt, c.cfg.BackoffBase, c.cfg.BackoffMax)
		logx.From(ctx).Warn("kafka handler failed, will retry",
			"topic", m.Topic, "partition", m.Partition, "offset", m.Offset, "attempt", attempt+1, "retry_in", delay, "err", err)
		if !sleep(ctx, delay) {
			return errShutdown
		}
	}
}

// invoke 调用 handler，把 panic 转为错误（panic 只是 handler 的一种失败方式，不应带走整个消费者）。
func (c *consumer) invoke(ctx context.Context, m Message) (err error) {
	defer func() {
		if r := recover(); r != nil {
			logx.From(ctx).Error("kafka handler panic", "topic", m.Topic, "offset", m.Offset, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			err = fmt.Errorf("kafkax: handler panic: %v", r)
		}
	}()
	return c.h(ctx, m)
}

// deadLetter 把消息连同失败原因发布到 <topic>_dlt。发布失败则返回错误（调用方因此不会提交位点）。
func (c *consumer) deadLetter(ctx context.Context, m Message, cause error, attempts int) error {
	msg := cause.Error()
	if len(msg) > maxErrorHeaderLen {
		msg = msg[:maxErrorHeaderLen]
	}
	headers := append([]Header(nil), m.Headers...)
	headers = append(headers,
		Header{HeaderOriginalTopic, []byte(m.Topic)},
		Header{HeaderOriginalPartition, []byte(strconv.Itoa(int(m.Partition)))},
		Header{HeaderOriginalOffset, []byte(strconv.FormatInt(m.Offset, 10))},
		Header{HeaderError, []byte(msg)},
		Header{HeaderAttempts, []byte(strconv.Itoa(attempts))},
		Header{HeaderConsumerGroup, []byte(c.cfg.Group)},
		Header{HeaderFailedAt, []byte(time.Now().UTC().Format(time.RFC3339Nano))},
	)
	pctx, cancel := context.WithTimeout(ctx, dltPublishTimeout)
	defer cancel()
	dlt := m.Topic + DLTSuffix
	if err := c.cfg.DLT.Publish(pctx, dlt, m.Key, m.Value, WithHeaders(headers...)); err != nil {
		if ctx.Err() != nil {
			return errShutdown
		}
		return fmt.Errorf("kafkax: dead-letter %s[%d]@%d to %s: %w", m.Topic, m.Partition, m.Offset, dlt, err)
	}
	logx.From(ctx).Error("kafka message dead-lettered",
		"topic", m.Topic, "partition", m.Partition, "offset", m.Offset, "dlt", dlt, "attempts", attempts, "err", cause)
	return nil
}

// sleep 等待 d 或 ctx 取消；返回 false 表示被取消。
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
