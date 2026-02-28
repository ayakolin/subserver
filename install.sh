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

# 检测包管理器
detect_package_manager() {
    if command -v apt &> /dev/null; then
        PM="apt"
    elif command -v yum &> /dev/null; then
        PM="yum"
    elif command -v dnf &> /dev/null; then
        PM="dnf"
    elif command -v pacman &> /dev/null; then
        PM="pacman"
    elif command -v apk &> /dev/null; then
        PM="apk"
    else
        log_error "不支持的包管理器，请手动安装 Go"
        exit 1
    fi
    log_info "检测到包管理器：$PM"
}

# 检查并安装 Go
install_go() {
    if command -v go &> /dev/null; then
        GO_VERSION=$(go version)
        log_info "Go 已安装：$GO_VERSION"
        return
    fi

    log_step "正在安装 Go..."

    case $PM in
        apt|yum|dnf)
            if [ "$PM" = "apt" ]; then
                sudo apt update -y
                sudo apt install -y golang-go
            else
                sudo $PM install -y golang
            fi
            ;;
        pacman)
            sudo pacman -S --noconfirm go
            ;;
        apk)
            sudo apk add --no-cache go
            ;;
    esac

    if command -v go &> /dev/null; then
        log_info "Go 安装成功：$(go version)"
    else
        log_error "Go 安装失败"
        exit 1
    fi
}

# 检查并安装 Git
install_git() {
    if command -v git &> /dev/null; then
        log_info "Git 已安装：$(git --version)"
        return
    fi

    log_step "正在安装 Git..."

    case $PM in
        apt)
            sudo apt update -y
            sudo apt install -y git
            ;;
        yum|dnf)
            sudo $PM install -y git
            ;;
        pacman)
            sudo pacman -S --noconfirm git
            ;;
        apk)
            sudo apk add --no-cache git
            ;;
    esac

    log_info "Git 安装成功"
}

# 克隆或更新代码
clone_repo() {
    if [ -d "$INSTALL_DIR" ]; then
        log_step "发现已存在的安装目录"
        read -p "是否覆盖安装？(y/N): " confirm
        if [[ $confirm =~ ^[Yy]$ ]]; then
            sudo rm -rf "$INSTALL_DIR"
        else
            log_info "取消安装"
            exit 0
        fi
    fi

    log_step "正在克隆代码到 $INSTALL_DIR..."
    sudo mkdir -p "$INSTALL_DIR"
    sudo chown "$USER:$USER" "$INSTALL_DIR"

    git clone https://github.com/rinca/subserver.git "$INSTALL_DIR"

    log_info "代码克隆成功"
}

# 构建项目
build_project() {
    log_step "正在构建项目..."

    cd "$INSTALL_DIR"

    # 下载依赖
    log_info "正在下载 Go 依赖..."
    go mod download

    # 构建
    log_info "正在编译二进制文件..."
    go build -o subserver .

    if [ -f "$INSTALL_DIR/subserver" ]; then
        chmod +x "$INSTALL_DIR/subserver"
        log_info "构建成功"
    else
        log_error "构建失败"
        exit 1
    fi
}

# 创建配置文件
create_config() {
    log_step "正在创建配置文件..."

    if [ -f "$CONFIG_FILE" ]; then
        log_warn "配置文件已存在，跳过创建"
        read -p "是否重新配置？(y/N): " confirm
        if [[ ! $confirm =~ ^[Yy]$ ]]; then
            return
        fi
    fi

    # 交互式配置
    echo ""
    log_info "=== 服务器配置 ==="
    read -p "HTTP 端口 (默认 8080): " http_port
    http_port=${http_port:-8080}

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

    # 生成配置文件
    cat > "$CONFIG_FILE" << EOF
# subserver 配置文件
# 由安装脚本自动生成

server:
  http_port: $http_port
  https_port: $https_port
EOF

    if [[ -n "$domains" ]]; then
        # 将逗号分隔的域名转换为 YAML 数组格式
        domains_yaml=$(echo "$domains" | tr ',' '\n' | sed 's/^/    - /' | tr -d ' ')
        echo "  domains:" >> "$CONFIG_FILE"
        echo "$domains_yaml" >> "$CONFIG_FILE"
    fi

    cat >> "$CONFIG_FILE" << EOF

tls:
  enabled: $tls_enabled
  cert_file: ""
  key_file: ""
  cert_dir: "./certs"
  acme_dir: ""
  email: "$email"

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

log:
  level: info
  format: text
EOF

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
ExecStart=$INSTALL_DIR/subserver
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

# 创建上传目录
create_upload_dir() {
    log_step "正在创建上传目录..."
    mkdir -p "$INSTALL_DIR/uploads"
    chmod 755 "$INSTALL_DIR/uploads"
    log_info "上传目录已创建"
}

# 启动服务
start_service() {
    log_step "正在启动服务..."

    sudo systemctl start "$SERVICE_NAME"

    sleep 2

    if sudo systemctl is-active --quiet "$SERVICE_NAME"; then
        log_info "服务启动成功"
    else
        log_warn "服务启动失败，请检查日志：journalctl -u $SERVICE_NAME"
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
        sudo rm -rf "$INSTALL_DIR/subserver"
        sudo rm -rf "$INSTALL_DIR/internal"
        sudo rm -rf "$INSTALL_DIR/go.mod"
        sudo rm -rf "$INSTALL_DIR/go.sum"
        sudo rm -rf "$INSTALL_DIR/main.go"
        sudo rm -rf "$INSTALL_DIR/index.html"
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
        exit 0
    fi

    # 检查是否为 root 用户
    if [ "$EUID" -ne 0 ]; then
        log_warn "建议使用 root 用户运行此脚本"
        log_warn "将使用 sudo 提权..."
    fi

    # 执行安装步骤
    detect_package_manager
    install_go
    install_git
    clone_repo
    build_project
    create_config
    create_upload_dir
    create_systemd_service
    start_service
    show_complete_info
}

# 运行主函数
main "$@"
