#!/bin/bash

# NSPass Agent 安装/升级脚本
# 使用方法: curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash

set -e

# 版本信息
SCRIPT_VERSION="2.0.0"
GITHUB_REPO="nspass/nspass-agent"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/nspass"
SERVICE_NAME="nspass-agent"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印函数
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

# 检查是否以root用户运行
check_root() {
    if [ "$EUID" -ne 0 ]; then
        print_error "请以root用户运行此脚本"
        exit 1
    fi
}

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
        armv7l|armv6l)
            ARCH="arm"
            ;;
        i386|i686)
            ARCH="386"
            ;;
        *)
            print_error "不支持的架构: $arch"
            print_error "支持的架构: x86_64, aarch64, armv7l, i386"
            exit 1
            ;;
    esac
    print_info "检测到系统架构: $arch (使用: $ARCH)"
}

# 检测操作系统
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        OS_VERSION=$VERSION_ID
        OS_NAME=$NAME
        print_info "检测到操作系统: $OS_NAME $OS_VERSION"
    else
        print_error "无法检测操作系统"
        exit 1
    fi
    
    # 检查systemd支持
    if ! command -v systemctl >/dev/null 2>&1; then
        print_error "此脚本需要systemd支持"
        exit 1
    fi
}

# 获取当前已安装的版本
get_current_version() {
    if [ -f "$INSTALL_DIR/nspass-agent" ]; then
        # 尝试获取版本，如果失败则返回空
        CURRENT_VERSION=$("$INSTALL_DIR/nspass-agent" --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "")
        if [ -n "$CURRENT_VERSION" ]; then
            print_info "当前安装版本: $CURRENT_VERSION"
        else
            print_warn "无法获取当前版本信息"
            CURRENT_VERSION=""
        fi
    else
        print_info "未检测到已安装的nspass-agent"
        CURRENT_VERSION=""
    fi
}

# 获取GitHub最新版本
get_latest_version() {
    print_step "获取最新版本信息..."
    
    # 尝试多种方式获取最新版本
    LATEST_VERSION=""
    
    # 方法1: 使用GitHub API
    if command -v curl >/dev/null 2>&1; then
        LATEST_VERSION=$(curl -s "https://api.github.com/repos/$GITHUB_REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4 2>/dev/null || echo "")
    fi
    
    # 方法2: 如果curl失败，尝试wget
    if [ -z "$LATEST_VERSION" ] && command -v wget >/dev/null 2>&1; then
        LATEST_VERSION=$(wget -qO- "https://api.github.com/repos/$GITHUB_REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4 2>/dev/null || echo "")
    fi
    
    if [ -z "$LATEST_VERSION" ]; then
        print_error "无法获取最新版本信息，请检查网络连接"
        exit 1
    fi
    
    print_info "最新版本: $LATEST_VERSION"
}

# 版本比较函数
version_compare() {
    # 移除版本号前的v
    local v1=$(echo "$1" | sed 's/^v//')
    local v2=$(echo "$2" | sed 's/^v//')
    
    # 如果版本相同
    if [ "$v1" = "$v2" ]; then
        return 0
    fi
    
    # 使用sort -V进行版本比较
    local newer=$(printf "%s\n%s" "$v1" "$v2" | sort -V | tail -n1)
    if [ "$newer" = "$v1" ]; then
        return 1  # v1 > v2
    else
        return 2  # v1 < v2
    fi
}

# 检查是否需要更新
check_update_needed() {
    get_current_version
    get_latest_version
    
    if [ -z "$CURRENT_VERSION" ]; then
        print_info "执行全新安装..."
        UPDATE_NEEDED=true
        return
    fi
    
    version_compare "$CURRENT_VERSION" "$LATEST_VERSION"
    case $? in
        0)
            print_info "当前版本已是最新版本"
            UPDATE_NEEDED=false
            ;;
        1)
            print_warn "当前版本较新 ($CURRENT_VERSION > $LATEST_VERSION)"
            UPDATE_NEEDED=false
            ;;
        2)
            print_info "发现新版本，准备更新 ($CURRENT_VERSION -> $LATEST_VERSION)"
            UPDATE_NEEDED=true
            ;;
    esac
}

# 安装系统依赖
install_dependencies() {
    print_step "检查并安装系统依赖..."
    
    local deps="wget curl tar"
    local missing_deps=""
    
    # 检查依赖
    for dep in $deps; do
        if ! command -v "$dep" >/dev/null 2>&1; then
            missing_deps="$missing_deps $dep"
        fi
    done
    
    if [ -n "$missing_deps" ]; then
        print_info "安装缺失的依赖:$missing_deps"
        
        case "$OS" in
            ubuntu|debian)
                apt-get update
                apt-get install -y $missing_deps
                ;;
            centos|rhel|fedora|rocky|almalinux)
                if command -v dnf >/dev/null 2>&1; then
                    dnf install -y $missing_deps
                else
                    yum install -y $missing_deps
                fi
                ;;
            arch|manjaro)
                pacman -Sy --noconfirm $missing_deps
                ;;
            opensuse*)
                zypper install -y $missing_deps
                ;;
            *)
                print_warn "未知的操作系统: $OS，请手动安装依赖包: $missing_deps"
                ;;
        esac
    else
        print_info "所有依赖已满足"
    fi
}

# 停止服务（如果运行中）
stop_service_if_running() {
    if systemctl is-active --quiet $SERVICE_NAME 2>/dev/null; then
        print_step "停止当前运行的服务..."
        systemctl stop $SERVICE_NAME
        print_info "服务已停止"
    fi
}

# 下载并安装二进制文件
download_and_install() {
    print_step "下载nspass-agent $LATEST_VERSION..."
    
    # 构建下载URL
    local filename="nspass-agent-linux-$ARCH.tar.gz"
    local download_url="https://github.com/$GITHUB_REPO/releases/download/$LATEST_VERSION/$filename"
    local temp_dir=$(mktemp -d)
    local temp_file="$temp_dir/$filename"
    
    print_info "下载URL: $download_url"
    
    # 下载文件
    if command -v curl >/dev/null 2>&1; then
        if ! curl -L -o "$temp_file" "$download_url"; then
            print_error "下载失败: $download_url"
            rm -rf "$temp_dir"
            exit 1
        fi
    elif command -v wget >/dev/null 2>&1; then
        if ! wget -O "$temp_file" "$download_url"; then
            print_error "下载失败: $download_url"
            rm -rf "$temp_dir"
            exit 1
        fi
    else
        print_error "需要curl或wget来下载文件"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    # 解压文件
    print_info "解压文件..."
    cd "$temp_dir"
    if ! tar -xzf "$filename"; then
        print_error "解压失败"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    # 查找二进制文件
    local binary_file=""
    for file in nspass-agent nspass-agent-linux-$ARCH nspass-agent-$ARCH; do
        if [ -f "$file" ]; then
            binary_file="$file"
            break
        fi
    done
    
    if [ -z "$binary_file" ]; then
        print_error "未找到二进制文件"
        ls -la
        rm -rf "$temp_dir"
        exit 1
    fi
    
    # 安装二进制文件
    print_info "安装二进制文件..."
    cp "$binary_file" "$INSTALL_DIR/nspass-agent"
    chmod +x "$INSTALL_DIR/nspass-agent"
    
    # 验证安装
    if ! "$INSTALL_DIR/nspass-agent" --version >/dev/null 2>&1; then
        print_error "二进制文件验证失败"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    # 清理临时文件
    rm -rf "$temp_dir"
    
    print_info "二进制文件安装完成"
}

# 创建配置目录和文件
setup_config() {
    print_step "设置配置文件..."
    
    # 创建配置目录
    mkdir -p "$CONFIG_DIR"
    mkdir -p "$CONFIG_DIR/proxy"
    mkdir -p "$CONFIG_DIR/iptables-backup"
    
    # 如果配置文件不存在，创建默认配置
    if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
        print_info "创建默认配置文件..."
        cat > "$CONFIG_DIR/config.yaml" << 'EOF'
# NSPass Agent 配置文件

# API配置
api:
  base_url: "https://api.nspass.com"
  token: "your-api-token-here"
  timeout: 30
  retry_count: 3

# 代理软件配置
proxy:
  bin_path: "/usr/local/bin"
  config_path: "/etc/nspass/proxy"
  enabled_types: ["shadowsocks", "trojan", "snell"]
  auto_start: true
  restart_on_fail: true

# iptables配置
iptables:
  enable: true
  backup_path: "/etc/nspass/iptables-backup"
  persistent_method: "iptables-save"
  chain_prefix: "NSPASS"

# 更新间隔（秒）
update_interval: 300

# 日志级别
log_level: "info"
EOF
        print_info "默认配置文件已创建: $CONFIG_DIR/config.yaml"
    else
        print_info "配置文件已存在，保持原有配置"
    fi
    
    # 设置正确的权限
    chown -R root:root "$CONFIG_DIR"
    chmod 755 "$CONFIG_DIR"
    chmod 644 "$CONFIG_DIR/config.yaml"
}

# 安装systemd服务
install_systemd_service() {
    print_step "安装systemd服务..."
    
    cat > "/etc/systemd/system/$SERVICE_NAME.service" << EOF
[Unit]
Description=NSPass Agent - 代理服务管理Agent
Documentation=https://github.com/$GITHUB_REPO
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
ExecStart=$INSTALL_DIR/nspass-agent --config $CONFIG_DIR/config.yaml
ExecReload=/bin/kill -HUP \$MAINPID
KillMode=process
Restart=on-failure
RestartSec=5s
TimeoutStopSec=30s

# 安全设置
NoNewPrivileges=false
PrivateTmp=true
ProtectSystem=false
ProtectHome=true

# 资源限制
LimitNOFILE=65536
LimitNPROC=65536

# 环境变量
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# 日志配置
StandardOutput=journal
StandardError=journal
SyslogIdentifier=nspass-agent

[Install]
WantedBy=multi-user.target
EOF

    # 重新加载systemd
    systemctl daemon-reload
    
    print_info "systemd服务已安装"
}

# 启用并启动服务
enable_and_start_service() {
    print_step "启用并启动服务..."
    
    # 启用服务
    if ! systemctl is-enabled --quiet $SERVICE_NAME 2>/dev/null; then
        systemctl enable $SERVICE_NAME
        print_info "服务已设置为开机自启"
    else
        print_info "服务已启用开机自启"
    fi
    
    # 启动服务
    if ! systemctl is-active --quiet $SERVICE_NAME 2>/dev/null; then
        systemctl start $SERVICE_NAME
        print_info "服务已启动"
    else
        print_info "服务已在运行"
    fi
    
    # 等待服务启动
    sleep 2
}

# 检查服务状态
check_service_status() {
    print_step "检查服务状态..."
    
    # 检查服务是否运行
    if systemctl is-active --quiet $SERVICE_NAME; then
        print_info "✓ 服务运行状态: 正常"
    else
        print_error "✗ 服务运行状态: 异常"
        print_warn "查看服务日志: journalctl -u $SERVICE_NAME -n 20"
        return 1
    fi
    
    # 检查服务是否启用
    if systemctl is-enabled --quiet $SERVICE_NAME; then
        print_info "✓ 开机自启状态: 已启用"
    else
        print_warn "✗ 开机自启状态: 未启用"
    fi
    
    # 显示服务详细状态
    print_info "服务详细状态:"
    systemctl status $SERVICE_NAME --no-pager -l
    
    return 0
}

# 显示安装后信息
show_post_install_info() {
    local installed_version=$("$INSTALL_DIR/nspass-agent" --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
    
    echo ""
    echo "======================================"
    print_info "NSPass Agent 安装完成！"
    echo "======================================"
    echo ""
    echo "📍 安装信息:"
    echo "   版本: $installed_version"
    echo "   二进制文件: $INSTALL_DIR/nspass-agent"
    echo "   配置文件: $CONFIG_DIR/config.yaml"
    echo "   服务名称: $SERVICE_NAME"
    echo ""
    echo "🔧 下一步操作:"
    echo "   1. 编辑配置文件设置API令牌:"
    echo "      nano $CONFIG_DIR/config.yaml"
    echo ""
    echo "💡 常用命令:"
    echo "   查看服务状态: systemctl status $SERVICE_NAME"
    echo "   查看服务日志: journalctl -u $SERVICE_NAME -f"
    echo "   重启服务:     systemctl restart $SERVICE_NAME"
    echo "   停止服务:     systemctl stop $SERVICE_NAME"
    echo ""
    echo "📚 更多信息: https://github.com/$GITHUB_REPO"
    echo ""
}

# 主安装流程
main() {
    echo "======================================"
    echo "NSPass Agent 安装/升级脚本 v$SCRIPT_VERSION"
    echo "======================================"
    echo ""
    
    # 检查运行环境
    check_root
    detect_arch
    detect_os
    
    # 检查是否需要更新
    check_update_needed
    
    if [ "$UPDATE_NEEDED" = false ]; then
        print_info "无需更新，脚本退出"
        exit 0
    fi
    
    # 安装流程
    install_dependencies
    stop_service_if_running
    download_and_install
    setup_config
    install_systemd_service
    enable_and_start_service
    
    # 检查安装结果
    if check_service_status; then
        show_post_install_info
    else
        print_error "安装完成但服务启动异常，请检查配置文件和日志"
        exit 1
    fi
}

# 脚本入口
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi 