package file

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// 允许的文件扩展名
var AllowedExts = map[string]bool{
	// YAML
	".yaml": true, ".yml": true,
	// JSON
	".json": true,
	// 纯文本
	".txt": true,
	// TOML
	".toml": true,
	// XML
	".xml": true,
	// INI
	".ini": true,
	// 属性文件
	".properties": true,
	// 环境变量
	".env": true,
	// 其他常见配置文件
	".conf": true, ".cfg": true, ".config": true,
	".rc": true,
	// 数据格式
	".csv": true, ".tsv": true,
	// 脚本类配置
	".sh": true, ".bash": true, ".zsh": true,
	// Makefile
	"makefile": true, "gnumakefile": true,
	// Docker 相关
	"dockerfile": true, ".dockerfile": true,
	// 无扩展名的常见配置文件
	"procfile": true, "gemfile": true, "rakefile": true,
}

// FileUpload 文件上传结果
type FileUpload struct {
	ID        string
	Name      string
	Content   []byte
	Size      int64
	MimeType  string
	CreatedAt time.Time
	ExpiresAt *time.Time
	Once      bool
}

// UploadJob 上传任务
type UploadJob struct {
	File      *multipart.FileHeader
	Once      bool
	ExpiresAt *time.Time
	Result    chan<- UploadResult
}

// UploadResult 上传结果
type UploadResult struct {
	Upload *FileUpload
	Err    error
}

// Cleaner 过期文件清理器
type Cleaner struct {
	db       *sql.DB
	interval time.Duration
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewCleaner 创建清理器
func NewCleaner(db *sql.DB, interval time.Duration) *Cleaner {
	return &Cleaner{
		db:       db,
		interval: interval,
	}
}

// Start 启动清理器
func (c *Cleaner) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Add(1)

	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.cleanExpired()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop 停止清理器
func (c *Cleaner) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
}

// cleanExpired 清理过期文件
func (c *Cleaner) cleanExpired() {
	query := `DELETE FROM files WHERE expires_at IS NOT NULL AND expires_at < ?`
	result, err := c.db.Exec(query, time.Now())
	if err != nil {
		return
	}

	deleted, _ := result.RowsAffected()
	if deleted > 0 {
		fmt.Printf("[Cleaner] 清理了 %d 个过期文件\n", deleted)
	}
}

// GenerateID 生成唯一的文件 ID (GUID 格式)
func GenerateID() string {
	return uuid.New().String()
}

// ValidateFile 验证文件是否合法
func ValidateFile(file *multipart.FileHeader, maxSize int64) error {
	// 检查文件大小
	if file.Size > maxSize {
		return fmt.Errorf("文件大小不能超过 %dMB", maxSize/(1024*1024))
	}

	// 检查文件类型
	ext := strings.ToLower(filepath.Ext(file.Filename))
	filename := strings.ToLower(file.Filename)
	if !AllowedExts[ext] && !AllowedExts[filename] {
		return fmt.Errorf("只能上传常见的配置文件格式 (yaml, yml, json, txt, toml, xml, ini, env 等)")
	}

	return nil
}

// SaveFile 保存上传的文件到数据库
func SaveFile(db *sql.DB, file *multipart.FileHeader, once bool, expiresAt *time.Time) (*FileUpload, error) {
	// 生成唯一 ID
	id := GenerateID()

	// 打开上传的文件
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("打开上传文件失败：%w", err)
	}
	defer src.Close()

	// 读取文件内容
	content, err := io.ReadAll(src)
	if err != nil {
		return nil, fmt.Errorf("读取文件内容失败：%w", err)
	}

	// 保存到数据库
	query := `
	INSERT INTO files (id, name, content, size, mime_type, created_at, expires_at, once, read_count)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)
	`

	_, err = db.Exec(query, id, file.Filename, content, file.Size, getMimeType(file.Filename), time.Now(), expiresAt, once)
	if err != nil {
		return nil, fmt.Errorf("保存文件到数据库失败：%w", err)
	}

	return &FileUpload{
		ID:        id,
		Name:      file.Filename,
		Content:   content,
		Size:      file.Size,
		MimeType:  getMimeType(file.Filename),
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
		Once:      once,
	}, nil
}

// GetFile 从数据库获取文件
func GetFile(db *sql.DB, id string) (*FileUpload, error) {
	// 使用事务来确保原子性
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 先查询文件信息和读取次数
	query := `
	SELECT id, name, content, size, mime_type, created_at, expires_at, once, read_count
	FROM files
	WHERE id = ?
	`

	var f FileUpload
	var once bool
	var readCount int
	var expiresAt sql.NullTime
	err = tx.QueryRow(query, id).Scan(
		&f.ID,
		&f.Name,
		&f.Content,
		&f.Size,
		&f.MimeType,
		&f.CreatedAt,
		&expiresAt,
		&once,
		&readCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	f.Once = once

	// 检查是否过期
	if expiresAt.Valid && time.Now().After(expiresAt.Time) {
		// 文件已过期，删除它
		_, err = tx.Exec(`DELETE FROM files WHERE id = ?`, id)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	// 如果是阅后即焚文件且已经读取过 (read_count >= 1)，返回文件不存在
	if once && readCount >= 1 {
		return nil, nil
	}

	// 增加读取次数
	_, err = tx.Exec(`UPDATE files SET read_count = read_count + 1 WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}

	// 如果是阅后即焚文件，删除文件
	if once {
		_, err = tx.Exec(`DELETE FROM files WHERE id = ?`, id)
		if err != nil {
			return nil, err
		}
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &f, nil
}

// getMimeType 根据文件扩展名获取 MIME 类型
func getMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".yaml", ".yml":
		return "application/x-yaml"
	case ".json":
		return "application/json"
	case ".txt":
		return "text/plain"
	case ".toml":
		return "application/toml"
	case ".xml":
		return "application/xml"
	case ".ini":
		return "text/plain"
	case ".properties":
		return "text/plain"
	case ".env":
		return "text/plain"
	case ".conf", ".cfg", ".config":
		return "text/plain"
	case ".rc":
		return "text/plain"
	case ".csv":
		return "text/csv"
	case ".tsv":
		return "text/tab-separated-values"
	case ".sh", ".bash", ".zsh":
		return "text/x-shellscript"
	case "makefile", "gnumakefile":
		return "text/plain"
	case "dockerfile", ".dockerfile":
		return "text/plain"
	case "procfile", "gemfile", "rakefile":
		return "text/plain"
	default:
		return "text/plain"
	}
}
