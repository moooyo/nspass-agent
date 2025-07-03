# IPTables 管理器使用说明

NSPass Agent 的 iptables 管理器实现了智能的规则对比和增量更新功能，能够安全、高效地管理防火墙规则。

## 🎯 核心特性

### 1. 内存配置维护
- 在内存中维护期望的规则配置状态
- 实时跟踪当前系统中的规则状态
- 管理自定义链的创建和删除

### 2. 配置对比与增量更新
- 自动对比期望配置与当前系统配置
- 只删除多余的规则，只添加缺失的规则
- 避免不必要的规则重建，提高性能

### 3. 详细日志记录
- 完整的操作日志，便于调试和审计
- 结构化日志格式（JSON）
- 不同级别的日志输出（debug, info, warn, error）

### 4. 安全的备份机制
- 每次更新前自动备份当前规则
- 支持多种持久化方法
- 可配置的备份路径

## 📝 配置选项

```yaml
iptables:
  enable: true                          # 是否启用iptables管理
  backup_path: "/etc/nspass/iptables-backup"  # 备份文件路径
  persistent_method: "iptables-save"    # 持久化方法
  chain_prefix: "NSPASS"                # 管理的链前缀
```

### 持久化方法
- `iptables-save`: 使用 iptables-save 命令保存规则
- `netfilter-persistent`: 使用 netfilter-persistent 服务

## 🔄 工作流程

### 1. 初始化阶段
```
启动时读取当前系统iptables配置 → 解析并存储到内存
```

### 2. 更新流程
```
API获取新配置 → 重建期望配置 → 重新读取当前配置 → 对比差异 → 应用变更 → 持久化
```

### 3. 配置对比
- **需要删除**: 当前有但期望没有的规则
- **需要添加**: 期望有但当前没有的规则
- **保持不变**: 两边都有且相同的规则

## 📊 日志示例

### 启动时日志
```json
{
  "timestamp": "2025-01-01T10:00:00Z",
  "level": "info",
  "message": "开始加载当前系统iptables配置"
}
```

### 配置对比日志
```json
{
  "timestamp": "2025-01-01T10:00:05Z",
  "level": "info", 
  "message": "配置差异分析完成",
  "rules_to_delete": 2,
  "rules_to_add": 3,
  "current_rules": 10,
  "desired_rules": 11
}
```

### 规则操作日志
```json
{
  "timestamp": "2025-01-01T10:00:06Z",
  "level": "info",
  "message": "成功添加规则",
  "rule_id": "rule-123",
  "table": "filter",
  "chain": "INPUT",
  "rule": "-p tcp --dport 80 -j ACCEPT"
}
```

## 🛠️ API 接口

### 规则配置格式
```json
{
  "iptables_rules": [
    {
      "id": "rule-1",
      "table": "filter",
      "chain": "INPUT", 
      "rule": "-p tcp --dport 22 -j ACCEPT",
      "action": "add",
      "enabled": true
    }
  ]
}
```

### 支持的表类型
- `filter`: 数据包过滤（默认表）
- `nat`: 网络地址转换
- `mangle`: 数据包修改
- `raw`: 原始数据包处理

### 支持的链类型
- `INPUT`: 进入本机的数据包
- `OUTPUT`: 本机发出的数据包
- `FORWARD`: 转发的数据包
- `PREROUTING`: 路由前处理
- `POSTROUTING`: 路由后处理

## 🔍 监控和调试

### 获取规则摘要
管理器提供 `GetRulesSummary()` 方法获取当前状态：

```json
{
  "desired_rules_count": 15,
  "current_rules_count": 12,
  "managed_chains": 3,
  "enabled": true,
  "chain_prefix": "NSPASS",
  "rules_by_table": {
    "filter": 10,
    "nat": 3,
    "mangle": 2
  }
}
```

### 调试模式
设置日志级别为 `debug` 查看详细操作：

```yaml
log_level: "debug"
```

这将显示：
- 执行的每个 iptables 命令
- 规则解析过程
- 配置对比细节

## ⚠️ 注意事项

### 1. 权限要求
- 必须以 root 权限运行
- 需要 iptables 命令可用

### 2. 安全考虑
- 只管理带有指定前缀的规则和链
- 不会影响系统默认的防火墙规则
- 自动备份机制防止配置丢失

### 3. 性能优化
- 使用规则键值缓存避免重复操作
- 批量处理规则变更
- 智能的差异检测算法

### 4. 故障恢复
- 每次更新前创建备份
- 支持手动清空所有管理的规则
- 详细的错误日志便于问题排查

## 🚀 使用示例

### 基本使用
```bash
# 启动服务（debug模式）
./nspass-agent --config /etc/nspass/config.yaml --log-level debug

# 查看日志
journalctl -u nspass-agent -f
```

### 手动测试
```bash
# 查看当前iptables规则
iptables -L -n --line-numbers

# 查看备份文件
ls -la /etc/nspass/iptables-backup/
```

通过这种智能的iptables管理方式，NSPass Agent能够安全、高效地管理防火墙规则，确保系统安全的同时提供灵活的配置能力。 