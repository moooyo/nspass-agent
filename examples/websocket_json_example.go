package main

import (
	"fmt"
	"log"

	"github.com/moooyo/nspass-proto/generated/model"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// 演示WebSocket JSON格式消息的创建和解析
func main() {
	fmt.Println("=== NSPass WebSocket JSON格式示例 ===")

	// 创建心跳消息
	heartbeatExample()
	fmt.Println()

	// 创建任务消息
	taskExample()
	fmt.Println()

	// 创建监控数据消息
	metricsExample()
}

// 心跳消息示例
func heartbeatExample() {
	fmt.Println("1. 心跳消息示例：")

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
	payload, err := anypb.New(heartbeat)
	if err != nil {
		log.Fatal("创建心跳载荷失败:", err)
	}

	// 创建WebSocket消息
	wsMessage := &model.WebSocketMessage{
		Id:        "msg_1234567890",
		Type:      model.WebSocketMessageType_WEBSOCKET_MESSAGE_TYPE_HEARTBEAT,
		Timestamp: timestamppb.Now(),
		Payload:   payload,
	}

	// 序列化为JSON
	jsonData, err := protojson.Marshal(wsMessage)
	if err != nil {
		log.Fatal("序列化JSON失败:", err)
	}

	fmt.Printf("JSON: %s\n", string(jsonData))

	// 从JSON反序列化
	var parsedMessage model.WebSocketMessage
	if err := protojson.Unmarshal(jsonData, &parsedMessage); err != nil {
		log.Fatal("反序列化JSON失败:", err)
	}

	fmt.Printf("解析成功 - 消息ID: %s, 类型: %s\n",
		parsedMessage.Id, parsedMessage.Type.String())
}

// 任务消息示例
func taskExample() {
	fmt.Println("2. 任务消息示例：")

	// 创建配置更新任务参数
	configParams := &model.ConfigUpdateTaskParams{
		ConfigType:      "proxy",
		ConfigContent:   `{"type": "shadowsocks", "server": "example.com"}`,
		RestartRequired: true,
	}

	paramsPayload, err := anypb.New(configParams)
	if err != nil {
		log.Fatal("创建任务参数失败:", err)
	}

	// 创建任务消息
	task := &model.TaskMessage{
		TaskId:      "task_123",
		TaskType:    model.TaskType_TASK_TYPE_CONFIG_UPDATE,
		Title:       "更新代理配置",
		Description: "更新Shadowsocks代理配置",
		Parameters:  paramsPayload,
		CreatedAt:   timestamppb.Now(),
		Priority:    5,
		MaxRetries:  3,
	}

	taskPayload, err := anypb.New(task)
	if err != nil {
		log.Fatal("创建任务载荷失败:", err)
	}

	// 创建WebSocket消息
	wsMessage := &model.WebSocketMessage{
		Id:        "msg_task_001",
		Type:      model.WebSocketMessageType_WEBSOCKET_MESSAGE_TYPE_TASK,
		Timestamp: timestamppb.Now(),
		Payload:   taskPayload,
	}

	// 序列化为JSON
	jsonData, err := protojson.Marshal(wsMessage)
	if err != nil {
		log.Fatal("序列化JSON失败:", err)
	}

	fmt.Printf("JSON长度: %d字节\n", len(jsonData))
	fmt.Printf("任务ID: task_123, 类型: CONFIG_UPDATE\n")
}

// 监控数据消息示例
func metricsExample() {
	fmt.Println("3. 监控数据消息示例：")

	// 创建系统监控数据
	systemMetrics := &model.SystemMetrics{
		CpuUsage:       45.2,
		MemoryUsage:    68.5,
		DiskUsage:      32.1,
		LoadAverage:    1.5,
		Uptime:         86400, // 1天
		ProcessCount:   156,
		MemoryTotal:    8589934592, // 8GB
		MemoryUsed:     5872025600, // 5.47GB
		TcpConnections: 42,
	}

	systemData, err := anypb.New(systemMetrics)
	if err != nil {
		log.Fatal("创建系统监控数据失败:", err)
	}

	// 创建监控消息
	metricsMsg := &model.MetricsMessage{
		AgentId:     "agent_001",
		MetricsType: model.MetricsType_METRICS_TYPE_SYSTEM,
		Timestamp:   timestamppb.Now(),
		Data:        systemData,
		Labels: map[string]string{
			"hostname": "server-001",
			"region":   "us-west-1",
		},
	}

	metricsPayload, err := anypb.New(metricsMsg)
	if err != nil {
		log.Fatal("创建监控载荷失败:", err)
	}

	// 创建WebSocket消息
	wsMessage := &model.WebSocketMessage{
		Id:        "msg_metrics_001",
		Type:      model.WebSocketMessageType_WEBSOCKET_MESSAGE_TYPE_METRICS,
		Timestamp: timestamppb.Now(),
		Payload:   metricsPayload,
	}

	// 序列化为JSON
	jsonData, err := protojson.Marshal(wsMessage)
	if err != nil {
		log.Fatal("序列化JSON失败:", err)
	}

	fmt.Printf("JSON长度: %d字节\n", len(jsonData))
	fmt.Printf("监控类型: SYSTEM, CPU使用率: %.1f%%\n", systemMetrics.CpuUsage)
}
