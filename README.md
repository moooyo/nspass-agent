# NSPass Agent

NSPass Agent 是一个高性能的Linux系统代理服务管理程序，支持多种代理协议、智能防火墙管理和企业级监控功能。

## ✨ 核心特性

### 🌐 **多协议代理支持**
- **Shadowsocks**: 基于shadowsocks-libev的高性能实现
- **Trojan**: 支持最新版本的Trojan代理协议
- **Snell**: 高速的Snell代理服务器支持
- **插件化架构**: 易于扩展支持更多代理协议

### 🔄 **智能进程监控** ⭐️ **NEW**
- **实时健康检查**: 定时检测代理进程运行状态
- **自动故障恢复**: 检测到进程崩溃时自动重启
- **智能重启策略**: 冷却期保护和重启次数限制
- **并发监控**: 支持同时监控多个代理进程
- **详细统计**: 重启历史、性能指标、状态分布
- **灵活配置**: 支持不同环境的差异化监控策略

### 🔧 **API驱动配置**
- **动态配置**: 从REST API自动获取和同步配置
- **智能重试**: 内置重试机制确保配置同步可靠性
- **版本管理**: 支持配置版本控制和回滚
- **实时更新**: 支持配置热更新无需重启服务

### 🛡️ **IPTables防火墙管理**
- **智能规则管理**: 基于iptables-save/restore的高效操作
- **原子性更新**: 确保规则更新的原子性和一致性
- **自动备份**: 自动备份和恢复防火墙规则
- **增量同步**: 智能对比和增量更新规则

### 📊 **企业级日志系统**
- **结构化日志**: 基于JSON格式的结构化日志输出
- **自动轮转**: 基于时间、大小的智能日志轮转
- **组件隔离**: 每个组件独立的日志命名空间
- **性能监控**: 内置性能指标和审计日志
- **多输出支持**: 同时输出到文件和控制台

### 🏗️ **生产就绪架构**
- **Systemd集成**: 完整的systemd服务文件和管理脚本
- **优雅关闭**: 支持信号处理和优雅关闭流程
- **健康检查**: 内置健康检查和状态监控接口
- **配置验证**: 启动时自动验证配置文件合法性
- **错误恢复**: 完善的错误处理和自动恢复机制

## 🎯 监控框架亮点

### 实时监控能力
```yaml
proxy:
  monitor:
    enable: true          # 启用监控
    check_interval: 30    # 30秒检查间隔
    restart_cooldown: 60  # 60秒重启冷却
    max_restarts: 10      # 每小时最多10次重启
    health_timeout: 5     # 5秒健康检查超时
```

### 智能故障恢复
- 🔍 **进程状态检测**: 定时检查所有代理进程健康状态
- ⚡ **快速故障恢复**: 检测到崩溃后立即自动重启
- 🛡️ **防护机制**: 冷却期和频率限制防止频繁重启
- 📈 **统计分析**: 详细的重启历史和性能统计

### 环境自适应配置
- **开发环境**: 快速检测(10s)，宽松重启策略(20次/小时)
- **生产环境**: 稳定优先(60s)，保守重启策略(5次/小时)  
- **高可用环境**: 平衡策略(15s)，适中重启限制(15次/小时)

## 📋 系统要求

- **操作系统**: Linux (Ubuntu 18.04+, CentOS 7+, Debian 9+)
- **Go版本**: Go 1.24+ (构建时)
- **系统权限**: root权限 (用于iptables和systemd操作)
- **依赖包**: iptables, systemctl
- **网络**: 能够访问配置API服务器

## 🚀 快速开始

### 1. 下载和安装
```bash
# 下载最新版本
wget https://github.com/nspass/nspass-agent/releases/latest/download/nspass-agent-linux-amd64.tar.gz

# 解压安装
tar -xzf nspass-agent-linux-amd64.tar.gz
sudo ./install.sh
```

### 2. 配置文件
创建配置文件 `/etc/nspass/config.yaml`:
```yaml
# API配置
api:
  base_url: "https://your-api-server.com"
  token: "your-api-token"

# 代理配置
proxy:
  enabled_types: ["shadowsocks", "trojan", "snell"]
  auto_start: true
  
  # 监控配置
  monitor:
    enable: true
    check_interval: 30
    max_restarts: 10

# 日志配置  
logger:
  level: "info"
  format: "json"
  output: "both"
  file: "/var/log/nspass/agent.log"
```

### 3. 启动服务
```bash
# 启动服务
sudo systemctl start nspass-agent
sudo systemctl enable nspass-agent

# 查看状态
sudo systemctl status nspass-agent

# 查看日志
sudo journalctl -u nspass-agent -f
```

## 📖 详细文档

- [日志系统使用指南](docs/logger-usage.md)
- [代理监控框架](docs/proxy-monitor.md)
- [IPTables管理说明](docs/iptables-persistent.md)
- [配置文件参考](configs/config-with-monitor.yaml)

## ⚙️ 配置示例

### 完整配置示例
参见: [configs/config-with-monitor.yaml](configs/config-with-monitor.yaml)

### 监控配置示例
```yaml
# 开发环境
proxy:
  monitor:
    enable: true
    check_interval: 10      # 快速检测
    restart_cooldown: 30    # 短冷却期
    max_restarts: 20        # 宽松策略

# 生产环境
proxy:
  monitor:
    enable: true
    check_interval: 60      # 稳定优先
    restart_cooldown: 120   # 保守策略
    max_restarts: 5         # 严格限制
```

## 🔧 构建和开发

### 本地构建
```bash
# 克隆仓库
git clone https://github.com/nspass/nspass-agent.git
cd nspass-agent

# 安装依赖
go mod tidy

# 构建
make build

# 运行测试
make test
```

### 开发环境运行
```bash
# 使用测试配置运行
go run cmd/nspass-agent/main.go -c test/test-monitor-config.yaml --log-level=debug
```

## 📊 监控和维护

### 监控指标
- 代理进程状态和运行时间
- 重启次数和成功率统计
- 健康检查耗时和超时次数
- 配置同步频率和错误率

### 故障排查
```bash
# 查看监控状态
curl localhost:8080/api/monitor/status

# 检查代理状态
sudo systemctl status nspass-agent

# 查看详细日志
sudo journalctl -u nspass-agent --no-pager -l
```

## 🤝 贡献指南

我们欢迎社区贡献！请查看 [CONTRIBUTING.md](CONTRIBUTING.md) 了解贡献流程。

### 开发规范
- 遵循Go代码规范
- 添加充分的单元测试
- 更新相关文档
- 提交前运行完整测试

## 📄 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

## 🆘 支持

- **文档**: [在线文档](https://nspass.github.io/nspass-agent/)
- **Issues**: [GitHub Issues](https://github.com/nspass/nspass-agent/issues)
- **讨论**: [GitHub Discussions](https://github.com/nspass/nspass-agent/discussions)

---

⭐ 如果这个项目对您有帮助，请给我们一个星标！