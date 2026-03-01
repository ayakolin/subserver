# SubServer - 文本文件分享服务器

一个简洁的 Go 文本文件分享服务器，支持文件上传和在线文本输入，生成带过期时间和阅后即焚功能的分享链接。

## 快速安装

```bash
# Linux/macOS 一键安装
curl -fsSL https://raw.githubusercontent.com/ayakolin/subserver/main/install.sh | bash

# 自定义端口
curl -fsSL https://raw.githubusercontent.com/ayakolin/subserver/main/install.sh | bash -s -- -p 8080

# 启用 HTTPS
curl -fsSL https://raw.githubusercontent.com/ayakolin/subserver/main/install.sh | bash -s -- -tls true -d example.com -tls-email admin@example.com

# 卸载
curl -fsSL https://raw.githubusercontent.com/ayakolin/subserver/main/install.sh | sudo bash -s -- -u -f
```

详细文档请参阅 [INSTALL.md](INSTALL.md)

## 特性

- **两种分享方式**：上传配置文件或直接输入文本
- **阅后即焚**：访问一次后自动删除
- **灵活过期**：自定义过期时间或永久有效
- **HTTPS 支持**：自动申请 Let's Encrypt 证书
- **SQLite 存储**：自动清理过期数据

## 手动编译

```bash
git clone https://github.com/ayakolin/subserver.git
cd subserver
go build -o subserver .
./subserver
```

## License

MIT
