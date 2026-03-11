// Package main 是雪毛儿边缘端的启动入口
//
// Java对照: 类似于 Spring Boot 的 Application 启动类
// 1. 初始化配置
// 2. 建立 gRPC 长连接
// 3. 模拟产生日志并流式发送
// 4. 支持优雅退出
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
	"github.com/liaozhangting/Snow/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// 1. 创建 Context（遵循规范 2.1）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. 加载配置（遵循规范 6.0）
	cfg := config.LoadEdgeConfig()
	log.Printf("[Edge] 雪毛儿正在启动，ID: %s, 目标云端: %s", cfg.DeviceID, cfg.CloudAddr)

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

	// 4. 开启流式发送
	stream, err := client.UploadLogs(ctx)
	if err != nil {
		log.Fatalf("开启日志流失败: %v", err)
	}

	// 5. 准备信号捕获和 WaitGroup（遵循规范 2.1）
	var wg sync.WaitGroup
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// 6. 模拟雪毛儿的日志产出
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if reply, err := stream.CloseAndRecv(); err != nil {
				log.Printf("接收云端响应失败: %v", err)
			} else {
				log.Printf("云端确认: %s", reply.Message)
			}
		}()

		for i := 0; i < 5; i++ {
			select {
			case <-ctx.Done():
				log.Printf("[Edge] 日志生产 Goroutine 收到退出信号，停止发送")
				return
			default:
				msg := &api.LogRequest{
					DeviceId:  cfg.DeviceID,
					Content:   "雪毛儿正在运行推理任务...",
					LogLevel:  "INFO",
					Timestamp: time.Now().UnixMilli(),
				}
				if err := stream.Send(msg); err != nil {
					log.Printf("发送日志失败: %v", err)
					return
				}
				time.Sleep(1 * time.Second)
			}
		}
	}()

	// 7. 等待信号（遵循规范 2.1）
	select {
	case <-signalChan:
		log.Printf("[Edge] 收到退出信号，开始优雅退出...")
		cancel() // 触发 Context 取消
	case <-ctx.Done():
		log.Printf("[Edge] Context 已取消")
	}

	// 8. 等待所有 Goroutine 退出
	wg.Wait()
	log.Println("[Edge] 优雅退出完成")
}
