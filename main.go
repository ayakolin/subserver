package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/caddyserver/certmagic"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

const (
	uploadDir = "./uploads"
	maxSize   = 1 << 20 // 1MB
)

// Config 配置结构
type Config struct {
	Server struct {
		HTTPPort  string   `yaml:"http_port"`
		HTTPSPort string   `yaml:"https_port"`
		Domains   []string `yaml:"domains"`
	} `yaml:"server"`
	TLS struct {
		Enabled  bool   `yaml:"enabled"`
		CertFile string `yaml:"cert_file"`
		KeyFile  string `yaml:"key_file"`
		CertDir  string `yaml:"cert_dir"`
		ACMEDir  string `yaml:"acme_dir"`
		Email    string `yaml:"email"`
	} `yaml:"tls"`
	DNS struct {
		Provider   string `yaml:"provider"`
		Cloudflare struct {
			APIToken string `yaml:"api_token"`
			APIKey   string `yaml:"api_key"`
			Email    string `yaml:"email"`
		} `yaml:"cloudflare"`
		Aliyun struct {
			AccessKeyID     string `yaml:"access_key_id"`
			AccessKeySecret string `yaml:"access_key_secret"`
		} `yaml:"aliyun"`
		Tencent struct {
			SecretID  string `yaml:"secret_id"`
			SecretKey string `yaml:"secret_key"`
		} `yaml:"tencent"`
		AWS struct {
			AccessKeyID     string `yaml:"access_key_id"`
			SecretAccessKey string `yaml:"secret_access_key"`
			Region          string `yaml:"region"`
		} `yaml:"aws"`
		Google struct {
			CredentialsFile string `yaml:"credentials_file"`
			Project         string `yaml:"project"`
		} `yaml:"google"`
		Azure struct {
			ClientID     string `yaml:"client_id"`
			ClientSecret string `yaml:"client_secret"`
			TenantID     string `yaml:"tenant_id"`
			Subscription string `yaml:"subscription_id"`
		} `yaml:"azure"`
	} `yaml:"dns"`
	Log struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"log"`
}

// 默认配置
var config = &Config{
	Server: struct {
		HTTPPort  string   `yaml:"http_port"`
		HTTPSPort string   `yaml:"https_port"`
		Domains   []string `yaml:"domains"`
	}{
		HTTPPort:  "8080",
		HTTPSPort: "443",
		Domains:   []string{},
	},
	TLS: struct {
		Enabled  bool   `yaml:"enabled"`
		CertFile string `yaml:"cert_file"`
		KeyFile  string `yaml:"key_file"`
		CertDir  string `yaml:"cert_dir"`
		ACMEDir  string `yaml:"acme_dir"`
		Email    string `yaml:"email"`
	}{
		Enabled:  false,
		CertDir:  "./certs",
	},
}

var allowedExts = map[string]bool{
	// YAML
	".yaml": true, ".yml": true,
	// JSON
	".json": true,
	// 纯文本
	".txt": true,
	// TOML
	".toml": true,
	// XML
	".xml": true,
	// INI
	".ini": true,
	// 属性文件
	".properties": true,
	// 环境变量
	".env": true,
	// 其他常见配置文件
	".conf": true, ".cfg": true, ".config": true,
	".rc": true,
	// 数据格式
	".csv": true, ".tsv": true,
	// 脚本类配置
	".sh": true, ".bash": true, ".zsh": true,
	// Makefile
	"makefile": true, "gnumakefile": true,
	// Docker 相关
	"dockerfile": true, ".dockerfile": true,
	// 无扩展名的常见配置文件
	"procfile": true, "gemfile": true, "rakefile": true,
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func loadConfig() error {
	// 尝试读取配置文件
	configFile := "config.yaml"
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// 配置文件不存在，使用默认配置
		return nil
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("读取配置文件失败：%w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return fmt.Errorf("解析配置文件失败：%w", err)
	}

	return nil
}

// setupCertMagic 配置 CertMagic 自动证书管理
func setupCertMagic() error {
	if !config.TLS.Enabled {
		return nil
	}

	// 如果配置了静态证书文件
	if config.TLS.CertFile != "" && config.TLS.KeyFile != "" {
		if _, err := os.Stat(config.TLS.CertFile); err != nil {
			return fmt.Errorf("证书文件不存在：%s", config.TLS.CertFile)
		}
		if _, err := os.Stat(config.TLS.KeyFile); err != nil {
			return fmt.Errorf("私钥文件不存在：%s", config.TLS.KeyFile)
		}
		return nil // 使用静态证书，不需要 CertMagic
	}

	// 自动证书管理配置
	certmagic.DefaultACME.Agreed = true
	certmagic.DefaultACME.Email = config.TLS.Email

	if config.TLS.ACMEDir != "" {
		certmagic.DefaultACME.CA = config.TLS.ACMEDir
	} else {
		certmagic.DefaultACME.CA = certmagic.LetsEncryptProductionCA
	}

	// 存储配置
	certmagic.Default.Storage = &certmagic.FileStorage{
		Path: config.TLS.CertDir,
	}

	// 根据域名获取证书
	domains := config.Server.Domains
	if len(domains) == 0 {
		return fmt.Errorf("启用 HTTPS 时必须配置域名")
	}

	// 配置 DNS 验证（如果配置了 DNS 提供商）
	// 注意：DNS 验证需要对应的 libdns 提供者包
	if config.DNS.Provider != "" {
		dnsProvider, err := getDNSProvider(config.DNS.Provider)
		if err != nil {
			return fmt.Errorf("获取 DNS 提供商失败：%w", err)
		}

		// 创建自定义 ACME 配置，使用 DNS 挑战
		customACME := certmagic.ACMEIssuer{
			Email:  config.TLS.Email,
			CA:     certmagic.DefaultACME.CA,
			Agreed: true,
			DNS01Solver: &certmagic.DNS01Solver{
				DNSManager: certmagic.DNSManager{
					DNSProvider: dnsProvider,
				},
			},
		}

		// 创建自定义配置
		cfg := certmagic.Config{
			Issuers: []certmagic.Issuer{&customACME},
			Storage: certmagic.Default.Storage,
		}

		// 管理证书
		err = cfg.ManageSync(context.Background(), domains)
		if err != nil {
			return fmt.Errorf("获取证书失败：%w", err)
		}

		log.Printf("已配置 DNS 验证：%s", config.DNS.Provider)
	} else {
		// 使用默认的 HTTP 验证
		cfg := certmagic.NewDefault()
		err := cfg.ManageSync(context.Background(), domains)
		if err != nil {
			return fmt.Errorf("获取证书失败：%w", err)
		}
	}

	log.Printf("证书已成功加载/更新")
	return nil
}

// getDNSProvider 根据配置获取 DNS 提供商
func getDNSProvider(provider string) (certmagic.DNSProvider, error) {
	// 注意：需要在 go.mod 中添加对应的 libdns 依赖
	// 并在 config.yaml 中配置相应的凭证
	switch strings.ToLower(provider) {
	case "cloudflare":
		if config.DNS.Cloudflare.APIToken == "" && config.DNS.Cloudflare.APIKey == "" {
			return nil, fmt.Errorf("Cloudflare API token 或 API key 未配置")
		}
		// 需要导入 github.com/libdns/cloudflare
		// return &cloudflare.Provider{...}, nil
		return nil, fmt.Errorf("Cloudflare DNS 验证需要导入 github.com/libdns/cloudflare 包")
	case "aliyun", "alicloud":
		if config.DNS.Aliyun.AccessKeyID == "" || config.DNS.Aliyun.AccessKeySecret == "" {
			return nil, fmt.Errorf("阿里云 AccessKey 未配置")
		}
		// 需要导入 github.com/libdns/aliyun
		return nil, fmt.Errorf("阿里云 DNS 验证需要导入 github.com/libdns/aliyun 包")
	case "tencent", "tencentcloud":
		if config.DNS.Tencent.SecretID == "" || config.DNS.Tencent.SecretKey == "" {
			return nil, fmt.Errorf("腾讯云 SecretKey 未配置")
		}
		// 需要导入 github.com/libdns/tencent
		return nil, fmt.Errorf("腾讯云 DNS 验证需要导入 github.com/libdns/tencent 包")
	case "aws", "route53":
		if config.DNS.AWS.AccessKeyID == "" || config.DNS.AWS.SecretAccessKey == "" {
			return nil, fmt.Errorf("AWS AccessKey 未配置")
		}
		// 需要导入 github.com/libdns/aws
		return nil, fmt.Errorf("AWS Route53 DNS 验证需要导入 github.com/libdns/aws 包")
	case "google", "gcp":
		if config.DNS.Google.CredentialsFile == "" {
			return nil, fmt.Errorf("Google Cloud credentials file 未配置")
		}
		// 需要导入 github.com/libdns/google
		return nil, fmt.Errorf("Google Cloud DNS 验证需要导入 github.com/libdns/google 包")
	case "azure":
		if config.DNS.Azure.ClientID == "" || config.DNS.Azure.ClientSecret == "" {
			return nil, fmt.Errorf("Azure credentials 未配置")
		}
		// 需要导入 github.com/libdns/azure
		return nil, fmt.Errorf("Azure DNS 验证需要导入 github.com/libdns/azure 包")
	default:
		return nil, fmt.Errorf("不支持的 DNS 提供商：%s", provider)
	}
}

// ensureUploadDir 确保上传目录存在
func ensureUploadDir() error {
	return os.MkdirAll(uploadDir, 0755)
}

func main() {
	// 加载配置文件
	if err := loadConfig(); err != nil {
		log.Printf("警告：%v，使用默认配置", err)
	}

	if err := ensureUploadDir(); err != nil {
		fmt.Printf("无法创建上传目录：%v\n", err)
		os.Exit(1)
	}

	r := gin.Default()

	// 上传页面
	r.GET("/", func(c *gin.Context) {
		c.File("./index.html")
	})

	// 上传 API
	r.POST("/upload", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请选择要上传的文件"})
			return
		}

		// 检查文件大小
		if file.Size > maxSize {
			c.JSON(http.StatusBadRequest, gin.H{"error": "文件大小不能超过 1MB"})
			return
		}

		// 检查文件类型
		ext := strings.ToLower(filepath.Ext(file.Filename))
		filename := strings.ToLower(file.Filename)
		if !allowedExts[ext] && !allowedExts[filename] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "只能上传常见的配置文件格式 (yaml, yml, json, txt, toml, xml, ini, env 等)"})
			return
		}

		// 生成唯一 ID
		id := generateID()
		fileext := filepath.Ext(file.Filename)
		if fileext == "" {
			fileext = ".config"
		}
		filename = id + fileext
		filepath := filepath.Join(uploadDir, filename)

		// 保存文件
		if err := c.SaveUploadedFile(file, filepath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败"})
			return
		}

		// 返回分享链接
		rawURL := fmt.Sprintf("%s/raw/%s", getHost(c), id)
		c.JSON(http.StatusOK, gin.H{
			"id":      id,
			"raw_url": rawURL,
		})
	})

	// Raw 文本访问
	r.GET("/raw/:id", func(c *gin.Context) {
		id := c.Param("id")

		// 在 uploads 目录中查找匹配的文件
		var filePath string
		entries, err := os.ReadDir(uploadDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasPrefix(entry.Name(), id+".") {
					filePath = filepath.Join(uploadDir, entry.Name())
					break
				}
			}
		}

		// 检查文件是否存在
		if filePath == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
			return
		}

		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.Header("Content-Disposition", "inline")
		c.File(filePath)
	})

	// 启动服务
	if config.TLS.Enabled {
		// 设置 CertMagic
		if err := setupCertMagic(); err != nil {
			log.Printf("警告：证书设置失败：%v", err)
			os.Exit(1)
		}

		domains := config.Server.Domains

		// 获取 TLS 配置
		tlsConfig, err := certmagic.TLS(domains)
		if err != nil {
			log.Printf("TLS 配置失败：%v", err)
			os.Exit(1)
		}

		// 创建 HTTPS 服务器
		httpsAddr := ":" + config.Server.HTTPSPort
		httpsServer := &http.Server{
			Addr:      httpsAddr,
			Handler:   r,
			TLSConfig: tlsConfig,
		}

		// HTTP 服务器 - 用于 ACME HTTP challenge 和重定向
		httpAddr := ":" + config.Server.HTTPPort
		httpServer := &http.Server{
			Addr:    httpAddr,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// HTTP 重定向到 HTTPS
				toURL := "https://" + r.Host + r.URL.RequestURI()
				http.Redirect(w, r, toURL, http.StatusMovedPermanently)
			}),
		}

		go func() {
			log.Printf("HTTP 服务器启动在 http://localhost%s (用于 ACME challenge 和重定向)", httpAddr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("HTTP 服务器错误：%v", err)
			}
		}()

		log.Printf("HTTPS 服务器启动在 https://localhost:%s", config.Server.HTTPSPort)
		if len(config.Server.Domains) > 0 {
			for _, domain := range config.Server.Domains {
				log.Printf("  - https://%s:%s", domain, config.Server.HTTPSPort)
			}
		}

		if err := httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTPS 服务器错误：%v", err)
			os.Exit(1)
		}
	} else {
		port := config.Server.HTTPPort
		fmt.Printf("服务器启动在 http://localhost:%s\n", port)
		if err := r.Run(":" + port); err != nil {
			log.Printf("服务器错误：%v", err)
			os.Exit(1)
		}
	}
}

func getHost(c *gin.Context) string {
	host := c.Request.Host
	if host == "" {
		return "http://localhost:8080"
	}
	return "http://" + host
}
