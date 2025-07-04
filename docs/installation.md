# NSPass Agent 安装和部署指南

本文档介绍如何安装、配置和部署NSPass Agent。

## 📦 快速安装

### 自动安装 (推荐)

在Linux系统上，使用以下命令自动安装最新版本：

#### 基础安装
```bash
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | sudo bash
```

#### 带配置参数安装 (推荐)
```bash
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | sudo bash -s -- --server-id=your-server-id --token=your-api-token
```

#### 参数说明
- `--server-id=<id>`: 服务器唯一标识符
- `--token=<token>`: API访问令牌
- `--help`: 显示帮助信息

#### 使用示例
```bash
# 使用具体的服务器ID和令牌
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | sudo bash -s -- --server-id=server001 --token=abc123def456

# 查看帮助信息
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash -s -- --help
```

### 手动安装

1. 从[GitHub Releases](https://github.com/nspass/nspass-agent/releases)下载对应平台的二进制文件
2. 解压并安装：

```bash
# 下载 (以Linux AMD64为例)
wget https://github.com/nspass/nspass-agent/releases/latest/download/nspass-agent-linux-amd64.tar.gz

# 解压
tar -xzf nspass-agent-linux-amd64.tar.gz

# 安装
sudo cp nspass-agent-linux-amd64 /usr/local/bin/nspass-agent
sudo chmod +x /usr/local/bin/nspass-agent
```

3. 创建配置文件和服务，参考下面的配置部分。

## ⚙️ 配置

### 自动配置 (推荐)

如果在安装时提供了 `server_id` 和 `token` 参数，配置文件会自动设置，无需手动编辑。

### 手动配置

如果需要修改配置或未在安装时提供参数，编辑配置文件 `/etc/nspass/config.yaml`：

```yaml
# 服务器ID（必须设置）
server_id: "your-server-id"

# API配置
api:
  base_url: "https://api.nspass.com"
  token: "your-api-token"
  timeout: 30

# 其他配置...
```

### 重要配置项

- `server_id`: 服务器唯一标识符 (必须)
- `api.token`: API访问令牌 (必须)
- `proxy.enabled_types`: 启用的代理类型
- `iptables.enable`: 是否启用iptables管理

### 配置验证

```bash
# 检查配置文件语法
nspass-agent --config /etc/nspass/config.yaml --check
```

## 🔧 服务管理

### 启动服务

```bash
# 启动并设置开机自启
sudo systemctl enable nspass-agent
sudo systemctl start nspass-agent

# 查看状态
sudo systemctl status nspass-agent
```

### 查看日志

```bash
# 查看系统日志
sudo journalctl -u nspass-agent -f

# 查看应用日志
sudo tail -f /var/log/nspass/agent.log
```

### 重启服务

```bash
sudo systemctl restart nspass-agent
```

## 🧪 开发和测试

### 本地测试安装

如果你在开发或测试环境中，可以使用测试安装脚本：

```bash
# 构建项目
make build

# 基础测试安装
sudo ./scripts/test-install.sh

# 带配置参数的测试安装
sudo ./scripts/test-install.sh --server-id=test-server-001 --token=test-token
```

### 构建发布版本

```bash
# 构建所有平台版本
make build-all

# 构建发布包
make release

# 发布到GitHub (需要GITHUB_TOKEN)
make release-github
```

## 🗑️ 卸载

### 自动卸载

```bash
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/uninstall.sh | sudo bash
```

### 手动卸载

```bash
# 停止和禁用服务
sudo systemctl stop nspass-agent
sudo systemctl disable nspass-agent

# 删除文件
sudo rm -f /usr/local/bin/nspass-agent
sudo rm -f /etc/systemd/system/nspass-agent.service
sudo rm -rf /etc/nspass
sudo rm -rf /var/log/nspass

# 重新加载systemd
sudo systemctl daemon-reload
```

## 📋 系统要求

### 支持的操作系统

- Ubuntu 18.04+
- Debian 10+
- CentOS 7+
- RHEL 7+
- Rocky Linux 8+
- AlmaLinux 8+
- Arch Linux
- openSUSE

### 支持的架构

- x86_64 (AMD64)
- ARM64 (AArch64)
- ARMv7
- i386

### 依赖

- systemd
- iptables (可选)
- curl 或 wget

## 🔒 安全考虑

### 权限

NSPass Agent需要以root权限运行，因为它需要：
- 管理系统服务
- 修改iptables规则
- 安装和配置代理软件

### 网络

确保以下网络连接可用：
- 到NSPass API服务器的连接
- 代理服务器的连接
- 必要的出站端口

### 配置文件安全

- 配置文件包含敏感信息，权限应设置为600
- 定期轮换API令牌
- 使用强密码和安全的服务器ID

## 🚨 故障排除

### 常见问题

1. **服务启动失败**
   ```bash
   # 检查配置文件
   nspass-agent --config /etc/nspass/config.yaml --check
   
   # 查看详细日志
   journalctl -u nspass-agent -n 50
   ```

2. **网络连接问题**
   ```bash
   # 测试API连接
   curl -v https://api.nspass.com/health
   
   # 检查DNS解析
   nslookup api.nspass.com
   ```

3. **权限问题**
   ```bash
   # 检查文件权限
   ls -la /etc/nspass/
   ls -la /usr/local/bin/nspass-agent
   ```

### 日志位置

- 系统日志: `journalctl -u nspass-agent`
- 应用日志: `/var/log/nspass/agent.log`
- 配置文件: `/etc/nspass/config.yaml`

## 📞 支持

如果遇到问题，请：

1. 查看本文档的故障排除部分
2. 检查[GitHub Issues](https://github.com/nspass/nspass-agent/issues)
3. 创建新的Issue，包含：
   - 操作系统信息
   - 错误日志
   - 配置文件（移除敏感信息）
   - 重现步骤

## 🤝 贡献

欢迎贡献代码！请参考：

1. Fork项目
2. 创建功能分支
3. 提交更改
4. 创建Pull Request

## 📜 许可证

本项目采用MIT许可证。详见[LICENSE](LICENSE)文件。
