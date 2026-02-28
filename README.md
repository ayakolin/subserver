# 配置文件分享服务器

一个简单的 Go + Gin 配置文件上传分享服务器，生成 raw 纯文本分享链接。

## 快速安装

### 一键安装（推荐）

```bash
git clone https://github.com/ayakolin/subserver.git
cd subserver
sudo ./install.sh
```

安装脚本会自动：
- 检测并安装 Go 和 Git 依赖
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

## 功能

- 上传配置文件（支持 yaml, yml, json, txt, toml, xml, ini, env, properties, conf, cfg, dockerfile 等常见格式）
- 生成唯一的分享链接
- 简洁的上传界面，支持拖拽上传
- 文件存储在本地 `./uploads` 目录

## 运行

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

### 直接运行

```bash
# 直接运行
go run main.go

# 或者先编译再运行
go build -o subserver .
./subserver
```

服务器默认在 `http://localhost:8080` 启动。

## 配置

配置文件 `config.yaml`：

```yaml
server:
  http_port: 8080      # HTTP 端口
  https_port: 443      # HTTPS 端口（启用 HTTPS 时）
  domains:             # 域名列表（用于 SSL 证书）
    - example.com

tls:
  enabled: false       # 是否启用 HTTPS
  email: ""            # 联系邮箱（用于 SSL 证书申请）

log:
  level: info          # 日志级别：debug, info, warn, error
```

## 卸载

```bash
sudo ./install.sh --uninstall
```

## API

### POST /upload

上传配置文件。

**请求：**
- Content-Type: `multipart/form-data`
- 表单字段：`file` (配置文件，支持 yaml, yml, json, txt, toml, xml, ini, env, properties, conf, cfg, dockerfile 等格式)

**响应：**
```json
{
  "id": "abc123def456",
  "raw_url": "http://localhost:8080/raw/abc123def456"
}
```

### GET /raw/:id

获取 raw 纯文本内容。

**响应：**
- Content-Type: `text/plain; charset=utf-8`
- 文件内容

## 目录结构

```
subserver/
├── main.go          # 主程序
├── index.html       # 上传页面
├── uploads/         # 存储上传的文件
└── go.mod           # Go 模块配置
```
