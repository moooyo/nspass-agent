#!/bin/bash

# NSPass Agent 发布脚本
# 构建多平台二进制文件并准备发布
# 使用方法: ./scripts/release.sh [版本号]

set -e

# 默认版本
DEFAULT_VERSION="v1.0.0"
VERSION=${1:-$DEFAULT_VERSION}

# 项目信息
PROJECT_NAME="nspass-agent"
GITHUB_REPO="nspass/nspass-agent"

# 构建参数
BUILD_DIR="build"
DIST_DIR="dist"
RELEASE_DIR="release"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

# 检查环境
check_environment() {
    print_step "检查构建环境..."
    
    # 检查Go环境
    if ! command -v go >/dev/null 2>&1; then
        print_error "Go环境未安装"
        exit 1
    fi
    
    local go_version=$(go version)
    print_info "Go版本: $go_version"
    
    # 检查git
    if ! command -v git >/dev/null 2>&1; then
        print_error "Git未安装"
        exit 1
    fi
    
    # 检查Makefile
    if [ ! -f "Makefile" ]; then
        print_error "Makefile未找到"
        exit 1
    fi
    
    print_info "环境检查完成"
}

# 清理旧构建
clean_build() {
    print_step "清理旧构建..."
    
    rm -rf "$BUILD_DIR"
    rm -rf "$DIST_DIR"
    rm -rf "$RELEASE_DIR"
    
    # 使用Makefile清理
    make clean 2>/dev/null || true
    
    print_info "清理完成"
}

# 构建多平台二进制文件
build_binaries() {
    print_step "构建多平台二进制文件..."
    
    # 创建构建目录
    mkdir -p "$RELEASE_DIR"
    
    # 支持的平台 - 仅Linux
    declare -A platforms=(
        ["linux-amd64"]="linux:amd64"
        ["linux-arm64"]="linux:arm64"
        ["linux-arm"]="linux:arm"
    )
    
    # 获取版本信息
    local commit=$(git rev-parse --short HEAD)
    local build_time=$(date -u '+%Y-%m-%d_%H:%M:%S')
    
    print_info "构建版本: $VERSION"
    print_info "提交哈希: $commit"
    print_info "构建时间: $build_time"
    
    # 构建各平台版本
    for platform_name in "${!platforms[@]}"; do
        local platform_info=${platforms[$platform_name]}
        local goos=$(echo $platform_info | cut -d: -f1)
        local goarch=$(echo $platform_info | cut -d: -f2)
        
        print_info "构建 $platform_name ($goos/$goarch)..."
        
        # 设置输出文件名
        local output_name="$PROJECT_NAME-$platform_name"
        if [ "$goos" = "windows" ]; then
            output_name="$output_name.exe"
        fi
        
        # 构建二进制文件
        CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build \
            -ldflags "-w -s -X 'main.Version=$VERSION' -X 'main.Commit=$commit' -X 'main.BuildTime=$build_time'" \
            -o "$RELEASE_DIR/$output_name" \
            ./cmd/$PROJECT_NAME
        
        if [ $? -eq 0 ]; then
            print_info "✓ $platform_name 构建成功"
            
            # 创建压缩包
            cd "$RELEASE_DIR"
            if [ "$goos" = "windows" ]; then
                zip "$output_name.zip" "$output_name" >/dev/null 2>&1
            else
                tar -czf "$output_name.tar.gz" "$output_name" >/dev/null 2>&1
            fi
            cd - >/dev/null
            
            print_info "✓ $platform_name 压缩包创建成功"
        else
            print_error "✗ $platform_name 构建失败"
            exit 1
        fi
    done
    
    print_info "所有平台构建完成"
}

# 验证构建文件
verify_binaries() {
    print_step "验证构建文件..."
    
    cd "$RELEASE_DIR"
    
    # 检查每个二进制文件
    for file in *; do
        if [ -f "$file" ]; then
            local size=$(ls -lh "$file" | awk '{print $5}')
            print_info "文件: $file (大小: $size)"
            
            # 验证可执行文件
            if [[ "$file" == *.exe ]] || [[ "$file" != *.* ]]; then
                if [ -x "$file" ]; then
                    print_info "✓ $file 可执行"
                else
                    print_warn "✗ $file 不可执行"
                fi
            fi
        fi
    done
    
    cd - >/dev/null
    
    print_info "验证完成"
}

# 生成校验和
generate_checksums() {
    print_step "生成校验和..."
    
    cd "$RELEASE_DIR"
    
    # 生成SHA256校验和
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum * > SHA256SUMS
        print_info "✓ SHA256校验和已生成"
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 * > SHA256SUMS
        print_info "✓ SHA256校验和已生成"
    else
        print_warn "无法生成SHA256校验和"
    fi
    
    cd - >/dev/null
}

# 创建发布说明
create_release_notes() {
    print_step "创建发布说明..."
    
    cat > "$RELEASE_DIR/RELEASE_NOTES.md" << EOF
# NSPass Agent $VERSION

## 新特性
- 代理软件管理功能
- iptables规则管理
- 系统监控和日志记录
- 多平台支持

## 下载

### Linux
- AMD64: \`nspass-agent-linux-amd64.tar.gz\`
- ARM64: \`nspass-agent-linux-arm64.tar.gz\`
- ARM: \`nspass-agent-linux-arm.tar.gz\`

### macOS
- AMD64: \`nspass-agent-darwin-amd64.tar.gz\`
- ARM64: \`nspass-agent-darwin-arm64.tar.gz\`

### Windows
- AMD64: \`nspass-agent-windows-amd64.exe.zip\`

## 安装

### 自动安装 (Linux)
\`\`\`bash
curl -sSL https://raw.githubusercontent.com/$GITHUB_REPO/main/scripts/install.sh | bash
\`\`\`

### 手动安装
1. 下载对应平台的二进制文件
2. 解压到 \`/usr/local/bin\`
3. 创建配置文件 \`/etc/nspass/config.yaml\`
4. 启动服务

## 配置

参考 \`configs/config.yaml\` 示例配置。

## 更新日志

$(git log --oneline --since="1 week ago" | head -10)

## 校验和

请检查 \`SHA256SUMS\` 文件验证下载文件的完整性。

---

构建信息:
- 版本: $VERSION
- 提交: $(git rev-parse --short HEAD)
- 构建时间: $(date -u '+%Y-%m-%d %H:%M:%S UTC')
EOF
    
    print_info "发布说明已创建"
}

# 显示发布信息
show_release_info() {
    echo ""
    echo "======================================"
    print_info "NSPass Agent $VERSION 构建完成!"
    echo "======================================"
    echo ""
    print_info "发布文件位置: $RELEASE_DIR/"
    echo ""
    echo "📦 构建的文件:"
    ls -la "$RELEASE_DIR/" | grep -E '\.(tar\.gz|zip|exe)$' | awk '{print "   " $9 " (" $5 ")"}'
    echo ""
    echo "🔍 校验和文件:"
    if [ -f "$RELEASE_DIR/SHA256SUMS" ]; then
        echo "   SHA256SUMS"
    fi
    echo ""
    echo "📝 发布说明:"
    echo "   RELEASE_NOTES.md"
    echo ""
    echo "🚀 下一步:"
    echo "   1. 测试构建的二进制文件"
    echo "   2. 创建GitHub release"
    echo "   3. 上传构建文件"
    echo "   4. 发布release"
    echo ""
    echo "💡 快速测试:"
    echo "   ./scripts/test-install.sh"
    echo ""
}

# 主构建流程
main() {
    echo "======================================"
    echo "NSPass Agent 发布脚本"
    echo "======================================"
    echo ""
    
    if [ "$VERSION" = "$DEFAULT_VERSION" ]; then
        print_warn "使用默认版本: $VERSION"
        print_warn "建议指定版本: $0 v1.0.1"
        echo ""
    fi
    
    # 执行构建流程
    check_environment
    clean_build
    build_binaries
    verify_binaries
    generate_checksums
    create_release_notes
    show_release_info
}

# 脚本入口
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi
