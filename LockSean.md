```markdown
# Cloud-Edge Inference System 开发规范

**版本**: v1.0  
**适用**: 所有Agent代码生成任务  
**目标**: 生成符合Go惯用法、可面试展示、Java开发者易理解的代码

---

## 一、代码风格规范

### 1.1 文件头模板

每个`.go`文件必须以以下注释开头：

```go
// Package xxx 提供xxx功能
// 
// Java对照: 类似于Java的xxx包，但注意:
// 1. Go没有public/private，首字母大写=导出，小写=包内私有
// 2. 没有继承，用组合(composition)代替
//
// TODO: [明确标记需要人工填充的部分]
package xxx

import (
    // 标准库
    "context"
    "time"
    
    // 第三方库
    "google.golang.org/grpc"
    
    // 项目内部
    "inference-log-system/edge/config"
)
```

### 1.2 命名规范

| 类型 | 规范 | 示例 | 反例 |
|:---|:---|:---|:---|
| 包名 | 全小写，简短 | `worker`, `config` | `WorkerPool`, `config_util` |
| 接口 | 动词+er 或 名词 | `Logger`, `TaskProcessor` | `ILogger`, `TaskProcessInterface` |
| 结构体 | 名词，大驼峰 | `InferenceTask`, `GRPCClient` | `inferenceTask`, `GrpcClient` |
| 方法 | 动词开头，大驼峰 | `ProcessTask`, `SendLogs` | `processTask`, `send_logs` |
| 变量 | 驼峰，简短 | `taskQueue`, `maxSize` | `task_queue`, `max_size` |
| 常量 | 大驼峰或全大写 | `MaxRetryCount`, `DefaultTimeout` | `maxRetry`, `DEFAULT_TIMEOUT` |
| 错误变量 | 以Err开头 | `ErrConnectionLost`, `ErrQueueFull` | `ConnectionError` |

### 1.3 代码格式

```go
// 必须: gofmt 格式化后的风格
// 必须: 使用tab缩进（不是空格）
// 必须: 行长度<120字符
// 必须: import分组：标准库/第三方/项目内部

// 正确示例
func (p *Pool) Submit(task Task) error {
    select {
    case p.taskQueue <- task:
        return nil
    case <-time.After(p.timeout):
        return ErrQueueFull  // 错误变量命名规范
    }
}

// 错误示例（不要这样写）
func (self *Pool) submit(task Task) error {  // 不要用self，方法名小写
    if len(self.taskQueue) >= cap(self.taskQueue) {  // 不要手动检查长度
        return errors.New("queue is full")  // 不要用匿名错误
    }
    self.taskQueue <- task
    return nil
}
```

---

## 二、并发编程规范（核心）

### 2.1 Goroutine管理

```go
// 必须: 每个Goroutine都要有退出机制
// 必须: 使用sync.WaitGroup等待Goroutine结束
// 必须: 禁止裸go func()，要封装在方法里

// 正确示例
func (p *Pool) Start() {
    for i := 0; i < p.workers; i++ {
        p.wg.Add(1)
        go p.runWorker(i)  // 封装在方法里，有名字
    }
}

func (p *Pool) runWorker(id int) {
    defer p.wg.Done()  // 必须: 确保计数器减1
    for {
        select {
        case task := <-p.taskQueue:
            p.process(task)
        case <-p.ctx.Done():
            log.Printf("Worker %d stopping", id)
            return  // 必须: 响应Context取消
        }
    }
}

// 必须: 提供Stop方法
func (p *Pool) Stop() {
    p.cancel()  // 触发Context取消
    p.wg.Wait() // 等待所有worker退出
}
```

### 2.2 Channel使用

```go
// 必须: 明确Channel所有权（谁写谁关）
// 必须: 用select处理多路复用
// 必须: 有缓冲Channel要说明容量设计依据

// 正确示例
type LogBuffer struct {
    logs     chan *pb.InferenceLog  // 只写（外部）+ 只读（内部）
    maxSize  int
    interval time.Duration
}

func NewLogBuffer(maxSize int, interval time.Duration) *LogBuffer {
    return &LogBuffer{
        logs:     make(chan *pb.InferenceLog, maxSize), // 有缓冲，防阻塞
        maxSize:  maxSize,
        interval: interval,
    }
}

// 发送（外部调用）
func (b *LogBuffer) Send(log *pb.InferenceLog) bool {
    select {
    case b.logs <- log:
        return true
    default:
        return false // 缓冲满，丢弃或报错
    }
}

// 消费（内部Goroutine）
func (b *LogBuffer) StartFlusher(sender func([]*pb.InferenceLog)) {
    ticker := time.NewTicker(b.interval)
    defer ticker.Stop()
    
    batch := make([]*pb.InferenceLog, 0, b.maxSize)
    
    for {
        select {
        case log := <-b.logs:
            batch = append(batch, log)
            if len(batch) >= b.maxSize {
                sender(batch)
                batch = batch[:0] // 重置，复用底层数组
            }
        case <-ticker.C:
            if len(batch) > 0 {
                sender(batch)
                batch = batch[:0]
            }
        }
    }
}
```

### 2.3 同步原语选择

| 场景 | 推荐方案 | Java对照 | 禁止 |
|:---|:---|:---|:---|
| 共享状态保护 | `sync.Mutex` | `synchronized` | 不要混用chan和mutex |
| 只读配置 | `atomic.Value` | `volatile` | 不要每次读都加锁 |
| 单次初始化 | `sync.Once` | 静态块 | 不要用`init()`函数做复杂逻辑 |
| 等待多个任务 | `sync.WaitGroup` | `CountDownLatch` | 不要sleep轮询 |
| 通知/唤醒 | `chan struct{}` | `wait/notify` | 不要用`sync.Cond`（复杂）|

---

## 三、错误处理规范

### 3.1 错误定义

```go
// 必须: 预定义错误变量，不要匿名
// 位置: 包级别的errors.go或放在相关文件顶部

package worker

import "errors"

var (
    ErrPoolClosed    = errors.New("worker pool is closed")
    ErrQueueFull     = errors.New("task queue is full")
    ErrTaskTimeout   = errors.New("task execution timeout")
    ErrInvalidConfig = errors.New("invalid pool configuration")
)
```

### 3.2 错误处理模式

```go
// 必须: 错误立即检查，不要累积
// 必须: 错误包装要添加上下文

// 正确示例
func (c *GRPCClient) SendBatch(logs []*pb.InferenceLog) error {
    if len(logs) == 0 {
        return nil // 提前返回，减少嵌套
    }
    
    ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
    defer cancel()
    
    stream, err := c.client.StreamLogs(ctx)
    if err != nil {
        return fmt.Errorf("create stream failed: %w", err) // 包装错误
    }
    
    for _, log := range logs {
        if err := stream.Send(log); err != nil {
            return fmt.Errorf("send log %s failed: %w", log.TaskId, err)
        }
    }
    
    if _, err := stream.CloseAndRecv(); err != nil {
        return fmt.Errorf("close stream failed: %w", err)
    }
    
    return nil
}

// 错误示例（不要这样写）
func (c *GRPCClient) SendBatch(logs []*pb.InferenceLog) error {
    ctx, _ := context.WithTimeout(c.ctx, 5*time.Second) // 错误: 没cancel
    stream, err := c.client.StreamLogs(ctx)
    // 错误: 没检查err就用stream
    for _, log := range logs {
        stream.Send(log) // 错误: 没处理错误
    }
    return nil
}
```

### 3.3 日志规范

```go
// 必须: 使用标准库log或结构化日志（logrus/zap）
// 必须: 关键路径有日志，但不要每条都打印

// 正确示例
import "log"

func (s *Server) Start() error {
    log.Printf("[Server] Starting on %s", s.addr) // 生命周期事件
    
    for {
        conn, err := s.listener.Accept()
        if err != nil {
            if s.isClosed() {
                log.Printf("[Server] Stopped gracefully")
                return nil
            }
            log.Printf("[Server] Accept error: %v", err) // 错误要记录
            continue
        }
        
        // 不要: log.Printf("New connection from %s", conn.RemoteAddr())
        // 太频繁，用metrics代替
        go s.handle(conn)
    }
}
```

---

## 四、gRPC规范

### 4.1 服务定义

```protobuf
// api/proto/log.proto
syntax = "proto3";
package logservice;

option go_package = "inference-log-system/api/proto";

import "google/protobuf/timestamp.proto";

message InferenceLog {
    string device_id = 1;
    string task_id = 2;
    int64 timestamp_ms = 3;  // 用int64存时间戳，比Timestamp省空间
    Status status = 4;
    int32 latency_ms = 5;
    int32 output_size = 6;
    string error_msg = 7;    // 失败时填充
    
    enum Status {
        UNKNOWN = 0;
        SUCCESS = 1;
        FAILED = 2;
        TIMEOUT = 3;
    }
}

message Ack {
    string batch_id = 1;
    int32 received_count = 2;
}

service LogService {
    // 客户端流式：Edge -> Cloud
    rpc StreamLogs(stream InferenceLog) returns (Ack);
    
    // 如果需要双向流，后续扩展
    // rpc BidirectionalStream(stream InferenceLog) returns (stream Command);
}
```

### 4.2 客户端实现

```go
// edge/client/grpc_client.go
package client

import (
    "context"
    "io"
    "sync"
    "time"
    
    "google.golang.org/grpc"
    "google.golang.org/grpc/connectivity"
    "google.golang.org/grpc/credentials/insecure"
    
    pb "inference-log-system/api/proto"
)

type GRPCClient struct {
    addr       string
    conn       *grpc.ClientConn
    client     pb.LogServiceClient
    ctx        context.Context
    cancel     context.CancelFunc
    mu         sync.RWMutex // 保护conn状态
    reconnect  chan struct{}
}

func NewGRPCClient(addr string) *GRPCClient {
    ctx, cancel := context.WithCancel(context.Background())
    return &GRPCClient{
        addr:      addr,
        ctx:       ctx,
        cancel:    cancel,
        reconnect: make(chan struct{}, 1),
    }
}

// Connect 建立连接，支持重连
func (c *GRPCClient) Connect() error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    if c.conn != nil && c.conn.GetState() == connectivity.Ready {
        return nil
    }
    
    conn, err := grpc.Dial(c.addr,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithBlock(),
        grpc.WithTimeout(5*time.Second),
    )
    if err != nil {
        return err
    }
    
    c.conn = conn
    c.client = pb.NewLogServiceClient(conn)
    return nil
}

// SendStream 流式发送，带重连逻辑
func (c *GRPCClient) SendStream(logs []*pb.InferenceLog) error {
    // TODO: 实现重连逻辑（用户Day 5填充）
    // 提示: 用for循环+c.breaker模式
    return nil
}

func (c *GRPCClient) Close() error {
    c.cancel()
    c.mu.Lock()
    defer c.mu.Unlock()
    if c.conn != nil {
        return c.conn.Close()
    }
    return nil
}
```

---

## 五、Kafka规范

### 5.1 生产者配置

```go
// cloud/producer/kafka.go
package producer

import (
    "context"
    "encoding/json"
    "time"
    
    "github.com/segmentio/kafka-go"
)

type LogProducer struct {
    writer *kafka.Writer
    // TODO: 批量缓冲（用户Day 4填充）
}

func NewLogProducer(brokers []string, topic string) *LogProducer {
    return &LogProducer{
        writer: &kafka.Writer{
            Addr:         kafka.TCP(brokers...),
            Topic:        topic,
            Balancer:     &kafka.LeastBytes{}, // 最小字节负载均衡
            BatchSize:    100,                 // 批量100条
            BatchBytes:   1024 * 1024,         // 或1MB
            BatchTimeout: 100 * time.Millisecond, // 或100ms
            RequiredAcks: kafka.RequireOne,    // 至少一个副本确认
            Async:        true,                // 异步生产，高性能
        },
    }
}

func (p *LogProducer) SendLog(log *InferenceLog) error {
    data, err := json.Marshal(log)
    if err != nil {
        return err
    }
    
    return p.writer.WriteMessages(context.Background(),
        kafka.Message{
            Key:   []byte(log.DeviceId), // 相同device_id进同一分区
            Value: data,
            Time:  time.Now(),
        },
    )
}

func (p *LogProducer) Close() error {
    return p.writer.Close()
}
```

---

## 六、配置管理规范

```go
// config/config.go
package config

import (
    "os"
    "strconv"
    "time"
)

type EdgeConfig struct {
    DeviceID        string
    CloudAddr       string
    WorkerCount     int
    QueueSize       int
    BatchSize       int
    FlushInterval   time.Duration
    MaxRetryCount   int
}

func LoadEdgeConfig() *EdgeConfig {
    return &EdgeConfig{
        DeviceID:      getEnv("DEVICE_ID", "edge-001"),
        CloudAddr:     getEnv("CLOUD_ADDR", "localhost:50051"),
        WorkerCount:   getEnvInt("WORKER_COUNT", 10),
        QueueSize:     getEnvInt("QUEUE_SIZE", 1000),
        BatchSize:     getEnvInt("BATCH_SIZE", 100),
        FlushInterval: getEnvDuration("FLUSH_INTERVAL", 100*time.Millisecond),
        MaxRetryCount: getEnvInt("MAX_RETRY", 3),
    }
}

// 辅助函数
func getEnv(key, defaultVal string) string { /* ... */ }
func getEnvInt(key string, defaultVal int) int { /* ... */ }
func getEnvDuration(key string, defaultVal time.Duration) time.Duration { /* ... */ }
```

---

## 七、测试与文档规范

### 7.1 单元测试模板

```go
// worker/pool_test.go
package worker

import (
    "context"
    "testing"
    "time"
)

func TestPool_Submit(t *testing.T) {
    // 必须: 表驱动测试
    tests := []struct {
        name      string
        workers   int
        queueSize int
        tasks     int
        wantErr   bool
    }{
        {"normal", 2, 10, 5, false},
        {"queue_full", 1, 1, 10, true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            p := NewPool(tt.workers, tt.queueSize)
            p.Start()
            defer p.Stop()
            
            // 测试逻辑...
        })
    }
}

// 必须: 基准测试（面试用）
func BenchmarkPool(b *testing.B) {
    p := NewPool(10, 1000)
    p.Start()
    defer p.Stop()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        p.Submit(Task{ID: strconv.Itoa(i)})
    }
}
```

### 7.2 README结构

```markdown
# Cloud-Edge Inference Log System

## 架构图
[粘贴架构图]

## 快速开始
```bash
docker-compose up -d
go run edge/main.go
go run cloud/main.go
```

## 性能数据
- 单机支持边缘节点: 10,000+
- 日志吞吐: 50,000 logs/s
- 内存占用: < 200MB (10K连接)

## 技术亮点
1. Goroutine池 vs Java线程池...
2. Kafka批量提交优化...
3. gRPC流控与重连...

## 面试Q&A
见docs/interview.md
```

---

## 八、禁止清单（DO NOT）

| 禁止项 | 原因 | 替代方案 |
|:---|:---|:---|
| 使用`init()`函数 | 隐式执行，难以测试 | 显式`NewXxx()`函数 |
| 使用`panic()` | 不可控崩溃 | 返回`error` |
| 使用`os.Exit()` | 无法优雅关闭 | `context.Cancel` |
| 裸`go func()` | 无法追踪、泄露 | 封装+WaitGroup |
| 忽略`error`返回值 | 静默失败 | 必须处理或显式`_`丢弃 |
| 全局变量 | 并发不安全、难测试 | 依赖注入 |
| `time.Sleep()`轮询 | 不精确、浪费CPU | `time.Ticker`或`chan` |
| 大写的`JSON`/`XML`字段 | Go惯用法是小写 | `json:"field_name"` |

---

## 九、Checklist（Agent自检）

每次提交代码前确认：

- [ ] 所有文件有规范头注释
- [ ] `gofmt`格式化通过
- [ ] `go vet`无警告
- [ ] 没有裸Goroutine，都有退出机制
- [ ] 错误都处理，没有`panic`
- [ ] 关键逻辑有Java对照注释
- [ ] TODO标记清晰，说明用户需要填什么

---

## 十、Java→Go快速对照

| Java概念 | Go对应 | 注意点 |
|:---|:---|:---|
| `ThreadPoolExecutor` | `Goroutine` + `Channel` + `sync.WaitGroup` | 没有固定池，用有缓冲Channel做任务队列 |
| `BlockingQueue` | 有缓冲`chan` | `ch := make(chan Task, 100)` |
| `synchronized`/`ReentrantLock` | `sync.Mutex` | 更简单，但没有读写锁分离 |
| `CompletableFuture` | `chan` + ` Goroutine` | 用chan传结果，而不是回调 |
| `Stream` | 没有，用`for`循环 | Go偏向命令式，少函数式 |
| `try-catch-finally` | `if err != nil` + `defer` | 每个错误显式处理 |
| `Spring Boot`配置 | 直接读`os.Getenv`或YAML库 | 没有依赖注入，手动初始化 |

---

**使用说明**：将此文档作为System Prompt的一部分，每次生成代码前要求Agent"请阅读并遵循《开发规范》第X、Y、Z条"。
```