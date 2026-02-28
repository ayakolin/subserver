package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	uploadDir = "./uploads"
	maxSize   = 1 << 20 // 1MB
)

var allowedExts = map[string]bool{
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

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func ensureUploadDir() error {
	return os.MkdirAll(uploadDir, 0755)
}

func main() {
	if err := ensureUploadDir(); err != nil {
		fmt.Printf("无法创建上传目录：%v\n", err)
		os.Exit(1)
	}

	r := gin.Default()

	// 上传页面
	r.GET("/", func(c *gin.Context) {
		c.File("./index.html")
	})

	// 上传 API
	r.POST("/upload", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请选择要上传的文件"})
			return
		}

		// 检查文件大小
		if file.Size > maxSize {
			c.JSON(http.StatusBadRequest, gin.H{"error": "文件大小不能超过 1MB"})
			return
		}

		// 检查文件类型
		ext := strings.ToLower(filepath.Ext(file.Filename))
		filename := strings.ToLower(file.Filename)
		if !allowedExts[ext] && !allowedExts[filename] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "只能上传常见的配置文件格式 (yaml, yml, json, txt, toml, xml, ini, env 等)"})
			return
		}

		// 生成唯一 ID
		id := generateID()
		fileext := filepath.Ext(file.Filename)
		if fileext == "" {
			fileext = ".config"
		}
		filename = id + fileext
		filepath := filepath.Join(uploadDir, filename)

		// 保存文件
		if err := c.SaveUploadedFile(file, filepath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败"})
			return
		}

		// 返回分享链接
		rawURL := fmt.Sprintf("%s/raw/%s", getHost(c), id)
		c.JSON(http.StatusOK, gin.H{
			"id":      id,
			"raw_url": rawURL,
		})
	})

	// Raw 文本访问
	r.GET("/raw/:id", func(c *gin.Context) {
		id := c.Param("id")

		// 在 uploads 目录中查找匹配的文件
		var filePath string
		entries, err := os.ReadDir(uploadDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasPrefix(entry.Name(), id+".") {
					filePath = filepath.Join(uploadDir, entry.Name())
					break
				}
			}
		}

		// 检查文件是否存在
		if filePath == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
			return
		}

		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.Header("Content-Disposition", "inline")
		c.File(filePath)
	})

	port := "8080"
	fmt.Printf("服务器启动在 http://localhost:%s\n", port)
	r.Run(":" + port)
}

func getHost(c *gin.Context) string {
	host := c.Request.Host
	if host == "" {
		return "http://localhost:8080"
	}
	return "http://" + host
}
