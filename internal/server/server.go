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
	"subserver/internal/handler"
)

// 服务器超时配置（优化高并发）
const (
	readTimeout       = 15 * time.Second  // 读取超时
	readHeaderTimeout = 5 * time.Second   // 读取头部超时
	writeTimeout      = 30 * time.Second  // 写入超时
	idleTimeout       = 60 * time.Second  // 空闲连接超时
	maxHeaderBytes    = 1 << 20           // 最大头部大小 1MB
)

// contextKey context 键类型（避免使用 string 导致的键冲突）
type contextKey string

const (
	ctxKeyAddr      contextKey = "addr"
	ctxKeyLocalAddr contextKey = "localAddr"
)

// Server 服务器结构
type Server struct {
	config   *config.Config
	router   *gin.Engine
	handler  *handler.Handler
	httpLn   net.Listener
	httpsLn  net.Listener
	httpSrv  *http.Server
	httpsSrv *http.Server
}

// NewServer 创建新的服务器
func NewServer(cfg *config.Config, router *gin.Engine) *Server {
	return &Server{
		config: cfg,
		router: router,
	}
}

// SetHandler 设置处理器（用于优雅关闭）
func (s *Server) SetHandler(h *handler.Handler) {
	s.handler = h
}

// Start 启动服务器
func (s *Server) Start() error {
	if s.config.EnableTLS {
		return s.startHTTPS()
	}
	return s.startHTTP()
}

// createOptimizedServer 创建优化的 HTTP 服务器
func createOptimizedServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		// 禁用 HTTP/2 以减少资源消耗（可选）
		// TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		BaseContext: func(listener net.Listener) context.Context {
			ctx := context.Background()
			return context.WithValue(ctx, ctxKeyAddr, addr)
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return context.WithValue(ctx, ctxKeyLocalAddr, c.LocalAddr())
		},
	}
}

// startHTTP 启动 HTTP 服务器
func (s *Server) startHTTP() error {
	port := s.config.HTTPPort
	addr := ":" + port

	// 创建优化的 HTTP 服务器
	s.httpSrv = createOptimizedServer(addr, s.router)

	// 使用优化的 ListenConfig
	lc := createListenConfig()
	listener, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return fmt.Errorf("监听端口失败：%w", err)
	}
	s.httpLn = listener

	// 记录启动信息
	log.Printf("服务器启动在 http://localhost:%s", port)
	log.Printf("使用 %d 个 CPU 核心", runtime.NumCPU())
	log.Printf("最大并发连接数已优化")

	// 启动多个 worker 协程处理连接
	return s.httpSrv.Serve(listener)
}

// startHTTPS 启动 HTTPS 服务器
func (s *Server) startHTTPS() error {
	var tlsConfig *tls.Config
	var err error

	if s.config.UseLocalCert {
		// 使用本地证书文件
		tlsConfig, err = s.loadLocalCert()
		if err != nil {
			return fmt.Errorf("加载本地证书失败：%w", err)
		}
	} else {
		// 使用自动证书（CertMagic）
		if err := cert.SetupTLS(s.config); err != nil {
			return fmt.Errorf("证书设置失败：%w", err)
		}

		domains := s.config.Domains

		// 获取 TLS 配置
		tlsConfig, err = cert.GetTLSConfig(domains)
		if err != nil {
			return fmt.Errorf("TLS 配置失败：%w", err)
		}
	}

	// 优化 TLS 配置
	tlsConfig = optimizeTLSConfig(tlsConfig)

	// 创建 HTTPS 服务器
	httpsAddr := ":" + s.config.HTTPSPort
	s.httpsSrv = createOptimizedServer(httpsAddr, s.router)
	s.httpsSrv.TLSConfig = tlsConfig

	// HTTP 服务器 - 用于重定向和 ACME challenge
	httpAddr := ":" + s.config.HTTPPort
	s.httpSrv = createOptimizedServer(httpAddr, http.HandlerFunc(s.httpRedirectHandler))

	// 启动 HTTP 服务器（用于重定向）
	go func() {
		log.Printf("HTTP 服务器启动在 http://localhost%s (用于重定向)", httpAddr)
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP 服务器错误：%v", err)
		}
	}()

	// 创建 HTTPS 监听器
	lc := createListenConfig()
	listener, err := lc.Listen(context.Background(), "tcp", httpsAddr)
	if err != nil {
		return fmt.Errorf("监听 HTTPS 端口失败：%w", err)
	}
	s.httpsLn = listener

	// 记录启动信息
	log.Printf("HTTPS 服务器启动在 https://localhost:%s", s.config.HTTPSPort)
	if s.config.UseLocalCert {
		log.Printf("使用本地证书：%s", s.config.CertFile)
	} else {
		log.Printf("使用 %d 个 CPU 核心", runtime.NumCPU())
		for _, domain := range s.config.Domains {
			log.Printf("  - https://%s:%s", domain, s.config.HTTPSPort)
		}
	}

	// 启动 HTTPS 服务器
	tlsListener := tls.NewListener(listener, tlsConfig)
	return s.httpsSrv.Serve(tlsListener)
}

// loadLocalCert 加载本地证书文件
func (s *Server) loadLocalCert() (*tls.Config, error) {
	certFile := s.config.CertFile
	keyFile := s.config.KeyFile

	// 如果没有指定证书文件，使用默认路径
	if certFile == "" {
		certFile = s.config.CertDir + "/cert.pem"
	}
	if keyFile == "" {
		keyFile = s.config.CertDir + "/key.pem"
	}

	// 加载证书
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("加载证书对失败：%w", err)
	}

	// 创建 TLS 配置
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	log.Printf("已加载本地证书：cert=%s, key=%s", certFile, keyFile)
	return tlsConfig, nil
}

// optimizeTLSConfig 优化 TLS 配置
func optimizeTLSConfig(cfg *tls.Config) *tls.Config {
	// 启用会话票证缓存
	cfg.SessionTicketsDisabled = false

	// 保留已有的 ALPN 协议（如 CertMagic 设置的 acme-tls/1），确保 h2 和 http/1.1 在前面
	hasH2 := false
	for _, p := range cfg.NextProtos {
		if p == "h2" {
			hasH2 = true
			break
		}
	}
	if !hasH2 {
		cfg.NextProtos = append([]string{"h2", "http/1.1"}, cfg.NextProtos...)
	}

	// 设置首选密码套件（仅 TLS 1.2，TLS 1.3 密码套件由 Go 自动管理）
	cfg.CipherSuites = []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	}

	// 设置首选曲线（X25519 性能最优）
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
	// 跳过 ACME challenge（文件验证方式）
	if certmagic.LooksLikeHTTPChallenge(r) {
		// 使用 CertMagic 的默认 HTTP 挑战处理器
		certmagic.DefaultACME.HTTPChallengeHandler(http.NotFoundHandler()).ServeHTTP(w, r)
		return
	}

	// HTTP -> HTTPS 重定向
	toURL := "https://" + r.Host + r.URL.RequestURI()
	http.Redirect(w, r, toURL, http.StatusMovedPermanently)
}

// Shutdown 优雅关闭服务器
func (s *Server) Shutdown(ctx context.Context) error {
	var err error

	// 先关闭处理器（停止工作协程和清理器）
	if s.handler != nil {
		s.handler.Shutdown()
	}

	if s.httpSrv != nil {
		if shutdownErr := s.httpSrv.Shutdown(ctx); shutdownErr != nil {
			err = shutdownErr
		}
	}

	if s.httpsSrv != nil {
		if shutdownErr := s.httpsSrv.Shutdown(ctx); shutdownErr != nil {
			err = shutdownErr
		}
	}

	return err
}
