# NSPass Agent 实现总结

## 🎯 已完成的功能

### 1. Proto接口定义 📋
- ✅ 在 `proto/api/agent/` 下创建了Agent服务的proto定义
- ✅ 定义了两个核心接口：
  - `GetServerConfig` - 获取服务器配置 (`GET /v1/agent/config/{server_id}`)
  - `ReportAgentStatus` - 上报Agent状态 (`POST /v1/agent/status`)

### 2. HTTP API客户端 🌐
- ✅ 扩展了现有的HTTP客户端支持新的Agent接口
- ✅ 实现了完整的数据转换和错误处理
- ✅ 支持重试机制和性能监控

### 3. 核心Agent服务 ⚙️
- ✅ 创建了 `pkg/agent/service.go` 实现核心业务逻辑
- ✅ 集成了API客户端、Proxy管理器和IPTables管理器
- ✅ 实现了自动配置更新和状态上报机制

### 4. 数据转换支持 🔄
- ✅ 实现了从Proto格式到内部API格式的转换
- ✅ 支持路由配置、出口配置和转发规则转换
- ✅ 自动处理代理服务和iptables规则生成

### 5. 系统监控功能 📊
- ✅ 集成了gopsutil库进行系统资源监控
- ✅ 自动收集CPU、内存、磁盘使用率
- ✅ 监控代理服务运行状态和网络地址

## 🔧 主要功能流程

### 配置获取和更新流程
1. **定期从后端获取配置** - Agent根据配置的 `server_id` 从后端API获取最新配置
2. **配置变化检测** - 通过配置哈希值检测是否有变化，避免无效更新
3. **Proxy服务更新** - 根据路由配置自动启动/停止/更新代理服务
4. **IPTables规则更新** - 根据转发规则自动配置iptables DNAT规则

### 状态上报流程
1. **系统信息收集** - 实时收集CPU、内存、磁盘使用率
2. **网络地址检测** - 自动获取当前的IPv4和IPv6地址
3. **服务状态监控** - 监控各个代理服务的运行状态
4. **定期状态上报** - 向后端上报完整的Agent运行状态

## 📁 文件结构

```
pkg/
├── agent/
│   └── service.go           # 核心Agent服务
├── api/
│   ├── client.go           # HTTP客户端（已扩展）
│   └── types.go            # 数据类型和转换函数
proto/api/agent/
└── agent_service.proto     # Agent服务Proto定义
configs/
└── agent-config.yaml       # 配置文件示例
```

## 🚀 使用方式

### 1. 配置文件
创建配置文件（参考 `configs/agent-config.yaml`）：
```yaml
server_id: "server-001"
api:
  base_url: "https://api.nspass.example.com"
  token: "your-api-token-here"
  timeout: 30
  retry_count: 3
update_interval: 300
```

### 2. 运行Agent
```bash
# 使用默认配置路径
./nspass-agent

# 指定配置文件
./nspass-agent -c /path/to/config.yaml

# 设置日志级别
./nspass-agent -l debug
```

### 3. 构建项目
```bash
# 清理并重新构建
make all

# 仅构建
make build

# 生成proto代码
make proto-gen
```

## 🔗 与后端的接口

### 获取服务器配置接口
```http
GET /v1/agent/config/{server_id}
Authorization: Bearer {token}
```

**响应数据包含：**
- 路由配置（代理服务配置）
- 出口配置（代理出口模式）
- 转发规则（iptables配置）
- 服务器元数据

### 上报Agent状态接口
```http
POST /v1/agent/status
Authorization: Bearer {token}
Content-Type: application/json

{
  "server_id": "server-001",
  "ipv4_address": "192.168.1.100",
  "ipv6_address": "2001:db8::1",
  "activity": {
    "active_connections": 5,
    "proxy_services": [...],
    "cpu_usage": 15.2,
    "memory_usage": 45.8,
    "disk_usage": 62.1
  }
}
```

## 🛡️ 安全特性

- ✅ **TLS支持** - 支持HTTPS通信和证书验证
- ✅ **Token认证** - 使用Bearer Token进行API认证
- ✅ **配置验证** - 启动时验证必要的配置项
- ✅ **错误恢复** - 网络异常时自动重试和恢复

## 📊 监控和日志

- ✅ **结构化日志** - 支持JSON格式日志输出
- ✅ **性能指标** - 记录API调用和配置更新的性能数据
- ✅ **状态监控** - 实时监控系统资源和服务状态
- ✅ **优雅关闭** - 接收信号时安全关闭所有服务

## 🔮 扩展性

Agent服务设计为模块化架构，支持轻松扩展：
- **新的代理协议** - 通过Proxy管理器添加新协议支持
- **自定义监控** - 扩展系统监控指标收集
- **插件架构** - 支持第三方插件集成
- **多平台支持** - 支持Linux、macOS等多个平台

---

## 🎉 总结

我们成功实现了一个完整的NSPass Agent服务，该服务能够：

1. **自动化配置管理** - 从后端拉取配置并自动应用到本地服务
2. **智能代理管理** - 根据配置自动启动和管理各种代理服务
3. **网络规则配置** - 自动生成和应用iptables转发规则
4. **实时监控上报** - 收集系统状态并定期上报给后端
5. **高可用性设计** - 支持错误恢复、优雅关闭和性能监控

整个系统通过HTTP API与后端通信，使用proto定义确保了接口的标准化和可扩展性。Agent可以作为独立服务运行，为nspass系统提供强大的边缘计算能力。 