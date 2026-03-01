//go:build linux || darwin

package server

import (
	"net"
	"syscall"
	"time"
)

// createListenConfig 创建优化的监听配置（Unix 平台）
func createListenConfig() *net.ListenConfig {
	return &net.ListenConfig{
		KeepAlive: 3 * time.Minute,
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// 启用 SO_REUSEADDR
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
		},
	}
}
