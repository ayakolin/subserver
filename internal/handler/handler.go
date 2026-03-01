package handler

import (
	"database/sql"
	"embed"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"subserver/internal/file"
)

//go:embed static/index.html
var indexHTML embed.FS

const (
	maxSize     = 1 << 20 // 1MB
	workerCount = 8       // 工作协程数量
)

// Handler HTTP 处理器
type Handler struct {
	db        *sql.DB
	maxSize   int64
	uploadWg  sync.WaitGroup
	jobChan   chan file.UploadJob
	cleaner   *file.Cleaner
	hostCache string
	hostMu    sync.RWMutex
}

// NewHandler 创建新的 Handler
func NewHandler(db *sql.DB) *Handler {
	h := &Handler{
		db:      db,
		maxSize: maxSize,
		jobChan: make(chan file.UploadJob, workerCount*2),
	}

	// 启动工作池
	for i := 0; i < workerCount; i++ {
		go h.uploadWorker()
	}

	// 启动过期文件清理器（每 5 分钟清理一次）
	h.cleaner = file.NewCleaner(db, 5*time.Minute)
	h.cleaner.Start()

	return h
}

// uploadWorker 文件上传工作协程
func (h *Handler) uploadWorker() {
	for job := range h.jobChan {
		upload, err := file.SaveFile(h.db, job.File, job.Once, job.ExpiresAt)
		job.Result <- file.UploadResult{
			Upload: upload,
			Err:    err,
		}
	}
}

// Shutdown 优雅关闭处理器
func (h *Handler) Shutdown() {
	close(h.jobChan)
	h.uploadWg.Wait()
	if h.cleaner != nil {
		h.cleaner.Stop()
	}
}

// UploadResponse 上传响应
type UploadResponse struct {
	ID        string `json:"id"`
	RawURL    string `json:"raw_url"`
	Once      bool   `json:"once,omitempty"`
	ExpiresAt *int64 `json:"expires_at,omitempty"`
}

// Upload 文件上传处理（异步）
func (h *Handler) Upload(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择要上传的文件"})
		return
	}

	// 验证文件
	if err := file.ValidateFile(fileHeader, h.maxSize); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取阅后即焚选项
	once := c.PostForm("once") == "true"

	// 获取过期时间选项（秒）
	var expiresAt *time.Time
	expireSeconds := c.PostForm("expire_seconds")
	if expireSeconds != "" {
		seconds, err := strconv.Atoi(expireSeconds)
		if err == nil && seconds > 0 {
			t := time.Now().Add(time.Duration(seconds) * time.Second)
			expiresAt = &t
		}
	}

	// 创建结果通道
	resultChan := make(chan file.UploadResult, 1)

	// 发送任务到工作池
	h.jobChan <- file.UploadJob{
		File:      fileHeader,
		Once:      once,
		ExpiresAt: expiresAt,
		Result:    resultChan,
	}

	// 等待结果
	result := <-resultChan
	if result.Err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败"})
		return
	}

	// 返回分享链接
	rawURL := h.getHost(c) + "/raw/" + result.Upload.ID
	response := UploadResponse{
		ID:     result.Upload.ID,
		RawURL: rawURL,
		Once:   once,
	}
	if expiresAt != nil {
		timestamp := expiresAt.Unix()
		response.ExpiresAt = &timestamp
	}
	c.JSON(http.StatusOK, response)
}

// GetRawFile 获取文件内容（使用只读查询优化）
func (h *Handler) GetRawFile(c *gin.Context) {
	id := c.Param("id")

	// 从数据库获取文件
	f, err := file.GetFile(h.db, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	// 文件不存在（可能是阅后即焚文件已被读取）
	if f == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	// 设置响应头
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", "inline")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", f.Content)
}

// getHost 获取当前请求的主机（带缓存）
func (h *Handler) getHost(c *gin.Context) string {
	h.hostMu.RLock()
	if h.hostCache != "" {
		h.hostMu.RUnlock()
		return h.hostCache
	}
	h.hostMu.RUnlock()

	host := c.Request.Host
	if host == "" {
		host = "http://localhost:8080"
	} else {
		host = "http://" + host
	}

	h.hostMu.Lock()
	h.hostCache = host
	h.hostMu.Unlock()

	return host
}

// Index 上传页面
func (h *Handler) Index(c *gin.Context) {
	content, err := indexHTML.ReadFile("static/index.html")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法加载页面"})
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", content)
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/", h.Index)
	router.POST("/upload", h.Upload)
	router.GET("/raw/:id", h.GetRawFile)
}
