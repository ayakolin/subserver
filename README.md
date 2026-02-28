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

### 一键安装（推荐）

```bash
git clone https://github.com/ayakolin/subserver.git
cd subserver
sudo ./install.sh
```

安装脚本会自动：
- 检测并安装 Go 依赖
- 编译二进制文件
- 创建 systemd 服务（开机自启）
- 引导式配置端口、HTTPS 等选项

### 手动安装

```bash
# 克隆仓库
git clone https://github.com/ayakolin/subserver.git
cd subserver

# 安装 Go (如未安装)
# Ubuntu/Debian
sudo apt install golang-go
# CentOS/RHEL
sudo yum install golang
# Arch
sudo pacman -S go

# 编译
go build -o subserver .

# 运行
./subserver
```

## 运行

### 开发模式

```bash
go run main.go
```

### 生产模式

```bash
go build -o subserver .
./subserver
```

### 服务管理（systemd）

```bash
# 查看状态
sudo systemctl status subserver

# 启动/停止/重启
sudo systemctl start subserver
sudo systemctl stop subserver
sudo systemctl restart subserver

# 查看日志
journalctl -u subserver -f
```

服务器默认在 `http://localhost:8080` 启动。

## 使用方法

1. **上传文件**：点击「上传文件」标签，选择或拖拽配置文件
2. **输入文本**：点击「输入文本」标签，直接输入或粘贴内容
3. **设置选项**（可选）：
   - 阅后即焚：访问一次后自动删除
   - 过期时间：选择预设时间或通过日历自定义
4. **获取链接**：上传成功后复制分享链接

## 配置

编辑 `config.yaml` 配置文件：

```yaml
server:
  http_port: 8080      # HTTP 端口
  https_port: 443      # HTTPS 端口（启用 HTTPS 时）
  domains:             # 域名列表（用于 SSL 证书申请）
    - example.com

tls:
  enabled: false       # 是否启用 HTTPS
  email: ""            # 联系邮箱（用于 SSL 证书）

log:
  level: info          # 日志级别：debug, info, warn, error
```

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
├── main.go                      # 主程序入口，数据库初始化
├── index.html                   # 前端页面（上传/输入界面）
├── internal/
│   ├── config/
│   │   └── config.go            # 配置加载
│   ├── file/
│   │   └── file.go              # 文件验证、存储逻辑
│   ├── handler/
│   │   └── handler.go           # HTTP 处理器
│   └── server/
│       └── server.go            # 服务器启动
├── data/                        # SQLite 数据库目录
├── install.sh                   # 自动化安装脚本
├── go.mod                       # Go 模块配置
└── README.md                    # 项目文档
```

## 卸载

```bash
sudo ./install.sh --uninstall
```

## License

MIT
