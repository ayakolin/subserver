package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
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
	// 加载配置文件
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Printf("警告：%v，使用默认配置", err)
	}

	// 确保数据目录存在
	if err := ensureDataDir(cfg.Database.SQLitePath); err != nil {
		fmt.Printf("无法创建数据目录：%v\n", err)
		os.Exit(1)
	}

	// 初始化数据库
	db, err := initDatabase(cfg.Database)
	if err != nil {
		fmt.Printf("无法初始化数据库：%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 设置 Gin 模式
	if cfg.Log.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建路由
	router := newEngine(cfg)

	// 创建处理器并注册路由
	h := handler.NewHandler(db)
	h.RegisterRoutes(router)

	// 创建并启动服务器
	srv := server.NewServer(cfg, router)
	if err := srv.Start(); err != nil {
		log.Printf("服务器错误：%v", err)
		os.Exit(1)
	}
}

// initDatabase 初始化数据库
func initDatabase(cfg config.DatabaseConfig) (*sql.DB, error) {
	var db *sql.DB
	var err error

	switch cfg.Type {
	case "sqlite":
		db, err = openSQLite(cfg.SQLitePath)
	case "mysql":
		db, err = openMySQL(cfg.MySQL)
	case "postgres":
		db, err = openPostgres(cfg.Postgres)
	default:
		// 默认使用 SQLite
		db, err = openSQLite(cfg.SQLitePath)
	}

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

// openSQLite 打开 SQLite 数据库
func openSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	// SQLite 优化配置
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * 60 * time.Second)

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

// openMySQL 打开 MySQL 数据库
func openMySQL(dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("MySQL DSN 未配置")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * 60 * time.Second)

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

// openPostgres 打开 PostgreSQL 数据库
func openPostgres(dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("PostgreSQL DSN 未配置")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * 60 * time.Second)

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
		once BOOLEAN NOT NULL DEFAULT 0,
		read_count INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_files_id ON files(id);
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
func newEngine(cfg *config.Config) *gin.Engine {
	engine := gin.New()

	// 使用自定义的 Logger 和 Recovery 中间件
	engine.Use(gin.LoggerWithWriter(gin.DefaultWriter))
	engine.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		c.AbortWithStatusJSON(500, gin.H{"error": "服务器内部错误"})
	}))

	return engine
}
