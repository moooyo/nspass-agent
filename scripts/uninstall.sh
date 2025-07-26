#!/bin/bash

# NSPass Agent 卸载脚本
# 使用方法: curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/uninstall.sh | bash

set -e

# 版本信息
SCRIPT_VERSION="2.1.0"
GITHUB_REPO="nspass/nspass-agent"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/nspass"
LOG_DIR="/var/log/nspass"
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

# 检查是否安装了NSPass Agent
check_installation() {
    print_step "检查NSPass Agent安装状态..."
    
    local found=false
    
    # 检查二进制文件
    if [ -f "$INSTALL_DIR/nspass-agent" ]; then
        local version=$("$INSTALL_DIR/nspass-agent" --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
        print_info "发现二进制文件: $INSTALL_DIR/nspass-agent (版本: $version)"
        found=true
    fi
    
    # 检查systemd服务
    if [ -f "/etc/systemd/system/$SERVICE_NAME.service" ]; then
        print_info "发现systemd服务文件: /etc/systemd/system/$SERVICE_NAME.service"
        found=true
    fi
    
    # 检查配置目录
    if [ -d "$CONFIG_DIR" ]; then
        print_info "发现配置目录: $CONFIG_DIR"
        found=true
    fi
    
    # 检查日志目录
    if [ -d "$LOG_DIR" ]; then
        print_info "发现日志目录: $LOG_DIR"
        found=true
    fi
    
    if [ "$found" = false ]; then
        print_warn "未发现NSPass Agent安装，脚本退出"
        exit 0
    fi
    
    print_info "NSPass Agent已安装，继续卸载..."
}

# 获取用户确认
get_user_confirmation() {
    echo ""
    echo "⚠️  警告: 即将卸载NSPass Agent"
    echo ""
    echo "此操作将："
    echo "  - 停止并禁用nspass-agent服务"
    echo "  - 删除二进制文件和systemd服务文件"
    echo "  - 可选择删除配置文件和数据"
    echo "  - 可选择清理相关的iptables规则"
    echo ""
    
    while true; do
        read -p "是否继续卸载？ [y/N]: " -n 1 -r
        echo ""
        case $REPLY in
            [Yy])
                break
                ;;
            [Nn]|"")
                print_info "用户取消操作"
                exit 0
                ;;
            *)
                echo "请输入 y 或 n"
                ;;
        esac
    done
}

# 停止并禁用服务
stop_and_disable_service() {
    print_step "停止并禁用nspass-agent服务..."
    
    local service_exists=false
    
    # 检查服务是否存在
    if systemctl list-unit-files "$SERVICE_NAME.service" --no-legend | grep -q "$SERVICE_NAME.service"; then
        service_exists=true
    fi
    
    if [ "$service_exists" = true ]; then
        # 停止服务
        if systemctl is-active --quiet $SERVICE_NAME 2>/dev/null; then
            print_info "停止服务..."
            systemctl stop $SERVICE_NAME
            print_info "服务已停止"
        else
            print_info "服务未运行"
        fi
        
        # 禁用服务
        if systemctl is-enabled --quiet $SERVICE_NAME 2>/dev/null; then
            print_info "禁用服务..."
            systemctl disable $SERVICE_NAME
            print_info "服务已禁用"
        else
            print_info "服务未启用"
        fi
    else
        print_warn "systemd服务不存在或已删除"
    fi
}

# 删除systemd服务文件
remove_systemd_service() {
    print_step "删除systemd服务文件..."
    
    local service_file="/etc/systemd/system/$SERVICE_NAME.service"
    
    if [ -f "$service_file" ]; then
        rm -f "$service_file"
        systemctl daemon-reload
        systemctl reset-failed 2>/dev/null || true
        print_info "systemd服务文件已删除"
    else
        print_warn "systemd服务文件不存在: $service_file"
    fi
}

# 删除二进制文件
remove_binary() {
    print_step "删除二进制文件..."
    
    local binary_file="$INSTALL_DIR/nspass-agent"
    
    if [ -f "$binary_file" ]; then
        rm -f "$binary_file"
        print_info "二进制文件已删除: $binary_file"
    else
        print_warn "二进制文件不存在: $binary_file"
    fi
}

# 询问并处理配置文件
handle_config_files() {
    local config_exists=false
    local log_exists=false
    
    # 检查配置目录
    if [ -d "$CONFIG_DIR" ]; then
        config_exists=true
    fi
    
    # 检查日志目录
    if [ -d "$LOG_DIR" ]; then
        log_exists=true
    fi
    
    if [ "$config_exists" = false ] && [ "$log_exists" = false ]; then
        print_info "配置和日志目录不存在，跳过"
        return
    fi
    
    echo ""
    print_step "处理配置文件和数据..."
    
    if [ "$config_exists" = true ]; then
        echo ""
        echo "配置目录: $CONFIG_DIR"
        echo "包含内容:"
        if [ -d "$CONFIG_DIR" ]; then
            ls -la "$CONFIG_DIR" 2>/dev/null | sed 's/^/  /' || echo "  (无法列出内容)"
        fi
    fi
    
    if [ "$log_exists" = true ]; then
        echo ""
        echo "日志目录: $LOG_DIR"
        echo "包含内容:"
        if [ -d "$LOG_DIR" ]; then
            ls -la "$LOG_DIR" 2>/dev/null | sed 's/^/  /' || echo "  (无法列出内容)"
        fi
    fi
    
    echo ""
    
    while true; do
        read -p "是否删除配置文件和日志数据？ [y/N]: " -n 1 -r
        echo ""
        case $REPLY in
            [Yy])
                print_info "删除配置文件和数据..."
                if [ "$config_exists" = true ]; then
                    rm -rf "$CONFIG_DIR"
                    print_info "配置目录已删除: $CONFIG_DIR"
                fi
                if [ "$log_exists" = true ]; then
                    rm -rf "$LOG_DIR"
                    print_info "日志目录已删除: $LOG_DIR"
                fi
                break
                ;;
            [Nn]|"")
                if [ "$config_exists" = true ]; then
                    print_info "保留配置文件: $CONFIG_DIR"
                fi
                if [ "$log_exists" = true ]; then
                    print_info "保留日志文件: $LOG_DIR"
                fi
                break
                ;;
            *)
                echo "请输入 y 或 n"
                ;;
        esac
    done
}

# 清理已安装的代理软件
cleanup_proxy_software() {
    echo ""
    print_step "检查代理软件..."
    
    local proxy_bin_dir="/usr/local/bin/proxy"
    local found_proxies=""
    
    # 检查代理程序目录
    if [ -d "$proxy_bin_dir" ]; then
        if [ -f "$proxy_bin_dir/go-shadowsocks2" ]; then
            found_proxies="$found_proxies go-shadowsocks2"
        fi
        
        if [ -f "$proxy_bin_dir/snell-server" ]; then
            found_proxies="$found_proxies snell-server"
        fi
        
        if [ -f "$proxy_bin_dir/trojan-go" ]; then
            found_proxies="$found_proxies trojan-go"
        fi
    fi
    
    # 检查系统路径中的旧版本代理软件
    if command -v ss-local >/dev/null 2>&1 || command -v ss-server >/dev/null 2>&1; then
        found_proxies="$found_proxies shadowsocks(系统)"
    fi
    
    if [ -f /usr/local/bin/trojan ] || command -v trojan >/dev/null 2>&1; then
        found_proxies="$found_proxies trojan(系统)"
    fi
    
    if [ -f /usr/local/bin/snell-server ] && [ ! -f "$proxy_bin_dir/snell-server" ]; then
        found_proxies="$found_proxies snell(系统)"
    fi
    
    if [ -z "$found_proxies" ]; then
        print_info "未发现相关代理软件"
        return
    fi
    
    echo ""
    echo "发现的代理软件:$found_proxies"
    echo ""
    
    while true; do
        read -p "是否删除这些代理软件？ [y/N]: " -n 1 -r
        echo ""
        case $REPLY in
            [Yy])
                print_info "清理代理软件..."
                
                # 清理代理程序目录
                if [ -d "$proxy_bin_dir" ]; then
                    print_info "删除代理程序目录: $proxy_bin_dir"
                    rm -rf "$proxy_bin_dir"
                fi
                
                # 清理系统路径中的旧版本
                if echo "$found_proxies" | grep -q "shadowsocks(系统)"; then
                    if command -v apt-get >/dev/null 2>&1; then
                        apt-get remove -y shadowsocks-libev 2>/dev/null || true
                        print_info "shadowsocks (apt) 已卸载"
                    elif command -v yum >/dev/null 2>&1; then
                        yum remove -y shadowsocks-libev 2>/dev/null || true
                        print_info "shadowsocks (yum) 已卸载"
                    elif command -v dnf >/dev/null 2>&1; then
                        dnf remove -y shadowsocks-libev 2>/dev/null || true
                        print_info "shadowsocks (dnf) 已卸载"
                    elif command -v pacman >/dev/null 2>&1; then
                        pacman -R --noconfirm shadowsocks-libev 2>/dev/null || true
                        print_info "shadowsocks (pacman) 已卸载"
                    fi
                fi
                
                # 清理系统路径中的 trojan
                if echo "$found_proxies" | grep -q "trojan(系统)"; then
                    rm -f /usr/local/bin/trojan 2>/dev/null || true
                    print_info "trojan (系统) 已删除"
                fi
                
                # 清理系统路径中的 snell
                if echo "$found_proxies" | grep -q "snell(系统)"; then
                    rm -f /usr/local/bin/snell-server 2>/dev/null || true
                    print_info "snell-server (系统) 已删除"
                fi
                
                break
                ;;
            [Nn]|"")
                print_info "保留已安装的代理软件"
                break
                ;;
            *)
                echo "请输入 y 或 n"
                ;;
        esac
    done
}

# 清理iptables规则
cleanup_iptables_rules() {
    echo ""
    print_step "检查iptables规则..."
    
    # 查找NSPass相关的链
    local nspass_chains=$(iptables -L -n 2>/dev/null | grep "Chain NSPASS" | awk '{print $2}' || true)
    
    if [ -z "$nspass_chains" ]; then
        print_info "未发现NSPass相关的iptables规则"
        return
    fi
    
    echo ""
    echo "发现的NSPass iptables链:"
    echo "$nspass_chains" | sed 's/^/  /'
    echo ""
    
    while true; do
        read -p "是否清理这些iptables规则？ [y/N]: " -n 1 -r
        echo ""
        case $REPLY in
            [Yy])
                print_info "清理iptables规则..."
                
                # 删除NSPass相关的链
                for chain in $nspass_chains; do
                    # 清空链
                    iptables -F "$chain" 2>/dev/null || true
                    # 删除链
                    iptables -X "$chain" 2>/dev/null || true
                    print_info "已删除链: $chain"
                done
                
                # 保存iptables规则（如果可能）
                if command -v iptables-save >/dev/null 2>&1; then
                    if [ -d /etc/iptables ]; then
                        iptables-save > /etc/iptables/rules.v4 2>/dev/null || true
                        print_info "iptables规则已保存"
                    elif [ -f /etc/sysconfig/iptables ]; then
                        iptables-save > /etc/sysconfig/iptables 2>/dev/null || true
                        print_info "iptables规则已保存"
                    fi
                fi
                
                print_info "iptables规则清理完成"
                break
                ;;
            [Nn]|"")
                print_info "保留iptables规则"
                break
                ;;
            *)
                echo "请输入 y 或 n"
                ;;
        esac
    done
}

# 检查残留文件和进程
check_residual_files() {
    print_step "检查残留文件和进程..."
    
    local found_residual=false
    
    # 检查相关进程
    local nspass_processes=$(ps aux | grep -v grep | grep nspass || true)
    if [ -n "$nspass_processes" ]; then
        print_warn "发现相关进程:"
        echo "$nspass_processes" | sed 's/^/  /'
        found_residual=true
    fi
    
    # 检查可能的残留文件
    local residual_paths=(
        "/tmp/nspass*"
        "/var/tmp/nspass*"
        "/run/nspass*"
        "/var/run/nspass*"
        "/var/cache/nspass*"
        "/home/*/nspass*"
        "/root/nspass*"
    )
    
    for path in "${residual_paths[@]}"; do
        if ls $path 2>/dev/null | head -1 >/dev/null; then
            print_warn "发现残留文件: $path"
            found_residual=true
        fi
    done
    
    # 检查cron作业
    if crontab -l 2>/dev/null | grep -q nspass; then
        print_warn "发现cron作业中包含nspass相关内容"
        found_residual=true
    fi
    
    if [ "$found_residual" = false ]; then
        print_info "未发现残留文件和进程"
    else
        echo ""
        print_warn "建议手动检查和清理上述残留项"
        echo ""
        echo "清理建议:"
        echo "  - 检查和停止相关进程"
        echo "  - 清理临时文件"
        echo "  - 检查cron作业"
        echo "  - 重启系统以确保清理完成"
    fi
}

# 显示卸载完成信息
show_uninstall_complete() {
    echo ""
    echo "======================================"
    print_info "NSPass Agent 卸载完成！"
    echo "======================================"
    echo ""
    echo "✅ 已完成的操作:"
    echo "   - 停止并禁用systemd服务"
    echo "   - 删除二进制文件和服务文件"
    echo "   - 根据您的选择处理了配置文件"
    echo "   - 根据您的选择清理了相关组件"
    echo ""
    echo "📝 后续建议:"
    echo "   - 检查系统日志确认清理完成"
    echo "   - 重启系统以确保所有变更生效"
    echo ""
    echo "🙏 感谢使用NSPass Agent！"
    echo "📚 更多信息: https://github.com/$GITHUB_REPO"
    echo ""
}

# 主卸载流程
main() {
    echo "======================================"
    echo "NSPass Agent 卸载脚本 v$SCRIPT_VERSION"
    echo "======================================"
    echo ""
    
    # 检查运行环境
    check_root
    check_installation
    
    # 获取用户确认
    get_user_confirmation
    
    # 执行卸载步骤
    stop_and_disable_service
    remove_systemd_service
    remove_binary
    
    # 可选的清理步骤
    handle_config_files
    cleanup_proxy_software
    cleanup_iptables_rules
    
    # 检查残留
    check_residual_files
    
    # 显示完成信息
    show_uninstall_complete
}

# 脚本入口
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi 