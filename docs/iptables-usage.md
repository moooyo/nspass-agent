# NSPass Agent IPTables 完整管理指南

NSPass Agent 的 iptables 管理器实现了智能的规则对比、增量更新和持久化功能，能够安全、高效地管理防火墙规则。

## 🎯 核心特性

### 1. 高效的规则管理
- **内存配置维护**: 在内存中维护期望的规则配置状态
- **增量更新**: 只删除多余的规则，只添加缺失的规则
- **原子性操作**: 使用 iptables-save/restore 一次性应用所有规则
- **批量处理**: 避免逐条执行命令的开销

### 2. 智能配置对比
- 自动对比期望配置与当前系统配置
- 实时跟踪当前系统中的规则状态
- 管理自定义链的创建和删除
- 避免不必要的规则重建，提高性能

### 3. 可靠的备份机制
- 每次更新前自动备份当前规则
- 支持多种持久化方法
- 可配置的备份路径
- 事务性：要么全部成功，要么全部失败

### 4. 企业级日志系统
- 完整的操作日志，便于调试和审计
- 结构化日志格式（JSON）
- 不同级别的日志输出（debug, info, warn, error）

## 📝 配置选项

```yaml
iptables:
  enable: true                          # 是否启用iptables管理
  backup_path: "/etc/nspass/iptables-backup"  # 备份文件路径
  persistent_method: "iptables-save"    # 持久化方法
  chain_prefix: "NSPASS"                # 管理的链前缀
```

### 持久化方法
- `iptables-save`: 使用 iptables-save 命令保存规则（推荐）
- `netfilter-persistent`: 使用 netfilter-persistent 服务

## 🔄 工作流程

### 完整管理流程
```
API获取配置 → 解析当前规则 → 生成新规则文件 → 原子性应用 → 持久化保存 → 备份管理
```

### 详细步骤

1. **初始化阶段**
   - 启动时读取当前系统iptables配置
   - 解析并存储到内存

2. **更新流程**
   - API获取新配置
   - 重建期望配置
   - 重新读取当前配置
   - 对比差异
   - 应用变更
   - 持久化

3. **配置对比**
   - **需要删除**: 当前有但期望没有的规则
   - **需要添加**: 期望有但当前没有的规则
   - **保持不变**: 两边都有且相同的规则

### 规则标识

所有管理的规则都会添加特殊标记：
```bash
-A FORWARD -s 192.168.1.0/24 -j ACCEPT -m comment --comment "NSPass:rule001"
```

通过 `NSPass:` 前缀标识我们管理的规则。

## 📊 生成的规则文件格式

系统会生成标准的 iptables-save 格式文件：

```bash
# 保存位置: /etc/nspass/iptables/rules.v4
*filter
:INPUT ACCEPT [0:0]
:FORWARD ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
:NSPASS_FORWARD - [0:0]
-A FORWARD -j NSPASS_FORWARD
-A NSPASS_FORWARD -s 192.168.1.0/24 -j ACCEPT -m comment --comment "NSPass:rule001"
-A NSPASS_FORWARD -s 10.0.0.0/8 -j DROP -m comment --comment "NSPass:rule002"
COMMIT

*nat
:PREROUTING ACCEPT [0:0]
:INPUT ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
:POSTROUTING ACCEPT [0:0]
:NSPASS_NAT - [0:0]
-A POSTROUTING -j NSPASS_NAT
-A NSPASS_NAT -s 192.168.0.0/16 -j MASQUERADE -m comment --comment "NSPass:nat001"
COMMIT
```

## 📋 日志示例

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

### 性能监控
```json
{
  "level": "info", 
  "msg": "iptables规则更新完成",
  "managed_rules": 15,
  "rules_added": 10,
  "rules_removed": 3,
  "last_update": "2024-12-31T14:30:22Z"
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

## 🔧 备份和恢复

### 自动备份
每次更新规则前，系统会自动备份当前配置：
```bash
/etc/nspass/iptables/backup/iptables_backup_20241231_143022.rules
```

### 手动恢复
如果需要手动恢复，可以使用：
```bash
# 恢复到特定备份
sudo iptables-restore /etc/nspass/iptables/backup/iptables_backup_20241231_143022.rules

# 或者清空所有规则
sudo iptables -F
sudo iptables -t nat -F
sudo iptables -t mangle -F
sudo iptables -t raw -F
```

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

### 查看当前规则
```bash
# 查看所有管理的规则
sudo iptables-save | grep "NSPass:"

# 查看特定表的规则
sudo iptables -t filter -L -n --line-numbers | grep -A 5 "NSPASS_"
```

### 调试模式
设置日志级别为 `debug` 查看详细操作：

```yaml
logger:
  level: "debug"
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
- 原子性操作避免中间状态

### 4. 故障恢复
- 每次更新前创建备份
- 支持手动清空所有管理的规则
- 详细的错误日志便于问题排查
- 失败时自动从备份恢复

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

## 🔧 故障排除

### 常见问题

1. **规则应用失败**
   - 检查规则语法是否正确
   - 确认所需的 iptables 模块已加载
   - 查看详细错误日志

2. **备份文件过多**
   - 定期清理旧的备份文件
   - 可以设置自动清理策略

3. **权限问题**
   - 确保程序以 root 权限运行
   - 检查文件系统权限

### 最佳实践

1. **生产环境部署**：确保充足的磁盘空间用于备份
2. **备份策略**：定期清理旧备份，保留最近几个版本
3. **监控告警**：监控规则应用失败的情况
4. **测试验证**：在测试环境验证规则后再应用到生产
5. **权限管理**：确保只有授权用户可以修改配置

## 📈 优势总结

基于 `iptables-save/restore` 的设计提供了高效、可靠、标准化的 iptables 管理能力。这种方式特别适合：

- 规则数量较多的环境
- 对稳定性要求较高的生产环境  
- 需要频繁更新规则的场景
- 要求操作原子性的系统

通过这种智能的iptables管理方式，NSPass Agent能够安全、高效地管理防火墙规则，确保系统安全的同时提供灵活的配置能力。 