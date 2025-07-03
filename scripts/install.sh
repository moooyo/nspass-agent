#!/bin/bash

# NSPass Agent 安装脚本
# 使用方法: curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
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

# 检查是否以root用户运行
check_root() {
    if [ "$EUID" -ne 0 ]; then
        print_error "请以root用户运行此脚本"
        exit 1
    fi
}

# 检测系统架构
detect_arch() {
    case $(uname -m) in
        x86_64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        i386|i686)
            ARCH="386"
            ;;
        *)
            print_error "不支持的架构: $(uname -m)"
            exit 1
            ;;
    esac
}

# 检测操作系统
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        VERSION=$VERSION_ID
    else
        print_error "无法检测操作系统"
        exit 1
    fi
}

# 安装依赖
install_dependencies() {
    print_info "安装依赖包..."
    
    case "$OS" in
        ubuntu|debian)
            apt-get update
            apt-get install -y wget curl unzip tar iptables
            ;;
        centos|rhel|fedora)
            if command -v dnf >/dev/null 2>&1; then
                dnf install -y wget curl unzip tar iptables
            else
                yum install -y wget curl unzip tar iptables
            fi
            ;;
        arch)
            pacman -Sy --noconfirm wget curl unzip tar iptables
            ;;
        *)
            print_warn "未知的操作系统: $OS，请手动安装依赖包"
            ;;
    esac
}

# 下载并安装nspass-agent
install_nspass_agent() {
    print_info "下载nspass-agent..."
    
    # 获取最新版本
    LATEST_VERSION=$(curl -s https://api.github.com/repos/nspass/nspass-agent/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
    if [ -z "$LATEST_VERSION" ]; then
        print_error "无法获取最新版本信息"
        exit 1
    fi
    
    print_info "最新版本: $LATEST_VERSION"
    
    # 下载URL
    DOWNLOAD_URL="https://github.com/nspass/nspass-agent/releases/download/$LATEST_VERSION/nspass-agent-linux-$ARCH.tar.gz"
    TEMP_DIR=$(mktemp -d)
    
    # 下载文件
    if ! wget -O "$TEMP_DIR/nspass-agent.tar.gz" "$DOWNLOAD_URL"; then
        print_error "下载失败: $DOWNLOAD_URL"
        exit 1
    fi
    
    # 解压并安装
    cd "$TEMP_DIR"
    tar -xzf nspass-agent.tar.gz
    
    # 复制到系统目录
    cp nspass-agent-linux-$ARCH /usr/local/bin/nspass-agent
    chmod +x /usr/local/bin/nspass-agent
    
    # 清理临时文件
    rm -rf "$TEMP_DIR"
    
    print_info "nspass-agent安装完成"
}

# 创建配置目录和文件
setup_config() {
    print_info "设置配置文件..."
    
    # 创建配置目录
    mkdir -p /etc/nspass
    mkdir -p /etc/nspass/proxy
    mkdir -p /etc/nspass/iptables-backup
    
    # 创建默认配置文件
    if [ ! -f /etc/nspass/config.yaml ]; then
        cat > /etc/nspass/config.yaml << 'EOF'
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
        print_info "默认配置文件已创建: /etc/nspass/config.yaml"
    else
        print_warn "配置文件已存在，跳过创建"
    fi
}

# 安装systemd服务
install_systemd_service() {
    print_info "安装systemd服务..."
    
    cat > /etc/systemd/system/nspass-agent.service << 'EOF'
[Unit]
Description=NSPass Agent - 代理服务管理Agent
Documentation=https://github.com/nspass/nspass-agent
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/nspass-agent --config /etc/nspass/config.yaml
ExecReload=/bin/kill -HUP $MAINPID
KillMode=process
Restart=on-failure
RestartSec=5s

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
    
    print_info "systemd服务安装完成"
}

# 主安装流程
main() {
    print_info "开始安装NSPass Agent..."
    
    check_root
    detect_arch
    detect_os
    
    print_info "系统信息: $OS $VERSION ($ARCH)"
    
    install_dependencies
    install_nspass_agent
    setup_config
    install_systemd_service
    
    print_info "安装完成！"
    echo ""
    print_info "接下来的步骤："
    echo "1. 编辑配置文件: /etc/nspass/config.yaml"
    echo "2. 设置API令牌和服务器地址"
    echo "3. 启用并启动服务:"
    echo "   systemctl enable nspass-agent"
    echo "   systemctl start nspass-agent"
    echo "4. 查看服务状态:"
    echo "   systemctl status nspass-agent"
    echo "   journalctl -u nspass-agent -f"
    echo ""
    print_info "更多信息请访问: https://github.com/nspass/nspass-agent"
}

# 运行主函数
main "$@" 