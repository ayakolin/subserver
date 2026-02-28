package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 配置结构
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	TLS      TLSConfig      `yaml:"tls"`
	DNS      DNSConfig      `yaml:"dns"`
	Log      LogConfig      `yaml:"log"`
	Database DatabaseConfig `yaml:"database"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	HTTPPort  string   `yaml:"http_port"`
	HTTPSPort string   `yaml:"https_port"`
	Domains   []string `yaml:"domains"`
}

// TLSConfig TLS/SSL 证书配置
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	CertDir  string `yaml:"cert_dir"`
	ACMEDir  string `yaml:"acme_dir"`
	Email    string `yaml:"email"`
}

// DNSConfig DNS 验证配置
type DNSConfig struct {
	Provider   string            `yaml:"provider"`
	Cloudflare DNSProviderConfig `yaml:"cloudflare"`
	Aliyun     DNSProviderConfig `yaml:"aliyun"`
	Tencent    DNSProviderConfig `yaml:"tencent"`
	AWS        DNSProviderConfig `yaml:"aws"`
	Google     DNSProviderConfig `yaml:"google"`
	Azure      DNSProviderConfig `yaml:"azure"`
}

// DNSProviderConfig DNS 提供商配置
type DNSProviderConfig map[string]string

// LogConfig 日志配置
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Type       string `yaml:"type"`
	SQLitePath string `yaml:"sqlite_path"`
	MySQL      string `yaml:"mysql"`
	Postgres   string `yaml:"postgres"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPPort:  "8080",
			HTTPSPort: "443",
			Domains:   []string{},
		},
		TLS: TLSConfig{
			Enabled: false,
			CertDir: "./certs",
		},
		DNS: DNSConfig{},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
		Database: DatabaseConfig{
			Type:       "sqlite",
			SQLitePath: "./data/subserver.db",
		},
	}
}

// LoadConfig 加载配置文件
func LoadConfig(configFile string) (*Config, error) {
	config := DefaultConfig()

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// 配置文件不存在，返回默认配置
		return config, nil
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败：%w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败：%w", err)
	}

	return config, nil
}
