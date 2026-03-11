// Package buffer 提供批量缓冲功能
//
// Java对照: 类似于 BufferedOutputStream + ScheduledExecutorService
// 1. 缓冲单条日志，批量发送减少网络IO
// 2. 双重触发条件：满BatchSize或超时FlushInterval
// 3. 优雅退出时刷完剩余数据
package buffer

import (
	"context"
	"sync"
	"time"

	"github.com/liaozhangting/Snow/api"
)

// FlushFunc 是批量发送的回调函数
type FlushFunc func(ctx context.Context, logs []*api.LogRequest) error

// Batcher 批量缓冲器
type Batcher struct {
	input         chan *api.LogRequest
	batchSize     int
	flushInterval time.Duration
	flush         FlushFunc
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// NewBatcher 创建一个新的批量缓冲器
func NewBatcher(batchSize int, flushInterval time.Duration, flush FlushFunc) *Batcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &Batcher{
		input:         make(chan *api.LogRequest, batchSize*2),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		flush:         flush,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start 启动缓冲器
func (b *Batcher) Start() {
	b.wg.Add(1)
	go b.run()
}

// Add 添加一条日志到缓冲
func (b *Batcher) Add(log *api.LogRequest) {
	select {
	case b.input <- log:
	case <-b.ctx.Done():
	}
}

// Stop 停止缓冲器，刷完剩余数据
func (b *Batcher) Stop() {
	b.cancel()
	close(b.input)
	b.wg.Wait()
}

// run 是缓冲器的主循环
func (b *Batcher) run() {
	defer b.wg.Done()

	batch := make([]*api.LogRequest, 0, b.batchSize)
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case log, ok := <-b.input:
			if !ok {
				// Channel已关闭，刷完剩余数据
				if len(batch) > 0 {
					b.doFlush(batch)
				}
				return
			}
			batch = append(batch, log)
			if len(batch) >= b.batchSize {
				b.doFlush(batch)
				batch = batch[:0] // 复用底层数组
			}
		case <-ticker.C:
			if len(batch) > 0 {
				b.doFlush(batch)
				batch = batch[:0]
			}
		case <-b.ctx.Done():
			// 收到退出信号，刷完剩余数据
			if len(batch) > 0 {
				b.doFlush(batch)
			}
			return
		}
	}
}

// doFlush 执行批量发送
func (b *Batcher) doFlush(logs []*api.LogRequest) {
	// 使用独立Context，避免被主Context取消影响
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := b.flush(ctx, logs); err != nil {
		// 发送失败，这里可以加重试逻辑
	}
}
