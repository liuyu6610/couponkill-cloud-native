package idgenerator

import (
	"sync"
	"time"
)

// 生成Go端唯一订单ID（与Java端区分）
type GoOrderIDGenerator struct {
	mu       sync.Mutex
	workerID int64
	sequence int64
	lastTime int64
}

var (
	goGenerator *GoOrderIDGenerator
	once        sync.Once
)

func init() {
	once.Do(func() {
		goGenerator = &GoOrderIDGenerator{
			workerID: 2, // 与Java端workerID区分（Java用1，Go用2）
		}
	})
}

// GenerateGoOrderID 生成Go端订单ID
func GenerateGoOrderID() string {
	return goGenerator.nextID()
}

func (g *GoOrderIDGenerator) nextID() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	currentTime := time.Now().UnixMilli()
	if currentTime < g.lastTime {
		panic("时钟回拨")
	}

	if currentTime == g.lastTime {
		g.sequence = (g.sequence + 1) & 0xFFF // 12位序列号
		if g.sequence == 0 {
			// 序列号溢出，等待下一毫秒
			for currentTime <= g.lastTime {
				currentTime = time.Now().UnixMilli()
			}
		}
	} else {
		g.sequence = 0
	}

	g.lastTime = currentTime

	// 结构：时间戳(41位) + workerID(10位) + 序列号(12位)
	id := (currentTime << 22) | (g.workerID << 12) | g.sequence
	return string(id)
}
