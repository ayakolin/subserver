package main

import (
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/gin-gonic/gin"
	"subserver/internal/config"
	"subserver/internal/handler"
	"subserver/internal/server"
)

func init() {
	// 设置 GOMAXPROCS 使用所有可用的 CPU 核心
	runtime.GOMAXPROCS(runtime.NumCPU())
}

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
	router := newEngine(cfg)

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

// newEngine 创建优化的 Gin 引擎
func newEngine(cfg *config.Config) *gin.Engine {
	engine := gin.New()

	// 使用自定义的 Logger 和 Recovery 中间件
	engine.Use(gin.LoggerWithWriter(gin.DefaultWriter))
	engine.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		c.AbortWithStatusJSON(500, gin.H{"error": "服务器内部错误"})
	}))

	return engine
}

// ensureUploadDir 确保上传目录存在
func ensureUploadDir() error {
	return os.MkdirAll("./uploads", 0755)
}
