# NSPass Agent 安装和卸载脚本使用指南

本文档详细介绍了 NSPass Agent 的自动化安装和卸载脚本的使用方法、功能特性和故障排除。

## 目录

- [安装脚本](#安装脚本)
- [卸载脚本](#卸载脚本)
- [高级用法](#高级用法)
- [故障排除](#故障排除)
- [安全说明](#安全说明)

## 安装脚本

### 基本用法

#### 在线安装（推荐）

```bash
# 使用 curl
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash

# 使用 wget
wget -qO- https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash
```

#### 本地安装

```bash
# 下载脚本
wget https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh

# 运行脚本
chmod +x install.sh
sudo ./install.sh
```

### 安装流程

安装脚本会按以下顺序执行：

1. **环境检查**
   - 检查 root 权限
   - 检测系统架构和操作系统
   - 验证 systemd 支持

2. **版本管理**
   - 获取当前安装版本（如果存在）
   - 从 GitHub API 获取最新版本
   - 比较版本并决定是否需要更新

3. **依赖安装**
   - 检查必需的系统工具（wget、curl、tar）
   - 根据发行版自动安装缺失的依赖

4. **服务停止**
   - 如果服务正在运行，优雅停止服务

5. **二进制文件安装**
   - 从 GitHub Releases 下载对应架构的二进制文件
   - 验证下载文件的完整性
   - 安装到 `/usr/local/bin/nspass-agent`

6. **配置设置**
   - 创建配置目录结构
   - 生成默认配置文件（如果不存在）
   - 设置正确的权限

7. **systemd 服务配置**
   - 创建 systemd 服务文件
   - 启用开机自启动
   - 启动服务

8. **状态验证**
   - 检查服务运行状态
   - 验证开机自启配置
   - 显示详细的服务状态

### 支持的系统

#### 发行版支持

- **Debian 系**：Ubuntu、Debian
- **Red Hat 系**：CentOS、RHEL、Fedora、Rocky Linux、AlmaLinux
- **Arch 系**：Arch Linux、Manjaro
- **SUSE 系**：openSUSE

#### 架构支持

- **x86_64** (amd64)
- **ARM64** (aarch64)
- **ARMv7** (armv7l)
- **x86** (i386)

### 配置文件

默认配置文件 `/etc/nspass/config.yaml`：

```yaml
# NSPass Agent 配置文件

# API配置
api:
  base_url: "https://api.nspass.com"
  token: "your-api-token-here"  # 需要手动设置
  timeout: 30
  retry_count: 3

# 代理软件配置
proxy:
  bin_path: "/usr/local/bin"
  config_path: "/etc/nspass/proxy"
  enabled_types: ["shadowsocks", "trojan", "snell"]
  auto_start: true
  restart_on_fail: true

# iptables配置
iptables:
  enable: true
  backup_path: "/etc/nspass/iptables-backup"
  persistent_method: "iptables-save"
  chain_prefix: "NSPASS"

# 更新间隔（秒）
update_interval: 300

# 日志级别
log_level: "info"
```

## 卸载脚本

### 基本用法

#### 在线卸载

```bash
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/uninstall.sh | bash
```

#### 本地卸载

```bash
# 下载脚本
wget https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/uninstall.sh

# 运行脚本
chmod +x uninstall.sh
sudo ./uninstall.sh
```

### 卸载流程

卸载脚本会按以下顺序执行：

1. **环境检查**
   - 检查 root 权限
   - 检测已安装的组件

2. **用户确认**
   - 显示即将执行的操作
   - 要求用户确认继续

3. **服务清理**
   - 停止 nspass-agent 服务
   - 禁用开机自启动
   - 删除 systemd 服务文件

4. **文件清理**
   - 删除二进制文件
   - 可选删除配置文件和数据

5. **可选组件清理**
   - 询问是否卸载相关代理软件
   - 询问是否清理 iptables 规则

6. **残留检查**
   - 检查相关进程
   - 检查临时文件
   - 提供清理建议

### 交互式选项

卸载过程中会提供以下选择：

1. **配置文件处理**
   - 删除：完全移除所有配置和数据
   - 保留：保留配置文件供将来使用

2. **代理软件清理**
   - 检测并提示卸载 Shadowsocks、Trojan、Snell
   - 支持多种包管理器的自动卸载

3. **iptables 规则清理**
   - 检测 NSPass 相关的 iptables 链
   - 安全删除相关规则并保存配置

## 高级用法

### 环境变量

可以通过环境变量自定义安装行为：

```bash
# 指定安装版本
export NSPASS_VERSION="v1.2.3"
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash

# 指定安装目录
export INSTALL_DIR="/opt/nspass"
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash

# 跳过服务启动
export SKIP_SERVICE_START="true"
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash
```

### 离线安装

```bash
# 1. 下载所需文件
wget https://github.com/nspass/nspass-agent/releases/download/v1.0.0/nspass-agent-linux-amd64.tar.gz
wget https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh

# 2. 设置离线模式
export OFFLINE_MODE="true"
export LOCAL_PACKAGE="nspass-agent-linux-amd64.tar.gz"

# 3. 运行安装脚本
sudo bash install.sh
```

### 批量部署

```bash
#!/bin/bash
# 批量部署脚本示例

SERVERS=(
    "server1.example.com"
    "server2.example.com"
    "server3.example.com"
)

for server in "${SERVERS[@]}"; do
    echo "正在部署到 $server..."
    ssh root@$server 'curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash'
    
    # 复制配置文件
    scp config.yaml root@$server:/etc/nspass/config.yaml
    
    # 重启服务
    ssh root@$server 'systemctl restart nspass-agent'
done
```

## 故障排除

### 常见问题

#### 1. 网络连接问题

**症状**：无法下载文件或获取版本信息

**解决方案**：
```bash
# 检查网络连接
curl -I https://api.github.com
curl -I https://github.com

# 使用代理
export http_proxy=http://proxy.example.com:8080
export https_proxy=http://proxy.example.com:8080
```

#### 2. 权限问题

**症状**：Permission denied 错误

**解决方案**：
```bash
# 确保以 root 用户运行
sudo su -
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash
```

#### 3. systemd 不支持

**症状**：systemctl command not found

**解决方案**：
```bash
# 检查 systemd 支持
systemctl --version

# 对于不支持 systemd 的系统，需要手动管理服务
```

#### 4. 架构不支持

**症状**：不支持的架构错误

**解决方案**：
```bash
# 检查系统架构
uname -m

# 查看支持的架构列表
curl -s https://api.github.com/repos/nspass/nspass-agent/releases/latest | grep "browser_download_url"
```

#### 5. 版本比较失败

**症状**：版本比较错误

**解决方案**：
```bash
# 强制重新安装
export FORCE_INSTALL="true"
curl -sSL https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh | bash
```

### 调试模式

启用详细输出进行问题诊断：

```bash
# 下载脚本并以调试模式运行
wget https://raw.githubusercontent.com/nspass/nspass-agent/main/scripts/install.sh
chmod +x install.sh
bash -x install.sh
```

### 日志查看

```bash
# 查看服务状态
systemctl status nspass-agent

# 查看服务日志
journalctl -u nspass-agent -f

# 查看安装日志
less /var/log/nspass-install.log
```

## 安全说明

### 脚本安全

1. **源码审查**：建议在生产环境使用前审查脚本源码
2. **HTTPS 下载**：脚本使用 HTTPS 确保传输安全
3. **签名验证**：未来版本将支持脚本签名验证

### 权限管理

1. **最小权限原则**：脚本仅请求必要的系统权限
2. **配置文件权限**：自动设置适当的文件权限
3. **服务用户**：服务以 root 用户运行（因需要管理网络规则）

### 网络安全

1. **防火墙配置**：脚本会配置必要的 iptables 规则
2. **API 通信**：所有 API 通信使用 HTTPS 加密
3. **令牌管理**：API 令牌需要手动配置，不会自动生成

### 建议做法

1. **测试环境验证**：在生产环境部署前先在测试环境验证
2. **备份配置**：升级前备份重要配置文件
3. **监控服务**：配置适当的监控和告警机制
4. **定期更新**：定期检查并应用安全更新

## 总结

NSPass Agent 的安装和卸载脚本提供了完整的自动化部署解决方案，具有以下特点：

- **智能化**：自动检测环境和版本，智能决策更新策略
- **兼容性**：支持主流 Linux 发行版和硬件架构
- **安全性**：多重确认机制和权限控制
- **可靠性**：完善的错误处理和回滚机制
- **用户友好**：清晰的输出信息和操作指导

通过这些脚本，用户可以轻松地在各种环境中部署和管理 NSPass Agent，大大简化了运维工作。 