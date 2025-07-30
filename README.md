# NSPass Agent

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/moooyo/nspass-agent)](https://github.com/moooyo/nspass-agent)
[![Build Status](https://img.shields.io/github/actions/workflow/status/moooyo/nspass-agent/build.yml)](https://github.com/moooyo/nspass-agent/actions)
[![Release](https://img.shields.io/github/v/release/moooyo/nspass-agent)](https://github.com/moooyo/nspass-agent/releases)

NSPass Agent 是一个强大的代理服务管理工具，用于管理和监控各种代理服务（如 Shadowsocks、Trojan、Snell 等）。它提供了统一的接口来管理多种代理协议，并支持实时监控、流量统计、规则管理等功能。

## ✨ 核心特性

- 🔗 **多协议支持**: 支持 Shadowsocks、Trojan、Snell 等多种代理协议
- 📊 **实时监控**: WebSocket 连接实时收集和上报系统监控数据
- 🛡️ **防火墙管理**: 自动管理 iptables 规则，支持流量转发和过滤
- 📈 **流量统计**: 详细的流量统计和历史记录
- 🔄 **动态配置**: 支持远程配置更新，无需重启服务
- 🚀 **高性能**: 基于 Go 语言开发，支持高并发处理
- 📱 **REST API**: 提供完整的 REST API 接口
- 🔐 **安全认证**: 支持 Token 认证和 TLS 加密
- 🔧 **易于部署**: 单二进制文件，支持 systemd 服务管理

## 🚀 快速开始

### 一键安装（推荐）

**方式一：一行命令安装（推荐，最简单）**

```bash
# 一行命令完成安装 - 直接复制粘贴即可
curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh -o install.sh && chmod +x install.sh && sudo DEBUG_MODE=1 ./install.sh -sid your-server-id -token your-api-token -endpoint https://api.your-domain.com

# 实际使用示例
curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh -o install.sh && chmod +x install.sh && sudo DEBUG_MODE=1 ./install.sh -sid 1 -token kuZp5DDPFtoRNE532eYAo23Jf1AledS8 -endpoint https://agent.nspass.xforward.de
```

**方式二：分步执行（适合调试）**

```bash
# 分步执行，便于调试
curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh -o install.sh
chmod +x install.sh
sudo DEBUG_MODE=1 ./install.sh -sid your-server-id -token your-api-token -endpoint https://api.your-domain.com

# 或使用预设环境
sudo DEBUG_MODE=1 ./install.sh -sid your-server-id -token your-api-token -env production
```

**方式三：传统管道安装**

如果你的环境支持管道安装，也可以使用：

```bash
# 基础安装（安装后需要手动配置）
curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh | sudo bash

# 带参数安装
curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh | sudo bash -s -- -sid your-server-id -token your-api-token -env production
```

**参数说明：**
- `-sid`: 服务器唯一标识符（短格式）
- `-token`: API 访问令牌（短格式）
- `-endpoint`: API 基础地址（短格式，手动指定）
- `-env`: 预设环境名称（短格式：production|staging|testing|development）
- `-h`: 显示帮助信息

**预设环境：**
- `production`: https://api.nspass.com（生产环境）
- `staging`: https://staging-api.nspass.com（预发布环境）
- `testing`: https://test-api.nspass.com（测试环境）
- `development`: https://dev-api.nspass.com（开发环境）

> ⚠️ **重要提示**: 必须指定 `-endpoint` 或 `-env` 参数之一（或使用位置参数）。推荐使用 `-env` 参数选择预设环境。

**使用示例：**

| 格式 | 命令 | 说明 |
|------|------|------|
| **一行命令（推荐）** | `curl -sSL install-url -o install.sh && chmod +x install.sh && sudo DEBUG_MODE=1 ./install.sh -sid server001 -token abc123 -endpoint https://api.custom.com` | 最可靠，一次完成 |
| **分步执行** | `下载 -> chmod +x -> 执行` | 便于调试和验证 |
| **管道安装** | `curl -sSL install-url \| sudo bash -s -- ...` | 简洁但可能有问题 |

```bash
# 🔥 推荐：一行命令安装（直接复制使用）
curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh -o install.sh && chmod +x install.sh && sudo DEBUG_MODE=1 ./install.sh -sid 1 -token kuZp5DDPFtoRNE532eYAo23Jf1AledS8 -endpoint https://agent.nspass.xforward.de

# 使用预设环境
curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh -o install.sh && chmod +x install.sh && sudo DEBUG_MODE=1 ./install.sh -sid server001 -token abc123def456 -env production

# 查看帮助
curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh -o install.sh && chmod +x install.sh && ./install.sh -h
```

### 手动下载安装

如果您偏好手动安装，可以从 GitHub Releases 下载预编译的二进制文件：

```bash
# 1. 下载最新版本（以 Linux AMD64 为例）
curl -L https://github.com/moooyo/nspass-agent/releases/latest/download/nspass-agent-linux-amd64.tar.gz -o nspass-agent.tar.gz

# 2. 解压
tar -xzf nspass-agent.tar.gz

# 3. 安装到系统路径
sudo cp nspass-agent-linux-amd64 /usr/local/bin/nspass-agent
sudo chmod +x /usr/local/bin/nspass-agent

# 4. 创建配置目录
sudo mkdir -p /etc/nspass

# 5. 创建日志目录
sudo mkdir -p /var/log/nspass

# 6. 下载示例配置文件
sudo curl -L https://raw.githubusercontent.com/moooyo/nspass-agent/main/configs/config.yaml -o /etc/nspass/config.yaml
```

### 支持的系统架构

| 操作系统 | 架构 | 下载链接 |
|---------|------|----------|
| Linux | x86_64 (AMD64) | [下载](https://github.com/moooyo/nspass-agent/releases/latest/download/nspass-agent-linux-amd64.tar.gz) |
| Linux | ARM64 | [下载](https://github.com/moooyo/nspass-agent/releases/latest/download/nspass-agent-linux-arm64.tar.gz) |
| Linux | ARM | [下载](https://github.com/moooyo/nspass-agent/releases/latest/download/nspass-agent-linux-arm.tar.gz) |

## ⚙️ 配置

### 基本配置

编辑配置文件 `/etc/nspass/config.yaml`：

```yaml
# 服务器配置
server:
  id: "your-server-id"          # 服务器唯一标识
  
# API 配置
api:
  base_url: "https://api.nspass.com"  # 根据实际环境修改
  token: "your-api-token"
  timeout: 30s
  
# 代理配置
proxy:
  enabled_types: ["shadowsocks", "trojan", "snell"]
  port_range:
    start: 10000
    end: 65535
    
# 监控配置
monitor:
  interval: 30s
  enabled: true
  
# 日志配置
log:
  level: "info"
  file: "/var/log/nspass/agent.log"
  max_size: 100
  max_backups: 5
  max_age: 7
```

### 高级配置

查看 [配置文档](docs/installation.md) 了解更多配置选项。

## 🛠️ 服务管理

### 启动服务

```bash
# 启动服务
sudo systemctl start nspass-agent

# 开机自启
sudo systemctl enable nspass-agent

# 查看状态
sudo systemctl status nspass-agent
```

### 服务操作

```bash
# 重启服务
sudo systemctl restart nspass-agent

# 停止服务
sudo systemctl stop nspass-agent

# 查看日志
sudo tail -f /var/log/nspass/agent.log

# 查看历史日志
sudo less /var/log/nspass/agent.log
```

### 命令行使用

```bash
# 查看版本信息
nspass-agent version

# 检查配置文件
nspass-agent config check

# 以调试模式运行（前台）
nspass-agent run --log-level=debug

# 指定配置文件
nspass-agent run --config=/path/to/config.yaml
```

## 🔧 开发和构建

### 环境要求

- Go 1.24 或更高版本
- Protocol Buffers 编译器（protoc）
- Make 工具

### 从源码构建

```bash
# 1. 克隆仓库
git clone https://github.com/moooyo/nspass-agent.git
cd nspass-agent

# 2. 安装依赖
go mod download

# 3. 生成 protobuf 文件
make gen-proto

# 4. 构建
make build

# 5. 运行（开发模式）
make run
```

### 可用的 Make 命令

```bash
make build        # 构建二进制文件
make run          # 运行应用
make validate     # 验证配置和代码
make gen-proto    # 生成 protobuf 文件
make clean        # 清理构建文件
make lint         # 代码检查
make format       # 格式化代码
make release      # 构建发布版本
```

## 📚 API 文档

NSPass Agent 提供完整的 REST API 接口，支持：

- 代理服务管理
- 系统状态监控
- 流量统计查询
- 配置管理
- 健康检查

详细的 API 文档请参考：[API 文档](docs/)

## 🔐 安全特性

- **Token 认证**: 所有 API 调用都需要有效的认证令牌
- **TLS 加密**: 支持 HTTPS 和 WSS 加密通信
- **权限控制**: 基于角色的访问控制
- **审计日志**: 完整的操作审计日志记录
- **防火墙集成**: 自动管理防火墙规则

## 📊 监控功能

### 系统监控

- CPU 使用率
- 内存使用情况
- 磁盘空间使用
- 网络流量统计
- 进程状态监控

### 代理监控

- 连接数统计
- 流量使用情况
- 延迟监控
- 错误率统计
- 服务可用性

## 📖 文档

- [安装指南](docs/installation.md)
- [配置说明](docs/installation.md)
- [API 文档](docs/)
- [故障排除](docs/)
- [开发指南](docs/)

## 🤝 贡献

我们欢迎社区贡献！请阅读我们的贡献指南：

1. Fork 项目
2. 创建功能分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开 Pull Request

## 📄 许可证

本项目采用 MIT 许可证 - 详情请查看 [LICENSE](LICENSE) 文件。

## 💬 支持

如果您遇到任何问题或有功能建议，请通过以下方式联系我们：

- [GitHub Issues](https://github.com/moooyo/nspass-agent/issues)
- [GitHub Discussions](https://github.com/moooyo/nspass-agent/discussions)

## 🙏 致谢

感谢所有为这个项目做出贡献的开发者和用户！

---

**NSPass Agent** - 让代理服务管理变得简单高效
