package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/gin-gonic/gin"
	"subserver/internal/cert"
	"subserver/internal/config"
)

// 服务器超时配置
const (
	readTimeout       = 30 * time.Second  // 读取超时
	readHeaderTimeout = 10 * time.Second  // 读取头部超时
	writeTimeout      = 60 * time.Second  // 写入超时
	idleTimeout       = 120 * time.Second // 空闲连接超时
	maxHeaderBytes    = 4 << 10           // 最大头部大小 4KB
)

// Server 服务器结构
type Server struct {
	config      *config.Config
	router      *gin.Engine
	httpServer  *http.Server
	httpsServer *http.Server
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
	if s.config.EnableTLS {
		return s.startHTTPS()
	}
	return s.startHTTP()
}

// createHTTPServer 创建优化的 HTTP 服务器
func createHTTPServer(addr string, handler http.Handler, isRedirect bool) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		BaseContext: func(listener net.Listener) context.Context {
			ctx := context.Background()
			return context.WithValue(ctx, "addr", addr)
		},
		ConnState: func(conn net.Conn, state http.ConnState) {
			// 连接状态监控，可用于调试
			if state == http.StateNew {
				// 新连接
			} else if state == http.StateClosed {
				// 连接关闭
			}
		},
	}
}

// startHTTP 启动 HTTP 服务器
func (s *Server) startHTTP() error {
	port := s.config.HTTPPort
	addr := ":" + port

	// 创建优化的 HTTP 服务器
	s.httpServer = createHTTPServer(addr, s.router, false)

	// 使用 ListenConfig 启用 TCP 优化
	lc := net.ListenConfig{
		KeepAlive: 3 * time.Minute,
	}
	listener, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return fmt.Errorf("监听端口失败：%w", err)
	}

	log.Printf("服务器启动在 http://localhost:%s", port)
	log.Printf("使用 %d 个 CPU 核心", runtime.NumCPU())

	return s.httpServer.Serve(listener)
}

// startHTTPS 启动 HTTPS 服务器
func (s *Server) startHTTPS() error {
	// 设置 TLS 证书
	if err := cert.SetupTLS(s.config); err != nil {
		return fmt.Errorf("证书设置失败：%w", err)
	}

	domains := s.config.Domains

	// 获取 TLS 配置
	tlsConfig, err := cert.GetTLSConfig(domains)
	if err != nil {
		return fmt.Errorf("TLS 配置失败：%w", err)
	}

	// 优化 TLS 配置
	tlsConfig = optimizeTLSConfig(tlsConfig)

	// 创建 HTTPS 服务器
	httpsAddr := ":" + s.config.HTTPSPort
	s.httpsServer = createHTTPServer(httpsAddr, s.router, false)
	s.httpsServer.TLSConfig = tlsConfig

	// HTTP 服务器 - 用于重定向和 ACME challenge
	httpAddr := ":" + s.config.HTTPPort
	s.httpServer = createHTTPServer(httpAddr, http.HandlerFunc(s.httpRedirectHandler), true)

	// 启动 HTTP 服务器
	go func() {
		log.Printf("HTTP 服务器启动在 http://localhost%s (用于重定向)", httpAddr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP 服务器错误：%v", err)
		}
	}()

	// 创建 HTTPS 监听器，使用 ListenConfig 启用 TCP 优化
	lc := net.ListenConfig{
		KeepAlive: 3 * time.Minute,
	}
	listener, err := lc.Listen(context.Background(), "tcp", httpsAddr)
	if err != nil {
		return fmt.Errorf("监听 HTTPS 端口失败：%w", err)
	}

	// 记录启动信息
	log.Printf("HTTPS 服务器启动在 https://localhost:%s", s.config.HTTPSPort)
	log.Printf("使用 %d 个 CPU 核心", runtime.NumCPU())
	for _, domain := range s.config.Domains {
		log.Printf("  - https://%s:%s", domain, s.config.HTTPSPort)
	}

	// 启动 HTTPS 服务器
	tlsListener := tls.NewListener(listener, tlsConfig)
	return s.httpsServer.Serve(tlsListener)
}

// optimizeTLSConfig 优化 TLS 配置
func optimizeTLSConfig(cfg *tls.Config) *tls.Config {
	// 启用会话票证缓存
	cfg.SessionTicketsDisabled = false

	// 设置首选密码套件（按性能排序）
	cfg.CipherSuites = []uint16{
		tls.TLS_AES_128_GCM_SHA256,       // TLS 1.3
		tls.TLS_AES_256_GCM_SHA384,       // TLS 1.3
		tls.TLS_CHACHA20_POLY1305_SHA256, // TLS 1.3
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,   // TLS 1.2
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,   // TLS 1.2
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,    // TLS 1.2
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, // TLS 1.2
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, // TLS 1.2
	}

	// 设置首选曲线
	cfg.CurvePreferences = []tls.CurveID{
		tls.X25519,
		tls.CurveP256,
	}

	// 最小 TLS 版本
	cfg.MinVersion = tls.VersionTLS12

	return cfg
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

// Shutdown 优雅关闭服务器
func (s *Server) Shutdown(ctx context.Context) error {
	var err error

	if s.httpServer != nil {
		if shutdownErr := s.httpServer.Shutdown(ctx); shutdownErr != nil {
			err = shutdownErr
		}
	}

	if s.httpsServer != nil {
		if shutdownErr := s.httpsServer.Shutdown(ctx); shutdownErr != nil {
			err = shutdownErr
		}
	}

	return err
}
