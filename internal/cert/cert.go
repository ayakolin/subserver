package cert

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/caddyserver/certmagic"
	"subserver/internal/config"
)

// SetupTLS 配置 TLS 证书
func SetupTLS(cfg *config.Config) error {
	if !cfg.TLS.Enabled {
		return nil
	}

	// 如果配置了静态证书文件
	if cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		return validateStaticCerts(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	}

	// 自动证书管理配置
	certmagic.DefaultACME.Agreed = true
	certmagic.DefaultACME.Email = cfg.TLS.Email

	if cfg.TLS.ACMEDir != "" {
		certmagic.DefaultACME.CA = cfg.TLS.ACMEDir
	} else {
		certmagic.DefaultACME.CA = certmagic.LetsEncryptProductionCA
	}

	// 存储配置
	certmagic.Default.Storage = &certmagic.FileStorage{
		Path: cfg.TLS.CertDir,
	}

	// 根据域名获取证书
	domains := cfg.Server.Domains
	if len(domains) == 0 {
		return fmt.Errorf("启用 HTTPS 时必须配置域名")
	}

	// 配置 DNS 验证（如果配置了 DNS 提供商）
	if cfg.DNS.Provider != "" {
		return setupWithDNSValidation(cfg, domains)
	}

	// 使用默认的 HTTP 验证
	return setupWithHTTPValidation(domains)
}

// validateStaticCerts 验证静态证书文件
func validateStaticCerts(certFile, keyFile string) error {
	if _, err := os.Stat(certFile); err != nil {
		return fmt.Errorf("证书文件不存在：%s", certFile)
	}
	if _, err := os.Stat(keyFile); err != nil {
		return fmt.Errorf("私钥文件不存在：%s", keyFile)
	}
	return nil
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

// setupWithDNSValidation 使用 DNS 验证设置证书
func setupWithDNSValidation(cfg *config.Config, domains []string) error {
	dnsProvider, err := getDNSProvider(cfg)
	if err != nil {
		return fmt.Errorf("获取 DNS 提供商失败：%w", err)
	}

	// 创建自定义 ACME 配置，使用 DNS 挑战
	customACME := certmagic.ACMEIssuer{
		Email:  cfg.TLS.Email,
		CA:     certmagic.DefaultACME.CA,
		Agreed: true,
		DNS01Solver: &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: dnsProvider,
			},
		},
	}

	// 创建自定义配置
	customCfg := certmagic.Config{
		Issuers: []certmagic.Issuer{&customACME},
		Storage: certmagic.Default.Storage,
	}

	// 管理证书
	err = customCfg.ManageSync(context.Background(), domains)
	if err != nil {
		return fmt.Errorf("获取证书失败：%w", err)
	}

	log.Printf("证书已成功加载/更新 (DNS 验证：%s)", cfg.DNS.Provider)
	return nil
}

// getDNSProvider 根据配置获取 DNS 提供商
func getDNSProvider(cfg *config.Config) (certmagic.DNSProvider, error) {
	// 注意：需要在 go.mod 中添加对应的 libdns 依赖
	provider := strings.ToLower(cfg.DNS.Provider)

	switch provider {
	case "cloudflare":
		if cfg.DNS.Cloudflare["api_token"] == "" && cfg.DNS.Cloudflare["api_key"] == "" {
			return nil, fmt.Errorf("Cloudflare API token 或 API key 未配置")
		}
		return nil, fmt.Errorf("Cloudflare DNS 验证需要导入 github.com/libdns/cloudflare 包")
	case "aliyun", "alicloud":
		if cfg.DNS.Aliyun["access_key_id"] == "" || cfg.DNS.Aliyun["access_key_secret"] == "" {
			return nil, fmt.Errorf("阿里云 AccessKey 未配置")
		}
		return nil, fmt.Errorf("阿里云 DNS 验证需要导入 github.com/libdns/aliyun 包")
	case "tencent", "tencentcloud":
		if cfg.DNS.Tencent["secret_id"] == "" || cfg.DNS.Tencent["secret_key"] == "" {
			return nil, fmt.Errorf("腾讯云 SecretKey 未配置")
		}
		return nil, fmt.Errorf("腾讯云 DNS 验证需要导入 github.com/libdns/tencent 包")
	case "aws", "route53":
		if cfg.DNS.AWS["access_key_id"] == "" || cfg.DNS.AWS["secret_access_key"] == "" {
			return nil, fmt.Errorf("AWS AccessKey 未配置")
		}
		return nil, fmt.Errorf("AWS Route53 DNS 验证需要导入 github.com/libdns/aws 包")
	case "google", "gcp":
		if cfg.DNS.Google["credentials_file"] == "" {
			return nil, fmt.Errorf("Google Cloud credentials file 未配置")
		}
		return nil, fmt.Errorf("Google Cloud DNS 验证需要导入 github.com/libdns/google 包")
	case "azure":
		if cfg.DNS.Azure["client_id"] == "" || cfg.DNS.Azure["client_secret"] == "" {
			return nil, fmt.Errorf("Azure credentials 未配置")
		}
		return nil, fmt.Errorf("Azure DNS 验证需要导入 github.com/libdns/azure 包")
	default:
		return nil, fmt.Errorf("不支持的 DNS 提供商：%s", cfg.DNS.Provider)
	}
}

// GetTLSConfig 获取 TLS 配置
func GetTLSConfig(domains []string) (*tls.Config, error) {
	return certmagic.TLS(domains)
}
