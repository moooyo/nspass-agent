#!/bin/bash

# NSPass Agent 安装脚本测试工具
# 此脚本仅用于测试检测功能，不会实际安装任何文件

set -e

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

echo "======================================"
echo "NSPass Agent 安装脚本功能测试"
echo "======================================"
echo ""

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
            ARCH="unknown"
            ;;
    esac
    print_info "检测到系统架构: $arch (映射为: $ARCH)"
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
        print_warn "无法检测操作系统信息"
        OS="unknown"
    fi
}

# 检查systemd支持
check_systemd() {
    if command -v systemctl >/dev/null 2>&1; then
        print_info "✓ systemd 支持: 可用"
        systemctl --version | head -1
    else
        print_warn "✗ systemd 支持: 不可用"
    fi
}

# 检查依赖工具
check_dependencies() {
    print_step "检查系统依赖..."
    
    local tools=("wget" "curl" "tar" "iptables")
    for tool in "${tools[@]}"; do
        if command -v "$tool" >/dev/null 2>&1; then
            local version=$("$tool" --version 2>/dev/null | head -1 || echo "版本未知")
            print_info "✓ $tool: 已安装 ($version)"
        else
            print_warn "✗ $tool: 未安装"
        fi
    done
}

# 检查网络连接
check_network() {
    print_step "检查网络连接..."
    
    # 检查GitHub API连通性
    if curl -s --connect-timeout 5 https://api.github.com >/dev/null 2>&1; then
        print_info "✓ GitHub API: 可访问"
    else
        print_warn "✗ GitHub API: 无法访问"
    fi
    
    # 检查GitHub网站连通性
    if curl -s --connect-timeout 5 https://github.com >/dev/null 2>&1; then
        print_info "✓ GitHub: 可访问"
    else
        print_warn "✗ GitHub: 无法访问"
    fi
}

# 模拟获取最新版本
simulate_version_check() {
    print_step "模拟版本检查..."
    
    # 尝试获取最新版本信息
    if command -v curl >/dev/null 2>&1; then
        local latest_version=$(curl -s "https://api.github.com/repos/nspass/nspass-agent/releases/latest" | grep '"tag_name"' | cut -d'"' -f4 2>/dev/null || echo "")
        if [ -n "$latest_version" ]; then
            print_info "GitHub 最新版本: $latest_version"
        else
            print_warn "无法获取版本信息（可能是网络问题或仓库不存在）"
        fi
    else
        print_warn "curl 不可用，无法检查版本"
    fi
}

# 检查现有安装
check_existing_installation() {
    print_step "检查现有安装..."
    
    # 检查二进制文件
    if [ -f "/usr/local/bin/nspass-agent" ]; then
        local version=$("/usr/local/bin/nspass-agent" --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
        print_info "发现已安装版本: $version"
    else
        print_info "未发现现有安装"
    fi
    
    # 检查systemd服务
    if [ -f "/etc/systemd/system/nspass-agent.service" ]; then
        print_info "发现systemd服务文件"
        if systemctl is-enabled nspass-agent >/dev/null 2>&1; then
            print_info "服务已启用"
        fi
        if systemctl is-active nspass-agent >/dev/null 2>&1; then
            print_info "服务正在运行"
        fi
    else
        print_info "未发现systemd服务文件"
    fi
    
    # 检查配置目录
    if [ -d "/etc/nspass" ]; then
        print_info "发现配置目录: /etc/nspass"
        if [ -f "/etc/nspass/config.yaml" ]; then
            print_info "发现配置文件: /etc/nspass/config.yaml"
        fi
    else
        print_info "未发现配置目录"
    fi
}

# 显示预期的安装路径
show_install_paths() {
    print_step "预期的安装路径..."
    
    echo "二进制文件: /usr/local/bin/nspass-agent"
    echo "配置目录: /etc/nspass/"
    echo "主配置文件: /etc/nspass/config.yaml"
    echo "代理配置目录: /etc/nspass/proxy/"
    echo "iptables备份目录: /etc/nspass/iptables-backup/"
    echo "systemd服务文件: /etc/systemd/system/nspass-agent.service"
}

# 主函数
main() {
    detect_arch
    detect_os
    echo ""
    
    check_systemd
    echo ""
    
    check_dependencies
    echo ""
    
    check_network
    echo ""
    
    simulate_version_check
    echo ""
    
    check_existing_installation
    echo ""
    
    show_install_paths
    echo ""
    
    echo "======================================"
    print_info "测试完成！"
    echo "======================================"
    echo ""
    echo "📝 总结:"
    echo "   - 系统架构: $ARCH"
    echo "   - 操作系统: $OS"
    echo "   - 如果所有检查都通过，安装脚本应该能正常工作"
    echo ""
    echo "💡 下一步:"
    echo "   - 如果测试结果正常，可以运行实际的安装脚本"
    echo "   - 如果有警告，请先解决相关问题"
    echo ""
}

# 运行主函数
main "$@" 