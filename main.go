// Package main 是雪毛儿边缘端的启动入口
//
// Java对照: 类似于 Spring Boot 的 Application 启动类
// 1. 初始化配置
// 2. 建立 gRPC 长连接
// 3. 初始化 Pool 和 Batcher
// 4. 模拟产生日志并流式发送
// 5. 支持优雅退出
//
// 数据流: 模拟任务 ──► Pool ──► Batcher ──► gRPC批量发送
//
//	(生产)    (并发)    (缓冲聚合)  (网络IO)
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/liaozhangting/Snow/api"
	"github.com/liaozhangting/Snow/buffer"
	"github.com/liaozhangting/Snow/config"
	"github.com/liaozhangting/Snow/worker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func main() {
	// 1. 创建 Context（遵循规范 2.1）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. 加载配置（遵循规范 6.0）
	cfg := config.LoadEdgeConfig()
	log.Printf("[Edge] 雪毛儿正在启动，ID: %s, 目标云端: %s", cfg.DeviceID, cfg.CloudAddr)
	log.Printf("[Edge] 配置: WorkerCount=%d, QueueSize=%d, BatchSize=%d, FlushInterval=%v, TaskCount=%d",
		cfg.WorkerCount, cfg.QueueSize, cfg.BatchSize, cfg.FlushInterval, cfg.TaskCount)

	// 3. 建立 gRPC 连接（遵循规范 4.2）
	conn, err := grpc.Dial(cfg.CloudAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("连接云端失败: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("关闭 gRPC 连接失败: %v", err)
		}
	}()

	client := api.NewLogServiceClient(conn)

	// 4. 初始化 Batcher（批量缓冲）
	var stream api.LogService_UploadLogsClient
	var streamMu sync.Mutex

	// main.go 里， Batcher之前创建流
	stream, err = client.UploadLogs(ctx)
	if err != nil {
		log.Fatalf("创建流失败： %v", err)
	}
	defer stream.CloseSend()
	batcher := buffer.NewBatcher(cfg.BatchSize, cfg.FlushInterval,
		func(ctx context.Context, logs []*api.LogRequest) error {
			for _, l := range logs {
				if err := stream.Send(l); err != nil {
					log.Printf("[Edge] 发送日志失败: %v", err)
					return err
				}
			}
			return nil
		})
	batcher.Start()
	defer batcher.Stop()

	// 5. 初始化 Pool（Goroutine 池）
	pool := worker.NewPool(cfg.WorkerCount, cfg.QueueSize)
	pool.Start()
	defer pool.Stop()

	// 6. 准备信号捕获（遵循规范 2.1）
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// 7. 模拟雪毛儿的任务产出
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("[Edge] 开始模拟 %d 个推理任务...", cfg.TaskCount)

		for i := 0; i < cfg.TaskCount; i++ {
			select {
			case <-ctx.Done():
				log.Printf("[Edge] 任务生产被中断，已生产 %d 个任务", i)
				return
			default:
				logReq := &api.LogRequest{
					DeviceId:  cfg.DeviceID,
					Content:   "雪毛儿正在运行推理任务...",
					LogLevel:  "INFO",
					Timestamp: time.Now().UnixMilli(),
				}

				task := worker.Task{Log: logReq}
				if err := pool.Submit(task); err != nil {
					log.Printf("[Edge] 提交任务失败: %v", err)
				} else {
					// 任务提交成功，写入 Batcher
					batcher.Add(logReq)
				}

				// 稍微控制一下生产速度
				time.Sleep(100 * time.Microsecond)
			}
		}
		log.Printf("[Edge] 任务生产完成，共 %d 个任务", cfg.TaskCount)
	}()

	// 8. 等待信号（遵循规范 2.1）
	select {
	case <-signalChan:
		log.Printf("[Edge] 收到退出信号，开始优雅退出...")
		cancel() // 触发 Context 取消
	case <-ctx.Done():
		log.Printf("[Edge] Context 已取消")
	}

	// 9. 优雅退出顺序(手动控制）
	wg.Wait()
	log.Printf("[Edge] Pool已处理 %d 个任务", pool.Processed())

	//  先关Batcher（刷盘） ，此时 conn 还开着
	batcher.Stop()

	// 再关流
	streamMu.Lock()
	if stream != nil {
		if reply, err := stream.CloseAndRecv(); err != nil {
			if status.Code(err) == codes.Canceled {
				log.Println("[Edge] 流已关闭，未等待云端响应（正常退出）")
			} else {
				log.Printf("[Edge] 接收云端响应失败: %v", err)
			}
		} else {
			log.Printf("云端确认: %s", reply.Message)
		}
	}
	streamMu.Unlock()

	log.Println("[Edge] 优雅退出完成")
}
