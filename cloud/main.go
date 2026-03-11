// Package main 是雪毛儿云端服务端的启动入口
//
// Java对照: 类似于 Spring Boot 的 Application + Controller
// 1. 启动 gRPC 服务端
// 2. 接收边缘端流式日志
// 3. 打印/存储日志
package main

import (
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/liaozhangting/Snow/api"
)

// LogServer 实现 LogService 接口
type LogServer struct {
	api.UnimplementedLogServiceServer
}

// UploadLogs 接收客户端流式日志
func (s *LogServer) UploadLogs(stream api.LogService_UploadLogsServer) error {
	count := 0
	deviceId := "unknown"
	log.Printf("[Cloud] 新的边缘端连接建立")

	for {
		// 直接调用 Recv，它会阻塞等待，或者在超时/断开时返回错误
		req, err := stream.Recv()

		if err == io.EOF {
			// 客户端主动正常关闭 (CloseSend)
			log.Printf("[Cloud] 连接正常关闭，接收到 %d 条", count)
			return stream.SendAndClose(&api.LogResponse{Message: "接收成功"})
		}

		if err != nil {
			// 这里包含了多种情况：
			// 1. Context 超时 (DeadlineExceeded)
			// 2. 客户端强制断开 (Canceled)
			// 3. 网络错误
			if status.Code(err) == codes.Canceled {
				log.Printf("[Cloud] 客户端 [%s] 主动断开", deviceId)
			} else {
				log.Printf("[Cloud] 接收错误： %v", err)
			}
			return err
		}

		count++
		deviceId = req.DeviceId
		log.Printf("[Cloud] 收到 [%s]的日志: %s", req.DeviceId, req.Content)
	}
}

func main() {
	// 1. 监听信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 2. 创建gRPC Server
	s := grpc.NewServer()
	api.RegisterLogServiceServer(s, &LogServer{})

	// 3. 启动Goroutine处理信号
	go func() {
		<-sigChan
		log.Println("[Cloud] 正在优雅关闭(最多五秒）...")

		done := make(chan struct{})
		go func() {
			s.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
			log.Println("[Cloud] 优雅关闭完成")
		case <-time.After(5 * time.Second):
			log.Println("[Cloud] 优雅关闭超时，强制关闭")
			s.Stop()
		}
		s.GracefulStop()
		log.Println("[Cloud] 已退出")
	}()

	// 4. 启动监听（只保留这一段，加错误处理）
	addr := ":50051"
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[Cloud] 监听失败: %v", err)
	}

	log.Printf("[Cloud] 雪毛儿云端启动，监听 %s", addr)

	// 5. 阻塞主线程
	if err := s.Serve(lis); err != nil {
		log.Fatalf("[Cloud] 服务启动失败: %v", err)
	}
}
