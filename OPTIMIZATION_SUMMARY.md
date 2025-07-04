# NSPass Agent 安装脚本优化总结

## 🎯 优化目标

优化 NSPass Agent 的安装和卸载脚本，实现从 GitHub Releases 自动下载和安装最新版本的功能，并支持通过命令行参数直接配置服务器ID和API令牌。

## 🚀 主要改进

### 1. 安装脚本优化 (`scripts/install.sh`)

#### 新增功能：
- **命令行参数支持**: 支持 `--server-id` 和 `--token` 参数
- **自动配置写入**: 将参数直接写入配置文件，无需手动编辑
- **多架构支持**: 自动检测系统架构（amd64、arm64、arm、386）
- **增强的版本检查**: 支持从 GitHub API 获取最新版本，包含错误处理和回退机制
- **智能下载**: 支持多种下载格式（直接二进制、tar.gz压缩包）
- **完整的日志系统**: 添加日志目录创建和权限设置
- **改进的配置管理**: 生成更完整的默认配置文件
- **增强的错误处理**: 更好的错误信息和故障排除建议

#### 使用方法：
```bash
# 基础安装
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | sudo bash

# 带配置参数安装
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | sudo bash -s -- --server-id=server001 --token=your-token

# 查看帮助
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash -s -- --help
```

#### 关键改进：
```bash
# 参数解析
parse_args() {
    # 支持 --server-id 和 --token 参数
    # 包含参数验证和帮助信息
}

# 配置文件生成
setup_config() {
    # 自动使用提供的参数
    # 支持现有配置文件的更新
    # 智能配置备份和恢复
}
```

### 2. 卸载脚本优化 (`scripts/uninstall.sh`)

#### 新增功能：
- **日志目录清理**: 支持清理 `/var/log/nspass` 目录
- **增强的残留检查**: 检查更多可能的残留文件位置
- **改进的用户交互**: 更清晰的提示信息和选择流程
- **完善的清理建议**: 提供详细的手动清理建议

#### 关键改进：
```bash
# 配置文件处理增强
handle_config_files() {
    # 分别处理配置目录和日志目录
    # 改进的用户交互
}

# 残留检查增强
check_residual_files() {
    # 检查更多路径
    # 检查cron作业
    # 提供清理建议
}
```

### 3. 测试安装脚本优化 (`scripts/test-install.sh`)

#### 功能特点：
- **参数支持**: 同样支持 `--server-id` 和 `--token` 参数
- **本地构建安装**: 使用本地构建的二进制文件进行测试
- **自动构建**: 如果没有找到构建文件，自动执行构建
- **测试配置**: 生成适合测试的配置文件，支持参数自动设置
- **开发友好**: 提供详细的调试信息和测试命令

#### 使用方法：
```bash
# 基础测试安装
sudo ./scripts/test-install.sh

# 带配置参数的测试安装
sudo ./scripts/test-install.sh --server-id=test-server-001 --token=test-token
```

### 4. 新增发布脚本 (`scripts/release.sh`)

#### 功能特点：
- **多平台构建**: 支持 Linux、macOS、Windows 的多种架构
- **自动化打包**: 自动生成压缩包和校验和文件
- **发布说明**: 自动生成发布说明文档
- **环境检查**: 完整的构建环境检查

#### 支持的平台：
```bash
platforms=(
    ["linux-amd64"]="linux:amd64"
    ["linux-arm64"]="linux:arm64"
    ["linux-arm"]="linux:arm"
    ["darwin-amd64"]="darwin:amd64"
    ["darwin-arm64"]="darwin:arm64"
    ["windows-amd64"]="windows:amd64"
)
```

### 5. 构建系统优化

#### Makefile 增强：
- 新增 `release` 目标：执行发布构建
- 新增 `release-github` 目标：发布到 GitHub
- 优化帮助文档：更清晰的命令说明

#### GitHub Actions 工作流：
- 自动化构建和发布流程
- 支持标签触发和手动触发
- 生成完整的发布包

## 📁 文件结构

```
scripts/
├── install.sh          # 主安装脚本 (优化)
├── uninstall.sh        # 卸载脚本 (优化)
├── test-install.sh     # 测试安装脚本 (新增)
└── release.sh          # 发布脚本 (新增)

docs/
└── installation.md     # 安装指南 (新增)

.github/workflows/
└── release.yml         # GitHub Actions 工作流 (优化)
```

## 🎉 使用方法

### 生产环境安装
```bash
# 自动安装最新版本
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | sudo bash

# 带配置参数的安装（推荐）
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | sudo bash -s -- --server-id=your-server-id --token=your-token

# 卸载
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/uninstall.sh | sudo bash
```

### 开发环境测试
```bash
# 测试安装本地构建
sudo ./scripts/test-install.sh --server-id=test-server --token=test-token

# 构建发布版本
./scripts/release.sh v1.0.0
```

### 发布新版本
```bash
# 使用 Makefile
make release VERSION=v1.0.0

# 发布到 GitHub
make release-github VERSION=v1.0.0
```

## 🔧 配置改进

### 默认配置文件增强
- 添加完整的配置项说明
- 包含安全的默认值
- 支持测试和生产环境配置

### 权限和安全
- 正确的文件权限设置
- 安全的目录创建
- 敏感信息保护

## 🚨 错误处理增强

### 网络错误处理
- GitHub API 访问失败的回退机制
- 下载失败的重试和错误提示
- 网络连接检查

### 系统兼容性
- 多种 Linux 发行版支持
- 依赖包自动安装
- 架构检测和验证

## 📊 测试和验证

### 功能测试
- 安装脚本功能测试
- 卸载脚本清理验证
- 服务启动和状态检查

### 兼容性测试
- 多架构支持测试
- 不同操作系统测试
- 网络环境测试

## 🔄 持续改进

### 监控和日志
- 详细的安装日志
- 错误信息收集
- 性能监控

### 用户体验
- 清晰的提示信息
- 详细的帮助文档
- 故障排除指南

## 📝 总结

通过这次优化，NSPass Agent 的安装和部署体验得到了显著改善：

1. **自动化程度提高**: 从手动安装到一键自动安装，支持参数直接配置
2. **配置简化**: 支持通过命令行参数直接设置服务器ID和API令牌，无需手动编辑配置文件
3. **错误处理完善**: 更好的错误提示和恢复机制
4. **平台支持扩展**: 支持更多架构和操作系统
5. **开发体验改善**: 完善的测试和发布工具，支持参数化测试
6. **文档完善**: 详细的安装和使用指南

### 🔑 核心改进

- **一键配置**: `--server-id` 和 `--token` 参数让配置变得极其简单
- **自动写入**: 参数直接写入配置文件，即装即用
- **向后兼容**: 仍然支持传统的手动配置方式
- **智能处理**: 现有配置文件的智能更新和备份
- **开发友好**: 测试脚本同样支持参数配置

这些改进使得 NSPass Agent 更容易部署、维护和使用，特别是批量部署场景下的体验得到了极大提升。

## 7. GitHub Actions 权限问题排查与解决 (2024-01-XX)

### 问题描述
在自动化 release 过程中遇到 403 Forbidden 错误，主要表现为：
- `Resource not accessible by integration`
- `Request failed due to following response errors: * You must have push access to the repository to create releases`

### 解决方案

#### 7.1 权限配置优化
- **增强 workflow 权限**: 在 `.github/workflows/release.yml` 中添加更全面的权限配置
- **升级 action 版本**: 使用最新的 `softprops/action-gh-release@v2`
- **添加调试步骤**: 包含 GitHub API 访问测试和权限验证

#### 7.2 故障排除工具
- **诊断脚本**: 创建 `diagnose-github-permissions-v2.sh` 增强版诊断工具
- **调试 workflow**: 新增 `debug-permissions.yml` 专门用于权限调试
- **场景测试**: 创建 `test-release-scenarios.yml` 测试不同发布场景

#### 7.3 备用方案
- **API 直接调用**: 在 action 失败时使用 GitHub API 直接创建 release
- **PAT 支持**: 支持使用 Personal Access Token 替代 GITHUB_TOKEN
- **手动发布**: 提供完整的手动发布流程文档

### 技术细节

#### 权限配置
```yaml
permissions:
  contents: write       # 创建 releases 和 tags
  packages: write       # 发布包
  issues: write         # 创建 issues
  pull-requests: write  # 创建 PRs
  actions: write        # 管理 workflow runs
```

#### 故障排除流程
1. **自动诊断**: 运行诊断脚本检查配置
2. **权限测试**: 测试 GitHub API 访问权限
3. **场景验证**: 使用不同场景测试 release 创建
4. **备用方案**: 如果 action 失败，使用 API 直接创建

#### 新增文件
- `.github/workflows/debug-permissions.yml` - 权限调试 workflow
- `.github/workflows/test-release-scenarios.yml` - 场景测试 workflow
- `scripts/diagnose-github-permissions-v2.sh` - 增强版诊断脚本
- `docs/troubleshooting-release-403.md` - 故障排除指南

### 优化效果
- **提高成功率**: 多层次的权限验证和备用方案
- **快速诊断**: 自动化诊断工具快速定位问题
- **详细文档**: 完整的故障排除指南和解决方案
- **灵活配置**: 支持 GITHUB_TOKEN 和 PAT 两种认证方式

### 使用方法
```bash
# 运行诊断
./scripts/diagnose-github-permissions-v2.sh

# 测试权限
gh workflow run debug-permissions.yml

# 测试场景
gh workflow run test-release-scenarios.yml -f scenario=basic

# 手动发布
gh release create v1.0.0 --title "Release v1.0.0" --notes-file release/RELEASE_NOTES.md
```
