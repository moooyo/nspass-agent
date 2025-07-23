# WebSocket JSON格式更新说明

## 概述

根据最新的proto定义，NSPass WebSocket通信现已统一使用JSON格式进行数据交换，替代之前的protobuf二进制格式。

## 主要更改

### 1. 消息格式变更
- **之前**: 使用protobuf二进制格式和`websocket.BinaryMessage`
- **现在**: 使用JSON格式和`websocket.TextMessage`

### 2. 序列化方式变更
- **之前**: `proto.Marshal()` / `proto.Unmarshal()`
- **现在**: `protojson.Marshal()` / `protojson.Unmarshal()`

### 3. WebSocket消息类型变更
- **之前**: `websocket.BinaryMessage`
- **现在**: `websocket.TextMessage`

## 更新的文件

### `/pkg/websocket/client.go`
- 更新了导入包，添加了`google.golang.org/protobuf/encoding/protojson`
- 修改了`readMessageLoop()`函数，使用JSON反序列化
- 修改了`sendMessage()`函数，使用JSON序列化
- 添加了详细的注释说明JSON格式的使用

### Proto文件更新
- 更新了所有proto文件中的`go_package`路径
- 重新生成了所有proto代码以使用正确的导入路径

## JSON消息格式示例

### 心跳消息
```json
{
  "id": "msg_1234567890",
  "type": "WEBSOCKET_MESSAGE_TYPE_HEARTBEAT",
  "timestamp": "2025-07-23T07:20:00Z",
  "payload": {
    "@type": "type.googleapis.com/nspass.model.v1.HeartbeatMessage",
    "agentId": "agent_001",
    "timestamp": "2025-07-23T07:20:00Z",
    "status": "online",
    "metadata": {
      "version": "1.0.0"
    }
  }
}
```

### 任务消息
```json
{
  "id": "msg_task_001",
  "type": "WEBSOCKET_MESSAGE_TYPE_TASK",
  "timestamp": "2025-07-23T07:20:00Z",
  "payload": {
    "@type": "type.googleapis.com/nspass.model.v1.TaskMessage",
    "taskId": "task_123",
    "taskType": "TASK_TYPE_CONFIG_UPDATE",
    "title": "更新代理配置",
    "description": "更新Shadowsocks代理配置",
    "parameters": {
      "@type": "type.googleapis.com/nspass.model.v1.ConfigUpdateTaskParams",
      "configType": "proxy",
      "configContent": "{\"type\": \"shadowsocks\", \"server\": \"example.com\"}",
      "restartRequired": true
    }
  }
}
```

## 兼容性说明

- 此更新是**破坏性变更**，与旧版本的二进制格式不兼容
- 需要同时更新服务端和客户端代码
- 建议在维护窗口期间进行更新

## 优势

1. **可读性**: JSON格式便于调试和日志记录
2. **兼容性**: 更好的跨语言支持
3. **标准化**: 符合Web标准的文本消息格式
4. **调试友好**: 可以直接查看和修改消息内容

## 测试验证

项目包含示例代码(`examples/websocket_json_example.go`)，演示了:
- 心跳消息的JSON序列化和反序列化
- 任务消息的创建和格式化
- 监控数据消息的处理

运行示例:
```bash
go run examples/websocket_json_example.go
```

## 注意事项

1. 确保在更新后重新生成proto代码: `make gen-proto`
2. 所有WebSocket连接现在使用TEXT消息类型
3. 时间戳使用RFC3339格式
4. 错误处理已更新以支持JSON格式的错误信息
