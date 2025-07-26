package websocket

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/iptables"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/nspass/nspass-agent/pkg/proxy"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// WebSocket通信协议说明：
// 根据proto定义，NSPass WebSocket通信统一使用Protocol Buffer二进制格式进行数据交换
// - 所有消息使用websocket.BinaryMessage类型发送
// - 消息内容为二进制格式的WebSocketMessage结构
// - 使用proto.Marshal和proto.Unmarshal进行序列化和反序列化
// - 支持重连机制和错误处理

// Client WebSocket客户端
type Client struct {
	config  *config.Config
	agentID string
	token   string

	conn   *websocket.Conn
	connMu sync.RWMutex

	// 控制相关
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 消息处理相关
	taskHandler   TaskHandler
	messageBuffer chan *model.WebSocketMessage

	// 状态相关
	connected      bool
	lastHeartbeat  time.Time
	reconnectDelay time.Duration

	// 重连相关
	shouldReconnect bool
	reconnectChan   chan struct{}

	// 监控数据收集器
	metricsCollector MetricsCollector

	// iptables管理器
	iptablesManager iptables.ManagerInterface

	// 代理管理器
	proxyManager *proxy.Manager

	log *logrus.Entry
}

// TaskHandler 任务处理器接口
type TaskHandler interface {
	HandleTask(ctx context.Context, task *model.TaskMessage) (*model.TaskResult, error)
	CheckTaskStatus(taskID string, taskType model.TaskType) (shouldExecute bool, existingResult *model.TaskResult)
	GetTaskStats() map[string]int
}

// MetricsCollector 监控数据收集器接口
type MetricsCollector interface {
	CollectSystemMetrics() (*model.SystemMetrics, error)
	CollectTrafficMetrics() (*model.TrafficMetrics, error)
	CollectConnectionMetrics() (*model.ConnectionMetrics, error)
	CollectPerformanceMetrics() (*model.PerformanceMetrics, error)
	CollectErrorMetrics() (*model.ErrorMetrics, error)
}

// NewClient 创建新的WebSocket客户端
func NewClient(cfg *config.Config, agentID, token string, taskHandler TaskHandler, metricsCollector MetricsCollector, iptablesManager iptables.ManagerInterface, proxyManager *proxy.Manager) *Client {
	ctx, cancel := context.WithCancel(context.Background())

	return &Client{
		config:           cfg,
		agentID:          agentID,
		token:            token,
		ctx:              ctx,
		cancel:           cancel,
		taskHandler:      taskHandler,
		metricsCollector: metricsCollector,
		iptablesManager:  iptablesManager,
		proxyManager:     proxyManager,
		messageBuffer:    make(chan *model.WebSocketMessage, 100),
		reconnectDelay:   10 * time.Second, // 设置为10秒重连延迟
		shouldReconnect:  true,
		reconnectChan:    make(chan struct{}, 1),
		log:              logger.GetComponentLogger("websocket-client"),
	}
}

// Start 启动WebSocket客户端
func (c *Client) Start() error {
	c.log.Info("启动WebSocket客户端")

	// 启动重连循环协程
	c.wg.Add(1)
	go c.reconnectLoop()

	// 启动读取消息的协程
	c.wg.Add(1)
	go c.readMessageLoop()

	// 启动消息处理协程
	c.wg.Add(1)
	go c.messageProcessLoop()

	// 启动心跳协程
	c.wg.Add(1)
	go c.heartbeatLoop()

	// 启动监控数据上报协程
	c.wg.Add(1)
	go c.metricsReportLoop()

	// 触发初始连接
	c.triggerReconnect()

	return nil
}

// Stop 停止WebSocket客户端
func (c *Client) Stop() error {
	c.log.Info("停止WebSocket客户端")

	// 停止重连
	c.shouldReconnect = false

	c.cancel()
	c.wg.Wait()

	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connMu.Unlock()

	close(c.messageBuffer)
	close(c.reconnectChan)

	c.log.Info("WebSocket客户端已停止")
	return nil
}

// reconnectLoop 重连循环，负责处理连接状态和重连逻辑
func (c *Client) reconnectLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			c.log.Info("重连循环退出")
			return
		case <-c.reconnectChan:
			if !c.shouldReconnect {
				c.log.Info("重连已禁用，跳过重连")
				continue
			}

			if c.isConnected() {
				c.log.Debug("连接正常，无需重连")
				continue
			}

			c.log.WithField("delay", c.reconnectDelay).Info("开始重连")

			// 等待重连延迟
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(c.reconnectDelay):
				// 继续重连
			}

			// 尝试重连
			if err := c.connect(); err != nil {
				c.log.WithError(err).Error("重连失败，将再次尝试")
				// 重连失败，再次触发重连
				c.triggerReconnect()
			} else {
				c.log.Info("重连成功")
			}
		}
	}
}

// triggerReconnect 触发重连
func (c *Client) triggerReconnect() {
	if !c.shouldReconnect {
		return
	}

	select {
	case c.reconnectChan <- struct{}{}:
		c.log.Debug("触发重连信号")
	default:
		c.log.Debug("重连信号已在队列中，跳过")
	}
}

// connect 建立WebSocket连接
func (c *Client) connect() error {
	c.log.Info("正在建立WebSocket连接")

	// 构建WebSocket URL
	wsURL, err := c.buildWebSocketURL(c.agentID)
	if err != nil {
		return fmt.Errorf("构建WebSocket URL失败: %w", err)
	}

	// 创建dialer
	dialer := websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: c.config.API.TLSSkipVerify,
	}

	// 设置请求头
	headers := make(map[string][]string)
	headers["Server-ID"] = []string{c.agentID}
	headers["Agent-Token"] = []string{c.token}
	headers["User-Agent"] = []string{"nspass-agent/1.0"}

	// 建立连接
	conn, response, err := dialer.Dial(wsURL, headers)
	if err != nil {
		return fmt.Errorf("建立WebSocket连接失败: %w", err)
	}

	defer response.Body.Close()

	if response.StatusCode == 401 {
		return fmt.Errorf("认证失败，状态码: %d", response.StatusCode)
	}

	if response.StatusCode != 101 {
		return fmt.Errorf("WebSocket连接失败,状态码: %d", response.StatusCode)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connected = true
	c.connMu.Unlock()

	c.log.Info("WebSocket连接建立成功")

	return nil
}

// buildWebSocketURL 构建WebSocket URL
func (c *Client) buildWebSocketURL(agentID string) (string, error) {
	u, err := url.Parse(c.config.API.BaseURL)
	if err != nil {
		return "", fmt.Errorf("解析API基础URL失败: %w", err)
	}

	if agentID == "" {
		return "", fmt.Errorf("agentID不能为空")
	}

	// 转换为WebSocket协议
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}

	// 添加WebSocket路径
	u.Path = fmt.Sprintf("/v1/agent/%s/websocket", agentID)

	return u.String(), nil
}

// readMessageLoop 读取消息循环
func (c *Client) readMessageLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			c.connMu.RLock()
			conn := c.conn
			connected := c.connected
			c.connMu.RUnlock()

			// 如果没有连接，等待重连
			if conn == nil || !connected {
				select {
				case <-c.ctx.Done():
					return
				case <-time.After(time.Second):
					continue
				}
			}

			// 设置读取超时
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))

			// 读取消息 - 使用二进制消息类型和Protocol Buffer格式
			msgType, messageData, err := conn.ReadMessage()
			if err != nil {
				c.log.WithError(err).Error("读取WebSocket消息失败")
				c.handleConnectionError(err)
				// 连接断开后等待重连，而不是退出循环
				select {
				case <-c.ctx.Done():
					return
				case <-time.After(time.Second):
					continue
				}
			}

			// 检查消息类型，应该是二进制消息
			if msgType != websocket.BinaryMessage {
				c.log.WithField("message_type", msgType).Warn("收到非二进制消息，忽略")
				continue
			}

			// 使用proto.Unmarshal解析二进制格式的WebSocket消息
			var wsMessage model.WebSocketMessage
			if err := proto.Unmarshal(messageData, &wsMessage); err != nil {
				c.log.WithError(err).WithField("message_size", len(messageData)).Error("解析WebSocket Protocol Buffer消息失败")
				continue
			}

			// 将消息发送到处理队列
			select {
			case c.messageBuffer <- &wsMessage:
			case <-c.ctx.Done():
				return
			default:
				c.log.Warn("消息缓冲区已满，丢弃消息")
			}
		}
	}
}

// messageProcessLoop 消息处理循环
func (c *Client) messageProcessLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		case message := <-c.messageBuffer:
			if message == nil {
				return
			}

			c.processMessage(message)
		}
	}
}

// processMessage 处理WebSocket消息
func (c *Client) processMessage(message *model.WebSocketMessage) {
	c.log.WithFields(logrus.Fields{
		"message_id":   message.MessageId,
		"message_type": message.MessageType.String(),
	}).Info("处理WebSocket消息")

	switch message.MessageType {
	case model.WebSocketMessageType_WEBSOCKET_MESSAGE_SERVER_TYPE_TASK:
		c.handleTaskMessage(message)
	case model.WebSocketMessageType_WEBSOCKET_MESSAGE_SERVER_TYPE_HEARTBEAT:
		c.handleHeartbeatMessage(message)
	case model.WebSocketMessageType_WEBSOCKET_MESSAGE_SERVER_TYPE_ACK:
		c.handleAckMessage(message)
	case model.WebSocketMessageType_WEBSOCKET_MESSAGE_SERVER_TYPE_EGRESS_CONFIG:
		c.handleEgressConfig(message)
	case model.WebSocketMessageType_WEBSOCKET_MESSAGE_SERVER_TYPE_IPTABLES_CONFIG:
		c.handleIptablesConfig(message)
	default:
		c.log.WithField("message_type", message.MessageType.String()).Warn("未知的消息类型")
	}
}

func (c *Client) handleEgressConfig(message *model.WebSocketMessage) {
	c.log.WithField("message_id", message.MessageId).Info("收到egress配置更新消息")

	// 检查代理管理器是否可用
	if c.proxyManager == nil {
		c.log.Error("代理管理器未设置")
		c.sendErrorAck(message.MessageId, "代理管理器未设置", "proxy manager not configured")
		return
	}

	// 尝试解析为单个EgressItem
	var agentEgressConfigs model.AgentEgressConfigs
	if err := message.Payload.UnmarshalTo(&agentEgressConfigs); err != nil {
		c.log.WithField("message_id", message.MessageId).Error("解析agentEgressConfigs消息失败")
		c.sendErrorAck(message.MessageId, "解析agentEgressConfigs消息失败", err.Error())
		return
	}

	c.log.WithField("egress_count", len(agentEgressConfigs.EgressItems)).Info("解析egress配置完成")

	// 应用代理配置
	if err := c.proxyManager.UpdateProxies(agentEgressConfigs.EgressItems); err != nil {
		c.log.WithError(err).Error("应用egress配置失败")
		c.sendErrorAck(message.MessageId, "应用egress配置失败", err.Error())
		return
	}

	c.log.Info("egress配置应用成功")

	// 发送成功确认
	c.sendEgressConfigSuccessAck(message.MessageId, agentEgressConfigs.EgressItems)
}

// sendEgressConfigSuccessAck 发送egress配置成功确认
func (c *Client) sendEgressConfigSuccessAck(messageID string, egressItems []*model.EgressItem) {
	// 创建成功结果数据
	resultMap := map[string]interface{}{
		"applied_egress_count": len(egressItems),
		"applied_at":           time.Now().Unix(),
		"status":               "success",
		"message":              "egress配置应用成功",
	}

	// 添加详细的egress信息
	egressDetails := make([]map[string]interface{}, 0, len(egressItems))
	for _, item := range egressItems {
		detail := map[string]interface{}{
			"egress_id":   item.EgressId,
			"egress_mode": item.EgressMode.String(),
			"server_id":   item.ServerId,
		}
		if item.EgressName != "" {
			detail["egress_name"] = item.EgressName
		}
		egressDetails = append(egressDetails, detail)
	}
	resultMap["egress_details"] = egressDetails

	// 获取代理状态摘要
	if proxyStatus := c.proxyManager.GetStatus(); proxyStatus != nil {
		resultMap["proxy_status"] = proxyStatus
	}

	ackMessage := &model.AckMessage{
		MessageId: messageID,
		Success:   true,
	}

	c.sendAckMessage(ackMessage)
}

func (c *Client) handleIptablesConfig(message *model.WebSocketMessage) {
	c.log.WithField("message_id", message.MessageId).Info("收到iptables配置更新消息")

	// 检查iptables管理器是否可用
	if c.iptablesManager == nil {
		c.log.Error("iptables管理器未设置")
		c.sendErrorAck(message.MessageId, "iptables管理器未设置", "iptables manager not configured")
		return
	}

	// 解析iptables服务器配置消息
	var serverConfig model.IptablesServerConfig
	if err := message.Payload.UnmarshalTo(&serverConfig); err != nil {
		c.log.WithError(err).Error("解析iptables配置消息失败")
		c.sendErrorAck(message.MessageId, "解析iptables配置消息失败", err.Error())
		return
	}

	c.log.WithFields(logrus.Fields{
		"server_id":      serverConfig.ServerId,
		"server_name":    serverConfig.ServerName,
		"total_configs":  serverConfig.TotalConfigs,
		"active_configs": serverConfig.ActiveConfigs,
		"configs_count":  len(serverConfig.Configs),
	}).Info("解析iptables配置完成")

	// 应用iptables配置
	if err := c.iptablesManager.UpdateRulesFromProto(serverConfig.Configs); err != nil {
		c.log.WithError(err).Error("应用iptables配置失败")
		c.sendErrorAck(message.MessageId, "应用iptables配置失败", err.Error())
		return
	}

	c.log.WithFields(logrus.Fields{
		"server_id":     serverConfig.ServerId,
		"applied_rules": len(serverConfig.Configs),
	}).Info("iptables配置应用成功")

	// 发送成功确认
	c.sendConfigSuccessAck(message.MessageId, &serverConfig)
}

// sendConfigSuccessAck 发送配置成功确认
func (c *Client) sendConfigSuccessAck(messageID string, serverConfig *model.IptablesServerConfig) {
	// 创建成功结果数据
	resultMap := map[string]interface{}{
		"server_id":      serverConfig.ServerId,
		"server_name":    serverConfig.ServerName,
		"applied_rules":  len(serverConfig.Configs),
		"total_configs":  serverConfig.TotalConfigs,
		"active_configs": serverConfig.ActiveConfigs,
		"applied_at":     time.Now().Unix(),
		"status":         "success",
		"message":        "iptables配置应用成功",
	}

	// 获取规则摘要信息
	if rulesSummary := c.iptablesManager.GetRulesSummary(); rulesSummary != nil {
		resultMap["rules_summary"] = rulesSummary
	}

	ackMessage := &model.AckMessage{
		MessageId: messageID,
		Success:   true,
	}

	c.sendAckMessage(ackMessage)
}

// handleTaskMessage 处理任务消息
func (c *Client) handleTaskMessage(message *model.WebSocketMessage) {
	if c.taskHandler == nil {
		c.log.Error("任务处理器未设置")
		return
	}

	// 解析任务消息
	var taskMessage model.TaskMessage
	if err := message.Payload.UnmarshalTo(&taskMessage); err != nil {
		c.log.WithError(err).Error("解析任务消息失败")
		c.sendErrorAck(message.MessageId, "解析任务消息失败", err.Error())
		return
	}

	c.log.WithFields(logrus.Fields{
		"task_id":   taskMessage.TaskId,
		"task_type": taskMessage.TaskType.String(),
		"title":     taskMessage.Title,
	}).Info("收到任务")

	// Check task status first
	shouldExecute, existingResult := c.taskHandler.CheckTaskStatus(taskMessage.TaskId, taskMessage.TaskType)

	if !shouldExecute {
		if existingResult != nil {
			// Task already completed, send immediate ACK with existing result
			c.log.WithField("task_id", taskMessage.TaskId).Info("任务已完成，发送缓存结果")
			c.sendTaskResultAck(message.MessageId, existingResult)
			return
		} else {
			// Task is running or cancelled, send appropriate ACK
			c.log.WithField("task_id", taskMessage.TaskId).Info("任务正在运行或已取消，发送状态ACK")
			runningResult := &model.TaskResult{
				TaskId: taskMessage.TaskId,
				Status: model.TaskStatus_TASK_STATUS_RUNNING,
				Output: "Task is currently running or was cancelled",
			}
			c.sendTaskResultAck(message.MessageId, runningResult)
			return
		}
	}

	// Task should be executed, process it asynchronously
	go c.executeTask(message.MessageId, &taskMessage)
}

// executeTask 执行任务
func (c *Client) executeTask(messageID string, task *model.TaskMessage) {
	startTime := time.Now()

	// 执行任务
	result, err := c.taskHandler.HandleTask(c.ctx, task)

	// 构建任务结果
	taskResult := &model.TaskResult{
		TaskId:      task.TaskId,
		StartedAt:   timestamppb.New(startTime),
		CompletedAt: timestamppb.New(time.Now()),
	}

	if err != nil {
		taskResult.Status = model.TaskStatus_TASK_STATUS_FAILED
		taskResult.ErrorMessage = err.Error()
		c.log.WithError(err).WithField("task_id", task.TaskId).Error("任务执行失败")
	} else {
		taskResult.Status = model.TaskStatus_TASK_STATUS_COMPLETED
		if result != nil {
			taskResult.Output = result.Output
			taskResult.ResultData = result.ResultData
		}
		c.log.WithField("task_id", task.TaskId).Info("任务执行成功")
	}

	// 发送任务结果确认
	c.sendTaskResultAck(messageID, taskResult)
}

// sendTaskResultAck 发送任务结果确认
func (c *Client) sendTaskResultAck(messageID string, taskResult *model.TaskResult) {
	resultData, err := anypb.New(taskResult)
	if err != nil {
		c.log.WithError(err).Error("创建任务结果数据失败")
		return
	}

	ackMessage := &model.AckMessage{
		MessageId: messageID,
		Success:   taskResult.Status == model.TaskStatus_TASK_STATUS_COMPLETED,
		Result:    resultData,
	}

	if taskResult.Status == model.TaskStatus_TASK_STATUS_FAILED {
		ackMessage.ErrorMessage = taskResult.ErrorMessage
	}

	c.sendAckMessage(ackMessage)
}

// sendErrorAck 发送错误确认
func (c *Client) sendErrorAck(messageID, errorMessage, details string) {
	ackMessage := &model.AckMessage{
		MessageId:    messageID,
		Success:      false,
		ErrorMessage: errorMessage,
	}

	if details != "" {
		errorData := &model.ErrorMessage{
			Code:      "PROCESSING_ERROR",
			Message:   errorMessage,
			Details:   details,
			Timestamp: timestamppb.Now(),
		}

		if resultData, err := anypb.New(errorData); err == nil {
			ackMessage.Result = resultData
		}
	}

	c.sendAckMessage(ackMessage)
}

// sendAckMessage 发送确认消息
func (c *Client) sendAckMessage(ackMessage *model.AckMessage) {
	payload, err := anypb.New(ackMessage)
	if err != nil {
		c.log.WithError(err).Error("创建确认消息载荷失败")
		return
	}

	wsMessage := &model.WebSocketMessage{
		MessageId:     c.generateMessageID(),
		MessageType:   model.WebSocketMessageType_WEBSOCKET_MESSAGE_AGENT_TYPE_ACK,
		Timestamp:     timestamppb.Now(),
		Payload:       payload,
		CorrelationId: ackMessage.MessageId,
	}

	c.sendMessage(wsMessage)
}

// handleHeartbeatMessage 处理心跳消息
func (c *Client) handleHeartbeatMessage(message *model.WebSocketMessage) {
	c.log.Debug("收到心跳消息")
	c.lastHeartbeat = time.Now()

	// 发送心跳确认
	c.sendHeartbeatAck(message.MessageId)
}

// sendHeartbeatAck 发送心跳确认
func (c *Client) sendHeartbeatAck(messageID string) {
	ackMessage := &model.AckMessage{
		MessageId: messageID,
		Success:   true,
	}

	c.sendAckMessage(ackMessage)
}

// handleAckMessage 处理确认消息
func (c *Client) handleAckMessage(message *model.WebSocketMessage) {
	c.log.WithField("correlation_id", message.CorrelationId).Debug("收到确认消息")

	// 这里可以处理待确认的消息队列
	// 实际实现中可以维护一个待确认消息的映射
}

// heartbeatLoop 心跳循环
func (c *Client) heartbeatLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			// 只在连接正常时发送心跳
			if c.isConnected() {
				c.sendHeartbeat()
			} else {
				c.log.Debug("连接未建立，跳过心跳发送")
			}
		}
	}
}

// sendHeartbeat 发送心跳
func (c *Client) sendHeartbeat() {
	heartbeatMessage := &model.HeartbeatMessage{
		AgentId:   c.agentID,
		Timestamp: timestamppb.Now(),
		Status:    "online",
		Metadata: map[string]string{
			"version": "1.0.0",
		},
	}

	payload, err := anypb.New(heartbeatMessage)
	if err != nil {
		c.log.WithError(err).Error("创建心跳消息载荷失败")
		return
	}

	wsMessage := &model.WebSocketMessage{
		MessageId:   c.generateMessageID(),
		MessageType: model.WebSocketMessageType_WEBSOCKET_MESSAGE_AGENT_TYPE_HEARTBEAT,
		Timestamp:   timestamppb.Now(),
		Payload:     payload,
	}

	c.sendMessage(wsMessage)
	c.log.Debug("发送心跳消息")
}

// metricsReportLoop 监控数据上报循环
func (c *Client) metricsReportLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(60 * time.Second) // 每分钟上报一次监控数据
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.isConnected() && c.metricsCollector != nil {
				c.reportMetrics()
			}
		}
	}
}

// reportMetrics 上报监控数据
func (c *Client) reportMetrics() {
	c.log.Debug("开始上报监控数据")

	// 上报系统监控数据
	c.reportSystemMetrics()

	// 上报流量监控数据
	c.reportTrafficMetrics()

	// 上报连接监控数据
	c.reportConnectionMetrics()

	// 上报性能监控数据（包含任务统计）
	c.reportPerformanceMetrics()

	// 上报错误监控数据
	c.reportErrorMetrics()
}

// reportSystemMetrics 上报系统监控数据
func (c *Client) reportSystemMetrics() {
	systemMetrics, err := c.metricsCollector.CollectSystemMetrics()
	if err != nil {
		c.log.WithError(err).Error("收集系统监控数据失败")
		return
	}

	c.sendMetrics(model.MetricsType_METRICS_TYPE_SYSTEM, systemMetrics)
}

// reportTrafficMetrics 上报流量监控数据
func (c *Client) reportTrafficMetrics() {
	trafficMetrics, err := c.metricsCollector.CollectTrafficMetrics()
	if err != nil {
		c.log.WithError(err).Error("收集流量监控数据失败")
		return
	}

	c.sendMetrics(model.MetricsType_METRICS_TYPE_TRAFFIC, trafficMetrics)
}

// reportConnectionMetrics 上报连接监控数据
func (c *Client) reportConnectionMetrics() {
	connectionMetrics, err := c.metricsCollector.CollectConnectionMetrics()
	if err != nil {
		c.log.WithError(err).Error("收集连接监控数据失败")
		return
	}

	c.sendMetrics(model.MetricsType_METRICS_TYPE_CONNECTION, connectionMetrics)
}

// reportPerformanceMetrics 上报性能监控数据
func (c *Client) reportPerformanceMetrics() {
	performanceMetrics, err := c.metricsCollector.CollectPerformanceMetrics()
	if err != nil {
		c.log.WithError(err).Error("收集性能监控数据失败")
		return
	}

	c.sendMetrics(model.MetricsType_METRICS_TYPE_PERFORMANCE, performanceMetrics)
}

// reportErrorMetrics 上报错误监控数据
func (c *Client) reportErrorMetrics() {
	errorMetrics, err := c.metricsCollector.CollectErrorMetrics()
	if err != nil {
		c.log.WithError(err).Error("收集错误监控数据失败")
		return
	}

	c.sendMetrics(model.MetricsType_METRICS_TYPE_ERROR, errorMetrics)
}

// sendMetrics 发送监控数据
func (c *Client) sendMetrics(metricsType model.MetricsType, data proto.Message) {
	metricsData, err := anypb.New(data)
	if err != nil {
		c.log.WithError(err).Error("创建监控数据载荷失败")
		return
	}

	metricsMessage := &model.MetricsMessage{
		AgentId:     c.agentID,
		MetricsType: metricsType,
		Timestamp:   timestamppb.Now(),
		Data:        metricsData,
		Labels: map[string]string{
			"agent_id": c.agentID,
			"version":  "1.0.0",
		},
	}

	payload, err := anypb.New(metricsMessage)
	if err != nil {
		c.log.WithError(err).Error("创建监控消息载荷失败")
		return
	}

	wsMessage := &model.WebSocketMessage{
		MessageId:   c.generateMessageID(),
		MessageType: model.WebSocketMessageType_WEBSOCKET_MESSAGE_AGENT_TYPE_METRICS,
		Timestamp:   timestamppb.Now(),
		Payload:     payload,
	}

	c.sendMessage(wsMessage)
	c.log.WithField("metrics_type", metricsType.String()).Debug("发送监控数据")
}

// sendMessage 发送WebSocket消息
func (c *Client) sendMessage(message *model.WebSocketMessage) {
	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()

	if conn == nil {
		c.log.Error("WebSocket连接未建立")
		return
	}

	// 使用proto.Marshal序列化消息为二进制格式
	messageData, err := proto.Marshal(message)
	if err != nil {
		c.log.WithError(err).Error("序列化WebSocket消息为Protocol Buffer失败")
		return
	}

	// 发送二进制消息
	if err := conn.WriteMessage(websocket.BinaryMessage, messageData); err != nil {
		c.log.WithError(err).Error("发送WebSocket消息失败")
		c.handleConnectionError(err)
		return
	}

	c.log.WithFields(logrus.Fields{
		"message_id":   message.MessageId,
		"message_type": message.MessageType.String(),
		"message_size": len(messageData),
	}).Debug("发送WebSocket Protocol Buffer消息成功")
}

// handleConnectionError 处理连接错误
func (c *Client) handleConnectionError(err error) {
	c.log.WithError(err).Error("WebSocket连接错误")

	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connected = false
	c.connMu.Unlock()

	// 触发重连
	c.triggerReconnect()
}

// isConnected 检查连接状态
func (c *Client) isConnected() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.connected && c.conn != nil
}

// generateMessageID 生成消息ID
func (c *Client) generateMessageID() string {
	return fmt.Sprintf("msg_agent_%d_%s", time.Now().UnixNano(), c.agentID)
}

// SetTaskStatsProvider sets the task stats provider for metrics collection
func (c *Client) SetTaskStatsProvider() {
	if collector, ok := c.metricsCollector.(*DefaultMetricsCollector); ok {
		collector.SetTaskStatsProvider(c.taskHandler)
		c.log.Info("Task stats provider set for metrics collection")
	}
}
