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

	_ "modernc.org/sqlite"
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
	localCert := flag.Bool("local-tls", false, "使用本地证书文件（而非自动申请证书）")
	certFile := flag.String("cert-file", "", "证书文件路径（与 -local-tls 一起使用）")
	keyFile := flag.String("key-file", "", "私钥文件路径（与 -local-tls 一起使用）")
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
		fmt.Println("  -local-tls       使用本地证书文件（而非自动申请证书）")
		fmt.Println("  -cert-file string 证书文件路径（与 -local-tls 一起使用）")
		fmt.Println("  -key-file string 私钥文件路径（与 -local-tls 一起使用）")
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
		fmt.Println("  subserver -tls -d example.com -tls-email admin@example.com  # 启用 HTTPS (自动证书)")
		fmt.Println("  subserver -tls -local-tls -cert-dir ./certs  # 使用本地证书")
		return
	}

	// 显示版本号
	if *showVersion {
		fmt.Println("SubServer v1.0.1")
		return
	}

	// 构建配置
	cfg := &config.Config{
		HTTPPort:     *httpPort,
		HTTPSPort:    *httpsPort,
		EnableTLS:    *enableTLS,
		UseLocalCert: *localCert,
		CertFile:     *certFile,
		KeyFile:      *keyFile,
		Domains:      parseDomains(*domains),
		TLSEmail:     *tlsEmail,
		DBPath:       *dbPath,
		LogLevel:     *logLevel,
		CertDir:      *certDir,
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
	h := handler.NewHandler(db, cfg.EnableTLS, cfg.HTTPSPort)
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
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// SQLite 优化配置：SQLite 为文件级锁，连接数不宜过多
	db.SetMaxOpenConns(2)           // SQLite 写操作需串行，保持较少连接
	db.SetMaxIdleConns(2)           // 最大空闲连接数
	db.SetConnMaxLifetime(30 * time.Minute) // 连接最大生命周期
	db.SetConnMaxIdleTime(5 * time.Minute)  // 连接最大空闲时间

	// 启用 WAL 模式（支持并发读写）
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("启用 WAL 模式失败：%w", err)
	}

	// 设置忙等待超时（避免 SQLITE_BUSY 错误）
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		db.Close()
		return nil, fmt.Errorf("设置 busy_timeout 失败：%w", err)
	}

	// 启用外键约束
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("启用外键约束失败：%w", err)
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// initSchema 初始化数据库表结构
func initSchema(db *sql.DB) error {
	schema := `
	-- 配置表
	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	-- 用户表
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- 文件表
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

	-- 分享表（关联用户和文件）
	CREATE TABLE IF NOT EXISTS shares (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		file_id TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
	);

	-- 索引
	CREATE INDEX IF NOT EXISTS idx_files_expires ON files(expires_at);
	CREATE INDEX IF NOT EXISTS idx_shares_user_id ON shares(user_id);
	CREATE INDEX IF NOT EXISTS idx_shares_file_id ON shares(file_id);
	CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
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
		log.Printf("panic recovered: %v", recovered)
		c.AbortWithStatusJSON(500, gin.H{"error": "服务器内部错误"})
	}))

	return engine
}
