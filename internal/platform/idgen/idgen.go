// Package idgen 提供 snowflake 风格的分布式 ID 生成器，以及“ID 一律输出为字符串”的 JSON 类型。
//
// 位布局：1 位符号（恒为 0）| 41 位毫秒时间戳（自 2024-01-01 UTC 起，约可用 69 年）| 10 位工作节点 | 12 位序列。
//
// 不变式：
//  1. 同一 Generator 产出的 ID 严格递增（含并发调用）。
//  2. 时钟回拨时使用“逻辑时钟”（取已发出的最大毫秒），不会倒退、不会重复、也不会阻塞等待。
//  3. 同一毫秒内序列号耗尽时，逻辑时钟前进 1ms 而不是等待真实时间。
//  4. 不同 worker 的 ID 因节点位不同而互不冲突（前提：部署时 worker 号不重复）。
package idgen

import (
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	workerBits = 10
	seqBits    = 12
	maxWorker  = 1<<workerBits - 1
	seqMask    = 1<<seqBits - 1

	workerShift = seqBits
	timeShift   = seqBits + workerBits
)

// epoch 是时间戳起点（毫秒）。
var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

// Option 调整 Generator。
type Option func(*Generator)

// WithClock 注入时钟（测试用）。
func WithClock(now func() time.Time) Option { return func(g *Generator) { g.now = now } }

// Generator 是并发安全的 snowflake 生成器。
type Generator struct {
	worker int64
	now    func() time.Time

	mu   sync.Mutex
	last int64 // 已发出的最大毫秒（相对 epoch）
	seq  int64
}

// New 创建生成器。workerID 必须在 0..1023。
func New(workerID int64, opts ...Option) (*Generator, error) {
	if workerID < 0 || workerID > maxWorker {
		return nil, fmt.Errorf("idgen: worker id %d out of range [0,%d]", workerID, maxWorker)
	}
	g := &Generator{worker: workerID, now: time.Now}
	for _, o := range opts {
		o(g)
	}
	return g, nil
}

// Next 返回下一个 ID（正数 int64）。
func (g *Generator) Next() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	ms := g.now().UnixMilli() - epoch
	if ms < g.last { // 时钟回拨：沿用逻辑时钟
		ms = g.last
	}
	if ms == g.last {
		g.seq = (g.seq + 1) & seqMask
		if g.seq == 0 { // 本毫秒序列耗尽：逻辑时钟前进
			ms = g.last + 1
		}
	} else {
		g.seq = 0
	}
	g.last = ms
	return ms<<timeShift | g.worker<<workerShift | g.seq
}

// NextString 返回十进制字符串形式的下一个 ID。
func (g *Generator) NextString() string { return strconv.FormatInt(g.Next(), 10) }

// WorkerOf 取出 ID 中的工作节点号。
func WorkerOf(id int64) int64 { return id >> workerShift & maxWorker }

// TimeOf 取出 ID 中的毫秒时间戳。
func TimeOf(id int64) time.Time { return time.UnixMilli(id>>timeShift + epoch).UTC() }

// WorkerIDFromName 把名字（通常是主机名/Pod 名）哈希为 0..1023 的节点号。
func WorkerIDFromName(name string) int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name)) // hash.Hash 的 Write 永不返回错误
	return int64(h.Sum32() & maxWorker)
}

// ResolveWorkerID 把配置值（CK_WORKER_ID）解析为节点号：>=0 直接使用，否则按主机名哈希。
// 多副本时哈希可能碰撞，生产环境建议由部署（如 StatefulSet 序号）显式给出。
func ResolveWorkerID(configured int) int64 {
	if configured >= 0 {
		return int64(configured)
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	return WorkerIDFromName(host)
}
