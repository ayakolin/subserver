package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"subserver/internal/config"
	"subserver/internal/handler"
	"subserver/internal/server"

	_ "github.com/mattn/go-sqlite3"
)

func init() {
	// 设置 GOMAXPROCS 使用所有可用的 CPU 核心
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	// 定义命令行参数
	httpPort := flag.String("p", "8080", "HTTP 端口")
	httpsPort := flag.String("tls-port", "443", "HTTPS 端口")
	enableTLS := flag.Bool("tls", false, "启用 HTTPS")
	domains := flag.String("d", "", "域名（多个域名用逗号分隔）")
	tlsEmail := flag.String("tls-email", "", "SSL 证书邮箱")
	dbPath := flag.String("db", "./data/subserver.db", "数据库文件路径")
	logLevel := flag.String("log", "info", "日志级别 (debug, info, warn, error)")
	certDir := flag.String("cert-dir", "./certs", "SSL 证书目录")
	showHelp := flag.Bool("h", false, "显示帮助信息")
	showVersion := flag.Bool("v", false, "显示版本号")

	flag.Parse()

	// 显示帮助信息
	if *showHelp {
		fmt.Println("SubServer - 文本文件分享服务器")
		fmt.Println("")
		fmt.Println("用法:")
		fmt.Println("  subserver [选项]")
		fmt.Println("")
		fmt.Println("选项:")
		fmt.Println("  -p string        HTTP 端口 (默认 \"8080\")")
		fmt.Println("  -tls             启用 HTTPS")
		fmt.Println("  -tls-port string HTTPS 端口 (默认 \"443\")")
		fmt.Println("  -d string        域名（多个域名用逗号分隔）")
		fmt.Println("  -tls-email string  SSL 证书邮箱")
		fmt.Println("  -db string       数据库文件路径 (默认 \"./data/subserver.db\")")
		fmt.Println("  -log string      日志级别 (默认 \"info\")")
		fmt.Println("  -cert-dir string SSL 证书目录 (默认 \"./certs\")")
		fmt.Println("  -h               显示帮助信息")
		fmt.Println("  -v               显示版本号")
		fmt.Println("")
		fmt.Println("示例:")
		fmt.Println("  subserver -p 8080                          # 简单启动")
		fmt.Println("  subserver -p 8080 -d example.com          # 指定域名")
		fmt.Println("  subserver -tls -d example.com -tls-email admin@example.com  # 启用 HTTPS")
		return
	}

	// 显示版本号
	if *showVersion {
		fmt.Println("SubServer v1.0.0")
		return
	}

	// 构建配置
	cfg := &config.Config{
		HTTPPort:  *httpPort,
		HTTPSPort: *httpsPort,
		EnableTLS: *enableTLS,
		Domains:   parseDomains(*domains),
		TLSEmail:  *tlsEmail,
		DBPath:    *dbPath,
		LogLevel:  *logLevel,
		CertDir:   *certDir,
	}

	// 确保数据目录存在
	if err := ensureDataDir(cfg.DBPath); err != nil {
		fmt.Printf("无法创建数据目录：%v\n", err)
		os.Exit(1)
	}

	// 初始化数据库（使用优化的连接池配置）
	db, err := initDatabase(cfg.DBPath)
	if err != nil {
		fmt.Printf("无法初始化数据库：%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 设置 Gin 模式
	if cfg.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建路由
	router := newEngine()

	// 创建处理器并注册路由
	h := handler.NewHandler(db)
	h.RegisterRoutes(router)

	// 创建服务器
	srv := server.NewServer(cfg, router)
	srv.SetHandler(h)

	// 设置信号处理实现优雅关闭
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 在后台启动服务器
	go func() {
		if err := srv.Start(); err != nil {
			log.Printf("服务器错误：%v", err)
			os.Exit(1)
		}
	}()

	// 等待退出信号
	sig := <-sigChan
	log.Printf("收到信号 %v，正在优雅关闭...", sig)

	// 创建关闭上下文
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 优雅关闭
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("关闭时出错：%v", err)
	}

	log.Printf("服务器已停止")
}

// parseDomains 解析域名字符串
func parseDomains(domainsStr string) []string {
	if domainsStr == "" {
		return []string{}
	}
	parts := strings.Split(domainsStr, ",")
	domains := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			domains = append(domains, p)
		}
	}
	return domains
}

// initDatabase 初始化数据库
func initDatabase(dbPath string) (*sql.DB, error) {
	db, err := openSQLite(dbPath)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败：%w", err)
	}

	// 初始化 schema
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化数据库 schema 失败：%w", err)
	}

	return db, nil
}

// openSQLite 打开 SQLite 数据库（优化连接池）
func openSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	// SQLite 优化配置（高并发）
	db.SetMaxOpenConns(100)         // 最大打开连接数
	db.SetMaxIdleConns(25)          // 最大空闲连接数
	db.SetConnMaxLifetime(30 * time.Minute) // 连接最大生命周期
	db.SetConnMaxIdleTime(5 * time.Minute)  // 连接最大空闲时间

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

// initSchema 初始化数据库表结构
func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS files (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		content BLOB NOT NULL,
		size INTEGER NOT NULL,
		mime_type TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME,
		once BOOLEAN NOT NULL DEFAULT 0,
		read_count INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_files_id ON files(id);
	CREATE INDEX IF NOT EXISTS idx_files_expires ON files(expires_at);
	`

	_, err := db.Exec(schema)
	return err
}

// ensureDataDir 确保数据目录存在
func ensureDataDir(dbPath string) error {
	dir := filepath.Dir(dbPath)
	return os.MkdirAll(dir, 0755)
}

// newEngine 创建优化的 Gin 引擎
func newEngine() *gin.Engine {
	engine := gin.New()

	// 使用自定义的 Logger 和 Recovery 中间件
	engine.Use(gin.LoggerWithWriter(gin.DefaultWriter))
	engine.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		c.AbortWithStatusJSON(500, gin.H{"error": "服务器内部错误"})
	}))

	return engine
}
