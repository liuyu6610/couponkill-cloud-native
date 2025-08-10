package pool

import "sync"

type WorkerPool struct {
	pool   chan func()
	wg     sync.WaitGroup
	closed bool
}

func NewWorkerPool(maxWorkers int) *WorkerPool {
	pool := &WorkerPool{
		pool: make(chan func(), maxWorkers),
	}

	for i := 0; i < maxWorkers; i++ {
		go pool.worker()
	}

	return pool
}

func (p *WorkerPool) worker() {
	for task := range p.pool {
		task()
		p.wg.Done()
	}
}

func (p *WorkerPool) Submit(task func()) {
	if p.closed {
		return
	}
	p.wg.Add(1)
	select {
	case p.pool <- task:
	default:
		// 如果池已满，直接执行任务
		go func() {
			task()
			p.wg.Done()
		}()
	}
}

func (p *WorkerPool) Close() {
	close(p.pool)
	p.closed = true
	p.wg.Wait()
}
