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

	"google.golang.org/grpc"

	"github.com/liaozhangting/Snow/api"
)

// LogServer 实现 LogService 接口
type LogServer struct {
	api.UnimplementedLogServiceServer
}

// UploadLogs 接收客户端流式日志
func (s *LogServer) UploadLogs(stream api.LogService_UploadLogsServer) error {
	count := 0
	log.Printf("[Cloud] 新的边缘端连接建立")

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			// 客户端发送完毕
			log.Printf("[Cloud] 连接关闭，共接收 %d 条日志", count)
			return stream.SendAndClose(&api.LogResponse{
				Message:       "接收成功",
				ReceivedCount: int32(count),
			})
		}
		if err != nil {
			log.Printf("[Cloud] 接收错误: %v", err)
			return err
		}

		count++
		log.Printf("[Cloud] 收到日志 [%s]: %s", req.DeviceId, req.Content)
	}
}

func main() {
	addr := ":50051"
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("监听失败: %v", err)
	}

	s := grpc.NewServer()
	api.RegisterLogServiceServer(s, &LogServer{})

	log.Printf("[Cloud] 雪毛儿云端启动，监听 %s", addr)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
