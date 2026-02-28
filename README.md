# 配置文件分享服务器

一个简单的 Go + Gin 配置文件上传分享服务器，生成 raw 纯文本分享链接。

## 功能

- 上传配置文件（支持 yaml, yml, json, txt, toml, xml, ini, env, properties, conf, cfg, dockerfile 等常见格式）
- 生成唯一的分享链接
- 简洁的上传界面，支持拖拽上传
- 文件存储在本地 `./uploads` 目录

## 运行

```bash
# 直接运行
go run main.go

# 或者先编译再运行
go build -o subserver .
./subserver
```

服务器默认在 `http://localhost:8080` 启动。

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
