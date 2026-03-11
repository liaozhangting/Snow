// Package config 提供边缘端配置管理
//
// Java对照: 类似于 Spring Boot 的 @ConfigurationProperties
// 但Go没有自动绑定，需要手动读取环境变量
package config

import (
	"os"
	"strconv"
	"time"
)

// EdgeConfig 边缘端配置
type EdgeConfig struct {
	// Day 1-2 已有
	DeviceID  string // 设备唯一标识
	CloudAddr string // 云端gRPC地址

	// Day 3 新增
	WorkerCount   int           // Pool大小，默认10
	QueueSize     int           // 任务队列长度，默认1000
	BatchSize     int           // 批量条数，默认100
	FlushInterval time.Duration // 刷新间隔(ms)，默认100
	TaskCount     int           // 模拟任务数，默认10000
}

// LoadEdgeConfig 加载配置
// TODO: 后续支持从配置文件读取
func LoadEdgeConfig() *EdgeConfig {
	return &EdgeConfig{
		DeviceID:      getEnv("DEVICE_ID", "snow-edge-001"),
		CloudAddr:     getEnv("CLOUD_ADDR", "localhost:50051"),
		WorkerCount:   getEnvInt("WORKER_COUNT", 10),
		QueueSize:     getEnvInt("QUEUE_SIZE", 1000),
		BatchSize:     getEnvInt("BATCH_SIZE", 100),
		FlushInterval: getEnvDuration("FLUSH_INTERVAL_MS", 100*time.Millisecond),
		TaskCount:     getEnvInt("TASK_COUNT", 10000),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if ms, err := strconv.Atoi(val); err == nil {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return defaultVal
}
