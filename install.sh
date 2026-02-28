#!/bin/bash

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

# 配置变量
INSTALL_DIR="/opt/subserver"
SERVICE_NAME="subserver"
CONFIG_FILE="$INSTALL_DIR/config.yaml"
SYSTEMD_SERVICE="/etc/systemd/system/${SERVICE_NAME}.service"
REPO_OWNER="rinca"
REPO_NAME="subserver"

# 检测系统架构
detect_arch() {
    local arch=$(uname -m)
    case $arch in
        x86_64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        armv7l)
            ARCH="armv7"
            ;;
        i386|i686)
            ARCH="386"
            ;;
        *)
            log_error "不支持的系统架构：$arch"
            exit 1
            ;;
    esac
    log_info "检测到系统架构：$ARCH"
}

# 检测操作系统
detect_os() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    case $OS in
        linux)
            OS="linux"
            ;;
        darwin)
            OS="darwin"
            ;;
        *)
            log_error "不支持的操作系统：$OS"
            exit 1
            ;;
    esac
    log_info "检测到操作系统：$OS"
}

# 获取最新版本号
get_latest_version() {
    log_step "正在获取最新版本信息..."

    # 尝试从 GitHub API 获取最新版本
    local response
    response=$(curl -s https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest)

    if echo "$response" | grep -q '"tag_name"'; then
        VERSION=$(echo "$response" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
        log_info "最新版本：$VERSION"
    else
        log_error "无法获取版本信息，请检查网络连接或仓库地址"
        exit 1
    fi
}

# 下载二进制文件
download_binary() {
    log_step "正在下载二进制文件..."

    # 构建下载 URL
    local binary_name="subserver_${VERSION}_${OS}_${ARCH}"
    local download_url="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${VERSION}/${binary_name}"

    # 创建临时目录
    local temp_dir=$(mktemp -d)
    local temp_binary="$temp_dir/subserver"

    # 下载二进制文件
    if curl -L -o "$temp_binary" "$download_url"; then
        log_info "二进制文件下载成功"
    else
        # 尝试不带版本号的命名格式
        binary_name="subserver-${OS}-${ARCH}"
        download_url="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${VERSION}/${binary_name}"

        if curl -L -o "$temp_binary" "$download_url"; then
            log_info "二进制文件下载成功"
        else
            log_error "无法下载二进制文件"
            log_warn "尝试从源码构建..."
            rm -rf "$temp_dir"
            BUILD_FROM_SOURCE=true
            return 1
        fi
    fi

    # 验证文件是否为有效的 ELF/Mach-O 二进制
    if file "$temp_binary" | grep -qE "(executable|ELF|Mach-O)"; then
        log_info "二进制文件验证通过"
    else
        log_error "下载的文件不是有效的可执行文件"
        rm -rf "$temp_dir"
        exit 1
    fi

    # 检查是否已存在二进制文件
    if [ -f "$INSTALL_DIR/subserver" ]; then
        log_warn "发现已安装的二进制文件"
        read -p "是否覆盖现有安装？(y/N): " confirm
        if [[ ! $confirm =~ ^[Yy]$ ]]; then
            log_info "取消覆盖，保留现有版本"
            rm -rf "$temp_dir"
            # 询问是否继续配置
            read -p "是否继续配置服务？(y/N): " continue_config
            if [[ $continue_config =~ ^[Yy]$ ]]; then
                return 0
            else
                log_info "取消安装"
                exit 0
            fi
        fi
        # 用户选择覆盖，先备份旧版本
        log_info "备份旧版本到 ${INSTALL_DIR}/subserver.bak"
        sudo cp "$INSTALL_DIR/subserver" "${INSTALL_DIR}/subserver.bak"
    fi

    # 移动到安装目录
    sudo mkdir -p "$INSTALL_DIR"
    sudo mv "$temp_binary" "$INSTALL_DIR/subserver"
    sudo chmod +x "$INSTALL_DIR/subserver"
    rm -rf "$temp_dir"

    log_info "二进制文件安装成功：$INSTALL_DIR/subserver"
}

# 从源码构建
build_from_source() {
    log_step "正在从源码构建..."

    # 检查 Go 是否安装
    if ! command -v go &> /dev/null; then
        log_error "Go 未安装，请先安装 Go"
        exit 1
    fi

    local temp_dir=$(mktemp -d)
    cd "$temp_dir"

    # 克隆仓库
    log_info "正在克隆仓库..."
    git clone --depth 1 --branch "${VERSION}" "https://github.com/${REPO_OWNER}/${REPO_NAME}.git" .

    # 构建
    log_info "正在编译..."
    go build -o subserver .

    if [ -f "$temp_dir/subserver" ]; then
        sudo mkdir -p "$INSTALL_DIR"
        sudo mv "$temp_dir/subserver" "$INSTALL_DIR/subserver"
        sudo chmod +x "$INSTALL_DIR/subserver"
        log_info "源码构建成功"
    else
        log_error "构建失败"
        rm -rf "$temp_dir"
        exit 1
    fi

    rm -rf "$temp_dir"
}

# 创建配置文件
create_config() {
    log_step "配置文件配置..."

    if [ -f "$CONFIG_FILE" ]; then
        log_warn "配置文件已存在"
        read -p "是否重新配置？(y/N): " confirm
        if [[ ! $confirm =~ ^[Yy]$ ]]; then
            log_info "使用现有配置文件，跳过创建"
            return
        fi
    fi

    echo ""
    log_info "=== 服务器配置 ==="
    echo ""

    # HTTP 端口
    read -p "HTTP 端口 (默认 8080): " http_port
    http_port=${http_port:-8080}

    # HTTPS 配置
    read -p "是否启用 HTTPS? (y/N): " enable_https
    if [[ $enable_https =~ ^[Yy]$ ]]; then
        read -p "HTTPS 端口 (默认 443): " https_port
        https_port=${https_port:-443}
        read -p "域名 (用于 SSL 证书，多个用逗号分隔): " domains
        read -p "联系邮箱 (用于 SSL 证书): " email
        tls_enabled="true"
    else
        https_port="443"
        domains=""
        email=""
        tls_enabled="false"
    fi

    # 上传配置
    echo ""
    log_info "=== 上传配置 ==="
    read -p "上传目录路径 (默认 ./uploads): " upload_dir
    upload_dir=${upload_dir:-./uploads}

    read -p "最大上传文件大小 (MB, 默认 1): " max_upload_size
    max_upload_size=${max_upload_size:-1}

    # 数据库配置
    echo ""
    log_info "=== 数据库配置 ==="
    read -p "数据库文件路径 (默认 ./data/subserver.db): " db_path
    db_path=${db_path:-./data/subserver.db}

    # 日志配置
    echo ""
    log_info "=== 日志配置 ==="
    echo "日志级别：info, warn, error, debug (默认 info): "
    read -p "日志级别 (默认 info): " log_level
    log_level=${log_level:-info}

    # 生成配置文件
    cat > "$CONFIG_FILE" << EOF
# subserver 配置文件
# 由安装脚本自动生成

server:
  http_port: $http_port
  https_port: $https_port

tls:
  enabled: $tls_enabled
  cert_file: ""
  key_file: ""
  cert_dir: "./certs"
  acme_dir: ""
  email: "$email"
EOF

    if [[ -n "$domains" ]]; then
        cat >> "$CONFIG_FILE" << EOF
  domains:
EOF
        echo "$domains" | tr ',' '\n' | while read -r domain; do
            domain=$(echo "$domain" | xargs)  # 去除空格
            if [ -n "$domain" ]; then
                echo "    - $domain" >> "$CONFIG_FILE"
            fi
        done
    fi

    cat >> "$CONFIG_FILE" << EOF

dns:
  provider: ""
  cloudflare:
    api_token: ""
    api_key: ""
    email: ""
  aliyun:
    access_key_id: ""
    access_key_secret: ""
  tencent:
    secret_id: ""
    secret_key: ""
  aws:
    access_key_id: ""
    secret_access_key: ""
    region: ""
  google:
    credentials_file: ""
    project: ""
  azure:
    client_id: ""
    client_secret: ""
    tenant_id: ""
    subscription_id: ""

upload:
  dir: "$upload_dir"
  max_size: $max_upload_size

database:
  sqlite_path: "$db_path"

log:
  level: $log_level
  format: text
EOF

    # 创建上传目录
    sudo mkdir -p "$INSTALL_DIR/uploads"
    sudo chmod 755 "$INSTALL_DIR/uploads"

    # 创建数据目录
    sudo mkdir -p "$INSTALL_DIR/data"
    sudo chmod 755 "$INSTALL_DIR/data"

    log_info "配置文件已创建：$CONFIG_FILE"
}

# 创建 systemd 服务
create_systemd_service() {
    log_step "正在创建 systemd 服务..."

    cat > "$SYSTEMD_SERVICE" << EOF
[Unit]
Description=Subserver - Config File Sharing Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/subserver --config $CONFIG_FILE
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

    # 重载 systemd
    sudo systemctl daemon-reload

    # 启用服务
    sudo systemctl enable "$SERVICE_NAME"

    log_info "Systemd 服务已创建"
}

# 启动服务
start_service() {
    log_step "正在启动服务..."

    sudo systemctl start "$SERVICE_NAME"

    sleep 2

    if sudo systemctl is-active --quiet "$SERVICE_NAME"; then
        log_info "服务启动成功"
    else
        log_warn "服务启动失败，请检查日志：journalctl -u $SERVICE_NAME -f"
    fi
}

# 显示完成信息
show_complete_info() {
    echo ""
    log_info "=========================================="
    log_info "    Subserver 安装完成!"
    log_info "=========================================="
    echo ""
    log_info "安装目录：$INSTALL_DIR"
    log_info "配置文件：$CONFIG_FILE"
    log_info "服务状态：$(sudo systemctl is-active "$SERVICE_NAME")"
    echo ""
    log_info "管理命令:"
    echo "  启动服务：sudo systemctl start $SERVICE_NAME"
    echo "  停止服务：sudo systemctl stop $SERVICE_NAME"
    echo "  重启服务：sudo systemctl restart $SERVICE_NAME"
    echo "  查看状态：sudo systemctl status $SERVICE_NAME"
    echo "  查看日志：journalctl -u $SERVICE_NAME -f"
    echo ""
    log_info "访问地址：http://localhost:${http_port:-8080}"
    echo ""
}

# 卸载函数
uninstall() {
    log_step "正在卸载 Subserver..."

    # 停止服务
    sudo systemctl stop "$SERVICE_NAME" 2>/dev/null || true
    sudo systemctl disable "$SERVICE_NAME" 2>/dev/null || true
    sudo rm -f "$SYSTEMD_SERVICE"
    sudo systemctl daemon-reload

    # 删除安装目录
    read -p "是否删除上传的文件？(y/N，删除后无法恢复): " remove_files
    if [[ $remove_files =~ ^[Yy]$ ]]; then
        sudo rm -rf "$INSTALL_DIR"
    else
        sudo rm -f "$INSTALL_DIR/subserver"
        sudo rm -f "$CONFIG_FILE"
        sudo rm -rf "$INSTALL_DIR/certs"
    fi

    log_info "卸载完成"
}

# 主函数
main() {
    echo ""
    echo "=========================================="
    echo "    Subserver 安装脚本"
    echo "=========================================="
    echo ""

    # 检查参数
    if [ "$1" = "--uninstall" ] || [ "$1" = "-u" ]; then
        uninstall
        exit 0
    fi

    if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
        echo "用法：$0 [选项]"
        echo ""
        echo "选项:"
        echo "  --uninstall, -u    卸载 Subserver"
        echo "  --help, -h         显示帮助信息"
        echo ""
        echo "不带参数时执行安装操作"
        echo ""
        echo "本脚本将从 GitHub 下载最新的二进制文件并安装为 systemd 服务"
        exit 0
    fi

    # 检查是否为 root 用户
    if [ "$EUID" -ne 0 ]; then
        log_warn "建议使用 root 用户运行此脚本"
        log_warn "将使用 sudo 提权..."
    fi

    # 检查 curl 是否安装
    if ! command -v curl &> /dev/null; then
        log_error "curl 未安装，请先安装 curl"
        exit 1
    fi

    # 执行安装步骤
    detect_arch
    detect_os
    get_latest_version

    # 尝试下载二进制文件，失败则从源码构建
    BUILD_FROM_SOURCE=false
    download_binary
    if [ "$BUILD_FROM_SOURCE" = true ]; then
        build_from_source
    fi

    create_config
    create_systemd_service
    start_service
    show_complete_info
}

# 运行主函数
main "$@"
