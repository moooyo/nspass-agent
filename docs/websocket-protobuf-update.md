# WebSocket Protocol Buffer格式更新说明

## 概述

根据最新的proto定义，NSPass WebSocket通信现已统一使用Protocol Buffer二进制格式进行数据交换，替代之前的JSON格式。

## 主要更改

### 1. 消息格式变更
- **之前**: 使用JSON格式和`websocket.TextMessage`
- **现在**: 使用Protocol Buffer二进制格式和`websocket.BinaryMessage`

### 2. 序列化方式变更
- **之前**: `protojson.Marshal()` / `protojson.Unmarshal()`
- **现在**: `proto.Marshal()` / `proto.Unmarshal()`

### 3. WebSocket消息类型变更
- **之前**: `websocket.TextMessage`
- **现在**: `websocket.BinaryMessage`

## 更新的文件

### `/pkg/websocket/client.go`
- 移除了`google.golang.org/protobuf/encoding/protojson`导入
- 修改了`readMessageLoop()`函数，使用Protocol Buffer二进制反序列化
- 修改了`sendMessage()`函数，使用Protocol Buffer二进制序列化
- 添加了详细的注释说明二进制格式的使用

### Proto文件更新
- 更新了所有proto文件中的通信协议说明
- 重新生成了所有proto代码以支持二进制通信

## Protocol Buffer二进制格式优势

与JSON格式相比，Protocol Buffer二进制格式具有以下优势：

### 性能优势
- **更快的序列化/反序列化速度**: 二进制格式比JSON快3-10倍
- **更小的消息体积**: 通常比JSON小20-50%
- **更低的CPU使用率**: 减少了字符串解析开销

### 数据完整性
- **类型安全**: 强类型检查，避免数据类型错误
- **向后兼容**: Protocol Buffer支持字段添加和删除
- **数据验证**: 自动验证必需字段和数据类型

## Protocol Buffer消息格式示例

### 消息结构
所有WebSocket消息都基于`WebSocketMessage`结构：

```go
type WebSocketMessage struct {
    Id            string                 // 消息ID
    Type          WebSocketMessageType   // 消息类型
    Timestamp     *timestamppb.Timestamp // 时间戳
    Payload       *anypb.Any            // 消息载荷
    CorrelationId string                // 关联ID
}
```

### 代码示例

#### 发送心跳消息
```go
// 创建心跳载荷
heartbeat := &model.HeartbeatMessage{
    AgentId:   "agent_001",
    Timestamp: timestamppb.Now(),
    Status:    "online",
    Metadata: map[string]string{
        "version": "1.0.0",
    },
}

// 转换为Any类型
payload, _ := anypb.New(heartbeat)

// 创建WebSocket消息
wsMessage := &model.WebSocketMessage{
    Id:        "msg_1234567890",
    Type:      model.WebSocketMessageType_WEBSOCKET_MESSAGE_TYPE_HEARTBEAT,
    Timestamp: timestamppb.Now(),
    Payload:   payload,
}

// 序列化为二进制数据
binaryData, _ := proto.Marshal(wsMessage)

// 发送二进制消息
conn.WriteMessage(websocket.BinaryMessage, binaryData)
```

#### 接收和解析消息
```go
// 读取二进制消息
msgType, messageData, err := conn.ReadMessage()
if msgType == websocket.BinaryMessage {
    // 反序列化WebSocket消息
    var wsMessage model.WebSocketMessage
    if err := proto.Unmarshal(messageData, &wsMessage); err == nil {
        // 处理消息
        switch wsMessage.Type {
        case model.WebSocketMessageType_WEBSOCKET_MESSAGE_TYPE_HEARTBEAT:
            // 处理心跳消息
        case model.WebSocketMessageType_WEBSOCKET_MESSAGE_TYPE_TASK:
            // 处理任务消息
        }
    }
}
```

## 兼容性说明

- 此更新是**破坏性变更**，与旧版本的JSON格式不兼容
- 需要同时更新服务端和客户端代码
- 建议在维护窗口期间进行更新

## 优势

1. **性能提升**: 二进制格式序列化速度更快，消息体积更小
2. **类型安全**: 强类型检查，避免数据类型错误
3. **向后兼容**: Protocol Buffer支持字段添加和删除
4. **内存效率**: 更低的内存使用和CPU开销
5. **网络效率**: 减少网络传输数据量

## 测试验证

可以通过以下方式验证Protocol Buffer格式的WebSocket通信：

1. 编译并运行Agent程序
2. 查看WebSocket连接日志，确认使用二进制消息格式
3. 监控消息体积和处理性能的改进

验证步骤:
```bash
# 编译项目
go build ./...

# 运行Agent（需要配置文件）
make run
```

## 注意事项

1. 确保在更新后重新生成proto代码: `make gen-proto`
2. 所有WebSocket连接现在使用BINARY消息类型
3. 时间戳使用google.protobuf.Timestamp格式
4. 错误处理已更新以支持二进制格式的错误信息
5. 调试时需要使用proto工具来查看消息内容
