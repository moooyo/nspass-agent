#!/bin/bash

# NSPass Proxy 升级脚本
# 用于远程升级代理软件到指定版本
# 使用方法: 
#   ./proxy_upgrade.sh --target-version=v1.2.3 --proxy-types=shadowsocks,trojan,snell

# 启用调试模式和错误退出
set -e
set -o pipefail

# 处理管道中的错误
trap 'echo "[ERROR] 升级脚本在第 $LINENO 行出错，退出码: $?" >&2; exit 1' ERR

# 调试模式开关
DEBUG_MODE=${DEBUG_MODE:-1}

# 脚本配置
SCRIPT_VERSION="1.0.0"
PROXY_BIN_DIR="/usr/local/bin/proxy"
LOG_DIR="/var/log/nspass"
BACKUP_DIR="/opt/nspass-backup/proxy"

# 升级参数
TARGET_VERSION=""
DOWNLOAD_URL=""
PROXY_TYPES=""
FORCE_UPGRADE=false
BACKUP_CURRENT=true
UPGRADE_TIMEOUT=300

# 支持的代理类型及其下载配置
declare -A PROXY_CONFIGS
PROXY_CONFIGS[shadowsocks]="go-shadowsocks2:https://github.com/shadowsocks/go-shadowsocks2/releases/download/v0.1.5/shadowsocks2-linux.gz"
PROXY_CONFIGS[snell]="snell-server:https://dl.nssurge.com/snell/snell-server-v4.1.1-linux-{ARCH}.zip"
PROXY_CONFIGS[trojan]="trojan-go:https://github.com/p4gefau1t/trojan-go/releases/download/{VERSION}/trojan-go-linux-{ARCH}.zip"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 调试函数
debug_log() {
    if [ "$DEBUG_MODE" = "1" ]; then
        echo -e "${BLUE}[DEBUG]${NC} $1" >&2
    fi
}

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
    echo "NSPass Proxy 升级脚本 v$SCRIPT_VERSION"
    echo ""
    echo "使用方法:"
    echo "  $0 [选项]"
    echo ""
    echo "选项:"
    echo "  --target-version <version>     目标版本 (可选，默认为最新版本)"
    echo "  --download-url <url>          升级脚本下载URL (可选)"
    echo "  --proxy-types <types>         要升级的代理类型，逗号分隔 (默认: shadowsocks,trojan,snell)"
    echo "  --force-upgrade               强制升级，即使版本相同"
    echo "  --no-backup                   不备份当前版本"
    echo "  --timeout <seconds>           升级超时时间 (默认: 300秒)"
    echo "  -h, --help                    显示此帮助信息"
    echo ""
    echo "支持的代理类型:"
    echo "  shadowsocks  - go-shadowsocks2"
    echo "  trojan       - trojan-go"
    echo "  snell        - snell-server"
    echo ""
    echo "示例:"
    echo "  $0                                              # 升级所有代理到最新版本"
    echo "  $0 --proxy-types=shadowsocks,trojan             # 只升级指定类型"
    echo "  $0 --target-version=v1.2.3 --force-upgrade     # 强制升级到指定版本"
    echo ""
}

# 解析命令行参数
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --target-version=*)
                TARGET_VERSION="${1#*=}"
                shift
                ;;
            --target-version)
                TARGET_VERSION="$2"
                shift 2
                ;;
            --download-url=*)
                DOWNLOAD_URL="${1#*=}"
                shift
                ;;
            --download-url)
                DOWNLOAD_URL="$2"
                shift 2
                ;;
            --proxy-types=*)
                PROXY_TYPES="${1#*=}"
                shift
                ;;
            --proxy-types)
                PROXY_TYPES="$2"
                shift 2
                ;;
            --force-upgrade)
                FORCE_UPGRADE=true
                shift
                ;;
            --no-backup)
                BACKUP_CURRENT=false
                shift
                ;;
            --timeout=*)
                UPGRADE_TIMEOUT="${1#*=}"
                shift
                ;;
            --timeout)
                UPGRADE_TIMEOUT="$2"
                shift 2
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

# 验证参数
validate_args() {
    # 如果没有指定代理类型，默认升级所有类型
    if [ -z "$PROXY_TYPES" ]; then
        PROXY_TYPES="shadowsocks,trojan,snell"
    fi
    
    # 验证代理类型
    IFS=',' read -ra TYPES_ARRAY <<< "$PROXY_TYPES"
    for proxy_type in "${TYPES_ARRAY[@]}"; do
        if [[ ! " ${!PROXY_CONFIGS[@]} " =~ " ${proxy_type} " ]]; then
            print_error "不支持的代理类型: $proxy_type"
            print_error "支持的类型: ${!PROXY_CONFIGS[@]}"
            exit 1
        fi
    done
    
    print_info "升级参数验证通过:"
    print_info "  目标版本: ${TARGET_VERSION:-"最新版本"}"
    print_info "  代理类型: $PROXY_TYPES"
    print_info "  下载URL: ${DOWNLOAD_URL:-"自动检测"}"
    print_info "  强制升级: $FORCE_UPGRADE"
    print_info "  备份当前版本: $BACKUP_CURRENT"
    print_info "  升级超时: ${UPGRADE_TIMEOUT}秒"
}

# 检查运行环境
check_environment() {
    print_step "检查运行环境..."
    
    # 检查是否以root用户运行
    if [ "$EUID" -ne 0 ]; then
        print_error "请以root用户运行此脚本"
        exit 1
    fi
    
    # 检查代理目录是否存在
    if [ ! -d "$PROXY_BIN_DIR" ]; then
        print_warn "代理目录不存在，将创建: $PROXY_BIN_DIR"
        mkdir -p "$PROXY_BIN_DIR"
    fi
    
    # 检查必要工具
    local missing_tools=""
    for tool in curl wget gzip unzip tar systemctl; do
        if ! command -v "$tool" >/dev/null 2>&1; then
            missing_tools="$missing_tools $tool"
        fi
    done
    
    if [ -n "$missing_tools" ]; then
        print_error "缺少必要工具:$missing_tools"
        print_error "请先安装这些工具"
        exit 1
    fi
    
    print_info "环境检查通过"
}

# 检测系统架构
detect_arch() {
    local arch=$(uname -m)
    case $arch in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        arm64|aarch64)
            ARCH="arm64"
            ;;
        armv7l)
            ARCH="armv7"
            ;;
        i386|i686)
            ARCH="386"
            ;;
        *)
            print_error "不支持的架构: $arch"
            exit 1
            ;;
    esac
    debug_log "检测到系统架构: $arch -> $ARCH"
}

# 获取代理当前版本
get_proxy_version() {
    local proxy_type="$1"
    local binary_path="$PROXY_BIN_DIR/${PROXY_CONFIGS[$proxy_type]%%:*}"
    
    if [ -f "$binary_path" ]; then
        # 尝试多种方式获取版本
        local version=""
        case $proxy_type in
            shadowsocks)
                # go-shadowsocks2 通常没有版本信息，返回文件修改时间
                version="$(stat -c %Y "$binary_path" 2>/dev/null || echo "unknown")"
                ;;
            trojan)
                # trojan-go 有版本信息
                version=$("$binary_path" -version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
                ;;
            snell)
                # snell-server 通常没有标准版本输出
                version="$(stat -c %Y "$binary_path" 2>/dev/null || echo "unknown")"
                ;;
            *)
                version="unknown"
                ;;
        esac
        echo "$version"
    else
        echo "not_installed"
    fi
}

# 停止所有代理进程
stop_all_proxies() {
    print_step "停止所有代理进程..."
    
    # 使用nspass-agent的proxy manager API或直接杀进程
    local proxy_processes=("go-shadowsocks2" "trojan-go" "snell-server")
    
    for process in "${proxy_processes[@]}"; do
        if pgrep -f "$process" >/dev/null; then
            print_info "停止 $process 进程"
            pkill -f "$process" || true
            
            # 等待进程完全停止
            local timeout=10
            local count=0
            while pgrep -f "$process" >/dev/null && [ $count -lt $timeout ]; do
                sleep 1
                count=$((count + 1))
            done
            
            if pgrep -f "$process" >/dev/null; then
                print_warn "$process 进程未能正常停止，强制杀死"
                pkill -9 -f "$process" || true
            fi
        fi
    done
    
    print_info "所有代理进程已停止"
}

# 备份当前代理二进制文件
backup_proxy_binaries() {
    if [ "$BACKUP_CURRENT" = false ]; then
        print_info "跳过备份"
        return 0
    fi
    
    print_step "备份当前代理二进制文件..."
    
    # 创建备份目录
    local backup_timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_full_dir="$BACKUP_DIR/$backup_timestamp"
    
    mkdir -p "$backup_full_dir"
    
    # 备份所有代理二进制文件
    if [ -d "$PROXY_BIN_DIR" ]; then
        cp -r "$PROXY_BIN_DIR" "$backup_full_dir/"
        print_info "已备份代理二进制文件到: $backup_full_dir"
    fi
    
    # 创建备份信息文件
    cat > "$backup_full_dir/backup_info.txt" << EOF
备份时间: $(date)
备份类型: Proxy升级备份
代理类型: $PROXY_TYPES
目标版本: ${TARGET_VERSION:-"最新版本"}
主机名: $(hostname)
系统: $(uname -a)
EOF
    
    # 记录当前版本信息
    echo "当前版本信息:" >> "$backup_full_dir/backup_info.txt"
    IFS=',' read -ra TYPES_ARRAY <<< "$PROXY_TYPES"
    for proxy_type in "${TYPES_ARRAY[@]}"; do
        local version=$(get_proxy_version "$proxy_type")
        echo "  $proxy_type: $version" >> "$backup_full_dir/backup_info.txt"
    done
    
    BACKUP_PATH="$backup_full_dir"
    print_info "备份完成: $backup_full_dir"
}

# 获取最新版本信息
get_latest_version() {
    local proxy_type="$1"
    
    case $proxy_type in
        trojan)
            # 从GitHub API获取trojan-go最新版本
            if command -v curl >/dev/null 2>&1; then
                local latest=$(curl -s https://api.github.com/repos/p4gefau1t/trojan-go/releases/latest | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/' || echo "")
                if [ -n "$latest" ]; then
                    echo "$latest"
                    return 0
                fi
            fi
            # 如果API失败，使用默认版本
            echo "v0.10.6"
            ;;
        shadowsocks)
            # go-shadowsocks2 版本相对固定
            echo "v0.1.5"
            ;;
        snell)
            # snell-server 版本相对固定
            echo "v4.1.1"
            ;;
        *)
            echo "latest"
            ;;
    esac
}

# 下载并安装单个代理
install_single_proxy() {
    local proxy_type="$1"
    local config_info="${PROXY_CONFIGS[$proxy_type]}"
    local binary_name="${config_info%%:*}"
    local download_template="${config_info#*:}"
    
    print_step "升级 $proxy_type ($binary_name)..."
    
    # 确定目标版本
    local target_version="$TARGET_VERSION"
    if [ -z "$target_version" ]; then
        target_version=$(get_latest_version "$proxy_type")
        print_info "使用最新版本: $target_version"
    fi
    
    # 构建下载URL
    local download_url="$download_template"
    download_url="${download_url//\{VERSION\}/$target_version}"
    download_url="${download_url//\{ARCH\}/$ARCH}"
    
    # snell特殊处理架构映射
    if [ "$proxy_type" = "snell" ]; then
        case "$ARCH" in
            amd64) download_url="${download_url//\{ARCH\}/amd64}" ;;
            arm64) download_url="${download_url//\{ARCH\}/aarch64}" ;;
            *) 
                print_warn "不支持的架构: $ARCH，跳过 $proxy_type"
                return 0
                ;;
        esac
    fi
    
    print_info "下载URL: $download_url"
    
    # 创建临时目录
    local temp_dir=$(mktemp -d)
    local temp_file="$temp_dir/$(basename "$download_url")"
    local target_path="$PROXY_BIN_DIR/$binary_name"
    
    # 下载文件
    local download_success=false
    
    if command -v curl >/dev/null 2>&1; then
        print_info "使用curl下载 $proxy_type..."
        if curl -k -L --connect-timeout 30 --max-time 300 -o "$temp_file" "$download_url" 2>&1; then
            download_success=true
            debug_log "curl下载成功"
        else
            print_warn "curl下载失败"
        fi
    fi
    
    if [ "$download_success" = false ] && command -v wget >/dev/null 2>&1; then
        print_info "使用wget下载 $proxy_type..."
        if wget --no-check-certificate --timeout=300 --tries=3 -O "$temp_file" "$download_url" 2>&1; then
            download_success=true
            debug_log "wget下载成功"
        else
            print_warn "wget下载失败"
        fi
    fi
    
    if [ "$download_success" = false ]; then
        print_error "$proxy_type 下载失败"
        rm -rf "$temp_dir"
        return 1
    fi
    
    # 检查下载的文件
    if [ ! -f "$temp_file" ] || [ ! -s "$temp_file" ]; then
        print_error "$proxy_type 下载的文件无效"
        rm -rf "$temp_dir"
        return 1
    fi
    
    # 解压和安装
    case "$proxy_type" in
        shadowsocks)
            # 解压gzip文件
            if gzip -d -c "$temp_file" > "$target_path"; then
                chmod +x "$target_path"
                print_info "✓ $proxy_type 安装完成"
            else
                print_error "$proxy_type 解压失败"
                rm -rf "$temp_dir"
                return 1
            fi
            ;;
        snell|trojan)
            # 解压zip文件
            local extract_dir="$temp_dir/extract"
            mkdir -p "$extract_dir"
            
            if unzip -q "$temp_file" -d "$extract_dir"; then
                # 查找二进制文件
                local binary_file=$(find "$extract_dir" -name "$binary_name" -type f | head -1)
                if [ -n "$binary_file" ]; then
                    cp "$binary_file" "$target_path"
                    chmod +x "$target_path"
                    print_info "✓ $proxy_type 安装完成"
                else
                    print_error "未找到 $proxy_type 二进制文件"
                    rm -rf "$temp_dir"
                    return 1
                fi
            else
                print_error "$proxy_type 解压失败"
                rm -rf "$temp_dir"
                return 1
            fi
            ;;
        *)
            print_error "未知的代理类型: $proxy_type"
            rm -rf "$temp_dir"
            return 1
            ;;
    esac
    
    # 清理临时文件
    rm -rf "$temp_dir"
    
    return 0
}

# 启动所有代理
restart_all_proxies() {
    print_step "重启所有代理服务..."
    
    # 这里需要通过nspass-agent的API重启代理
    # 或者通过systemctl重启nspass-agent服务来重新加载代理
    
    if systemctl is-active --quiet nspass-agent 2>/dev/null; then
        print_info "重启nspass-agent服务以重新加载代理..."
        if systemctl restart nspass-agent; then
            print_info "nspass-agent服务重启成功"
            
            # 等待服务完全启动
            sleep 5
            
            # 检查服务状态
            if systemctl is-active --quiet nspass-agent; then
                print_info "✓ 服务状态正常"
                return 0
            else
                print_error "✗ 服务启动异常"
                return 1
            fi
        else
            print_error "nspass-agent服务重启失败"
            return 1
        fi
    else
        print_warn "nspass-agent服务未运行，无法自动重启代理"
        print_info "请手动启动nspass-agent服务: systemctl start nspass-agent"
        return 0
    fi
}

# 验证升级结果
verify_upgrade() {
    print_step "验证升级结果..."
    
    local success_count=0
    local total_count=0
    
    IFS=',' read -ra TYPES_ARRAY <<< "$PROXY_TYPES"
    for proxy_type in "${TYPES_ARRAY[@]}"; do
        total_count=$((total_count + 1))
        
        local binary_name="${PROXY_CONFIGS[$proxy_type]%%:*}"
        local binary_path="$PROXY_BIN_DIR/$binary_name"
        
        if [ -f "$binary_path" ] && [ -x "$binary_path" ]; then
            local current_version=$(get_proxy_version "$proxy_type")
            print_info "✓ $proxy_type ($binary_name): $current_version"
            success_count=$((success_count + 1))
        else
            print_error "✗ $proxy_type ($binary_name): 文件不存在或不可执行"
        fi
    done
    
    print_info "升级验证完成: $success_count/$total_count 成功"
    
    if [ "$success_count" -eq "$total_count" ]; then
        return 0
    else
        return 1
    fi
}

# 清理临时文件
cleanup() {
    # 清理可能遗留的临时文件
    find /tmp -name "proxy_upgrade_*" -type d -mtime +1 -exec rm -rf {} + 2>/dev/null || true
}

# 显示升级结果
show_upgrade_result() {
    local success=$1
    
    echo ""
    echo "======================================"
    if [ "$success" = "0" ]; then
        print_info "Proxy升级成功完成！"
    else
        print_error "Proxy升级失败！"
    fi
    echo "======================================"
    echo ""
    
    print_info "升级信息:"
    print_info "  代理类型: $PROXY_TYPES"
    print_info "  目标版本: ${TARGET_VERSION:-"最新版本"}"
    
    if [ "$success" = "0" ]; then
        echo ""
        print_info "代理状态:"
        IFS=',' read -ra TYPES_ARRAY <<< "$PROXY_TYPES"
        for proxy_type in "${TYPES_ARRAY[@]}"; do
            local binary_name="${PROXY_CONFIGS[$proxy_type]%%:*}"
            local binary_path="$PROXY_BIN_DIR/$binary_name"
            local version=$(get_proxy_version "$proxy_type")
            print_info "  $proxy_type ($binary_name): $version"
        done
        
        echo ""
        print_info "常用命令:"
        print_info "  查看代理状态: ls -la $PROXY_BIN_DIR"
        print_info "  查看服务状态: systemctl status nspass-agent"
        print_info "  查看日志: journalctl -u nspass-agent -f"
        print_info "  重启服务: systemctl restart nspass-agent"
    else
        echo ""
        print_error "故障排除:"
        print_error "  检查代理文件: ls -la $PROXY_BIN_DIR"
        print_error "  查看服务状态: systemctl status nspass-agent"
        print_error "  查看日志: journalctl -u nspass-agent -n 50"
        if [ -n "$BACKUP_PATH" ]; then
            print_error "  手动回滚: cp -r $BACKUP_PATH/proxy/* $PROXY_BIN_DIR/"
        fi
    fi
    
    echo ""
}

# 主升级流程
main() {
    echo "======================================"
    echo "NSPass Proxy 升级脚本 v$SCRIPT_VERSION"
    echo "======================================"
    echo ""
    
    # 设置陷阱处理
    trap cleanup EXIT
    trap 'print_error "升级被中断"; cleanup; exit 1' INT TERM
    
    # 解析和验证参数
    parse_args "$@"
    validate_args
    
    # 检查环境
    check_environment
    detect_arch
    
    # 开始升级流程
    local upgrade_success=false
    local failed_proxies=""
    
    # 设置超时
    (
        sleep $UPGRADE_TIMEOUT
        print_error "升级超时 (${UPGRADE_TIMEOUT}秒)"
        exit 1
    ) &
    local timeout_pid=$!
    
    # 执行升级步骤
    if backup_proxy_binaries && stop_all_proxies; then
        # 升级各个代理
        local success_count=0
        local total_count=0
        
        IFS=',' read -ra TYPES_ARRAY <<< "$PROXY_TYPES"
        for proxy_type in "${TYPES_ARRAY[@]}"; do
            total_count=$((total_count + 1))
            
            if install_single_proxy "$proxy_type"; then
                success_count=$((success_count + 1))
            else
                failed_proxies="$failed_proxies $proxy_type"
            fi
        done
        
        # 重启所有代理
        if [ "$success_count" -gt 0 ]; then
            if restart_all_proxies && verify_upgrade; then
                if [ "$success_count" -eq "$total_count" ]; then
                    upgrade_success=true
                    print_info "所有代理升级成功"
                else
                    print_warn "部分代理升级成功 ($success_count/$total_count)"
                    print_warn "失败的代理:$failed_proxies"
                fi
            else
                print_error "代理重启或验证失败"
            fi
        else
            print_error "所有代理升级失败"
        fi
    else
        print_error "备份或停止代理失败"
    fi
    
    # 停止超时计时器
    kill $timeout_pid 2>/dev/null || true
    
    # 清理临时文件
    cleanup
    
    # 显示结果
    if [ "$upgrade_success" = true ]; then
        show_upgrade_result 0
        exit 0
    else
        show_upgrade_result 1
        exit 1
    fi
}

# 脚本入口
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi 