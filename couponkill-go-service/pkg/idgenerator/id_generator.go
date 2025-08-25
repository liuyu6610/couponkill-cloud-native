package idgenerator

import (
	"sync"
	"time"
)

// Snowflake 算法参数
const (
	epoch          = int64(1577836800000)             // 设置起始时间戳(毫秒)：2020-01-01 00:00:00
	nodeBits       = uint(10)                         // 节点ID所占位数
	sequenceBits   = uint(12)                         // 序列号所占位数
	nodeMax        = int64(-1 ^ (-1 << nodeBits))     // 节点ID最大值
	sequenceMax    = int64(-1 ^ (-1 << sequenceBits)) // 序列号最大值
	nodeShift      = sequenceBits                     // 节点ID左移位数
	timestampShift = sequenceBits + nodeBits          // 时间戳左移位数
)

// IDGenerator ID生成器结构体
type IDGenerator struct {
	mu        sync.Mutex
	timestamp int64
	node      int64
	sequence  int64
}

// NewIDGenerator 创建新的ID生成器
func NewIDGenerator(node int64) *IDGenerator {
	if node < 0 || node > nodeMax {
		panic("Node number must be between 0 and " + string(rune(nodeMax)))
	}

	return &IDGenerator{
		timestamp: 0,
		node:      node,
		sequence:  0,
	}
}

// NextID 生成下一个ID
func (g *IDGenerator) NextID() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixNano() / 1e6 // 转换为毫秒

	if now < g.timestamp {
		// 如果当前时间小于上一次生成ID的时间，则等待时间追上
		now = g.timestamp
	}

	if now == g.timestamp {
		// 如果在同一毫秒内，则序列号递增
		g.sequence = (g.sequence + 1) & sequenceMax
		if g.sequence == 0 {
			// 如果序列号溢出，则等待下一毫秒
			for now <= g.timestamp {
				now = time.Now().UnixNano() / 1e6
			}
		}
	} else {
		// 如果不在同一毫秒内，则序列号重置为0
		g.sequence = 0
	}

	g.timestamp = now

	// 组合ID
	id := ((now - epoch) << timestampShift) |
		(g.node << nodeShift) |
		g.sequence

	return id
}

// GenerateOrderID 生成订单ID
func GenerateOrderID() int64 {
	// 使用固定的节点ID，实际应用中可以根据需要动态分配
	generator := NewIDGenerator(1)
	return generator.NextID()
}
