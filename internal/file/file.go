package file

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
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
	ID       string
	FilePath string
}

// GenerateID 生成唯一的文件 ID
func GenerateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
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

// SaveFile 保存上传的文件
func SaveFile(file *multipart.FileHeader, uploadDir string) (*FileUpload, error) {
	// 生成唯一 ID
	id := GenerateID()
	fileext := filepath.Ext(file.Filename)
	if fileext == "" {
		fileext = ".config"
	}
	filename := id + fileext
	filepath := filepath.Join(uploadDir, filename)

	// 打开上传的文件
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("打开上传文件失败：%w", err)
	}
	defer src.Close()

	// 创建目标文件
	dst, err := os.Create(filepath)
	if err != nil {
		return nil, fmt.Errorf("创建文件失败：%w", err)
	}
	defer dst.Close()

	// 复制文件内容
	if _, err := io.Copy(dst, src); err != nil {
		return nil, fmt.Errorf("保存文件失败：%w", err)
	}

	return &FileUpload{
		ID:       id,
		FilePath: filepath,
	}, nil
}

// FindFileByID 根据 ID 查找文件
func FindFileByID(uploadDir, id string) (string, error) {
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), id+".") {
			return filepath.Join(uploadDir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("文件不存在")
}
