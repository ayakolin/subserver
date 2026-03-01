//go:build windows

package server

import (
	"net"
	"time"
)

// createListenConfig 创建优化的监听配置（Windows 平台）
func createListenConfig() *net.ListenConfig {
	return &net.ListenConfig{
		KeepAlive: 3 * time.Minute,
		// Windows 不支持 SO_REUSEADDR 的相同用法，跳过
	}
}
