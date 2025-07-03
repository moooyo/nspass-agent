# IPTables 高效管理

## 概述

NSPass Agent 使用基于 `iptables-save` 和 `iptables-restore` 的高效方式管理 iptables 规则，提供原子性操作、高性能和可靠的备份恢复机制。

## 核心优势

### 1. 高效性
- **原子操作**：一次性应用所有规则，避免中间状态
- **批量处理**：避免逐条执行命令的开销
- **更快速度**：特别是在规则数量较多时

### 2. 可靠性
- **事务性**：要么全部成功，要么全部失败
- **状态一致**：避免规则应用过程中的不一致状态
- **错误恢复**：失败时自动从备份恢复

### 3. 标准化
- **Linux 标准**：使用 Linux 系统标准的规则管理方式
- **兼容性好**：与 iptables-persistent 包兼容
- **易于调试**：生成标准格式的规则文件

## 工作原理

### 整体流程
```
API 获取配置 → 解析当前规则 → 生成新规则文件 → 原子性应用 → 备份保存
```

### 详细步骤

1. **规则获取**: 从 API 获取最新的防火墙规则配置
2. **状态解析**: 使用 `iptables-save` 获取当前系统规则
3. **规则合并**: 移除旧的管理规则，添加新的管理规则
4. **原子应用**: 使用 `iptables-restore` 一次性应用所有规则
5. **备份保存**: 自动备份当前规则并保存配置文件

### 规则标识

所有管理的规则都会添加特殊标记：
```bash
-A FORWARD -s 192.168.1.0/24 -j ACCEPT -m comment --comment "NSPass:rule001"
```

通过 `NSPass:` 前缀标识我们管理的规则。

## 配置方法

### 配置文件设置

```yaml
iptables:
  enable: true
  chain_prefix: "NSPASS_"
  backup_path: "/etc/nspass/iptables/backup"
```

### 配置说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| enable | 是否启用iptables管理 | false |
| chain_prefix | 自定义链前缀 | "NSPASS_" |
| backup_path | 备份文件保存路径 | "/etc/nspass/iptables/backup" |

## 生成的规则文件格式

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

## 备份和恢复

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

## 调试和监控

### 查看当前规则
```bash
# 查看所有管理的规则
sudo iptables-save | grep "NSPass:"

# 查看特定表的规则
sudo iptables -t filter -L -n --line-numbers | grep -A 5 "NSPASS_"
```

### 日志监控
程序使用结构化 JSON 日志，便于监控：
```json
{
  "level": "info",
  "msg": "iptables规则更新完成",
  "managed_rules": 15,
  "last_update": "2024-12-31T14:30:22Z",
  "time": "2024-12-31T14:30:22Z"
}
```

### 性能监控
```json
{
  "level": "info", 
  "msg": "新规则文件内容生成完成",
  "new_content_size": 2048,
  "rules_added": 10,
  "rules_removed": 3,
  "time": "2024-12-31T14:30:22Z"
}
```

## 故障排除

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

### 日志级别调整
可以通过配置调整日志级别：
```yaml
system:
  log_level: "debug"  # 查看详细调试信息
```

## 最佳实践

1. **生产环境部署**：确保充足的磁盘空间用于备份
2. **备份策略**：定期清理旧备份，保留最近几个版本
3. **监控告警**：监控规则应用失败的情况
4. **测试验证**：在测试环境验证规则后再应用到生产
5. **权限管理**：确保只有授权用户可以修改配置

## API 规则格式

通过 API 获取的规则格式示例：
```json
{
  "rules": [
    {
      "id": "rule001",
      "table": "filter",
      "chain": "FORWARD",
      "rule": "-s 192.168.1.0/24 -j ACCEPT",
      "action": "add",
      "enabled": true
    },
    {
      "id": "nat001", 
      "table": "nat",
      "chain": "POSTROUTING",
      "rule": "-s 192.168.0.0/16 -j MASQUERADE",
      "action": "add",
      "enabled": true
    }
  ]
}
```

## 总结

基于 `iptables-save/restore` 的设计提供了高效、可靠、标准化的 iptables 管理能力。这种方式特别适合：

- 规则数量较多的环境
- 对稳定性要求较高的生产环境  
- 需要频繁更新规则的场景
- 要求操作原子性的系统

通过这种方式，NSPass Agent 能够更好地管理复杂的网络策略，同时保证系统的稳定性和可靠性。 