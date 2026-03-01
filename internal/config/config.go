package config

// Config 配置结构
type Config struct {
	HTTPPort   string
	HTTPSPort  string
	EnableTLS  bool
	Domains    []string
	TLSEmail   string
	DBPath     string
	LogLevel   string
	CertDir    string
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		HTTPPort:  "8080",
		HTTPSPort: "443",
		EnableTLS: false,
		Domains:   []string{},
		TLSEmail:  "",
		DBPath:    "./data/subserver.db",
		LogLevel:  "info",
		CertDir:   "./certs",
	}
}
