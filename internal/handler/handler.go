package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"subserver/internal/file"
)

const (
	maxSize = 1 << 20 // 1MB
)

// Handler HTTP 处理器
type Handler struct {
	db      *sql.DB
	maxSize int64
}

// NewHandler 创建新的 Handler
func NewHandler(db *sql.DB) *Handler {
	return &Handler{
		db:      db,
		maxSize: maxSize,
	}
}

// UploadResponse 上传响应
type UploadResponse struct {
	ID     string `json:"id"`
	RawURL string `json:"raw_url"`
	Once   bool   `json:"once,omitempty"`
}

// Upload 文件上传处理
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

	// 保存文件到数据库
	upload, err := file.SaveFile(h.db, fileHeader, once)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败"})
		return
	}

	// 返回分享链接
	rawURL := getHost(c) + "/raw/" + upload.ID
	c.JSON(http.StatusOK, UploadResponse{
		ID:     upload.ID,
		RawURL: rawURL,
		Once:   once,
	})
}

// GetRawFile 获取文件内容
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

// getHost 获取当前请求的主机
func getHost(c *gin.Context) string {
	host := c.Request.Host
	if host == "" {
		return "http://localhost:8080"
	}
	return "http://" + host
}

// Index 上传页面
func (h *Handler) Index(c *gin.Context) {
	c.File("./index.html")
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/", h.Index)
	router.POST("/upload", h.Upload)
	router.GET("/raw/:id", h.GetRawFile)
}
