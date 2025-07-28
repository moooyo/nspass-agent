#!/bin/bash

# NSPass Agent 升级脚本
# 用于远程升级 NSPass Agent 到指定版本
# 使用方法: 
#   ./agent_upgrade.sh --target-version=v1.2.3 --download-url=https://releases.example.com/agent_upgrade_v1.2.3.sh

# 启用调试模式和错误退出
set -e
set -o pipefail

# 处理管道中的错误
trap 'echo "[ERROR] 升级脚本在第 $LINENO 行出错，退出码: $?" >&2; exit 1' ERR

# 调试模式开关
DEBUG_MODE=${DEBUG_MODE:-1}

# 脚本配置
SCRIPT_VERSION="1.0.0"
GITHUB_REPO="moooyo/nspass-agent"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/nspass"
LOG_DIR="/var/log/nspass"
SERVICE_NAME="nspass-agent"
BACKUP_DIR="/opt/nspass-backup"

# 升级参数
TARGET_VERSION=""
DOWNLOAD_URL=""
FORCE_UPGRADE=false
BACKUP_CURRENT=true
UPGRADE_TIMEOUT=300
ROLLBACK_ON_FAILURE=true

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
    echo "NSPass Agent 升级脚本 v$SCRIPT_VERSION"
    echo ""
    echo "使用方法:"
    echo "  $0 [选项]"
    echo ""
    echo "选项:"
    echo "  --target-version <version>     目标版本 (必需)"
    echo "  --download-url <url>          升级脚本下载URL (可选)"
    echo "  --force-upgrade               强制升级，即使版本相同"
    echo "  --no-backup                   不备份当前版本"
    echo "  --timeout <seconds>           升级超时时间 (默认: 300秒)"
    echo "  --no-rollback                 失败时不回滚"
    echo "  -h, --help                    显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 --target-version=v1.2.3"
    echo "  $0 --target-version=v1.2.3 --force-upgrade"
    echo "  $0 --target-version=v1.2.3 --download-url=https://releases.example.com/agent_upgrade_v1.2.3.sh"
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
            --no-rollback)
                ROLLBACK_ON_FAILURE=false
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

# 验证参数
validate_args() {
    if [ -z "$TARGET_VERSION" ]; then
        print_error "必须指定目标版本"
        show_help
        exit 1
    fi
    
    # 验证版本格式
    if ! echo "$TARGET_VERSION" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+)?$'; then
        print_error "版本格式不正确，应为: vX.Y.Z 或 vX.Y.Z-suffix"
        exit 1
    fi
    
    print_info "升级参数验证通过:"
    print_info "  目标版本: $TARGET_VERSION"
    print_info "  下载URL: ${DOWNLOAD_URL:-"自动检测"}"
    print_info "  强制升级: $FORCE_UPGRADE"
    print_info "  备份当前版本: $BACKUP_CURRENT"
    print_info "  升级超时: ${UPGRADE_TIMEOUT}秒"
    print_info "  失败回滚: $ROLLBACK_ON_FAILURE"
}

# 检查运行环境
check_environment() {
    print_step "检查运行环境..."
    
    # 检查是否以root用户运行
    if [ "$EUID" -ne 0 ]; then
        print_error "请以root用户运行此脚本"
        exit 1
    fi
    
    # 检查systemd
    if ! command -v systemctl >/dev/null 2>&1; then
        print_error "此脚本需要systemd支持"
        exit 1
    fi
    
    # 检查当前Agent是否存在
    if [ ! -f "$INSTALL_DIR/nspass-agent" ]; then
        print_error "未找到现有的NSPass Agent安装"
        exit 1
    fi
    
    print_info "环境检查通过"
}

# 获取当前版本
get_current_version() {
    if [ -f "$INSTALL_DIR/nspass-agent" ]; then
        CURRENT_VERSION=$("$INSTALL_DIR/nspass-agent" --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "")
        if [ -n "$CURRENT_VERSION" ]; then
            print_info "当前版本: $CURRENT_VERSION"
        else
            print_warn "无法获取当前版本信息"
            CURRENT_VERSION="unknown"
        fi
    else
        print_error "Agent二进制文件不存在"
        exit 1
    fi
}

# 检查是否需要升级
check_upgrade_needed() {
    get_current_version
    
    if [ "$CURRENT_VERSION" = "$TARGET_VERSION" ] && [ "$FORCE_UPGRADE" = false ]; then
        print_info "当前版本已经是目标版本 ($TARGET_VERSION)，无需升级"
        exit 0
    fi
    
    print_info "准备升级: $CURRENT_VERSION -> $TARGET_VERSION"
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
    debug_log "检测到系统架构: $arch -> $ARCH"
}

# 备份当前版本
backup_current_version() {
    if [ "$BACKUP_CURRENT" = false ]; then
        print_info "跳过备份"
        return 0
    fi
    
    print_step "备份当前版本..."
    
    # 创建备份目录
    local backup_timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_version_dir="$BACKUP_DIR/$CURRENT_VERSION"
    local backup_full_dir="$backup_version_dir/$backup_timestamp"
    
    mkdir -p "$backup_full_dir"
    
    # 备份二进制文件
    if [ -f "$INSTALL_DIR/nspass-agent" ]; then
        cp "$INSTALL_DIR/nspass-agent" "$backup_full_dir/nspass-agent"
        print_info "已备份二进制文件"
    fi
    
    # 备份配置文件
    if [ -d "$CONFIG_DIR" ]; then
        cp -r "$CONFIG_DIR" "$backup_full_dir/config"
        print_info "已备份配置文件"
    fi
    
    # 备份systemd服务文件
    if [ -f "/etc/systemd/system/$SERVICE_NAME.service" ]; then
        cp "/etc/systemd/system/$SERVICE_NAME.service" "$backup_full_dir/"
        print_info "已备份systemd服务文件"
    fi
    
    # 创建备份信息文件
    cat > "$backup_full_dir/backup_info.txt" << EOF
备份时间: $(date)
备份版本: $CURRENT_VERSION
目标版本: $TARGET_VERSION
备份原因: Agent升级
主机名: $(hostname)
系统: $(uname -a)
EOF
    
    # 记录备份路径供回滚使用
    BACKUP_PATH="$backup_full_dir"
    
    print_info "备份完成: $backup_full_dir"
}

# 停止Agent服务
stop_agent_service() {
    print_step "停止Agent服务..."
    
    if systemctl is-active --quiet $SERVICE_NAME 2>/dev/null; then
        print_info "正在停止服务..."
        systemctl stop $SERVICE_NAME
        
        # 等待服务完全停止
        local timeout=30
        local count=0
        while systemctl is-active --quiet $SERVICE_NAME 2>/dev/null && [ $count -lt $timeout ]; do
            sleep 1
            count=$((count + 1))
        done
        
        if systemctl is-active --quiet $SERVICE_NAME 2>/dev/null; then
            print_error "服务停止超时"
            return 1
        fi
        
        print_info "服务已停止"
    else
        print_info "服务未运行"
    fi
}

# 下载新版本Agent
download_new_version() {
    print_step "下载新版本Agent..."
    
    # 如果没有提供下载URL，使用默认GitHub releases
    if [ -z "$DOWNLOAD_URL" ]; then
        local filename="nspass-agent-linux-$ARCH.tar.gz"
        DOWNLOAD_URL="https://github.com/$GITHUB_REPO/releases/download/$TARGET_VERSION/$filename"
        print_info "使用默认下载URL: $DOWNLOAD_URL"
    fi
    
    local temp_dir=$(mktemp -d)
    local temp_file="$temp_dir/nspass-agent-new"
    
    print_info "下载URL: $DOWNLOAD_URL"
    print_info "临时目录: $temp_dir"
    
    # 下载文件
    local download_success=false
    
    if command -v curl >/dev/null 2>&1; then
        print_info "使用curl下载..."
        if curl -L --connect-timeout 30 --max-time 300 -o "$temp_file" "$DOWNLOAD_URL" 2>&1; then
            download_success=true
            debug_log "curl下载成功"
        else
            print_warn "curl下载失败"
        fi
    fi
    
    if [ "$download_success" = false ] && command -v wget >/dev/null 2>&1; then
        print_info "使用wget下载..."
        if wget --timeout=300 --tries=3 -O "$temp_file" "$DOWNLOAD_URL" 2>&1; then
            download_success=true
            debug_log "wget下载成功"
        else
            print_warn "wget下载失败"
        fi
    fi
    
    if [ "$download_success" = false ]; then
        print_error "下载失败"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    # 检查下载的文件
    if [ ! -f "$temp_file" ] || [ ! -s "$temp_file" ]; then
        print_error "下载的文件无效"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    # 如果是压缩文件，解压
    if echo "$DOWNLOAD_URL" | grep -q "\.tar\.gz$"; then
        print_info "解压下载的文件..."
        cd "$temp_dir"
        if tar -xzf "$temp_file" 2>&1; then
            # 查找二进制文件
            local binary_file=$(find "$temp_dir" -name "nspass-agent" -type f | head -1)
            if [ -n "$binary_file" ]; then
                temp_file="$binary_file"
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
    
    # 验证二进制文件
    if ! chmod +x "$temp_file" || ! "$temp_file" --version >/dev/null 2>&1; then
        print_error "下载的二进制文件无效"
        rm -rf "$temp_dir"
        exit 1
    fi
    
    # 验证版本
    local downloaded_version=$("$temp_file" --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "")
    if [ "$downloaded_version" != "$TARGET_VERSION" ]; then
        print_warn "下载版本 ($downloaded_version) 与目标版本 ($TARGET_VERSION) 不匹配"
    fi
    
    NEW_BINARY_PATH="$temp_file"
    TEMP_DIR="$temp_dir"
    
    print_info "新版本下载完成: $downloaded_version"
}

# 安装新版本
install_new_version() {
    print_step "安装新版本..."
    
    if [ -z "$NEW_BINARY_PATH" ] || [ ! -f "$NEW_BINARY_PATH" ]; then
        print_error "新版本二进制文件不存在"
        exit 1
    fi
    
    # 安装新版本
    print_info "复制新版本到 $INSTALL_DIR/nspass-agent"
    
    if ! cp "$NEW_BINARY_PATH" "$INSTALL_DIR/nspass-agent"; then
        print_error "复制文件失败"
        exit 1
    fi
    
    if ! chmod +x "$INSTALL_DIR/nspass-agent"; then
        print_error "设置执行权限失败"
        exit 1
    fi
    
    print_info "新版本安装完成"
}

# 启动Agent服务
start_agent_service() {
    print_step "启动Agent服务..."
    
    # 重新加载systemd配置
    systemctl daemon-reload
    
    # 启动服务
    if ! systemctl start $SERVICE_NAME; then
        print_error "启动服务失败"
        return 1
    fi
    
    # 等待服务启动
    sleep 3
    
    # 检查服务状态
    if systemctl is-active --quiet $SERVICE_NAME; then
        print_info "服务启动成功"
        return 0
    else
        print_error "服务启动失败"
        return 1
    fi
}

# 验证升级结果
verify_upgrade() {
    print_step "验证升级结果..."
    
    # 检查版本
    local installed_version=$("$INSTALL_DIR/nspass-agent" --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "")
    if [ "$installed_version" = "$TARGET_VERSION" ]; then
        print_info "✓ 版本验证通过: $installed_version"
    else
        print_error "✗ 版本验证失败: 期望 $TARGET_VERSION，实际 $installed_version"
        return 1
    fi
    
    # 检查服务状态
    if systemctl is-active --quiet $SERVICE_NAME; then
        print_info "✓ 服务状态正常"
    else
        print_error "✗ 服务状态异常"
        return 1
    fi
    
    # 检查进程是否运行
    if pgrep -f "nspass-agent" >/dev/null; then
        print_info "✓ 进程运行正常"
    else
        print_error "✗ 进程未运行"
        return 1
    fi
    
    print_info "升级验证通过"
    return 0
}

# 回滚到之前版本
rollback_upgrade() {
    if [ "$ROLLBACK_ON_FAILURE" = false ]; then
        print_warn "已禁用自动回滚"
        return 1
    fi
    
    if [ -z "$BACKUP_PATH" ] || [ ! -d "$BACKUP_PATH" ]; then
        print_error "未找到备份，无法回滚"
        return 1
    fi
    
    print_step "回滚到之前版本..."
    
    # 停止当前服务
    systemctl stop $SERVICE_NAME 2>/dev/null || true
    
    # 恢复二进制文件
    if [ -f "$BACKUP_PATH/nspass-agent" ]; then
        cp "$BACKUP_PATH/nspass-agent" "$INSTALL_DIR/nspass-agent"
        chmod +x "$INSTALL_DIR/nspass-agent"
        print_info "已恢复二进制文件"
    fi
    
    # 恢复配置文件
    if [ -d "$BACKUP_PATH/config" ]; then
        cp -r "$BACKUP_PATH/config/"* "$CONFIG_DIR/"
        print_info "已恢复配置文件"
    fi
    
    # 恢复systemd服务文件
    if [ -f "$BACKUP_PATH/$SERVICE_NAME.service" ]; then
        cp "$BACKUP_PATH/$SERVICE_NAME.service" "/etc/systemd/system/"
        systemctl daemon-reload
        print_info "已恢复systemd服务文件"
    fi
    
    # 启动服务
    if systemctl start $SERVICE_NAME; then
        print_info "回滚完成，服务已启动"
        return 0
    else
        print_error "回滚后服务启动失败"
        return 1
    fi
}

# 清理临时文件
cleanup() {
    if [ -n "$TEMP_DIR" ] && [ -d "$TEMP_DIR" ]; then
        rm -rf "$TEMP_DIR"
        debug_log "已清理临时文件: $TEMP_DIR"
    fi
}

# 显示升级结果
show_upgrade_result() {
    local success=$1
    
    echo ""
    echo "======================================"
    if [ "$success" = "0" ]; then
        print_info "Agent升级成功完成！"
    else
        print_error "Agent升级失败！"
    fi
    echo "======================================"
    echo ""
    
    print_info "升级信息:"
    print_info "  原版本: $CURRENT_VERSION"
    print_info "  目标版本: $TARGET_VERSION"
    
    if [ "$success" = "0" ]; then
        local final_version=$("$INSTALL_DIR/nspass-agent" --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
        print_info "  当前版本: $final_version"
        print_info "  服务状态: $(systemctl is-active $SERVICE_NAME 2>/dev/null || echo 'unknown')"
        
        echo ""
        print_info "常用命令:"
        print_info "  查看服务状态: systemctl status $SERVICE_NAME"
        print_info "  查看日志: journalctl -u $SERVICE_NAME -f"
        print_info "  重启服务: systemctl restart $SERVICE_NAME"
    else
        echo ""
        print_error "故障排除:"
        print_error "  查看服务状态: systemctl status $SERVICE_NAME"
        print_error "  查看日志: journalctl -u $SERVICE_NAME -n 50"
        if [ -n "$BACKUP_PATH" ]; then
            print_error "  手动回滚: cp $BACKUP_PATH/nspass-agent $INSTALL_DIR/"
        fi
    fi
    
    echo ""
}

# 主升级流程
main() {
    echo "======================================"
    echo "NSPass Agent 升级脚本 v$SCRIPT_VERSION"
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
    
    # 检查是否需要升级
    check_upgrade_needed
    
    # 开始升级流程
    local upgrade_success=false
    
    # 设置超时
    (
        sleep $UPGRADE_TIMEOUT
        print_error "升级超时 (${UPGRADE_TIMEOUT}秒)"
        exit 1
    ) &
    local timeout_pid=$!
    
    # 执行升级步骤
    if backup_current_version && \
       stop_agent_service && \
       download_new_version && \
       install_new_version && \
       start_agent_service && \
       verify_upgrade; then
        upgrade_success=true
        print_info "升级流程完成"
    else
        print_error "升级过程中发生错误"
        if rollback_upgrade; then
            print_warn "已回滚到之前版本"
        else
            print_error "回滚失败，请手动检查"
        fi
        upgrade_success=false
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