package cert

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"

	"github.com/caddyserver/certmagic"
	"subserver/internal/config"
)

// SetupTLS 配置 TLS 证书
func SetupTLS(cfg *config.Config) error {
	if !cfg.EnableTLS {
		return nil
	}

	// 自动证书管理配置
	certmagic.DefaultACME.Agreed = true
	certmagic.DefaultACME.Email = cfg.TLSEmail
	certmagic.DefaultACME.CA = certmagic.LetsEncryptProductionCA

	// 存储配置
	certmagic.Default.Storage = &certmagic.FileStorage{
		Path: cfg.CertDir,
	}

	// 根据域名获取证书
	domains := cfg.Domains
	if len(domains) == 0 {
		return fmt.Errorf("启用 HTTPS 时必须配置域名")
	}

	// 使用默认的 HTTP 验证
	return setupWithHTTPValidation(domains)
}

// setupWithHTTPValidation 使用 HTTP 验证设置证书
func setupWithHTTPValidation(domains []string) error {
	cfg := certmagic.NewDefault()
	err := cfg.ManageSync(context.Background(), domains)
	if err != nil {
		return fmt.Errorf("获取证书失败：%w", err)
	}
	log.Printf("证书已成功加载/更新 (HTTP 验证)")
	return nil
}

// GetTLSConfig 获取 TLS 配置
func GetTLSConfig(domains []string) (*tls.Config, error) {
	return certmagic.TLS(domains)
}
