#!/usr/bin/env bash

set -e

# =============================================================================
# subserver 安装脚本
# 支持从 GitHub Release 自动下载对应架构的二进制文件并进行引导式部署
# 用法：curl -fsSL https://github.com/ayakolin/subserver/releases/latest/download/install.sh | bash
# =============================================================================

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

# 检查是否以 root 运行
is_root() {
    [ "$(id -u)" = "0" ]
}

# 智能 sudo - 如果已经是 root 则直接执行，否则使用 sudo
run_cmd() {
    if is_root; then
        "$@"
    else
        sudo "$@"
    fi
}

# 默认配置
DEFAULT_PORT="8080"
DEFAULT_TLS="false"
DEFAULT_TLS_PORT="443"
DEFAULT_LOCAL_TLS="false"
DEFAULT_DB="./data/subserver.db"
DEFAULT_LOG="info"
DEFAULT_CERT_DIR="./certs"

# GitHub 仓库信息
GITHUB_OWNER="${GITHUB_OWNER:-ayakolin}"
GITHUB_REPO="${GITHUB_REPO:-subserver}"

# 安装目录
INSTALL_DIR="/usr/local/bin"
SERVICE_NAME="subserver"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
CONFIG_DIR="/etc/subserver"
CONFIG_FILE="${CONFIG_DIR}/config.env"
DATA_DIR="/var/lib/subserver"

# 用户输入的配置
USER_PORT=""
USER_TLS=""
USER_TLS_PORT=""
USER_LOCAL_TLS=""
USER_CERT_FILE=""
USER_KEY_FILE=""
USER_DOMAIN=""
USER_TLS_EMAIL=""
USER_DB=""
USER_LOG=""
USER_CERT_DIR=""

# 检测系统架构
detect_arch() {
    local arch=$(uname -m)
    case "$arch" in
        x86_64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        armv7l)
            echo "arm"
            ;;
        *)
            log_error "不支持的架构：$arch"
            exit 1
            ;;
    esac
}

# 检测操作系统
detect_os() {
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        linux)
            echo "linux"
            ;;
        darwin)
            echo "darwin"
            ;;
        *)
            log_error "不支持的操作系统：$os"
            exit 1
            ;;
    esac
}

# 获取最新 release 版本
get_latest_version() {
    local url="https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/releases/latest"
    local version
    version=$(curl -fsSL "$url" 2>/dev/null | grep '"tag_name"' | head -1 | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "$version" ]; then
        # 如果 API 调用失败，尝试从 releases 页面获取
        version="latest"
    fi
    echo "$version"
}

# 下载二进制文件
download_binary() {
    local os="$1"
    local arch="$2"
    local version="$3"

    local binary_name="subserver_${version}_${os}_${arch}"
    local extension="tar.gz"
    local checksum_file=""
    local download_url=""

    # Windows 使用 zip，其他用 tar.gz
    if [ "$os" = "windows" ]; then
        binary_name="${binary_name}.exe"
        extension="zip"
    fi

    local archive_name="${binary_name}.${extension}"
    local base_url="https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}/releases"

    if [ "$version" = "latest" ]; then
        download_url="${base_url}/latest/download/${archive_name}"
        checksum_file="${archive_name}.sha256"
    else
        download_url="${base_url}/download/${version}/${archive_name}"
        checksum_file="${archive_name}.sha256"
    fi

    log_step "下载版本：$version"
    log_info "下载地址：$download_url"

    # 创建临时目录
    local tmp_dir=$(mktemp -d)
    cd "$tmp_dir"

    # 下载文件
    if ! curl -fsSL "$download_url" -o "$archive_name" 2>/dev/null; then
        log_error "下载失败，请检查网络连接或仓库地址"
        cd - > /dev/null
        rm -rf "$tmp_dir"
        return 1
    fi

    # 下载并验证 checksum（可选）
    local checksum_url="${download_url%.${extension}}.${extension}.sha256"
    if curl -fsSL "$checksum_url" -o "${checksum_file}" 2>/dev/null; then
        log_info "验证 checksum..."
        if sha256sum -c "${checksum_file}" >/dev/null 2>&1; then
            log_info "Checksum 验证通过"
        else
            log_warn "Checksum 验证失败，继续安装..."
        fi
    else
        log_warn "无法下载 checksum 文件，跳过验证"
    fi

    # 解压文件
    log_step "解压文件..."
    if [ "$extension" = "zip" ]; then
        if ! command -v unzip &> /dev/null; then
            log_error "需要安装 unzip 来解压 zip 文件"
            cd - > /dev/null
            rm -rf "$tmp_dir"
            return 1
        fi
        unzip -o "$archive_name" >/dev/null 2>&1
    else
        tar -xzf "$archive_name"
    fi

    # 找到二进制文件
    local binary_file
    if [ "$os" = "windows" ]; then
        binary_file="${binary_name}"
    else
        binary_file="subserver"
    fi

    if [ ! -f "$binary_file" ]; then
        # 尝试查找解压后的文件，排除 checksum 文件和压缩包
        # 优先查找名为 subserver 的可执行文件
        binary_file=$(find . -name "subserver" -type f | head -1 | sed 's|^\./||')

        # 如果还没找到，查找 subserver 开头的文件（排除 .sha256 和 .tar.gz）
        if [ -z "$binary_file" ] || [ ! -f "$binary_file" ]; then
            binary_file=$(find . -type f -name "subserver*" ! -name "*.sha256" ! -name "*.tar.gz" ! -name "*.zip" | head -1 | sed 's|^\./||')
        fi
    fi

    if [ -z "$binary_file" ] || [ ! -f "$binary_file" ]; then
        log_error "未找到二进制文件"
        cd - > /dev/null
        rm -rf "$tmp_dir"
        return 1
    fi

    # 安装到目标目录
    log_step "安装到 $INSTALL_DIR..."
    if [ "$os" = "darwin" ]; then
        # macOS 可能需要 sudo
        if [ ! -w "$INSTALL_DIR" ]; then
            run_cmd cp "$binary_file" "${INSTALL_DIR}/subserver"
            run_cmd chmod +x "${INSTALL_DIR}/subserver"
        else
            cp "$binary_file" "${INSTALL_DIR}/subserver"
            chmod +x "${INSTALL_DIR}/subserver"
        fi
    else
        # Linux
        if [ ! -w "$INSTALL_DIR" ]; then
            run_cmd cp "$binary_file" "${INSTALL_DIR}/subserver"
            run_cmd chmod +x "${INSTALL_DIR}/subserver"
        else
            cp "$binary_file" "${INSTALL_DIR}/subserver"
            chmod +x "${INSTALL_DIR}/subserver"
        fi
    fi

    # 清理临时目录
    cd - > /dev/null
    rm -rf "$tmp_dir"

    log_info "安装完成"
    return 0
}

# 提示用户输入（带默认值）
prompt_input() {
    local prompt="$1"
    local default="$2"
    local result_var="$3"

    if [ -n "$default" ]; then
        echo -n "${prompt} [${default}]: "
    else
        echo -n "${prompt}: "
    fi

    if [ "$INTERACTIVE" = "false" ]; then
        echo ""
        eval "${result_var}=\"${default}\""
        return
    fi

    read -r input
    if [ -z "$input" ] && [ -n "$default" ]; then
        eval "${result_var}=\"${default}\""
    else
        eval "${result_var}=\"${input}\""
    fi
}

# 提示用户选择（是/否）
prompt_yes_no() {
    local prompt="$1"
    local default="$2"
    local result_var="$3"

    local default_display
    if [ "$default" = "true" ] || [ "$default" = "yes" ] || [ "$default" = "1" ]; then
        default_display="Y/n"
    else
        default_display="y/N"
    fi

    echo -n "${prompt} [${default_display}]: "

    if [ "$INTERACTIVE" = "false" ]; then
        echo ""
        eval "${result_var}=\"${default}\""
        return
    fi

    read -r input
    if [ -z "$input" ]; then
        eval "${result_var}=\"${default}\""
    else
        case "$(echo "$input" | tr '[:upper:]' '[:lower:]')" in
            y|yes|true|1)
                eval "${result_var}=\"true\""
                ;;
            *)
                eval "${result_var}=\"false\""
                ;;
        esac
    fi
}

# 交互式配置
interactive_setup() {
    echo ""
    log_step "开始配置 subserver"
    echo "================================"
    echo ""

    # HTTP 端口
    prompt_input "HTTP 端口" "$DEFAULT_PORT" "USER_PORT"

    # 是否启用 HTTPS
    prompt_yes_no "启用 HTTPS" "$DEFAULT_TLS" "USER_TLS"

    if [ "$USER_TLS" = "true" ]; then
        # HTTPS 端口
        prompt_input "HTTPS 端口" "$DEFAULT_TLS_PORT" "USER_TLS_PORT"

        # 证书类型选择
        echo ""
        echo "请选择证书获取方式:"
        echo "  1) 使用 Let's Encrypt 自动申请证书（需要域名）"
        echo "  2) 使用本地已有证书文件"
        echo -n "请选择 [1]: "

        if [ "$INTERACTIVE" = "false" ]; then
            cert_choice="1"
        else
            read -r cert_choice
            [ -z "$cert_choice" ] && cert_choice="1"
        fi

        if [ "$cert_choice" = "2" ]; then
            USER_LOCAL_TLS="true"
            prompt_input "证书文件路径" "" "USER_CERT_FILE"
            prompt_input "私钥文件路径" "" "USER_KEY_FILE"
        else
            USER_LOCAL_TLS="false"
            prompt_input "域名（多个用逗号分隔）" "" "USER_DOMAIN"
            prompt_input "SSL 证书邮箱" "" "USER_TLS_EMAIL"
        fi
    fi

    # 数据库路径
    prompt_input "数据库文件路径" "$DEFAULT_DB" "USER_DB"

    # 日志级别
    prompt_input "日志级别 (debug/info/warn/error)" "$DEFAULT_LOG" "USER_LOG"

    # 证书目录
    if [ "$USER_TLS" = "true" ] && [ "$USER_LOCAL_TLS" = "false" ]; then
        prompt_input "SSL 证书目录" "$DEFAULT_CERT_DIR" "USER_CERT_DIR"
    fi

    echo ""
    log_info "配置完成"
}

# 创建配置文件
create_config() {
    log_step "创建配置文件..."

    # 创建配置目录
    if [ ! -d "$CONFIG_DIR" ]; then
        run_cmd mkdir -p "$CONFIG_DIR"
    fi

    # 处理数据库路径（转换为绝对路径）
    local db_path="$USER_DB"
    if [[ ! "$db_path" = /* ]]; then
        # 相对路径，转换为 /var/lib/subserver 下的绝对路径
        run_cmd mkdir -p "$DATA_DIR"
        db_path="${DATA_DIR}/$(basename "$db_path")"
    fi

    # 生成配置文件（用于参考和备份）
    cat > "$CONFIG_FILE" << EOF
# subserver 配置文件
# 由安装脚本生成于 $(date)

# HTTP 端口
PORT=${USER_PORT}

# HTTPS 配置
TLS_ENABLED=${USER_TLS}
TLS_PORT=${USER_TLS_PORT}
LOCAL_TLS=${USER_LOCAL_TLS}
EOF

    if [ "$USER_LOCAL_TLS" = "true" ]; then
        cat >> "$CONFIG_FILE" << EOF
CERT_FILE=${USER_CERT_FILE}
KEY_FILE=${USER_KEY_FILE}
EOF
    else
        cat >> "$CONFIG_FILE" << EOF
DOMAINS=${USER_DOMAIN}
TLS_EMAIL=${USER_TLS_EMAIL}
CERT_DIR=${USER_CERT_DIR}
EOF
    fi

    cat >> "$CONFIG_FILE" << EOF

# 数据库配置
DB_PATH=${db_path}

# 日志配置
LOG_LEVEL=${USER_LOG}
EOF

    run_cmd chmod 644 "$CONFIG_FILE"
    log_info "配置文件已创建：$CONFIG_FILE"
}

# 创建 systemd 服务文件
create_service() {
    log_step "创建 systemd 服务..."

    # 处理数据库路径（转换为绝对路径）
    local db_path="$USER_DB"
    if [[ ! "$db_path" = /* ]]; then
        db_path="${DATA_DIR}/$(basename "$db_path")"
    fi

    # 构建 ExecStart 命令行
    local exec_start="/usr/local/bin/subserver -p ${USER_PORT} -db ${db_path} -log ${USER_LOG}"

    # 添加 HTTPS 相关参数
    if [ "$USER_TLS" = "true" ]; then
        exec_start="${exec_start} -tls -tls-port ${USER_TLS_PORT}"
        if [ "$USER_LOCAL_TLS" = "true" ]; then
            exec_start="${exec_start} -local-tls -cert-file ${USER_CERT_FILE} -key-file ${USER_KEY_FILE}"
        else
            exec_start="${exec_start} -d ${USER_DOMAIN} -tls-email ${USER_TLS_EMAIL} -cert-dir ${USER_CERT_DIR}"
        fi
    fi

    cat > "$SERVICE_FILE" << EOF
[Unit]
Description=subserver - Simple File Sharing Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/var/lib/subserver
ExecStart=${exec_start}
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

    run_cmd chmod 644 "$SERVICE_FILE"
    log_info "服务文件已创建：$SERVICE_FILE"
}

# 启动服务
start_service() {
    log_step "启动服务..."

    # 重新加载 systemd
    if ! run_cmd systemctl daemon-reload 2>&1; then
        log_error "systemctl daemon-reload 失败"
        return 1
    fi

    # 启用服务
    if ! run_cmd systemctl enable "$SERVICE_NAME" 2>&1; then
        log_warn "systemctl enable 失败，服务可能已存在"
    fi

    # 启动服务
    if ! run_cmd systemctl start "$SERVICE_NAME" 2>&1; then
        log_error "服务启动失败"
        run_cmd systemctl status "$SERVICE_NAME" --no-pager -l 2>&1 || true
        return 1
    fi

    # 检查状态
    sleep 2
    if run_cmd systemctl is-active --quiet "$SERVICE_NAME"; then
        log_info "服务启动成功"
        run_cmd systemctl status "$SERVICE_NAME" --no-pager -l
    else
        log_error "服务状态异常"
        run_cmd systemctl status "$SERVICE_NAME" --no-pager -l
        return 1
    fi
}

# 显示配置摘要
show_summary() {
    echo ""
    echo "================================"
    log_info "安装完成!"
    echo "================================"
    echo ""
    echo "配置摘要:"
    echo "  HTTP 端口：${USER_PORT}"
    if [ "$USER_TLS" = "true" ]; then
        echo "  HTTPS 端口：${USER_TLS_PORT}"
        if [ "$USER_LOCAL_TLS" = "true" ]; then
            echo "  证书文件：${USER_CERT_FILE}"
            echo "  私钥文件：${USER_KEY_FILE}"
        else
            echo "  域名：${USER_DOMAIN}"
            echo "  证书邮箱：${USER_TLS_EMAIL}"
        fi
    fi
    echo "  数据库路径：${USER_DB}"
    echo "  日志级别：${USER_LOG}"
    echo ""
    echo "访问地址:"
    if [ "$USER_TLS" = "true" ]; then
        echo "  HTTPS: https://localhost:${USER_TLS_PORT}"
    else
        echo "  HTTP: http://localhost:${USER_PORT}"
    fi
    echo ""
    echo "管理命令:"
    echo "  启动服务：sudo systemctl start ${SERVICE_NAME}"
    echo "  停止服务：sudo systemctl stop ${SERVICE_NAME}"
    echo "  重启服务：sudo systemctl restart ${SERVICE_NAME}"
    echo "  查看状态：sudo systemctl status ${SERVICE_NAME}"
    echo "  查看日志：sudo journalctl -u ${SERVICE_NAME} -f"
    echo ""
}

# 卸载函数
do_uninstall() {
    echo ""
    echo "================================"
    echo "  subserver 卸载程序"
    echo "================================"
    echo ""

    # 检查是否在管道模式下
    if [ "$INTERACTIVE" = "false" ] && [ "$FORCE_UNINSTALL" != "true" ]; then
        log_error "卸载操作需要确认，请使用 -f 参数或交互式运行"
        echo ""
        echo "用法:"
        echo "  sudo bash install.sh --uninstall     # 交互式卸载"
        echo "  sudo bash install.sh --uninstall -f  # 强制卸载（不确认）"
        echo "  curl -fsSL ... | sudo bash -s -- --uninstall -f"
        exit 1
    fi

    # 确认卸载
    if [ "$FORCE_UNINSTALL" != "true" ]; then
        echo -n "${YELLOW}警告：这将卸载 subserver 及其配置文件。继续？[y/N]: ${NC}"
        read -r confirm
        if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
            log_info "取消卸载"
            exit 0
        fi
    fi

    log_step "停止服务..."
    if has_systemd; then
        if run_cmd systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
            run_cmd systemctl stop "$SERVICE_NAME" && log_info "服务已停止"
        else
            log_warn "服务未运行"
        fi

        log_step "禁用服务..."
        if run_cmd systemctl is-enabled --quiet "$SERVICE_NAME" 2>/dev/null; then
            run_cmd systemctl disable "$SERVICE_NAME" && log_info "服务已禁用"
        else
            log_warn "服务未启用"
        fi
    fi

    log_step "删除服务文件..."
    if [ -f "$SERVICE_FILE" ]; then
        run_cmd rm -f "$SERVICE_FILE" && log_info "服务文件已删除：$SERVICE_FILE"
    else
        log_warn "服务文件不存在：$SERVICE_FILE"
    fi

    if has_systemd; then
        run_cmd systemctl daemon-reload
    fi

    log_step "删除二进制文件..."
    if [ -f "${INSTALL_DIR}/subserver" ]; then
        run_cmd rm -f "${INSTALL_DIR}/subserver" && log_info "二进制文件已删除：${INSTALL_DIR}/subserver"
    else
        log_warn "二进制文件不存在：${INSTALL_DIR}/subserver"
    fi

    # 询问是否删除配置文件
    if [ "$FORCE_UNINSTALL" != "true" ]; then
        echo -n "是否删除配置文件？[y/N]: "
        read -r remove_config
        if [[ "$remove_config" =~ ^[Yy]$ ]]; then
            REMOVE_CONFIG="true"
        else
            REMOVE_CONFIG="false"
        fi
    else
        REMOVE_CONFIG="$REMOVE_CONFIG_ON_UNINSTALL"
    fi

    if [ "$REMOVE_CONFIG" = "true" ]; then
        log_step "删除配置文件..."
        if [ -f "$CONFIG_FILE" ]; then
            run_cmd rm -f "$CONFIG_FILE" && log_info "配置文件已删除：$CONFIG_FILE"
        fi
        if [ -d "$CONFIG_DIR" ]; then
            run_cmd rmdir "$CONFIG_DIR" 2>/dev/null && log_info "配置目录已删除：$CONFIG_DIR" || true
        fi
    else
        log_info "保留配置文件"
    fi

    # 询问是否删除数据文件
    if [ "$FORCE_UNINSTALL" != "true" ]; then
        echo -n "是否删除数据库和证书文件？${RED}（警告：这将永久删除数据）${NC} [y/N]: "
        read -r remove_data
        if [[ "$remove_data" =~ ^[Yy]$ ]]; then
            REMOVE_DATA="true"
        else
            REMOVE_DATA="false"
        fi
    else
        REMOVE_DATA="$REMOVE_DATA_ON_UNINSTALL"
    fi

    if [ "$REMOVE_DATA" = "true" ]; then
        log_step "删除数据文件..."
        if [ -d "$DATA_DIR" ]; then
            run_cmd rm -rf "$DATA_DIR" && log_info "数据目录已删除：$DATA_DIR"
        fi

        # 删除默认证书目录
        if [ -d "./certs" ]; then
            run_cmd rm -rf "./certs" && log_info "证书目录已删除：./certs"
        fi

        # 删除默认数据库文件
        if [ -f "./data/subserver.db" ]; then
            run_cmd rm -f "./data/subserver.db" && log_info "数据库文件已删除：./data/subserver.db"
        fi
    else
        log_info "保留数据文件"
    fi

    echo ""
    echo "================================"
    log_info "卸载完成!"
    echo "================================"
    echo ""

    if [ "$REMOVE_CONFIG" = "false" ] || [ "$REMOVE_DATA" = "false" ]; then
        echo "以下文件已被保留:"
        if [ "$REMOVE_CONFIG" = "false" ] && [ -f "$CONFIG_FILE" ]; then
            echo "  - $CONFIG_FILE"
        fi
        if [ "$REMOVE_DATA" = "false" ]; then
            echo "  - $DATA_DIR (数据目录)"
        fi
        echo ""
    fi

    log_info "感谢您使用 subserver!"
}

# 检测是否支持 systemd
has_systemd() {
    # 首先检查 systemctl 命令是否存在
    if ! command -v systemctl &> /dev/null; then
        return 1
    fi

    # 检查是否在容器中运行（某些容器没有完整的 systemd）
    if [ -f /.dockerenv ] || [ -f /run/.containerenv ]; then
        return 1
    fi

    # 检查 systemd 是否正在运行
    if [ -d "/run/systemd/system" ] || [ -f "/run/systemd/container" ]; then
        return 0
    fi

    # 备用检测：尝试运行 systemctl --version
    if systemctl --version &> /dev/null; then
        return 0
    fi

    return 1
}

# 主函数
main() {
    echo ""
    echo "================================"
    echo "  subserver 安装脚本"
    echo "================================"
    echo ""

    # 检测操作系统和架构
    local os=$(detect_os)
    local arch=$(detect_arch)

    log_info "检测到系统：${os}/${arch}"

    # 检查依赖
    if ! command -v curl &> /dev/null; then
        log_error "需要安装 curl"
        exit 1
    fi

    if ! command -v tar &> /dev/null; then
        log_error "需要安装 tar"
        exit 1
    fi

    if ! command -v gzip &> /dev/null; then
        log_error "需要安装 gzip"
        exit 1
    fi

    if ! command -v sha256sum &> /dev/null; then
        log_warn "未找到 sha256sum，将跳过 checksum 验证"
    fi

    # 检查 sudo 是否存在（非 root 用户需要）
    if ! is_root && ! command -v sudo &> /dev/null; then
        log_error "非 root 用户需要 sudo 命令，请安装：apt-get install sudo"
        exit 1
    fi

    # 获取最新版本
    local version
    version=$(get_latest_version)
    log_info "最新版本：$version"

    # 下载并安装二进制
    if ! download_binary "$os" "$arch" "$version"; then
        log_error "安装失败"
        exit 1
    fi

    # 创建数据目录
    if [ "$os" = "linux" ]; then
        run_cmd mkdir -p "$DATA_DIR"
        run_cmd mkdir -p "$CONFIG_DIR"

        # 交互式配置
        interactive_setup

        # 创建配置文件
        create_config

        # 检查 systemd
        if has_systemd; then
            create_service
            if ! start_service; then
                log_warn "systemd 服务启动失败，你可以手动运行："
                log_warn "  subserver -p ${USER_PORT} -db ${USER_DB} -log ${USER_LOG}"
            fi
        else
            log_warn "未检测到 systemd，跳过服务创建"
            log_info "请手动运行：subserver -p ${USER_PORT} -db ${USER_DB} -log ${USER_LOG}"
        fi

        show_summary
    else
        # macOS 简化安装
        log_info "macOS 安装完成"
        log_info "二进制文件已安装到：$INSTALL_DIR/subserver"
        log_info "请手动配置并运行 subserver"
    fi

    echo ""
    log_info "安装完成!"
}

# 解析命令行参数（支持非交互式）
INTERACTIVE="true"
UNINSTALL="false"
FORCE_UNINSTALL="false"
REMOVE_CONFIG="false"
REMOVE_CONFIG_ON_UNINSTALL="false"
REMOVE_DATA="false"
REMOVE_DATA_ON_UNINSTALL="false"

# 检测是否在管道模式下运行（curl | bash）
if [ ! -t 0 ]; then
    # 标准输入不是终端，说明是在管道模式下
    INTERACTIVE="false"
fi

# 检查是否直接传入了 --uninstall 参数
# 同时过滤掉长参数，避免 getopts 报错
FILTERED_ARGS=()
for arg in "$@"; do
    case "$arg" in
        --uninstall)
            UNINSTALL="true"
            ;;
        -f|--force)
            FORCE_UNINSTALL="true"
            ;;
        --remove-config)
            REMOVE_CONFIG_ON_UNINSTALL="true"
            ;;
        --remove-data)
            REMOVE_DATA_ON_UNINSTALL="true"
            ;;
        -*)
            # 跳过所有长参数，避免 getopts 报错
            ;;
        *)
            FILTERED_ARGS+=("$arg")
            ;;
    esac
done

# 使用 getopts 解析短参数
while getopts "p:tls:tls-port:local-tls:cert-file:key-file:d:tls-email:db:log:cert-dir:huf" opt 2>/dev/null; do
    case $opt in
        p)
            USER_PORT="$OPTARG"
            INTERACTIVE="false"
            ;;
        tls)
            USER_TLS="$OPTARG"
            INTERACTIVE="false"
            ;;
        tls-port)
            USER_TLS_PORT="$OPTARG"
            INTERACTIVE="false"
            ;;
        local-tls)
            USER_LOCAL_TLS="$OPTARG"
            INTERACTIVE="false"
            ;;
        cert-file)
            USER_CERT_FILE="$OPTARG"
            INTERACTIVE="false"
            # 如果提供了 cert-file 但没有明确设置 local-tls，自动启用
            if [ -z "$USER_LOCAL_TLS" ]; then
                USER_LOCAL_TLS="true"
            fi
            ;;
        key-file)
            USER_KEY_FILE="$OPTARG"
            INTERACTIVE="false"
            # 如果提供了 key-file 但没有明确设置 local-tls，自动启用
            if [ -z "$USER_LOCAL_TLS" ]; then
                USER_LOCAL_TLS="true"
            fi
            ;;
        d)
            USER_DOMAIN="$OPTARG"
            INTERACTIVE="false"
            ;;
        tls-email)
            USER_TLS_EMAIL="$OPTARG"
            INTERACTIVE="false"
            ;;
        db)
            USER_DB="$OPTARG"
            INTERACTIVE="false"
            ;;
        log)
            USER_LOG="$OPTARG"
            INTERACTIVE="false"
            ;;
        cert-dir)
            USER_CERT_DIR="$OPTARG"
            INTERACTIVE="false"
            ;;
        u)
            UNINSTALL="true"
            ;;
        f)
            FORCE_UNINSTALL="true"
            ;;
        h)
            echo "用法：install.sh [选项]"
            echo ""
            echo "安装选项:"
            echo "  -p PORT          HTTP 端口 (默认：8080)"
            echo "  -tls BOOL        启用 HTTPS (默认：false)"
            echo "  -tls-port PORT   HTTPS 端口 (默认：443)"
            echo "  -local-tls BOOL  使用本地证书文件 (默认：false)"
            echo "  -cert-file PATH  证书文件路径（与 -local-tls 一起使用）"
            echo "  -key-file PATH   私钥文件路径（与 -local-tls 一起使用）"
            echo "  -d DOMAIN        域名（多个用逗号分隔）"
            echo "  -tls-email EMAIL SSL 证书邮箱"
            echo "  -db PATH         数据库文件路径 (默认：./data/subserver.db)"
            echo "  -log LEVEL       日志级别 (默认：info)"
            echo "  -cert-dir DIR    SSL 证书目录 (默认：./certs)"
            echo ""
            echo "卸载选项:"
            echo "  -u, --uninstall  卸载 subserver"
            echo "  -f, --force      强制卸载（不询问确认）"
            echo "  --remove-config  卸载时删除配置文件"
            echo "  --remove-data    卸载时删除数据文件（数据库、证书）"
            echo ""
            echo "其他:"
            echo "  -h               显示帮助"
            echo ""
            echo "示例:"
            echo "  安装:"
            echo "    curl -fsSL ... | bash -s -- -p 8080"
            echo "    curl -fsSL ... | bash -s -- -p 443 -tls true -d example.com -tls-email admin@example.com"
            echo ""
            echo "  卸载:"
            echo "    sudo bash install.sh -u              # 交互式卸载"
            echo "    sudo bash install.sh -u -f           # 强制卸载"
            echo "    sudo bash install.sh -u -f --remove-config --remove-data  # 完全卸载"
            echo "    curl -fsSL ... | sudo bash -s -- -u -f"
            exit 0
            ;;
        \?)
            # 忽略无效选项
            ;;
        *)
            ;;
    esac
done

# 设置默认值（如果未提供）
[ -z "$USER_PORT" ] && USER_PORT="$DEFAULT_PORT"
[ -z "$USER_TLS" ] && USER_TLS="$DEFAULT_TLS"
[ -z "$USER_TLS_PORT" ] && USER_TLS_PORT="$DEFAULT_TLS_PORT"
[ -z "$USER_LOCAL_TLS" ] && USER_LOCAL_TLS="$DEFAULT_LOCAL_TLS"
[ -z "$USER_DB" ] && USER_DB="$DEFAULT_DB"
[ -z "$USER_LOG" ] && USER_LOG="$DEFAULT_LOG"
[ -z "$USER_CERT_DIR" ] && USER_CERT_DIR="$DEFAULT_CERT_DIR"

# 执行卸载或安装
if [ "$UNINSTALL" = "true" ]; then
    do_uninstall
else
    main
fi
