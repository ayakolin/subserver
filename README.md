# SubServer - 文本文件分享服务器

一个简洁高效的 Go + Gin 文本文件分享服务器，支持文件上传和在线文本输入，生成带过期时间和阅后即焚功能的纯文本分享链接。

## 特性

- **两种分享方式**：支持上传配置文件或直接输入文本
- **阅后即焚**：文件/文本访问一次后自动删除
- **灵活过期**：支持自定义过期时间或永久有效
- **时间预设**：1 分钟、5 分钟、10 分钟、30 分钟、1 小时、1 天、7 天快捷选项
- **日历选择**：可视化日期时间选择器
- **安全存储**：SQLite 数据库存储，自动清理过期数据
- **简洁界面**：现代化设计，支持拖拽上传
- **纯文本输出**：适合分享配置文件、代码片段等

## 快速开始

### 编译和运行

```bash
# 克隆仓库
git clone https://github.com/ayakolin/subserver.git
cd subserver

# 编译
go build -o subserver .

# 运行（默认端口 8080）
./subserver
```

## 命令行参数

```bash
./subserver -h  # 查看帮助
```

**可用参数：**

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-p` | HTTP 端口 | `8080` |
| `-tls` | 启用 HTTPS | `false` |
| `-tls-port` | HTTPS 端口 | `443` |
| `-d` | 域名（多个用逗号分隔） | - |
| `-tls-email` | SSL 证书邮箱 | - |
| `-db` | 数据库文件路径 | `./data/subserver.db` |
| `-log` | 日志级别 | `info` |
| `-cert-dir` | SSL 证书目录 | `./certs` |

**示例：**

```bash
# 简单启动
./subserver

# 指定端口
./subserver -p 8888

# 指定域名
./subserver -p 8080 -d example.com

# 启用 HTTPS
./subserver -tls -d example.com -tls-email admin@example.com

# 完整配置
./subserver -p 8080 -tls -d example.com -tls-email admin@example.com -db ./data/db.sqlite3 -log debug
```

## 使用方法

1. **上传文件**：点击上传区域，选择或拖拽配置文件
2. **输入文本**：直接输入或粘贴内容
3. **设置选项**（可选）：
   - 阅后即焚：访问一次后自动删除
   - 过期时间：选择预设时间或通过日历自定义
4. **获取链接**：上传成功后复制分享链接

## 支持的文件格式

| 类型 | 扩展名 |
|------|--------|
| YAML | `.yaml`, `.yml` |
| JSON | `.json` |
| 纯文本 | `.txt` |
| TOML | `.toml` |
| XML | `.xml` |
| INI/配置 | `.ini`, `.conf`, `.cfg`, `.config`, `.rc` |
| 数据格式 | `.csv`, `.tsv` |
| 脚本 | `.sh`, `.bash`, `.zsh` |
| 环境变量 | `.env`, `.properties` |
| Docker | `Dockerfile`, `.dockerfile` |
| Makefile | `Makefile`, `makefile`, `gnumakefile` |
| 其他 | `Procfile`, `Gemfile`, `Rakefile` |

文件大小限制：最大 1MB

## API

### POST /upload

上传文件或文本。

**请求参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| file | File | 是 | 文件对象或文本 Blob |
| once | String | 否 | `true` 启用阅后即焚 |
| expire_seconds | String | 否 | 过期时间（秒），0 或空表示永不过期 |

**响应示例：**

```json
{
  "id": "9904c385-3473-4560-b412-add3b1826ede",
  "raw_url": "http://localhost:8080/raw/9904c385-3473-4560-b412-add3b1826ede",
  "once": true,
  "expires_at": 1772298482
}
```

### GET /raw/:id

获取纯文本内容。

**响应：**
- Content-Type: `text/plain; charset=utf-8`
- 文件/文本内容

## 项目结构

```
subserver/
├── main.go                      # 主程序入口，命令行参数解析
├── internal/
│   ├── config/
│   │   └── config.go            # 配置结构
│   ├── cert/
│   │   └── cert.go              # TLS 证书管理
│   ├── file/
│   │   └── file.go              # 文件验证、存储逻辑
│   ├── handler/
│   │   ├── handler.go           # HTTP 处理器
│   │   └── static/
│   │       └── index.html       # 前端页面（已嵌入到二进制）
│   └── server/
│       └── server.go            # 服务器启动
├── data/                        # SQLite 数据库目录
├── go.mod                       # Go 模块配置
└── README.md                    # 项目文档
```

## License

MIT
