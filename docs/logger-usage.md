# NSPass Agent 日志系统使用说明

## 概述

NSPass Agent 采用基于 `logrus` 的统一日志管理系统，提供结构化日志、日志轮转、性能监控等企业级特性。

## 核心特性

### 1. 统一日志管理
- **组件隔离**: 每个组件(api、proxy、iptables等)都有独立的logger
- **结构化日志**: 支持JSON和文本两种格式
- **性能监控**: 内置性能指标和审计日志功能
- **状态追踪**: 自动记录组件状态变更

### 2. 灵活的输出配置
- **多种输出方式**: stdout、file、both
- **自动日志轮转**: 基于文件大小、时间、数量的轮转策略
- **日志压缩**: 自动压缩旧日志文件节省空间
- **目录管理**: 自动创建必要的日志目录

### 3. 开发者友好
- **调用者信息**: debug级别自动显示函数调用位置
- **错误增强**: 自动记录错误类型和上下文信息
- **便捷方法**: 提供丰富的辅助方法简化使用

## 配置选项

### logger 配置块

```yaml
logger:
  level: "info"           # 日志级别
  format: "json"          # 日志格式
  output: "both"          # 输出方式
  file: "/var/log/nspass/agent.log"  # 日志文件路径
  max_size: 100           # 单文件最大大小(MB)
  max_backups: 5          # 保留的旧文件数量
  max_age: 30             # 文件保留天数
  compress: true          # 是否压缩旧文件
```

### 配置说明

#### 日志级别 (level)
- `debug`: 显示所有日志，包括详细调试信息
- `info`: 显示一般信息、警告和错误（推荐生产环境）
- `warn`: 只显示警告和错误
- `error`: 只显示错误信息

#### 日志格式 (format)
- `json`: 结构化JSON格式，便于日志分析系统处理
- `text`: 人类可读的文本格式，便于直接查看

#### 输出方式 (output)
- `stdout`: 输出到标准输出（终端）
- `file`: 输出到文件
- `both`: 同时输出到终端和文件

## 环境配置建议

### 开发环境
```yaml
logger:
  level: "debug"
  format: "text"
  output: "stdout"
```

### 生产环境
```yaml
logger:
  level: "info"
  format: "json"
  output: "file"
  file: "/var/log/nspass/agent.log"
  max_size: 50
  max_backups: 10
  max_age: 7
  compress: true
```

### 高性能环境
```yaml
logger:
  level: "warn"
  format: "json"
  output: "file"
  file: "/var/log/nspass/agent.log"
  max_size: 20
  max_backups: 3
  max_age: 3
  compress: true
```

## 日志类型和示例

### 1. 启动日志
记录组件启动信息：
```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "info",
  "message": "组件启动",
  "component": "nspass-agent",
  "version": "1.0.0",
  "config": {...},
  "lifecycle": "startup"
}
```

### 2. 性能指标日志
自动记录操作性能：
```json
{
  "timestamp": "2024-01-15T10:30:05Z",
  "level": "info",
  "message": "性能指标",
  "operation": "iptables_update",
  "duration_ms": 1234,
  "rules_count": 15,
  "performance": true
}
```

### 3. 状态变更日志
跟踪组件状态变化：
```json
{
  "timestamp": "2024-01-15T10:30:10Z",
  "level": "info",
  "message": "状态变更",
  "component": "shadowsocks",
  "state_from": "stopped",
  "state_to": "running",
  "reason": "正常启动",
  "state_change": true
}
```

### 4. 错误日志
增强的错误信息：
```json
{
  "timestamp": "2024-01-15T10:30:15Z",
  "level": "error",
  "message": "API请求失败",
  "error": "connection timeout",
  "error_type": "*net.OpError",
  "component": "api",
  "url": "https://api.example.com",
  "retry_count": 3
}
```

### 5. 审计日志
记录重要操作：
```json
{
  "timestamp": "2024-01-15T10:30:20Z",
  "level": "info",
  "message": "审计日志",
  "action": "proxy_configuration_update",
  "user": "system",
  "proxy_id": "ss-001",
  "audit": true
}
```

## 组件专用Logger

### 获取组件Logger
```go
// 各组件的专用logger
apiLogger := logger.GetAPILogger()
proxyLogger := logger.GetProxyLogger()
iptablesLogger := logger.GetIPTablesLogger()
configLogger := logger.GetConfigLogger()
systemLogger := logger.GetSystemLogger()
```

### 自定义组件Logger
```go
customLogger := logger.GetComponentLogger("my-component")
customLogger.Info("自定义组件日志")
```

## 便捷方法

### 基础日志方法
```go
// 带字段的日志
logger.WithField("key", "value").Info("消息")
logger.WithFields(logrus.Fields{
    "field1": "value1",
    "field2": "value2",
}).Error("错误消息")

// 带错误的日志
logger.WithError(err).Error("操作失败")
```

### 专用日志方法
```go
// 性能日志
logger.LogPerformance("operation_name", duration, fields)

// 错误日志
logger.LogError(err, "描述", fields)

// 启动日志
logger.LogStartup("component", "version", config)

// 关闭日志
logger.LogShutdown("component", duration)

// 状态变更日志
logger.LogStateChange("component", "from", "to", "reason")

// 审计日志
logger.LogAudit("action", "user", fields)
```

## 日志文件管理

### 日志轮转策略
日志文件按以下条件自动轮转：
1. 文件大小达到 `max_size`
2. 文件数量超过 `max_backups`
3. 文件年龄超过 `max_age` 天

### 日志文件命名
- 当前日志: `agent.log`
- 轮转日志: `agent-2024-01-15T10-30-00.000.log`
- 压缩日志: `agent-2024-01-15T10-30-00.000.log.gz`

### 清理策略
系统会自动清理超过保留策略的日志文件，无需手动维护。

## 故障排查

### 常见问题

#### 1. 日志文件不存在
检查：
- 日志目录是否有写权限
- 磁盘空间是否充足
- 日志路径配置是否正确

#### 2. 日志级别不生效
检查：
- 配置文件语法是否正确
- 是否重启服务使配置生效
- 命令行参数是否覆盖配置文件

#### 3. 日志文件过大
调整配置：
- 减小 `max_size`
- 减少 `max_backups`
- 启用 `compress`

### 调试模式
启用debug级别查看详细信息：
```bash
nspass-agent --log-level=debug
```

或在配置文件中：
```yaml
logger:
  level: "debug"
```

## 性能考虑

### 日志性能影响
- JSON格式比文本格式稍慢，但便于分析
- 文件输出比stdout输出稍慢
- Debug级别会影响性能，生产环境建议使用info级别

### 优化建议
1. 生产环境使用info或warn级别
2. 启用日志压缩节省空间
3. 合理设置轮转策略避免单文件过大
4. 使用日志收集系统（如ELK）处理大量日志

## 与外部系统集成

### ELK Stack
JSON格式的日志可直接被Elasticsearch索引：
```yaml
logger:
  format: "json"
  output: "file"
```

### Prometheus监控
性能日志包含duration_ms字段，可用于Prometheus指标收集。

### 日志聚合
支持将日志发送到rsyslog、fluentd等日志聚合系统。

## 最佳实践

1. **环境区分**: 不同环境使用不同的日志配置
2. **敏感信息**: 避免在日志中记录密码、密钥等敏感信息
3. **结构化**: 优先使用结构化字段而非字符串拼接
4. **适度记录**: 平衡信息量和性能，避免过度日志
5. **定期检查**: 定期检查日志文件大小和磁盘使用情况 