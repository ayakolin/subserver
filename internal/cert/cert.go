package cert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/caddyserver/certmagic"
	"subserver/internal/config"
)

// SetupTLS 配置 TLS 证书（使用文件验证方式，支持自动续期）
func SetupTLS(cfg *config.Config) error {
	if !cfg.EnableTLS {
		return nil
	}

	// 根据域名获取证书
	domains := cfg.Domains
	if len(domains) == 0 {
		return fmt.Errorf("启用 HTTPS 时必须配置域名")
	}

	// 自动证书管理配置
	certmagic.DefaultACME.Agreed = true
	certmagic.DefaultACME.Email = cfg.TLSEmail
	certmagic.DefaultACME.CA = certmagic.LetsEncryptProductionCA

	// 存储配置 - 证书将存储在此目录，包含自动续期所需的状态文件
	certmagic.Default.Storage = &certmagic.FileStorage{
		Path: cfg.CertDir,
	}

	// 启用 HTTP 挑战，禁用 TLS-ALPN 挑战（文件验证方式）
	certmagic.DefaultACME.DisableHTTPChallenge = false
	certmagic.DefaultACME.DisableTLSALPNChallenge = true

	// 创建验证目录
	validationDir := filepath.Join(cfg.CertDir, ".well-known", "acme-challenge")
	if err := os.MkdirAll(validationDir, 0755); err != nil {
		return fmt.Errorf("创建验证目录失败：%w", err)
	}

	// 使用 NewDefault 创建配置（会自动初始化 cache）
	// 这是 certmagic v0.20+ 的推荐用法
	cm := certmagic.NewDefault()

	// 同步获取和管理证书（包含自动续期）
	// ManageSync 会阻塞直到证书获取成功，之后会在后台自动续期
	// 证书会在到期前 30 天自动续期
	err := cm.ManageSync(context.Background(), domains)
	if err != nil {
		return fmt.Errorf("获取证书失败：%w", err)
	}

	log.Printf("证书已成功加载/更新 (文件验证)")
	log.Printf("验证目录：%s", validationDir)
	log.Printf("证书存储目录：%s", cfg.CertDir)
	log.Printf("证书到期前 30 天会自动续期")

	return nil
}

// StartHTTPChallengeServer 启动 HTTP 挑战服务器（用于文件验证）
// 这个服务器需要在 80 端口运行以响应 Let's Encrypt 的验证请求
func StartHTTPChallengeServer(port string, certDir string) error {
	validationDir := filepath.Join(certDir, ".well-known", "acme-challenge")

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/acme-challenge/", func(w http.ResponseWriter, r *http.Request) {
		// 从文件系统读取验证文件
		token := r.URL.Path[len("/.well-known/acme-challenge/"):]
		challengeFile := filepath.Join(validationDir, token)

		data, err := os.ReadFile(challengeFile)
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	log.Printf("HTTP 挑战服务器启动在端口 %s，验证目录：%s", port, validationDir)
	return http.ListenAndServe(":"+port, mux)
}

// GetTLSConfig 获取 TLS 配置
// 注意：此函数假设证书已经通过 SetupTLS 或 ManageSync 进行管理
func GetTLSConfig(domains []string) (*tls.Config, error) {
	// 使用 NewDefault 创建配置（包含已初始化的 cache）
	cm := certmagic.NewDefault()

	// TLSConfig 不需要参数，它会从 cache 中获取已管理的证书
	// 调用前需要确保证书已经通过 ManageSync 管理
	tlsConfig := cm.TLSConfig()

	// 设置 ALPN 协议以支持 HTTP/2 和 HTTP/1.1，同时保留 ACME TLS-ALPN 挑战
	tlsConfig.NextProtos = append([]string{"h2", "http/1.1"}, tlsConfig.NextProtos...)

	return tlsConfig, nil
}

// RenewCertificate 手动续期证书
func RenewCertificate(domains []string, certDir string) error {
	// 使用 NewDefault 创建配置（包含已初始化的 cache）
	cm := certmagic.NewDefault()
	ctx := context.Background()
	for _, domain := range domains {
		log.Printf("正在续期证书：%s", domain)
		// RenewCertSync 会检查是否需要续期（到期前 30 天）
		// force=true 时强制续期
		if err := cm.RenewCertSync(ctx, domain, false); err != nil {
			return fmt.Errorf("续期证书 %s 失败：%w", domain, err)
		}
	}

	log.Printf("证书续期完成")
	return nil
}

// CheckCertificateExpiry 检查证书过期时间
func CheckCertificateExpiry(domains []string, certDir string) (map[string]int64, error) {
	result := make(map[string]int64)

	for _, domain := range domains {
		// CertMagic 存储证书的路径格式：certificates/<sanitized-domain>/<domain>.crt
		// sanitized domain 会将 * 替换为 _wildcard
		sanitizedDomain := strings.ReplaceAll(domain, "*", "_wildcard")
		certFile := filepath.Join(certDir, "certificates", sanitizedDomain, domain+".crt")

		if _, err := os.Stat(certFile); os.IsNotExist(err) {
			result[domain] = -1 // 证书不存在
			continue
		}

		keyFile := filepath.Join(filepath.Dir(certFile), domain+".key")

		// 读取证书并检查过期时间
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			result[domain] = -2 // 证书加载失败
			continue
		}

		if len(cert.Certificate) > 0 {
			parsedCert, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				result[domain] = -3 // 证书解析失败
				continue
			}

			// 返回距离过期的秒数
			result[domain] = int64(time.Until(parsedCert.NotAfter).Seconds())
		} else {
			result[domain] = -4 // 证书数据无效
		}
	}

	return result, nil
}

// GetCertificateInfo 获取证书详细信息
func GetCertificateInfo(domain string, certDir string) (map[string]interface{}, error) {
	sanitizedDomain := strings.ReplaceAll(domain, "*", "_wildcard")
	certFile := filepath.Join(certDir, "certificates", sanitizedDomain, domain+".crt")

	info := make(map[string]interface{})
	info["domain"] = domain
	info["cert_file"] = certFile

	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		info["exists"] = false
		return info, fmt.Errorf("证书不存在")
	}

	info["exists"] = true

	keyFile := filepath.Join(filepath.Dir(certFile), domain+".key")

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		info["error"] = err.Error()
		return info, err
	}

	if len(cert.Certificate) > 0 {
		parsedCert, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			info["error"] = err.Error()
			return info, err
		}

		info["not_before"] = parsedCert.NotBefore
		info["not_after"] = parsedCert.NotAfter
		info["expired"] = time.Now().After(parsedCert.NotAfter)
		info["expires_in_seconds"] = int64(time.Until(parsedCert.NotAfter).Seconds())
		info["subject"] = parsedCert.Subject.CommonName
		info["issuer"] = parsedCert.Issuer.CommonName
		info["dns_names"] = parsedCert.DNSNames
	}

	return info, nil
}
