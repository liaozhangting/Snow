// Package config 提供边缘端配置管理
//
// Java对照: 类似于 Spring Boot 的 @ConfigurationProperties
// 但Go没有自动绑定，需要手动读取环境变量
package config

import (
	"os"
)

// EdgeConfig 边缘端配置
type EdgeConfig struct {
	DeviceID  string // 设备唯一标识
	CloudAddr string // 云端gRPC地址
}

// LoadEdgeConfig 加载配置
// TODO: 后续支持从配置文件读取
func LoadEdgeConfig() *EdgeConfig {
	return &EdgeConfig{
		DeviceID:  getEnv("DEVICE_ID", "snow-edge-001"),
		CloudAddr: getEnv("CLOUD_ADDR", "localhost:50051"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
