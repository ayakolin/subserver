package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/caddyserver/certmagic"
	"github.com/gin-gonic/gin"
	"subserver/internal/cert"
	"subserver/internal/config"
)

// Server 服务器结构
type Server struct {
	config *config.Config
	router *gin.Engine
}

// NewServer 创建新的服务器
func NewServer(cfg *config.Config, router *gin.Engine) *Server {
	return &Server{
		config: cfg,
		router: router,
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	if s.config.TLS.Enabled {
		return s.startHTTPS()
	}
	return s.startHTTP()
}

// startHTTP 启动 HTTP 服务器
func (s *Server) startHTTP() error {
	port := s.config.Server.HTTPPort
	log.Printf("服务器启动在 http://localhost:%s", port)
	return s.router.Run(":" + port)
}

// startHTTPS 启动 HTTPS 服务器
func (s *Server) startHTTPS() error {
	// 设置 TLS 证书
	if err := cert.SetupTLS(s.config); err != nil {
		return fmt.Errorf("证书设置失败：%w", err)
	}

	domains := s.config.Server.Domains

	// 获取 TLS 配置
	tlsConfig, err := cert.GetTLSConfig(domains)
	if err != nil {
		return fmt.Errorf("TLS 配置失败：%w", err)
	}

	// 创建 HTTPS 服务器
	httpsAddr := ":" + s.config.Server.HTTPSPort
	httpsServer := &http.Server{
		Addr:      httpsAddr,
		Handler:   s.router,
		TLSConfig: tlsConfig,
	}

	// HTTP 服务器 - 用于重定向
	httpAddr := ":" + s.config.Server.HTTPPort
	httpServer := &http.Server{
		Addr:    httpAddr,
		Handler: http.HandlerFunc(s.httpRedirectHandler),
	}

	// 启动 HTTP 服务器
	go func() {
		log.Printf("HTTP 服务器启动在 http://localhost%s (用于重定向)", httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP 服务器错误：%v", err)
		}
	}()

	// 记录启动信息
	log.Printf("HTTPS 服务器启动在 https://localhost:%s", s.config.Server.HTTPSPort)
	for _, domain := range s.config.Server.Domains {
		log.Printf("  - https://%s:%s", domain, s.config.Server.HTTPSPort)
	}

	// 启动 HTTPS 服务器
	return httpsServer.ListenAndServeTLS("", "")
}

// httpRedirectHandler HTTP 重定向到 HTTPS
func (s *Server) httpRedirectHandler(w http.ResponseWriter, r *http.Request) {
	// 跳过 ACME challenge
	if certmagic.LooksLikeHTTPChallenge(r) {
		certmagic.DefaultACME.HTTPChallengeHandler(http.NotFoundHandler()).ServeHTTP(w, r)
		return
	}

	toURL := "https://" + r.Host + r.URL.RequestURI()
	http.Redirect(w, r, toURL, http.StatusMovedPermanently)
}

// Shutdown 关闭服务器
func (s *Server) Shutdown(ctx context.Context) error {
	// 可以添加服务器关闭逻辑
	return nil
}
