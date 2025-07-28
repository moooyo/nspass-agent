#!/bin/bash

# NSPass Agent 安装/升级脚本
# 使用方法: 
#   curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh | bash
#   或
#   curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh | bash -s -- --server-id=your-server-id --token=your-token --base-url=https://api.nspass.com

# 启用调试模式和错误退出
set -e
set -o pipefail

# 处理管道中的错误
trap 'echo "[ERROR] 脚本在第 $LINENO 行出错，退出码: $?" >&2; exit 1' ERR

# 调试模式开关 (设置为 1 启用详细输出)
DEBUG_MODE=${DEBUG_MODE:-1}

# 调试函数
debug_log() {
    if [ "$DEBUG_MODE" = "1" ]; then
        echo -e "${BLUE}[DEBUG]${NC} $1" >&2
    fi
}

# 执行命令并记录
exec_with_log() {
    local cmd="$1"
    local desc="$2"
    
    debug_log "执行命令: $cmd"
    if [ -n "$desc" ]; then
        debug_log "操作描述: $desc"
    fi
    
    if eval "$cmd"; then
        debug_log "命令执行成功"
        return 0
    else
        local exit_code=$?
        print_error "命令执行失败 (退出码: $exit_code): $cmd"
        return $exit_code
    fi
}

# 管道安装检测
detect_pipe_install() {
    if [ -t 0 ]; then
        debug_log "检测到交互式安装（非管道）"
        PIPE_INSTALL=false
    else
        debug_log "检测到管道安装"
        PIPE_INSTALL=true
        # 管道安装时，确保错误能被看到
        exec 2>&1
    fi
}

# 版本信息
SCRIPT_VERSION="2.1.0"
GITHUB_REPO="moooyo/nspass-agent"
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
    echo "  curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh | bash -s server001 your-token production"
    echo "  curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh | bash -s -- -sid server001 -token your-token -env production"
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
    debug_log "开始从 GitHub API 获取版本信息"
    
    # 尝试多种方式获取最新版本
    LATEST_VERSION=""
    local api_url="https://api.github.com/repos/$GITHUB_REPO/releases/latest"
    debug_log "API URL: $api_url"
    
    # 方法1: 使用GitHub API (curl)
    if command -v curl >/dev/null 2>&1; then
        debug_log "尝试使用 curl 获取版本信息..."
        
        # 测试网络连接
        if ! curl -s --connect-timeout 10 --max-time 30 -I "https://api.github.com" >/dev/null 2>&1; then
            print_warn "无法连接到 GitHub API，网络可能有问题"
        else
            debug_log "GitHub API 连接正常"
        fi
        
        local api_response
        api_response=$(curl -s --connect-timeout 10 --max-time 30 "$api_url" 2>&1)
        local curl_exit_code=$?
        
        debug_log "curl 退出码: $curl_exit_code"
        debug_log "API 响应长度: ${#api_response} 字符"
        
        if [ $curl_exit_code -eq 0 ] && [ -n "$api_response" ]; then
            # 检查响应是否包含错误
            if echo "$api_response" | grep -q '"message".*"rate limit\|"message".*"API rate limit'; then
                print_warn "GitHub API 限流，响应: $(echo "$api_response" | head -c 200)..."
            elif echo "$api_response" | grep -q '"message"'; then
                local error_msg=$(echo "$api_response" | grep -o '"message":"[^"]*"' | head -1)
                print_warn "GitHub API 错误: $error_msg"
            elif echo "$api_response" | grep -q "tag_name"; then
                LATEST_VERSION=$(echo "$api_response" | grep '"tag_name"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")
                debug_log "从 API 响应解析版本: $LATEST_VERSION"
            else
                debug_log "API 响应格式异常: $(echo "$api_response" | head -c 200)..."
            fi
        else
            print_warn "curl 请求失败，退出码: $curl_exit_code"
            if [ -n "$api_response" ]; then
                debug_log "错误响应: $(echo "$api_response" | head -c 200)..."
            fi
        fi
    else
        debug_log "curl 命令不可用"
    fi
    
    # 方法2: 如果curl失败，尝试wget
    if [ -z "$LATEST_VERSION" ] && command -v wget >/dev/null 2>&1; then
        debug_log "尝试使用 wget 获取版本信息..."
        
        local api_response
        api_response=$(wget --timeout=30 --tries=2 -qO- "$api_url" 2>&1)
        local wget_exit_code=$?
        
        debug_log "wget 退出码: $wget_exit_code"
        debug_log "API 响应长度: ${#api_response} 字符"
        
        if [ $wget_exit_code -eq 0 ] && echo "$api_response" | grep -q "tag_name"; then
            LATEST_VERSION=$(echo "$api_response" | grep '"tag_name"' | head -1 | cut -d'"' -f4 2>/dev/null || echo "")
            debug_log "从 wget 响应解析版本: $LATEST_VERSION"
        else
            print_warn "wget 请求失败，退出码: $wget_exit_code"
            if [ -n "$api_response" ]; then
                debug_log "错误响应: $(echo "$api_response" | head -c 200)..."
            fi
        fi
    else
        debug_log "wget 命令不可用或已获取到版本"
    fi
    
    # 方法3: 如果API失败，使用默认版本（开发阶段）
    if [ -z "$LATEST_VERSION" ]; then
        print_warn "无法从GitHub API获取版本信息，可能原因："
        print_warn "  1. 网络连接问题"
        print_warn "  2. GitHub API 限流"
        print_warn "  3. 仓库不存在或私有"
        print_warn "  4. curl/wget 配置问题"
        print_warn "使用默认版本 v1.0.0"
        LATEST_VERSION="v1.0.0"
    fi
    
    print_info "目标版本: $LATEST_VERSION"
    debug_log "版本获取完成"
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
    
    local deps="wget curl tar gzip unzip"
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
    debug_log "开始下载和安装流程"
    
    # 构建下载URL - 优先尝试tar.gz格式
    local filename="nspass-agent-linux-$ARCH.tar.gz"
    local download_url="https://github.com/$GITHUB_REPO/releases/download/$LATEST_VERSION/$filename"
    local temp_dir=$(mktemp -d)
    local temp_file="$temp_dir/$filename"
    
    print_info "下载URL: $download_url"
    print_info "临时目录: $temp_dir"
    debug_log "目标文件: $temp_file"
    
    # 检查临时目录是否创建成功
    if [ ! -d "$temp_dir" ]; then
        print_error "无法创建临时目录"
        exit 1
    fi
    debug_log "临时目录创建成功"
    
    # 测试网络连接
    print_info "测试网络连接..."
    if ! ping -c 1 github.com >/dev/null 2>&1; then
        print_warn "无法 ping 通 github.com，但继续尝试下载"
    else
        debug_log "网络连接正常"
    fi
    
    # 下载文件
    local download_success=false
    local download_method=""
    
    # 尝试使用 curl 下载 tar.gz
    if command -v curl >/dev/null 2>&1; then
        print_info "使用 curl 下载 tar.gz..."
        debug_log "curl 版本: $(curl --version | head -1)"
        
        if curl -L --connect-timeout 30 --max-time 300 -o "$temp_file" "$download_url" 2>&1; then
            download_success=true
            download_method="curl (tar.gz)"
            debug_log "curl tar.gz 下载成功"
        else
            local curl_exit_code=$?
            print_warn "curl tar.gz 下载失败，退出码: $curl_exit_code"
        fi
    else
        debug_log "curl 不可用"
    fi
    
    # 如果 curl 失败，尝试 wget
    if [ "$download_success" = false ] && command -v wget >/dev/null 2>&1; then
        print_info "使用 wget 下载 tar.gz..."
        debug_log "wget 版本: $(wget --version | head -1)"
        
        if wget --timeout=300 --tries=3 -O "$temp_file" "$download_url" 2>&1; then
            download_success=true
            download_method="wget (tar.gz)"
            debug_log "wget tar.gz 下载成功"
        else
            local wget_exit_code=$?
            print_warn "wget tar.gz 下载失败，退出码: $wget_exit_code"
        fi
    else
        debug_log "wget 不可用或已成功下载"
    fi
    
    # 如果 tar.gz 下载失败，尝试直接下载二进制文件（作为备选）
    if [ "$download_success" = false ]; then
        print_warn "tar.gz 下载失败，尝试直接下载二进制文件..."
        filename="nspass-agent-linux-$ARCH"
        download_url="https://github.com/$GITHUB_REPO/releases/download/$LATEST_VERSION/$filename"
        temp_file="$temp_dir/$filename"
        
        print_info "备选下载URL: $download_url"
        
        # 重新尝试 curl
        if command -v curl >/dev/null 2>&1; then
            print_info "使用 curl 下载二进制文件..."
            if curl -L --connect-timeout 30 --max-time 300 -o "$temp_file" "$download_url" 2>&1; then
                download_success=true
                download_method="curl (binary)"
                debug_log "curl 二进制文件下载成功"
            else
                local curl_exit_code=$?
                print_warn "curl 二进制文件下载失败，退出码: $curl_exit_code"
            fi
        fi
        
        # 重新尝试 wget
        if [ "$download_success" = false ] && command -v wget >/dev/null 2>&1; then
            print_info "使用 wget 下载二进制文件..."
            if wget --timeout=300 --tries=3 -O "$temp_file" "$download_url" 2>&1; then
                download_success=true
                download_method="wget (binary)"
                debug_log "wget 二进制文件下载成功"
            else
                local wget_exit_code=$?
                print_warn "wget 二进制文件下载失败，退出码: $wget_exit_code"
            fi
        fi
    fi
    
    # 最终检查下载是否成功
    if [ "$download_success" = false ]; then
        print_error "下载失败，尝试的方法都无效"
        print_error "可能的原因："
        print_error "  1. 网络连接问题"
        print_error "  2. GitHub releases 中不存在该版本文件"
        print_error "  3. 防火墙或代理阻止了下载"
        print_error "  4. 系统架构 ($ARCH) 不支持"
        print_error ""
        print_error "请手动检查以下URL是否可访问："
        print_error "  https://github.com/$GITHUB_REPO/releases/download/$LATEST_VERSION/nspass-agent-linux-$ARCH.tar.gz"
        print_error "  https://github.com/$GITHUB_REPO/releases/download/$LATEST_VERSION/nspass-agent-linux-$ARCH"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    print_info "下载成功，使用方法: $download_method"
    
    # 检查下载的文件
    debug_log "检查下载的文件..."
    if [ ! -f "$temp_file" ]; then
        print_error "下载的文件不存在: $temp_file"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    local file_size=$(stat -c%s "$temp_file" 2>/dev/null || wc -c < "$temp_file")
    debug_log "文件大小: $file_size 字节"
    
    if [ "$file_size" -eq 0 ]; then
        print_error "下载的文件为空"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    if [ "$file_size" -lt 1000 ]; then  # 小于 1KB 可能是错误页面
        print_error "下载的文件大小异常小 ($file_size 字节)"
        print_error "文件内容:"
        cat "$temp_file"
        print_error ""
        print_error "这通常表示下载的是404错误页面而不是实际文件"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    # 检查文件类型并处理
    if echo "$filename" | grep -q "\.tar\.gz$"; then
        # 如果是 tar.gz 格式，需要解压
        print_info "正在解压文件..."
        debug_log "解压文件: $temp_file"
        
        cd "$temp_dir"
        if tar -xzf "$filename" 2>&1; then
            debug_log "解压成功"
            
            # 查找二进制文件
            local binary_file=""
            local possible_names=("nspass-agent" "nspass-agent-linux-$ARCH" "nspass-agent-$ARCH" "nspass-agent-linux" "agent")
            
            debug_log "查找二进制文件..."
            for file in "${possible_names[@]}"; do
                debug_log "检查文件: $file"
                if [ -f "$file" ]; then
                    binary_file="$file"
                    debug_log "找到二进制文件: $binary_file"
                    break
                fi
            done
            
            # 如果没找到预期的文件名，列出所有文件
            if [ -z "$binary_file" ]; then
                print_info "未找到预期的二进制文件，临时目录内容："
                ls -la "$temp_dir"
                
                # 尝试找到可执行文件或最大的文件
                binary_file=$(find "$temp_dir" -type f -executable | head -1)
                if [ -z "$binary_file" ]; then
                    # 找最大的文件，可能是二进制文件
                    binary_file=$(find "$temp_dir" -type f -exec ls -la {} + | grep -v "\.tar\.gz$" | sort -k5 -nr | head -1 | awk '{print $NF}')
                fi
                
                if [ -n "$binary_file" ]; then
                    binary_file=$(basename "$binary_file")
                    print_info "找到文件: $binary_file"
                fi
            fi
            
            if [ -n "$binary_file" ]; then
                temp_file="$temp_dir/$binary_file"
                debug_log "最终二进制文件路径: $temp_file"
            else
                print_error "未找到二进制文件"
                print_error "临时目录内容:"
                ls -la "$temp_dir"
                rm -rf "$temp_dir"
                exit 1
            fi
        else
            print_error "解压失败"
            rm -rf "$temp_dir"
            exit 1
        fi
    fi
    
    # 检查文件类型
    if command -v file >/dev/null 2>&1; then
        local file_type=$(file "$temp_file")
        debug_log "文件类型: $file_type"
        
        if ! echo "$file_type" | grep -q "executable\|ELF"; then
            print_warn "文件似乎不是可执行文件: $file_type"
        fi
    fi
    
    # 安装二进制文件
    print_info "安装二进制文件..."
    debug_log "复制文件: $temp_file -> $INSTALL_DIR/nspass-agent"
    
    if ! cp "$temp_file" "$INSTALL_DIR/nspass-agent"; then
        print_error "复制文件失败"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    if ! chmod +x "$INSTALL_DIR/nspass-agent"; then
        print_error "设置执行权限失败"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    debug_log "文件权限设置完成"
    
    # 验证安装
    print_info "验证安装..."
    if "$INSTALL_DIR/nspass-agent" --version >/dev/null 2>&1; then
        local installed_version=$("$INSTALL_DIR/nspass-agent" --version 2>/dev/null | head -1)
        print_info "验证成功，安装版本: $installed_version"
    else
        print_error "二进制文件验证失败"
        print_error "可能的原因："
        print_error "  1. 下载的文件损坏"
        print_error "  2. 系统架构不匹配"
        print_error "  3. 缺少运行时依赖"
        
        # 尝试获取更多信息
        if command -v ldd >/dev/null 2>&1; then
            print_error "依赖库检查:"
            ldd "$INSTALL_DIR/nspass-agent" 2>&1 || true
        fi
        
        rm -rf "$temp_dir"
        exit 1
    fi
    
    # 清理临时文件
    rm -rf "$temp_dir"
    debug_log "临时文件清理完成"
    
    print_info "二进制文件安装完成"
}

# 安装代理程序
install_proxy_binaries() {
    print_step "安装代理程序..."
    
    # 创建代理程序安装目录
    local proxy_bin_dir="/usr/local/bin/proxy"
    mkdir -p "$proxy_bin_dir"
    
    # 检测系统架构
    local os_arch
    case "$(uname -m)" in
        x86_64|amd64) os_arch="amd64" ;;
        arm64|aarch64) os_arch="arm64" ;;
        armv7l) os_arch="armv7" ;;
        i386|i686) os_arch="386" ;;
        *) 
            print_warn "不支持的架构: $(uname -m)，跳过代理程序安装"
            return 0
            ;;
    esac
    
    # 安装 go-shadowsocks2
    install_go_shadowsocks2 "$proxy_bin_dir" "$os_arch"
    
    # 安装 snell-server
    install_snell_server "$proxy_bin_dir" "$os_arch"
    
    # 安装 trojan-go
    install_trojan_go "$proxy_bin_dir" "$os_arch"
    
    # 设置目录权限
    chmod 755 "$proxy_bin_dir"
    chown -R root:root "$proxy_bin_dir"
    
    print_info "代理程序安装完成"
}

# 安装 go-shadowsocks2
install_go_shadowsocks2() {
    local install_dir="$1"
    local arch="$2"
    
    print_info "安装 go-shadowsocks2..."
    
    local download_url
    case "$(uname -s)" in
        Linux)
            download_url="https://github.com/shadowsocks/go-shadowsocks2/releases/download/v0.1.5/shadowsocks2-linux.gz"
            ;;
        Darwin)
            if [ "$arch" = "arm64" ]; then
                download_url="https://github.com/shadowsocks/go-shadowsocks2/releases/download/v0.1.5/shadowsocks2-macos-arm64.gz"
            else
                download_url="https://github.com/shadowsocks/go-shadowsocks2/releases/download/v0.1.5/shadowsocks2-macos-amd64.gz"
            fi
            ;;
        *)
            print_warn "不支持的操作系统: $(uname -s)，跳过 go-shadowsocks2 安装"
            return 0
            ;;
    esac
    
    local temp_file="/tmp/shadowsocks2.gz"
    local target_file="$install_dir/go-shadowsocks2"
    
    # 检查是否已存在
    if [ -f "$target_file" ]; then
        print_info "go-shadowsocks2 已存在，跳过安装"
        return 0
    fi
    
    # 下载文件
    if command -v curl >/dev/null 2>&1; then
        curl -k -L -o "$temp_file" "$download_url" || {
            print_error "下载 go-shadowsocks2 失败"
            return 1
        }
    elif command -v wget >/dev/null 2>&1; then
        wget --no-check-certificate -O "$temp_file" "$download_url" || {
            print_error "下载 go-shadowsocks2 失败"
            return 1
        }
    else
        print_error "缺少 curl 或 wget，无法下载 go-shadowsocks2"
        return 1
    fi
    
    # 解压并安装
    if command -v gzip >/dev/null 2>&1; then
        gzip -d -c "$temp_file" > "$target_file" || {
            print_error "解压 go-shadowsocks2 失败"
            rm -f "$temp_file"
            return 1
        }
    else
        print_error "缺少 gzip，无法解压 go-shadowsocks2"
        rm -f "$temp_file"
        return 1
    fi
    
    # 设置权限
    chmod +x "$target_file"
    rm -f "$temp_file"
    
    print_info "✓ go-shadowsocks2 安装完成"
}

# 安装 snell-server
install_snell_server() {
    local install_dir="$1"
    local arch="$2"
    
    print_info "安装 snell-server..."
    
    # 根据架构确定下载链接
    local download_url
    case "$arch" in
        amd64)
            download_url="https://dl.nssurge.com/snell/snell-server-v4.1.1-linux-amd64.zip"
            ;;
        arm64)
            download_url="https://dl.nssurge.com/snell/snell-server-v4.1.1-linux-aarch64.zip"
            ;;
        *)
            print_warn "不支持的架构: $arch，跳过 snell-server 安装"
            return 0
            ;;
    esac
    
    local temp_file="/tmp/snell-server.zip"
    local target_file="$install_dir/snell-server"
    
    # 检查是否已存在
    if [ -f "$target_file" ]; then
        print_info "snell-server 已存在，跳过安装"
        return 0
    fi
    
    # 下载文件
    if command -v curl >/dev/null 2>&1; then
        curl -k -L -o "$temp_file" "$download_url" || {
            print_error "下载 snell-server 失败"
            return 1
        }
    elif command -v wget >/dev/null 2>&1; then
        wget --no-check-certificate -O "$temp_file" "$download_url" || {
            print_error "下载 snell-server 失败"
            return 1
        }
    else
        print_error "缺少 curl 或 wget，无法下载 snell-server"
        return 1
    fi
    
    # 解压并安装
    if command -v unzip >/dev/null 2>&1; then
        local temp_dir="/tmp/snell-extract"
        mkdir -p "$temp_dir"
        unzip -q "$temp_file" -d "$temp_dir" || {
            print_error "解压 snell-server 失败"
            rm -rf "$temp_file" "$temp_dir"
            return 1
        }
        
        # 查找 snell-server 二进制文件
        local snell_binary=$(find "$temp_dir" -name "snell-server" -type f | head -1)
        if [ -n "$snell_binary" ]; then
            cp "$snell_binary" "$target_file"
            chmod +x "$target_file"
        else
            print_error "未找到 snell-server 二进制文件"
            rm -rf "$temp_file" "$temp_dir"
            return 1
        fi
        
        rm -rf "$temp_file" "$temp_dir"
    else
        print_error "缺少 unzip，无法解压 snell-server"
        rm -f "$temp_file"
        return 1
    fi
    
    print_info "✓ snell-server 安装完成"
}

# 安装 trojan-go
install_trojan_go() {
    local install_dir="$1"
    local arch="$2"
    
    print_info "安装 trojan-go..."
    
    # 获取最新版本
    local latest_version
    if command -v curl >/dev/null 2>&1; then
        latest_version=$(curl -k -s https://api.github.com/repos/p4gefau1t/trojan-go/releases/latest | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/' | head -1)
    elif command -v wget >/dev/null 2>&1; then
        latest_version=$(wget --no-check-certificate -qO- https://api.github.com/repos/p4gefau1t/trojan-go/releases/latest | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/' | head -1)
    fi
    
    # 如果获取版本失败，使用默认版本
    if [ -z "$latest_version" ]; then
        latest_version="v0.10.6"
        print_warn "无法获取最新版本，使用默认版本: $latest_version"
    fi
    
    # 根据架构确定下载链接
    local download_url="https://github.com/p4gefau1t/trojan-go/releases/download/${latest_version}/trojan-go-linux-${arch}.zip"
    
    local temp_file="/tmp/trojan-go.zip"
    local target_file="$install_dir/trojan-go"
    
    # 检查是否已存在
    if [ -f "$target_file" ]; then
        print_info "trojan-go 已存在，跳过安装"
        return 0
    fi
    
    # 下载文件
    if command -v curl >/dev/null 2>&1; then
        curl -k -L -o "$temp_file" "$download_url" || {
            print_error "下载 trojan-go 失败"
            return 1
        }
    elif command -v wget >/dev/null 2>&1; then
        wget --no-check-certificate -O "$temp_file" "$download_url" || {
            print_error "下载 trojan-go 失败"
            return 1
        }
    else
        print_error "缺少 curl 或 wget，无法下载 trojan-go"
        return 1
    fi
    
    # 解压并安装
    if command -v unzip >/dev/null 2>&1; then
        local temp_dir="/tmp/trojan-extract"
        mkdir -p "$temp_dir"
        unzip -q "$temp_file" -d "$temp_dir" || {
            print_error "解压 trojan-go 失败"
            rm -rf "$temp_file" "$temp_dir"
            return 1
        }
        
        # 查找 trojan-go 二进制文件
        local trojan_binary=$(find "$temp_dir" -name "trojan-go" -type f | head -1)
        if [ -n "$trojan_binary" ]; then
            cp "$trojan_binary" "$target_file"
            chmod +x "$target_file"
        else
            print_error "未找到 trojan-go 二进制文件"
            rm -rf "$temp_file" "$temp_dir"
            return 1
        fi
        
        rm -rf "$temp_file" "$temp_dir"
    else
        print_error "缺少 unzip，无法解压 trojan-go"
        rm -f "$temp_file"
        return 1
    fi
    
    print_info "✓ trojan-go 安装完成"
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
  bin_path: "/usr/local/bin/proxy"
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

# WebSocket配置
websocket:
  enabled: true
  server_url: "ws://localhost:8080/ws"
  agent_id: "agent-001"
  heartbeat_interval: 30s
  metrics_interval: 60s
  reconnect_interval: 5s
  max_reconnect_attempts: 0

# 升级配置
upgrade:
  enabled: true
  agent_script_url: "https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/agent_upgrade.sh"
  proxy_script_url: "https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/proxy_upgrade.sh"
  timeout: 600
  verify_checksum: false
  backup_enabled: true

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
    echo "🔄 升级功能:"
    echo "   Agent支持远程升级功能，通过WebSocket接收升级指令"
    echo "   升级脚本URL在配置文件中硬编码，确保安全性"
    echo "   升级配置位置: $CONFIG_DIR/config.yaml (upgrade节)"
    echo "   手动升级脚本: scripts/agent_upgrade.sh 和 scripts/proxy_upgrade.sh"
    echo ""
    echo "📚 更多信息: https://github.com/$GITHUB_REPO"
    echo ""
}

# 主安装流程
main() {
    # 检测安装方式
    detect_pipe_install
    
    echo "======================================"
    echo "NSPass Agent 安装/升级脚本 v$SCRIPT_VERSION"
    echo "======================================"
    echo ""
    
    # 显示执行环境信息
    print_info "执行环境信息:"
    print_info "  脚本版本: $SCRIPT_VERSION"
    print_info "  执行时间: $(date)"
    print_info "  执行用户: $(whoami)"
    print_info "  当前目录: $(pwd)"
    print_info "  Shell: $0"
    print_info "  安装方式: $([ "$PIPE_INSTALL" = "true" ] && echo "管道安装" || echo "本地安装")"
    print_info "  调试模式: $([ "$DEBUG_MODE" = "1" ] && echo "已启用" || echo "已禁用")"
    echo ""
    
    # 显示传入的参数
    if [ $# -gt 0 ]; then
        print_info "传入参数: $*"
    else
        print_info "无传入参数"
    fi
    echo ""
    
    # 解析命令行参数
    debug_log "开始解析命令行参数"
    parse_args "$@"
    debug_log "参数解析完成"
    
    # 解析环境预设
    debug_log "开始解析环境预设"
    parse_env_preset
    debug_log "环境预设解析完成"
    
    # 验证参数
    debug_log "开始验证参数"
    validate_args
    debug_log "参数验证完成"
    
    # 检查运行环境
    debug_log "开始检查运行环境"
    print_step "检查运行环境..."
    
    check_root
    debug_log "root权限检查完成"
    
    detect_arch
    debug_log "架构检测完成: $ARCH"
    
    detect_os
    debug_log "操作系统检测完成: $OS"
    
    # 显示系统信息
    print_info "系统信息:"
    print_info "  操作系统: $OS_NAME $OS_VERSION"
    print_info "  架构: $(uname -m) -> $ARCH"
    print_info "  内核: $(uname -r)"
    print_info "  主机名: $(hostname)"
    
    # 检查网络环境
    print_info "网络环境检查:"
    if command -v ip >/dev/null 2>&1; then
        local ip_addr=$(ip route get 1.1.1.1 2>/dev/null | grep -oP 'src \K\S+' | head -1)
        print_info "  本机IP: ${ip_addr:-未知}"
    fi
    
    if command -v curl >/dev/null 2>&1; then
        print_info "  curl: $(curl --version | head -1 | cut -d' ' -f1-2)"
    else
        print_warn "  curl: 未安装"
    fi
    
    if command -v wget >/dev/null 2>&1; then
        print_info "  wget: $(wget --version | head -1 | cut -d' ' -f1-3)"
    else
        print_warn "  wget: 未安装"
    fi
    echo ""
    
    # 检查是否需要更新
    debug_log "开始检查更新需求"
    check_update_needed
    debug_log "更新需求检查完成: UPDATE_NEEDED=$UPDATE_NEEDED"
    
    if [ "$UPDATE_NEEDED" = false ]; then
        print_info "无需更新，脚本退出"
        exit 0
    fi
    
    # 显示安装计划
    print_info "安装计划:"
    print_info "  目标版本: $LATEST_VERSION"
    print_info "  架构: $ARCH"
    print_info "  安装路径: $INSTALL_DIR"
    print_info "  配置目录: $CONFIG_DIR"
    print_info "  日志目录: $LOG_DIR"
    if [ -n "$SERVER_ID" ]; then
        print_info "  服务器ID: $SERVER_ID"
        print_info "  API地址: $API_BASE_URL"
    fi
    echo ""
    
    # 开始安装流程
    debug_log "开始安装流程"
    
    install_dependencies
    debug_log "依赖安装完成"
    
    stop_service_if_running
    debug_log "服务停止完成"
    
    download_and_install
    debug_log "下载安装完成"
    
    install_proxy_binaries
    debug_log "代理程序安装完成"
    
    setup_config
    debug_log "配置设置完成"
    
    install_systemd_service
    debug_log "systemd服务安装完成"
    
    enable_and_start_service
    debug_log "服务启动完成"
    
    # 检查安装结果
    print_step "检查安装结果..."
    if check_service_status; then
        debug_log "服务状态检查通过"
        show_post_install_info
        print_info "安装成功完成！"
    else
        print_error "安装完成但服务启动异常"
        print_error "请检查以下内容："
        print_error "  1. 配置文件: $CONFIG_DIR/config.yaml"
        print_error "  2. 服务日志: journalctl -u $SERVICE_NAME -n 20"
        print_error "  3. 应用日志: tail -f $LOG_DIR/agent.log"
        
        # 显示详细的错误诊断信息
        print_error ""
        print_error "详细诊断信息:"
        
        # 检查配置文件
        if [ -f "$CONFIG_DIR/config.yaml" ]; then
            print_error "配置文件存在: $CONFIG_DIR/config.yaml"
        else
            print_error "配置文件不存在: $CONFIG_DIR/config.yaml"
        fi
        
        # 检查二进制文件
        if [ -f "$INSTALL_DIR/nspass-agent" ]; then
            print_error "二进制文件存在: $INSTALL_DIR/nspass-agent"
            local file_info=$(ls -la "$INSTALL_DIR/nspass-agent")
            print_error "文件信息: $file_info"
        else
            print_error "二进制文件不存在: $INSTALL_DIR/nspass-agent"
        fi
        
        # 检查服务文件
        if [ -f "/etc/systemd/system/$SERVICE_NAME.service" ]; then
            print_error "服务文件存在: /etc/systemd/system/$SERVICE_NAME.service"
        else
            print_error "服务文件不存在: /etc/systemd/system/$SERVICE_NAME.service"
        fi
        
        # 显示最近的日志
        print_error ""
        print_error "最近的服务日志:"
        journalctl -u $SERVICE_NAME -n 10 --no-pager 2>/dev/null || echo "无法获取服务日志"
        
        exit 1
    fi
    
    debug_log "安装流程完全结束"
}

# 脚本入口
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi 