package handler

import (
	"database/sql"
	"embed"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"subserver/internal/auth"
	"subserver/internal/file"
	"subserver/internal/share"
)

// 确保 sql 包被正确引用
var _ = sql.ErrNoRows

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
	auth      *auth.Auth
	shareMgr  *share.Manager
}

// NewHandler 创建新的 Handler
func NewHandler(db *sql.DB) *Handler {
	// 生成或获取 JWT 密钥
	jwtSecret := getOrCreateJWTSecret(db)

	h := &Handler{
		db:      db,
		maxSize: maxSize,
		jobChan: make(chan file.UploadJob, workerCount*2),
		auth:    auth.NewAuth(db, jwtSecret),
		shareMgr: share.NewManager(db),
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

// getOrCreateJWTSecret 获取或创建 JWT 密钥
func getOrCreateJWTSecret(db *sql.DB) string {
	query := `SELECT value FROM config WHERE key = ?`
	var secret string
	err := db.QueryRow(query, "jwt_secret").Scan(&secret)
	if err == nil {
		return secret
	}

	// 生成新的密钥
	secret, err = auth.GenerateSecureSecret()
	if err != nil {
		secret = "default-secret-change-me"
	}

	// 保存到数据库
	query = `INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)`
	db.Exec(query, "jwt_secret", secret)

	return secret
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

// AuthMiddleware JWT 认证中间件
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Header 获取 token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// 尝试从 cookie 获取
			authHeader, _ = c.Cookie("auth_token")
		}

		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权访问"})
			c.Abort()
			return
		}

		// 去掉 "Bearer " 前缀
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			// 没有 Bearer 前缀，直接使用
			tokenString = authHeader
		}

		// 验证 token
		user, err := h.auth.ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的认证 token"})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Next()
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

	// 尝试从请求中获取用户信息（如果已认证）
	var userID int64
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		authHeader, _ = c.Cookie("auth_token")
	}
	if authHeader != "" {
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		user, err := h.auth.ValidateToken(tokenString)
		if err == nil {
			userID = user.ID
			c.Set("user_id", userID)
			c.Set("username", user.Username)
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

	// 如果是登录用户，创建分享记录
	if userID > 0 {
		_, _ = h.shareMgr.CreateShare(userID, result.Upload.ID)
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

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Register 用户注册
func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名和密码不能为空"})
		return
	}

	if len(req.Username) < 3 || len(req.Username) > 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名长度必须在 3-20 个字符之间"})
		return
	}

	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码长度至少为 6 个字符"})
		return
	}

	user, err := h.auth.Register(req.Username, req.Password)
	if err == auth.ErrUserExists {
		c.JSON(http.StatusConflict, gin.H{"error": "用户名已存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "注册失败"})
		return
	}

	// 生成 token
	token, err := h.auth.GenerateToken(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成 token 失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "注册成功",
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
		},
		"token": token,
	})
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login 用户登录
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	user, err := h.auth.Login(req.Username, req.Password)
	if err == auth.ErrUserNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	if err == auth.ErrInvalidPassword {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "密码错误"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败"})
		return
	}

	// 生成 token
	token, err := h.auth.GenerateToken(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成 token 失败"})
		return
	}

	// 设置 cookie
	c.SetCookie("auth_token", token, int(24*time.Hour.Seconds()), "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"message": "登录成功",
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
		},
		"token": token,
	})
}

// GetUserInfo 获取当前用户信息
func (h *Handler) GetUserInfo(c *gin.Context) {
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")

	c.JSON(http.StatusOK, gin.H{
		"id":       userID,
		"username": username,
	})
}

// Logout 用户登出
func (h *Handler) Logout(c *gin.Context) {
	// 清除 cookie
	c.SetCookie("auth_token", "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "登出成功"})
}

// ShareListItem 分享列表项
type ShareListItem struct {
	ID        int64     `json:"id"`
	FileID    string    `json:"file_id"`
	FileName  string    `json:"file_name"`
	FileSize  int64     `json:"file_size"`
	RawURL    string    `json:"raw_url"`
	ShareURL  string    `json:"share_url"`
	CreatedAt time.Time `json:"created_at"`
}

// GetUserShares 获取用户的分享列表
func (h *Handler) GetUserShares(c *gin.Context) {
	userID, _ := c.Get("user_id")

	shares, err := h.shareMgr.GetUserShares(userID.(int64))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取分享列表失败"})
		return
	}

	host := h.getHost(c)
	items := make([]ShareListItem, 0, len(shares))
	for _, s := range shares {
		item := ShareListItem{
			ID:        s.ID,
			FileID:    s.FileID,
			ShareURL:  host + "/share/" + s.FileID,
			CreatedAt: s.CreatedAt,
		}
		if s.File != nil {
			item.FileName = s.File.Name
			item.FileSize = s.File.Size
			item.RawURL = host + "/raw/" + s.FileID
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"shares": items,
	})
}

// UpdateShareRequest 更新分享请求
type UpdateShareRequest struct {
	Content  string `json:"content,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// UpdateShare 更新分享内容
func (h *Handler) UpdateShare(c *gin.Context) {
	userID, _ := c.Get("user_id")
	fileID := c.Param("file_id")

	// 检查是否是文件上传
	fileHeader, err := c.FormFile("file")
	if err == nil {
		// 文件上传
		if err := file.ValidateFile(fileHeader, h.maxSize); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		src, err := fileHeader.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
			return
		}
		defer src.Close()

		content, err := io.ReadAll(src)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件内容失败"})
			return
		}

		mimeType := file.GetMimeType(fileHeader.Filename)
		if err := h.shareMgr.UpdateShareContent(fileID, fileHeader.Filename, content, mimeType); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
		return
	}

	// 文本输入
	var req UpdateShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if req.Content == "" && req.Filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "内容或文件名不能为空"})
		return
	}

	// 获取原文件信息
	share, err := h.shareMgr.GetShareByID(0, userID.(int64))
	if err != nil {
		// 尝试直接通过 file_id 获取
		query := `SELECT name FROM files WHERE id = ?`
		var filename string
		err := h.db.QueryRow(query, fileID).Scan(&filename)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "分享不存在"})
			return
		}
		filename = req.Filename
		if filename == "" {
			filename = "updated_file.txt"
		}
		content := []byte(req.Content)
		mimeType := "text/plain"

		if err := h.shareMgr.UpdateShareContent(fileID, filename, content, mimeType); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
		return
	}

	filename := req.Filename
	if filename == "" && share.File != nil {
		filename = share.File.Name
	}
	if filename == "" {
		filename = "updated_file.txt"
	}

	content := []byte(req.Content)
	mimeType := "text/plain"

	if err := h.shareMgr.UpdateShareContent(fileID, filename, content, mimeType); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// DeleteShare 删除分享
func (h *Handler) DeleteShare(c *gin.Context) {
	userID, _ := c.Get("user_id")
	shareIDStr := c.Param("id")

	shareID, err := strconv.ParseInt(shareIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分享 ID"})
		return
	}

	if err := h.shareMgr.DeleteShare(shareID, userID.(int64)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// GetShareDetail 获取分享详情
func (h *Handler) GetShareDetail(c *gin.Context) {
	userID, _ := c.Get("user_id")
	fileID := c.Param("file_id")

	// 通过 file_id 查找用户的分享
	query := `
	SELECT s.id, s.file_id,
	       f.name, f.content, f.size, f.mime_type, f.expires_at, f.once, f.read_count
	FROM shares s
	LEFT JOIN files f ON s.file_id = f.id
	WHERE s.file_id = ? AND s.user_id = ?
	`

	var shareID int64
	var fileName sql.NullString
	var content sql.NullString
	var fileSize sql.NullInt64
	var mimeType sql.NullString
	var expiresAt sql.NullTime
	var once bool
	var readCount sql.NullInt64

	err := h.db.QueryRow(query, fileID, userID).Scan(
		&shareID,
		&fileID,
		&fileName,
		&content,
		&fileSize,
		&mimeType,
		&expiresAt,
		&once,
		&readCount,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "分享不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取失败"})
		return
	}

	result := gin.H{
		"id":         shareID,
		"file_id":    fileID,
		"file_name":  fileName.String,
		"file_size":  fileSize.Int64,
		"mime_type":  mimeType.String,
		"read_count": readCount.Int64,
		"once":       once,
		"raw_url":    h.getHost(c) + "/raw/" + fileID,
	}

	if expiresAt.Valid {
		result["expires_at"] = expiresAt.Time
	}

	if content.Valid {
		result["content"] = content.String
	}

	c.JSON(http.StatusOK, result)
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(router *gin.Engine) {
	// 公开路由（支持可选的认证）
	router.GET("/", h.Index)
	router.POST("/upload", h.Upload)
	router.GET("/raw/:id", h.GetRawFile)

	// 认证相关
	router.POST("/api/register", h.Register)
	router.POST("/api/login", h.Login)

	// 需要认证的路由
	api := router.Group("")
	api.Use(h.AuthMiddleware())
	{
		api.GET("/api/user", h.GetUserInfo)
		api.POST("/api/logout", h.Logout)
		api.GET("/api/user/shares", h.GetUserShares)
		api.GET("/api/share/:file_id", h.GetShareDetail)
		api.POST("/api/share/:file_id/update", h.UpdateShare)
		api.DELETE("/api/share/:id", h.DeleteShare)
	}
}
