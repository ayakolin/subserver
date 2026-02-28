package handler

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"subserver/internal/file"
)

const (
	uploadDir = "./uploads"
	maxSize   = 1 << 20 // 1MB
)

// Handler HTTP 处理器
type Handler struct {
	uploadDir string
	maxSize   int64
}

// NewHandler 创建新的 Handler
func NewHandler() *Handler {
	return &Handler{
		uploadDir: uploadDir,
		maxSize:   maxSize,
	}
}

// Index 上传页面
func (h *Handler) Index(c *gin.Context) {
	c.File("./index.html")
}

// UploadResponse 上传响应
type UploadResponse struct {
	ID     string `json:"id"`
	RawURL string `json:"raw_url"`
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

	// 保存文件
	upload, err := file.SaveFile(fileHeader, h.uploadDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败"})
		return
	}

	// 返回分享链接
	rawURL := getHost(c) + "/raw/" + upload.ID
	c.JSON(http.StatusOK, UploadResponse{
		ID:     upload.ID,
		RawURL: rawURL,
	})
}

// GetRawFile 获取文件内容
func (h *Handler) GetRawFile(c *gin.Context) {
	id := c.Param("id")

	// 查找文件
	filePath, err := file.FindFileByID(h.uploadDir, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	// 打开文件
	f, err := os.Open(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
		return
	}
	defer f.Close()

	// 获取文件信息
	stat, err := f.Stat()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件信息失败"})
		return
	}

	// 设置响应头
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", "inline")
	c.DataFromReader(http.StatusOK, stat.Size(), "text/plain; charset=utf-8", f, nil)
}

// getHost 获取当前请求的主机
func getHost(c *gin.Context) string {
	host := c.Request.Host
	if host == "" {
		return "http://localhost:8080"
	}
	return "http://" + host
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/", h.Index)
	router.POST("/upload", h.Upload)
	router.GET("/raw/:id", h.GetRawFile)
}
