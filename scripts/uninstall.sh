#!/bin/bash

# NSPass Agent 卸载脚本

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

# 停止并禁用服务
stop_service() {
    print_info "停止nspass-agent服务..."
    
    if systemctl is-active --quiet nspass-agent; then
        systemctl stop nspass-agent
        print_info "服务已停止"
    else
        print_warn "服务未运行"
    fi
    
    if systemctl is-enabled --quiet nspass-agent; then
        systemctl disable nspass-agent
        print_info "服务已禁用"
    else
        print_warn "服务未启用"
    fi
}

# 删除systemd服务文件
remove_systemd_service() {
    print_info "删除systemd服务文件..."
    
    if [ -f /etc/systemd/system/nspass-agent.service ]; then
        rm -f /etc/systemd/system/nspass-agent.service
        systemctl daemon-reload
        print_info "systemd服务文件已删除"
    else
        print_warn "systemd服务文件不存在"
    fi
}

# 删除二进制文件
remove_binary() {
    print_info "删除二进制文件..."
    
    if [ -f /usr/local/bin/nspass-agent ]; then
        rm -f /usr/local/bin/nspass-agent
        print_info "二进制文件已删除"
    else
        print_warn "二进制文件不存在"
    fi
}

# 询问是否删除配置文件
remove_config() {
    echo ""
    read -p "是否删除配置文件和数据？ [y/N]: " -n 1 -r
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        print_info "删除配置文件和数据..."
        
        if [ -d /etc/nspass ]; then
            rm -rf /etc/nspass
            print_info "配置目录已删除: /etc/nspass"
        fi
    else
        print_info "保留配置文件: /etc/nspass"
    fi
}

# 清理已安装的代理软件
cleanup_proxies() {
    echo ""
    read -p "是否卸载已安装的代理软件？ [y/N]: " -n 1 -r
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        print_info "清理代理软件..."
        
        # 清理shadowsocks
        if command -v ss-local >/dev/null 2>&1; then
            if command -v apt-get >/dev/null 2>&1; then
                apt-get remove -y shadowsocks-libev
            elif command -v yum >/dev/null 2>&1; then
                yum remove -y shadowsocks-libev
            elif command -v pacman >/dev/null 2>&1; then
                pacman -R --noconfirm shadowsocks-libev
            fi
            print_info "shadowsocks已卸载"
        fi
        
        # 清理trojan
        if [ -f /usr/local/bin/trojan ]; then
            rm -f /usr/local/bin/trojan
            print_info "trojan已删除"
        fi
        
        # 清理snell
        if [ -f /usr/local/bin/snell-server ]; then
            rm -f /usr/local/bin/snell-server
            print_info "snell-server已删除"
        fi
    else
        print_info "保留已安装的代理软件"
    fi
}

# 清理iptables规则
cleanup_iptables() {
    echo ""
    read -p "是否清理NSPass相关的iptables规则？ [y/N]: " -n 1 -r
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        print_info "清理iptables规则..."
        
        # 查找并删除NSPass相关的链
        CHAINS=$(iptables -L -n | grep "Chain NSPASS" | awk '{print $2}' || true)
        for chain in $CHAINS; do
            iptables -F "$chain" 2>/dev/null || true
            iptables -X "$chain" 2>/dev/null || true
            print_info "已删除链: $chain"
        done
        
        # 保存iptables规则
        if command -v iptables-save >/dev/null 2>&1; then
            iptables-save > /etc/iptables/rules.v4 2>/dev/null || true
        fi
        
        print_info "iptables规则清理完成"
    else
        print_info "保留iptables规则"
    fi
}

# 主卸载流程
main() {
    print_info "开始卸载NSPass Agent..."
    
    check_root
    
    stop_service
    remove_systemd_service
    remove_binary
    
    remove_config
    cleanup_proxies
    cleanup_iptables
    
    print_info "NSPass Agent卸载完成！"
    echo ""
    print_info "感谢使用NSPass Agent！"
}

# 运行主函数
main "$@" 