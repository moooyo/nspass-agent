#!/bin/bash

# NSPass Agent 测试安装脚本
# 使用本地构建的二进制文件进行测试安装
# 使用方法: sudo ./scripts/test-install.sh [--server-id=<id>] [--token=<token>]

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

# 项目根目录
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="$PROJECT_ROOT/dist"

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
    echo "NSPass Agent 测试安装脚本 v$SCRIPT_VERSION"
    echo ""
    echo "使用方法:"
    echo "  $0 [选项]"
    echo ""
    echo "选项:"
    echo "  --server-id=<id>     设置服务器ID"
    echo "  --token=<token>      设置API令牌"
    echo "  --help               显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 --server-id=test-server-001 --token=test-token"
    echo ""
}

# 解析命令行参数
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --server-id=*)
                SERVER_ID="${1#*=}"
                shift
                ;;
            --token=*)
                API_TOKEN="${1#*=}"
                shift
                ;;
            --help)
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

# 验证参数
validate_args() {
    if [ -n "$SERVER_ID" ] && [ -n "$API_TOKEN" ]; then
        print_info "使用提供的测试配置参数:"
        print_info "  服务器ID: $SERVER_ID"
        print_info "  API令牌: ${API_TOKEN:0:10}..."
        return 0
    elif [ -n "$SERVER_ID" ] || [ -n "$API_TOKEN" ]; then
        print_error "server-id 和 token 参数必须同时提供"
        show_help
        exit 1
    else
        print_warn "未提供配置参数，将使用默认测试配置"
        # 设置默认测试参数
        SERVER_ID="test-server-001"
        API_TOKEN="test-token-please-replace"
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
            exit 1
            ;;
    esac
    print_info "检测到系统架构: $arch (使用: $ARCH)"
}

# 检查本地构建文件
check_local_build() {
    print_step "检查本地构建文件..."
    
    # 检查是否有构建文件
    local binary_candidates=(
        "$DIST_DIR/nspass-agent"
        "$DIST_DIR/nspass-agent-linux-$ARCH"
        "$PROJECT_ROOT/nspass-agent"
    )
    
    for binary in "${binary_candidates[@]}"; do
        if [ -f "$binary" ]; then
            LOCAL_BINARY="$binary"
            print_info "找到本地二进制文件: $LOCAL_BINARY"
            return 0
        fi
    done
    
    print_error "未找到本地构建的二进制文件"
    print_error "请先运行: make build"
    exit 1
}

# 构建项目（如果需要）
build_project() {
    print_step "构建项目..."
    
    if [ ! -f "$PROJECT_ROOT/Makefile" ]; then
        print_error "未找到Makefile"
        exit 1
    fi
    
    cd "$PROJECT_ROOT"
    
    # 清理并构建
    print_info "清理旧构建..."
    make clean 2>/dev/null || true
    
    print_info "构建项目..."
    if ! make build; then
        print_error "构建失败"
        exit 1
    fi
    
    print_info "构建完成"
}

# 安装本地构建的二进制文件
install_local_binary() {
    print_step "安装本地构建的二进制文件..."
    
    # 停止服务（如果运行中）
    if systemctl is-active --quiet $SERVICE_NAME 2>/dev/null; then
        print_info "停止当前运行的服务..."
        systemctl stop $SERVICE_NAME
    fi
    
    # 复制二进制文件
    print_info "复制二进制文件..."
    cp "$LOCAL_BINARY" "$INSTALL_DIR/nspass-agent"
    chmod +x "$INSTALL_DIR/nspass-agent"
    
    # 验证安装
    if ! "$INSTALL_DIR/nspass-agent" --version >/dev/null 2>&1; then
        print_error "二进制文件验证失败"
        exit 1
    fi
    
    local version=$("$INSTALL_DIR/nspass-agent" --version 2>/dev/null | head -1 || echo "unknown")
    print_info "二进制文件安装完成，版本: $version"
}

# 创建配置目录和文件
setup_config() {
    print_step "设置配置文件..."
    
    # 创建配置目录
    mkdir -p "$CONFIG_DIR"
    mkdir -p "$CONFIG_DIR/proxy"
    mkdir -p "$CONFIG_DIR/iptables-backup"
    mkdir -p "$LOG_DIR"
    
    # 如果配置文件不存在，创建测试配置
    if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
        print_info "创建测试配置文件..."
        
        # 使用项目中的示例配置作为模板
        if [ -f "$PROJECT_ROOT/configs/config.yaml" ]; then
            cp "$PROJECT_ROOT/configs/config.yaml" "$CONFIG_DIR/config.yaml"
            
            # 如果提供了参数，更新配置文件
            if [ -n "$SERVER_ID" ] && [ -n "$API_TOKEN" ]; then
                # 使用 sed 更新配置
                if command -v sed >/dev/null 2>&1; then
                    sed -i "s/^server_id: .*/server_id: \"$SERVER_ID\"/" "$CONFIG_DIR/config.yaml"
                    sed -i "/^api:/,/^[^ ]/ s/^  token: .*/  token: \"$API_TOKEN\"/" "$CONFIG_DIR/config.yaml"
                    print_info "已应用配置参数到示例配置"
                fi
            fi
        else
            # 创建基础测试配置
            cat > "$CONFIG_DIR/config.yaml" << EOF
# NSPass Agent 测试配置文件

# 服务器ID（测试用）
server_id: "$SERVER_ID"

# API配置
api:
  base_url: "https://api.nspass.com"
  token: "$API_TOKEN"
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
  auto_start: false
  restart_on_fail: false

  # 进程监控配置
  monitor:
    enable: false
    check_interval: 30
    restart_cooldown: 60
    max_restarts: 10
    health_timeout: 5

# iptables配置
iptables:
  enable: false
  backup_path: "/etc/nspass/iptables-backup"
  persistent_method: "iptables-save"
  chain_prefix: "NSPASS"

# 日志配置
logger:
  level: "debug"
  format: "text"
  output: "both"
  file: "/var/log/nspass/agent.log"
  max_size: 100
  max_backups: 5
  max_age: 30
  compress: true

# 更新间隔（秒）
update_interval: 60
EOF
        fi
        
        print_info "测试配置文件已创建: $CONFIG_DIR/config.yaml"
        print_info "✓ 服务器ID: $SERVER_ID"
        print_info "✓ API令牌: ${API_TOKEN:0:10}..."
        print_warn "⚠️  这是测试配置，仅用于开发和测试"
    else
        print_info "配置文件已存在，保持原有配置"
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
  max_backups: 5
  max_age: 30
  compress: true

# 更新间隔（秒）
update_interval: 60
EOF
        fi
        
        print_info "测试配置文件已创建: $CONFIG_DIR/config.yaml"
        print_warn "⚠️  这是测试配置，请根据需要修改"
    else
        print_info "配置文件已存在，保持原有配置"
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

# 安装systemd服务
install_systemd_service() {
    print_step "安装systemd服务..."
    
    # 使用项目中的服务文件或创建默认的
    if [ -f "$PROJECT_ROOT/systemd/nspass-agent.service" ]; then
        print_info "使用项目中的systemd服务文件"
        cp "$PROJECT_ROOT/systemd/nspass-agent.service" "/etc/systemd/system/$SERVICE_NAME.service"
    else
        print_info "创建默认systemd服务文件"
        cat > "/etc/systemd/system/$SERVICE_NAME.service" << EOF
[Unit]
Description=NSPass Agent - 代理服务管理Agent (测试版)
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
    fi
    
    # 重新加载systemd
    systemctl daemon-reload
    
    print_info "systemd服务已安装"
}

# 启用并启动服务
enable_and_start_service() {
    print_step "启用并启动服务..."
    
    # 启用服务
    systemctl enable $SERVICE_NAME
    print_info "服务已设置为开机自启"
    
    # 启动服务
    systemctl start $SERVICE_NAME
    print_info "服务已启动"
    
    # 等待服务启动
    sleep 3
}

# 检查服务状态
check_service_status() {
    print_step "检查服务状态..."
    
    # 检查服务是否运行
    if systemctl is-active --quiet $SERVICE_NAME; then
        print_info "✓ 服务运行状态: 正常"
        
        # 显示服务详细状态
        print_info "服务详细状态:"
        systemctl status $SERVICE_NAME --no-pager -l
        
        return 0
    else
        print_error "✗ 服务运行状态: 异常"
        print_warn "查看服务日志:"
        echo "  systemctl status $SERVICE_NAME"
        echo "  journalctl -u $SERVICE_NAME -n 20"
        echo "  tail -f $LOG_DIR/agent.log"
        return 1
    fi
}

# 显示测试完成信息
show_test_complete_info() {
    local installed_version=$("$INSTALL_DIR/nspass-agent" --version 2>/dev/null | head -1 || echo "unknown")
    
    echo ""
    echo "======================================"
    print_info "NSPass Agent 测试安装完成！"
    echo "======================================"
    echo ""
    echo "📍 安装信息:"
    echo "   版本: $installed_version"
    echo "   二进制文件: $INSTALL_DIR/nspass-agent"
    echo "   配置文件: $CONFIG_DIR/config.yaml"
    echo "   日志目录: $LOG_DIR"
    echo "   服务名称: $SERVICE_NAME"
    echo ""
    echo "🧪 测试命令:"
    echo "   查看服务状态: systemctl status $SERVICE_NAME"
    echo "   查看实时日志: journalctl -u $SERVICE_NAME -f"
    echo "   查看日志文件: tail -f $LOG_DIR/agent.log"
    echo "   测试配置:     $INSTALL_DIR/nspass-agent --config $CONFIG_DIR/config.yaml --help"
    echo ""
    echo "🛠️  调试命令:"
    echo "   重启服务:     systemctl restart $SERVICE_NAME"
    echo "   停止服务:     systemctl stop $SERVICE_NAME"
    echo "   禁用服务:     systemctl disable $SERVICE_NAME"
    echo ""
    echo "🗑️  清理测试:"
    echo "   卸载测试版本: $PROJECT_ROOT/scripts/uninstall.sh"
    echo ""
    echo "📚 项目信息: https://github.com/$GITHUB_REPO"
    echo ""
}

# 主安装流程
main() {
    echo "======================================"
    echo "NSPass Agent 测试安装脚本 v$SCRIPT_VERSION"
    echo "======================================"
    echo ""
    
    # 解析命令行参数
    parse_args "$@"
    
    # 验证参数
    validate_args
    
    # 检查运行环境
    check_root
    detect_arch
    
    # 检查本地构建
    if ! check_local_build; then
        print_warn "未找到本地构建，尝试构建项目..."
        build_project
        check_local_build
    fi
    
    # 安装流程
    install_local_binary
    setup_config
    install_systemd_service
    enable_and_start_service
    
    # 检查安装结果
    if check_service_status; then
        show_test_complete_info
    else
        print_error "测试安装完成但服务启动异常"
        print_error "请检查配置文件和日志"
        exit 1
    fi
}

# 脚本入口
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi
