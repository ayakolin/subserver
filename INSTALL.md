# subserver 安装脚本

自动下载、安装和配置 subserver 的 Bash 脚本。支持从 GitHub Release 自动获取对应架构的二进制文件，并进行引导式部署。

## 快速安装

### 一键安装（交互式）

```bash
curl -fsSL https://github.com/ayakolin/subserver/releases/latest/download/install.sh | bash
```

### 非交互式安装（带参数）

```bash
# 简单安装（默认配置）
curl -fsSL https://github.com/ayakolin/subserver/releases/latest/download/install.sh | bash -s -- -p 8080

# 启用 HTTPS（自动申请证书）
curl -fsSL https://github.com/ayakolin/subserver/releases/latest/download/install.sh | bash -s -- \
  -p 443 \
  -tls true \
  -d example.com \
  -tls-email admin@example.com

# 使用本地证书
curl -fsSL https://github.com/ayakolin/subserver/releases/latest/download/install.sh | bash -s -- \
  -tls true \
  -local-tls true \
  -cert-file /etc/ssl/certs/example.com.crt \
  -key-file /etc/ssl/private/example.com.key

# 自定义配置
curl -fsSL https://github.com/ayakolin/subserver/releases/latest/download/install.sh | bash -s -- \
  -p 8080 \
  -db /var/lib/subserver/data.db \
  -log debug \
  -cert-dir /var/lib/subserver/certs
```

## 命令行参数

### 安装参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-p` | HTTP 端口 | 8080 |
| `-tls` | 启用 HTTPS | false |
| `-tls-port` | HTTPS 端口 | 443 |
| `-local-tls` | 使用本地证书文件 | false |
| `-cert-file` | 证书文件路径（与 `-local-tls` 一起使用） | - |
| `-key-file` | 私钥文件路径（与 `-local-tls` 一起使用） | - |
| `-d` | 域名（多个用逗号分隔） | - |
| `-tls-email` | SSL 证书邮箱 | - |
| `-db` | 数据库文件路径 | ./data/subserver.db |
| `-log` | 日志级别 (debug/info/warn/error) | info |
| `-cert-dir` | SSL 证书目录（含验证文件） | ./certs |
| `-h` | 显示帮助信息 | - |

### 卸载参数

| 参数 | 说明 |
|------|------|
| `-u`, `--uninstall` | 卸载 subserver |
| `-f`, `--force` | 强制卸载（不询问确认） |
| `--remove-config` | 卸载时删除配置文件 |
| `--remove-data` | 卸载时删除数据文件（数据库、证书） |

## 安装说明

### 系统要求

- Linux (x86_64, aarch64, armv7l) 或 macOS (x86_64, arm64)
- curl
- tar
- systemd（用于 Linux 服务管理，可选）

### 安装流程

1. **自动检测** - 脚本会自动检测操作系统和 CPU 架构
2. **下载二进制** - 从 GitHub Release 下载最新版本的对应架构二进制文件
3. **Checksum 验证** - 可选，如果存在 checksum 文件会自动验证
4. **安装二进制** - 将二进制文件复制到 `/usr/local/bin/subserver`
5. **配置** - 交互式询问配置参数（或通过命令行参数提供）
6. **创建服务** - 在 Linux 上创建 systemd 服务文件
7. **启动服务** - 启用并启动 subserver 服务

### 安装后的文件位置

| 文件/目录 | 路径 |
|-----------|------|
| 二进制文件 | `/usr/local/bin/subserver` |
| 配置文件 | `/etc/subserver/config.env` |
| 服务文件 | `/etc/systemd/system/subserver.service` |
| 数据目录 | `/var/lib/subserver` |

## 服务管理

```bash
# 启动服务
sudo systemctl start subserver

# 停止服务
sudo systemctl stop subserver

# 重启服务
sudo systemctl restart subserver

# 查看状态
sudo systemctl status subserver

# 查看日志
sudo journalctl -u subserver -f
```

## 卸载

### 交互式卸载

```bash
# 下载并运行卸载
sudo bash install.sh -u

# 或者从 GitHub 直接运行
curl -fsSL https://github.com/ayakolin/subserver/releases/latest/download/install.sh | sudo bash -s -- -u
```

### 强制卸载（不询问确认）

```bash
# 使用脚本卸载
sudo bash install.sh -u -f

# 从 GitHub 直接运行
curl -fsSL https://github.com/ayakolin/subserver/releases/latest/download/install.sh | sudo bash -s -- -u -f
```

### 完全卸载（包括配置和数据）

```bash
# 删除所有内容
sudo bash install.sh -u -f --remove-config --remove-data

# 从 GitHub 直接运行
curl -fsSL https://github.com/ayakolin/subserver/releases/latest/download/install.sh | sudo bash -s -- -u -f --remove-config --remove-data
```

### 卸载时保留数据

```bash
# 只删除程序，保留配置文件和数据
sudo bash install.sh -u

# 删除程序和服务，保留配置文件
sudo bash install.sh -u -f

# 删除程序和服务，删除配置文件但保留数据
sudo bash install.sh -u -f --remove-config
```

**注意：** 卸载时会执行以下操作：
1. 停止并禁用 systemd 服务
2. 删除服务文件 (`/etc/systemd/system/subserver.service`)
3. 删除二进制文件 (`/usr/local/bin/subserver`)

默认情况下，配置文件和数据会被保留，以便重新安装时恢复使用。

## 手动运行

如果不使用 systemd 服务，也可以直接手动运行：

```bash
subserver -p 8080 -d example.com -tls-email admin@example.com
```

## 环境变量

可以通过环境变量覆盖默认行为：

```bash
# 覆盖 GitHub 仓库（用于测试或镜像）
export GITHUB_OWNER=your-org
export GITHUB_REPO=subserver

# 运行安装脚本
curl -fsSL ... | bash
```

## 故障排除

### 权限错误

如果遇到权限错误，请确保以 root 用户运行或使用 sudo：

```bash
curl -fsSL ... | sudo bash
```

### systemd 不可用

如果系统不支持 systemd，脚本会跳过服务创建，需要手动运行 subserver。

### 下载失败

检查网络连接，或者手动下载二进制文件：

```bash
# 查看最新版本
curl -fsSL https://api.github.com/repos/ayakolin/subserver/releases/latest

# 手动下载并安装
wget https://github.com/ayakolin/subserver/releases/latest/download/subserver_latest_linux_amd64.tar.gz
tar -xzf subserver_latest_linux_amd64.tar.gz
sudo cp subserver /usr/local/bin/
```

## 安全注意事项

- 在生产环境中建议使用 HTTPS
- 使用强密码保护管理员账户
- 定期备份数据库文件
- 限制服务器的网络访问

## 许可证

与 subserver 项目相同。
