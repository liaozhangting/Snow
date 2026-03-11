// Package worker 提供 Goroutine Pool 功能
//
// Java对照: 类似于 ThreadPoolExecutor(corePoolSize, maxPoolSize, workQueue)
// 1. 有界队列控制内存使用
// 2. 固定数量Worker避免Goroutine爆炸
// 3. Context控制优雅退出
package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/liaozhangting/Snow/api"
)

var (
	ErrPoolClosed = errors.New("worker pool is closed")
	ErrQueueFull  = errors.New("task queue is full")
)

// Task 表示一个推理任务
type Task struct {
	Log *api.LogRequest
}

// Pool 是 Goroutine 池，用于并发处理推理任务
type Pool struct {
	tasks       chan Task
	workers     int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	closed      atomic.Bool
	processed   atomic.Int64
}

// NewPool 创建一个新的 Goroutine 池
func NewPool(workers int, queueSize int) *Pool {
	ctx, cancel := context.WithCancel(context.Background())
	return &Pool{
		tasks:   make(chan Task, queueSize),
		workers: workers,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动 Worker 池
func (p *Pool) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.runWorker(i)
	}
}

// Submit 提交一个任务到池
func (p *Pool) Submit(task Task) error {
	if p.closed.Load() {
		return ErrPoolClosed
	}

	select {
	case p.tasks <- task:
		return nil
	default:
		return ErrQueueFull
	}
}

// Stop 停止 Worker 池，优雅退出
func (p *Pool) Stop() {
	if p.closed.Swap(true) {
		return
	}

	p.cancel()
	close(p.tasks)
	p.wg.Wait()
}

// Processed 返回已处理的任务数
func (p *Pool) Processed() int64 {
	return p.processed.Load()
}

// runWorker 是 Worker 的主循环
func (p *Pool) runWorker(id int) {
	defer p.wg.Done()

	for {
		select {
		case task, ok := <-p.tasks:
			if !ok {
				return
			}
			p.processTask(&task)
		case <-p.ctx.Done():
			return
		}
	}
}

// processTask 处理单个任务（子类可覆盖）
func (p *Pool) processTask(task *Task) {
	p.processed.Add(1)
	// 实际处理逻辑由 Batcher 回调实现
}
