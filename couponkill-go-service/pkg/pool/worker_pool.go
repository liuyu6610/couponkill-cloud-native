package pool

import (
	"sync"
	"sync/atomic"
)

type WorkerPool struct {
	pool       chan func()
	wg         sync.WaitGroup
	closed     int32 // 使用原子操作替代mutex
	maxWorkers int
}

func NewWorkerPool(maxWorkers int) *WorkerPool {
	pool := &WorkerPool{
		pool:       make(chan func(), maxWorkers*2), // 增加缓冲区大小
		maxWorkers: maxWorkers,
	}

	for i := 0; i < maxWorkers; i++ {
		go pool.worker()
	}

	return pool
}

func (p *WorkerPool) worker() {
	for task := range p.pool {
		if task != nil {
			task()
		}
		p.wg.Done()
	}
}

func (p *WorkerPool) Submit(task func()) {
	// 使用原子操作检查是否已关闭
	if atomic.LoadInt32(&p.closed) == 1 {
		// 如果池已关闭，直接执行任务
		go func() {
			if task != nil {
				task()
			}
		}()
		return
	}

	p.wg.Add(1)

	// 尝试将任务提交到工作池
	select {
	case p.pool <- task:
		// 任务成功提交到池中
	default:
		// 如果池已满，启动新的goroutine执行任务
		go func() {
			if task != nil {
				task()
			}
			p.wg.Done()
		}()
	}
}

func (p *WorkerPool) Close() {
	// 使用原子操作设置关闭标志
	if !atomic.CompareAndSwapInt32(&p.closed, 0, 1) {
		// 已经关闭过了
		return
	}

	// 等待所有任务完成
	p.wg.Wait()

	// 关闭通道
	close(p.pool)
}

// GetMaxWorkers 返回工作池的最大工作协程数
func (p *WorkerPool) GetMaxWorkers() int {
	return p.maxWorkers
}

// GetCurrentWaitingTasks 返回当前等待执行的任务数
func (p *WorkerPool) GetCurrentWaitingTasks() int {
	return len(p.pool)
}
