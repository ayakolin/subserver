package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"subserver/internal/config"
	"subserver/internal/handler"
	"subserver/internal/server"
)

func main() {
	// 加载配置文件
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Printf("警告：%v，使用默认配置", err)
	}

	// 确保上传目录存在
	if err := ensureUploadDir(); err != nil {
		fmt.Printf("无法创建上传目录：%v\n", err)
		os.Exit(1)
	}

	// 设置 Gin 模式
	if cfg.Log.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建路由
	router := gin.Default()

	// 创建处理器并注册路由
	h := handler.NewHandler()
	h.RegisterRoutes(router)

	// 创建并启动服务器
	srv := server.NewServer(cfg, router)
	if err := srv.Start(); err != nil {
		log.Printf("服务器错误：%v", err)
		os.Exit(1)
	}
}

// ensureUploadDir 确保上传目录存在
func ensureUploadDir() error {
	return os.MkdirAll("./uploads", 0755)
}
