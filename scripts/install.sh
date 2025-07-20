#!/bin/bash

# NSPass Agent 安装/升级脚本
# 使用方法: 
#   curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash
#   或
#   curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash -s -- --server-id=your-server-id --token=your-token --base-url=https://api.nspass.com

set -e

# 版本信息
SCRIPT_VERSION="2.1.0"
GITHUB_REPO="nspass/nspass-agent"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/nspass"
LOG_DIR="/var/log/nspass"
SERVICE_NAME="nspass-agent"

# 配置参数
SERVER_ID=""
API_TOKEN=""
API_BASE_URL=""
ENV_PRESET=""

# 预设环境 API 地址
PRESET_URLS=(
    "production:https://api.nspass.com"
    "staging:https://staging-api.nspass.com"  
    "testing:https://test-api.nspass.com"
    "development:https://dev-api.nspass.com"
)

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

# 显示帮助信息
show_help() {
    echo "NSPass Agent 安装脚本 v$SCRIPT_VERSION"
    echo ""
    echo "使用方法:"
    echo "  $0 [选项]"
    echo "  $0 <server-id> <token> <env>                 # 位置参数"
    echo ""
    echo "简化格式示例："
    echo "  $0 server001 your-api-token production       # 位置参数"
    echo "  $0 -sid server001 -token your-token -env production    # 短参数"
    echo ""
    echo "选项:"
    echo "  -sid, --server-id <id>     设置服务器ID"
    echo "  -token, --token <token>    设置API令牌"
    echo "  -endpoint, --base-url <url>  设置API基础URL"
    echo "  -env, --env <environment>  使用预设环境 (production|staging|testing|development)"
    echo "  -h, --help                 显示此帮助信息"
    echo ""
    echo "预设环境："
    echo "  production   - https://api.nspass.com"
    echo "  staging      - https://staging-api.nspass.com"
    echo "  testing      - https://test-api.nspass.com" 
    echo "  development  - https://dev-api.nspass.com"
    echo ""
    echo "示例:"
    echo "  $0 server001 your-token production                           # 位置参数"
    echo "  $0 -sid server001 -token your-token -env production          # 短参数"
    echo "  $0 -sid server001 -token your-token -endpoint https://api.nspass.com  # 自定义端点"
    echo ""
    echo "远程安装:"
    echo "  curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash -s server001 your-token production"
    echo "  curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash -s -- -sid server001 -token your-token -env production"
    echo ""
}

# 解析命令行参数
parse_args() {
    # 简化参数解析：支持位置参数
    if [ $# -eq 3 ] && [[ "$1" != -* ]] && [[ "$2" != -* ]] && [[ "$3" != -* ]]; then
        SERVER_ID="$1"
        API_TOKEN="$2"
        ENV_PRESET="$3"
        return
    fi
    
    # 支持短参数和长参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -sid|--server-id)
                SERVER_ID="$2"
                shift 2
                ;;
            --server-id=*)
                SERVER_ID="${1#*=}"
                shift
                ;;
            -token|--token)
                API_TOKEN="$2"
                shift 2
                ;;
            --token=*)
                API_TOKEN="${1#*=}"
                shift
                ;;
            -endpoint|--base-url)
                API_BASE_URL="$2"
                shift 2
                ;;
            --base-url=*)
                API_BASE_URL="${1#*=}"
                shift
                ;;
            -env|--env)
                ENV_PRESET="$2"
                shift 2
                ;;
            --env=*)
                ENV_PRESET="${1#*=}"
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                print_error "未知参数: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# 解析环境预设
parse_env_preset() {
    if [ -n "$ENV_PRESET" ]; then
        for preset in "${PRESET_URLS[@]}"; do
            env_name="${preset%%:*}"
            env_url="${preset#*:}"
            if [ "$env_name" = "$ENV_PRESET" ]; then
                API_BASE_URL="$env_url"
                print_info "使用预设环境: $ENV_PRESET -> $API_BASE_URL"
                return 0
            fi
        done
        print_error "未知的预设环境: $ENV_PRESET"
        print_error "支持的预设环境: production, staging, testing, development"
        exit 1
    fi
}

# 验证参数
validate_args() {
    if [ -n "$SERVER_ID" ] && [ -n "$API_TOKEN" ] && [ -n "$API_BASE_URL" ]; then
        print_info "使用提供的配置参数:"
        print_info "  服务器ID: $SERVER_ID"
        print_info "  API令牌: ${API_TOKEN:0:10}..."
        print_info "  API基础URL: $API_BASE_URL"
        return 0
    elif [ -n "$SERVER_ID" ] || [ -n "$API_TOKEN" ] || [ -n "$API_BASE_URL" ]; then
        print_error "server-id、token 和 base-url 参数必须同时提供"
        show_help
        exit 1
    else
        print_warn "未提供配置参数，将使用默认配置"
        return 0
    fi
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
        local api_response=$(curl -s "https://api.github.com/repos/$GITHUB_REPO/releases/latest" 2>/dev/null)
        if [ $? -eq 0 ] && echo "$api_response" | grep -q "tag_name"; then
            LATEST_VERSION=$(echo "$api_response" | grep '"tag_name"' | cut -d'"' -f4 2>/dev/null || echo "")
        fi
    fi
    
    # 方法2: 如果curl失败，尝试wget
    if [ -z "$LATEST_VERSION" ] && command -v wget >/dev/null 2>&1; then
        local api_response=$(wget -qO- "https://api.github.com/repos/$GITHUB_REPO/releases/latest" 2>/dev/null)
        if [ $? -eq 0 ] && echo "$api_response" | grep -q "tag_name"; then
            LATEST_VERSION=$(echo "$api_response" | grep '"tag_name"' | cut -d'"' -f4 2>/dev/null || echo "")
        fi
    fi
    
    # 方法3: 如果API失败，使用默认版本（开发阶段）
    if [ -z "$LATEST_VERSION" ]; then
        print_warn "无法从GitHub API获取版本信息，使用默认版本"
        LATEST_VERSION="v1.0.0"
    fi
    
    print_info "目标版本: $LATEST_VERSION"
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
    local filename="nspass-agent-linux-$ARCH"
    local download_url="https://github.com/$GITHUB_REPO/releases/download/$LATEST_VERSION/$filename"
    local temp_dir=$(mktemp -d)
    local temp_file="$temp_dir/$filename"
    
    print_info "下载URL: $download_url"
    
    # 下载文件
    local download_success=false
    if command -v curl >/dev/null 2>&1; then
        if curl -L -o "$temp_file" "$download_url" 2>/dev/null; then
            download_success=true
        fi
    elif command -v wget >/dev/null 2>&1; then
        if wget -O "$temp_file" "$download_url" 2>/dev/null; then
            download_success=true
        fi
    fi
    
    # 如果下载失败，尝试tar.gz格式
    if [ "$download_success" = false ]; then
        print_warn "直接下载失败，尝试tar.gz格式..."
        filename="nspass-agent-linux-$ARCH.tar.gz"
        download_url="https://github.com/$GITHUB_REPO/releases/download/$LATEST_VERSION/$filename"
        temp_file="$temp_dir/$filename"
        
        if command -v curl >/dev/null 2>&1; then
            if curl -L -o "$temp_file" "$download_url" 2>/dev/null; then
                download_success=true
            fi
        elif command -v wget >/dev/null 2>&1; then
            if wget -O "$temp_file" "$download_url" 2>/dev/null; then
                download_success=true
            fi
        fi
        
        if [ "$download_success" = true ]; then
            print_info "正在解压文件..."
            cd "$temp_dir"
            if tar -xzf "$filename" 2>/dev/null; then
                # 查找二进制文件
                local binary_file=""
                for file in nspass-agent nspass-agent-linux-$ARCH nspass-agent-$ARCH; do
                    if [ -f "$file" ]; then
                        binary_file="$file"
                        break
                    fi
                done
                
                if [ -n "$binary_file" ]; then
                    temp_file="$temp_dir/$binary_file"
                else
                    print_error "未找到二进制文件"
                    rm -rf "$temp_dir"
                    exit 1
                fi
            else
                print_error "解压失败"
                rm -rf "$temp_dir"
                exit 1
            fi
        fi
    fi
    
    if [ "$download_success" = false ]; then
        print_error "下载失败，请检查网络连接或版本是否存在"
        print_error "尝试的URL: $download_url"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    # 检查下载的文件
    if [ ! -f "$temp_file" ] || [ ! -s "$temp_file" ]; then
        print_error "下载的文件不存在或为空"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    # 安装二进制文件
    print_info "安装二进制文件..."
    cp "$temp_file" "$INSTALL_DIR/nspass-agent"
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
    mkdir -p "$LOG_DIR"
    
    # 确定配置参数
    local config_server_id="your-server-id-here"
    local config_api_token="your-api-token-here"
    local config_base_url="https://api.nspass.com"
    local config_created=false
    
    if [ -n "$SERVER_ID" ] && [ -n "$API_TOKEN" ] && [ -n "$API_BASE_URL" ]; then
        config_server_id="$SERVER_ID"
        config_api_token="$API_TOKEN"
        config_base_url="$API_BASE_URL"
        print_info "使用提供的配置参数"
    fi
    
    # 如果配置文件不存在，创建默认配置
    if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
        print_info "创建配置文件..."
        cat > "$CONFIG_DIR/config.yaml" << EOF
# NSPass Agent 配置文件
# 请根据实际需要修改以下配置

# 服务器ID（必须设置）
server_id: "$config_server_id"

# API配置
api:
  base_url: "$config_base_url"
  token: "$config_api_token"
  timeout: 30
  retry_count: 3
  retry_delay: 5
  tls: true
  tls_skip_verify: false

# 代理软件配置
proxy:
  bin_path: "/usr/local/bin"
  config_path: "/etc/nspass/proxy"
  enabled_types: ["shadowsocks", "trojan", "snell"]
  auto_start: true
  restart_on_fail: true

  # 进程监控配置
  monitor:
    enable: true
    check_interval: 30
    restart_cooldown: 60
    max_restarts: 10
    health_timeout: 5

# iptables配置
iptables:
  enable: true
  backup_path: "/etc/nspass/iptables-backup"
  persistent_method: "iptables-save"
  chain_prefix: "NSPASS"

# 日志配置
logger:
  level: "info"
  format: "json"
  output: "both"
  file: "/var/log/nspass/agent.log"
  max_size: 100
  max_backups: 5
  max_age: 30
  compress: true

# 更新间隔（秒）
update_interval: 300
EOF
        config_created=true
        print_info "配置文件已创建: $CONFIG_DIR/config.yaml"
        
        if [ -n "$SERVER_ID" ] && [ -n "$API_TOKEN" ] && [ -n "$API_BASE_URL" ]; then
            print_info "✓ 已设置服务器ID: $SERVER_ID"
            print_info "✓ 已设置API令牌: ${API_TOKEN:0:10}..."
            print_info "✓ 已设置API基础URL: $API_BASE_URL"
        else
            print_warn "⚠️  请编辑配置文件设置正确的 server_id、api.token 和 api.base_url"
        fi
    else
        print_info "配置文件已存在，保持原有配置"
        
        # 如果提供了参数，询问是否更新现有配置
        if [ -n "$SERVER_ID" ] && [ -n "$API_TOKEN" ]; then
            print_warn "检测到现有配置文件，但提供了新的配置参数"
            echo ""
            while true; do
                read -p "是否更新现有配置文件中的 server_id 和 token？ [y/N]: " -n 1 -r
                echo ""
                case $REPLY in
                    [Yy])
                        update_existing_config
                        break
                        ;;
                    [Nn]|"")
                        print_info "保持现有配置不变"
                        break
                        ;;
                    *)
                        echo "请输入 y 或 n"
                        ;;
                esac
            done
        fi
    fi
    
    # 设置正确的权限
    chown -R root:root "$CONFIG_DIR"
    chown -R root:root "$LOG_DIR"
    chmod 755 "$CONFIG_DIR"
    chmod 755 "$LOG_DIR"
    chmod 644 "$CONFIG_DIR/config.yaml"
    chmod 750 "$CONFIG_DIR/proxy"
    chmod 750 "$CONFIG_DIR/iptables-backup"
}

# 更新现有配置文件
update_existing_config() {
    print_step "更新现有配置文件..."
    
    local config_file="$CONFIG_DIR/config.yaml"
    local backup_file="$CONFIG_DIR/config.yaml.backup.$(date +%Y%m%d_%H%M%S)"
    
    # 备份现有配置
    cp "$config_file" "$backup_file"
    print_info "已备份现有配置: $backup_file"
    
    # 使用 sed 更新配置
    if command -v sed >/dev/null 2>&1; then
        # 更新 server_id
        sed -i "s/^server_id: .*/server_id: \"$SERVER_ID\"/" "$config_file"
        
        # 更新 api.token
        sed -i "/^api:/,/^[^ ]/ s/^  token: .*/  token: \"$API_TOKEN\"/" "$config_file"
        
        print_info "✓ 已更新服务器ID: $SERVER_ID"
        print_info "✓ 已更新API令牌: ${API_TOKEN:0:10}..."
    else
        print_error "sed 命令不可用，无法自动更新配置"
        print_warn "请手动编辑配置文件: $config_file"
    fi
    chmod 644 "$CONFIG_DIR/config.yaml"
    chmod 750 "$CONFIG_DIR/proxy"
    chmod 750 "$CONFIG_DIR/iptables-backup"
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
    echo "   日志目录: $LOG_DIR"
    echo "   服务名称: $SERVICE_NAME"
    echo ""
    
    # 根据是否提供了配置参数显示不同的信息
    if [ -n "$SERVER_ID" ] && [ -n "$API_TOKEN" ]; then
        echo "✅ 配置状态:"
        echo "   服务器ID: $SERVER_ID"
        echo "   API令牌: ${API_TOKEN:0:10}..."
        echo "   配置已完成，服务可以正常运行"
        echo ""
        echo "🔧 下一步操作:"
        echo "   服务已启动，可以开始使用"
        echo ""
    else
        echo "⚠️  配置状态:"
        echo "   需要手动配置服务器ID和API令牌"
        echo ""
        echo "🔧 下一步操作:"
        echo "   1. 编辑配置文件设置API令牌和服务器ID:"
        echo "      nano $CONFIG_DIR/config.yaml"
        echo "   2. 设置完成后重启服务:"
        echo "      systemctl restart $SERVICE_NAME"
        echo ""
    fi
    
    echo "💡 常用命令:"
    echo "   查看服务状态: systemctl status $SERVICE_NAME"
    echo "   查看实时日志: journalctl -u $SERVICE_NAME -f"
    echo "   查看日志文件: tail -f $LOG_DIR/agent.log"
    echo "   重启服务:     systemctl restart $SERVICE_NAME"
    echo "   停止服务:     systemctl stop $SERVICE_NAME"
    echo "   查看配置:     $INSTALL_DIR/nspass-agent --config $CONFIG_DIR/config.yaml --help"
    echo ""
    echo "📋 配置检查:"
    echo "   配置文件语法检查: $INSTALL_DIR/nspass-agent --config $CONFIG_DIR/config.yaml --check"
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
    
    # 解析命令行参数
    parse_args "$@"
    
    # 解析环境预设
    parse_env_preset
    
    # 验证参数
    validate_args
    
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