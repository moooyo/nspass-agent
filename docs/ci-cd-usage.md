# CI/CD 使用说明

## 概述

本项目配置了完整的 CI/CD 流程，包括自动化测试、构建和发布功能。

## Workflow 文件说明

### 1. 构建和测试 (`.github/workflows/build.yml`)

**触发条件：**
- 推送到 `main` 或 `develop` 分支
- 对 `main` 或 `develop` 分支的 Pull Request

**执行内容：**
- 运行单元测试
- 代码静态分析 (go vet)
- 代码风格检查 (golangci-lint)
- 测试多平台构建（Linux、macOS、Windows）

### 2. 发布版本 (`.github/workflows/release.yml`)

**触发条件：**
- 推送新的版本标签（格式：`v*`，如 `v1.0.0`、`v2.1.3`）

**执行内容：**
- 构建多平台二进制文件
- 生成压缩包
- 自动生成更新日志
- 创建 GitHub Release
- 上传二进制文件到 Release

## 支持的平台

构建的二进制文件支持以下平台：

- **Linux**: amd64, arm64, 386, arm
- **macOS**: amd64 (Intel), arm64 (Apple Silicon)  
- **Windows**: amd64, 386

## 如何发布新版本

### 1. 准备发布

确保你的代码已经合并到 `main` 分支，并且所有测试都通过。

### 2. 创建版本标签

```bash
# 创建并推送版本标签
git tag v1.0.0
git push origin v1.0.0
```

或者创建带说明的标签：

```bash
# 创建带说明的标签
git tag -a v1.0.0 -m "Release version 1.0.0"
git push origin v1.0.0
```

### 3. 监控构建过程

1. 推送标签后，GitHub Actions 会自动开始构建
2. 访问 GitHub 仓库的 "Actions" 页面查看构建进度
3. 构建完成后，在 "Releases" 页面查看新发布的版本

## 版本号规范

建议使用 [语义化版本](https://semver.org/) 规范：

- `v1.0.0` - 主版本.次版本.修订版本
- `v1.0.0-alpha.1` - 预发布版本
- `v1.0.0-beta.1` - 测试版本

## 发布内容

每个 Release 包含：

1. **二进制文件**: 各平台的可执行文件（压缩包格式）
2. **校验和文件**: `checksums.txt` 用于验证文件完整性
3. **更新日志**: 自动生成的变更记录
4. **安装说明**: 各平台的安装指南

## 使用发布的二进制文件

### Linux/macOS

```bash
# 下载对应平台的文件
wget https://github.com/your-username/nspass-agent/releases/download/v1.0.0/nspass-agent-linux-amd64.tar.gz

# 解压
tar -xzf nspass-agent-linux-amd64.tar.gz

# 移动到系统路径
sudo mv nspass-agent-linux-amd64 /usr/local/bin/nspass-agent
sudo chmod +x /usr/local/bin/nspass-agent
```

### Windows

1. 下载 `nspass-agent-windows-amd64.zip`
2. 解压文件
3. 将 `nspass-agent-windows-amd64.exe` 移动到 PATH 目录

## 校验文件完整性

```bash
# 下载校验和文件
wget https://github.com/your-username/nspass-agent/releases/download/v1.0.0/checksums.txt

# 验证文件
sha256sum -c checksums.txt
```

## 故障排除

### 构建失败

1. 检查 Go 版本兼容性
2. 确保所有依赖都在 `go.mod` 中正确声明
3. 检查代码是否通过 `go vet` 和测试

### 发布失败

1. 检查 GitHub Token 权限
2. 确保标签格式正确（以 `v` 开头）
3. 检查是否有权限创建 Release

### 跨平台构建问题

1. 确保代码不依赖特定平台的功能
2. 使用 `CGO_ENABLED=0` 避免 C 依赖
3. 处理平台特定的文件路径问题

## 本地测试

在推送标签前，可以本地测试构建：

```bash
# 测试多平台构建
make build-all

# 测试特定平台
GOOS=linux GOARCH=amd64 make build
GOOS=darwin GOARCH=arm64 make build
GOOS=windows GOARCH=amd64 make build
``` 