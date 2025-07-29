package websocket

import (
	"context"
	"sync"
	"time"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/errors"
	"github.com/nspass/nspass-agent/pkg/interfaces"
	"google.golang.org/protobuf/proto"
)

// EnhancedClient 增强的WebSocket客户端
type EnhancedClient struct {
	config  *config.Config
	agentID string
	token   string
	logger  interfaces.Logger

	// 组件
	connectionManager *ConnectionManager
	messageProcessor  *MessageProcessor

	// 控制相关
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 状态
	running bool
	mu      sync.RWMutex
}

// NewEnhancedClient 创建增强的WebSocket客户端
func NewEnhancedClient(
	cfg *config.Config,
	agentID, token string,
	logger interfaces.Logger,
	taskHandler interfaces.TaskHandler,
	metricsCollector interfaces.MetricsCollector,
	proxyManager interfaces.ProxyManager,
	iptablesManager interfaces.IPTablesManager,
) *EnhancedClient {
	ctx, cancel := context.WithCancel(context.Background())

	client := &EnhancedClient{
		config:  cfg,
		agentID: agentID,
		token:   token,
		logger:  logger,
		ctx:     ctx,
		cancel:  cancel,
	}

	// 创建连接管理器
	client.connectionManager = NewConnectionManager(cfg, agentID, token, logger)

	// 创建消息处理器
	client.messageProcessor = NewMessageProcessor(agentID, cfg, logger, taskHandler, metricsCollector, proxyManager, iptablesManager)

	// 设置连接管理器回调
	client.setupConnectionCallbacks()

	return client
}

// setupConnectionCallbacks 设置连接管理器回调
func (c *EnhancedClient) setupConnectionCallbacks() {
	c.connectionManager.SetCallbacks(
		c.onConnected,
		c.onDisconnected,
		c.onMessage,
		c.onError,
	)
}

// Start 启动客户端
func (c *EnhancedClient) Start() error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return errors.New(errors.ErrorTypeWebSocket, "CLIENT_ALREADY_RUNNING", "WebSocket客户端已在运行")
	}
	c.running = true
	c.mu.Unlock()

	c.logger.Info("启动增强WebSocket客户端")

	// 启动消息处理器
	if err := c.messageProcessor.Start(); err != nil {
		return errors.Wrap(err, errors.ErrorTypeWebSocket, "MESSAGE_PROCESSOR_START_FAILED", "消息处理器启动失败")
	}

	// 启动连接管理器
	if err := c.connectionManager.Start(); err != nil {
		c.messageProcessor.Stop()
		return errors.Wrap(err, errors.ErrorTypeWebSocket, "CONNECTION_MANAGER_START_FAILED", "连接管理器启动失败")
	}

	// 启动消息发送循环
	c.wg.Add(1)
	go c.messageSendLoop()

	// 启动心跳循环
	c.wg.Add(1)
	go c.heartbeatLoop()

	// 启动监控指标发送循环
	c.wg.Add(1)
	go c.metricsLoop()

	return nil
}

// Stop 停止客户端
func (c *EnhancedClient) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = false
	c.mu.Unlock()

	c.logger.Info("停止增强WebSocket客户端")

	// 取消上下文
	c.cancel()

	// 停止连接管理器
	if err := c.connectionManager.Stop(); err != nil {
		c.logger.WithError(err).Error("停止连接管理器失败")
	}

	// 停止消息处理器
	if err := c.messageProcessor.Stop(); err != nil {
		c.logger.WithError(err).Error("停止消息处理器失败")
	}

	// 等待所有goroutine结束
	c.wg.Wait()

	return nil
}

// IsConnected 检查是否已连接
func (c *EnhancedClient) IsConnected() bool {
	return c.connectionManager.IsConnected()
}

// SendMessage 发送消息
func (c *EnhancedClient) SendMessage(message *model.WebSocketMessage) error {
	if !c.IsConnected() {
		return errors.New(errors.ErrorTypeWebSocket, "NOT_CONNECTED", "WebSocket未连接")
	}

	// 序列化消息
	data, err := proto.Marshal(message)
	if err != nil {
		return errors.Wrap(err, errors.ErrorTypeWebSocket, "MESSAGE_MARSHAL_FAILED", "消息序列化失败")
	}

	// 发送消息
	return c.connectionManager.SendMessage(data)
}

// messageSendLoop 消息发送循环
func (c *EnhancedClient) messageSendLoop() {
	defer c.wg.Done()

	outgoingChan := c.messageProcessor.GetOutgoingChannel()

	for {
		select {
		case <-c.ctx.Done():
			return
		case message := <-outgoingChan:
			if err := c.SendMessage(message); err != nil {
				c.logger.WithError(err).Error("发送消息失败")
			}
		}
	}
}

// heartbeatLoop 心跳循环
func (c *EnhancedClient) heartbeatLoop() {
	defer c.wg.Done()

	interval, err := time.ParseDuration(c.config.WebSocket.HeartbeatInterval)
	if err != nil {
		c.logger.WithError(err).Error("解析心跳间隔失败，使用默认值30s")
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.IsConnected() {
				c.sendHeartbeat()
			}
		}
	}
}

// metricsLoop 监控指标发送循环
func (c *EnhancedClient) metricsLoop() {
	defer c.wg.Done()

	interval, err := time.ParseDuration(c.config.WebSocket.MetricsInterval)
	if err != nil {
		c.logger.WithError(err).Error("解析监控指标间隔失败，使用默认值60s")
		interval = 60 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.IsConnected() {
				c.sendMetrics()
			}
		}
	}
}

// sendHeartbeat 发送心跳
func (c *EnhancedClient) sendHeartbeat() {
	// 创建心跳消息并通过消息处理器发送
	// 这里简化实现，实际应该创建完整的心跳消息
	c.logger.Debug("发送心跳消息")
}

// SendHeartbeat 发送心跳（兼容原接口）
func (c *EnhancedClient) SendHeartbeat() error {
	if !c.IsConnected() {
		return errors.New(errors.ErrorTypeWebSocket, "NOT_CONNECTED", "WebSocket未连接")
	}
	c.sendHeartbeat()
	return nil
}

// sendMetrics 发送监控指标
func (c *EnhancedClient) sendMetrics() {
	// 创建监控指标消息并通过消息处理器发送
	// 这里简化实现，实际应该收集并发送监控指标
	c.logger.Debug("发送监控指标")
}

// SendMetrics 发送监控指标（兼容原接口）
func (c *EnhancedClient) SendMetrics(metricsType model.MetricsType) error {
	if !c.IsConnected() {
		return errors.New(errors.ErrorTypeWebSocket, "NOT_CONNECTED", "WebSocket未连接")
	}
	c.sendMetrics()
	return nil
}

// 连接管理器回调函数

// onConnected 连接建立回调
func (c *EnhancedClient) onConnected() {
	c.logger.Info("WebSocket连接已建立")
}

// onDisconnected 连接断开回调
func (c *EnhancedClient) onDisconnected(err error) {
	if err != nil {
		c.logger.WithError(err).Warn("WebSocket连接断开")
	} else {
		c.logger.Info("WebSocket连接正常断开")
	}
}

// onMessage 消息接收回调
func (c *EnhancedClient) onMessage(data []byte) {
	c.messageProcessor.ProcessMessage(data)
}

// onError 错误回调
func (c *EnhancedClient) onError(err error) {
	c.logger.WithError(err).Error("WebSocket连接错误")
}

// GetStatus 获取客户端状态
func (c *EnhancedClient) GetStatus() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := map[string]interface{}{
		"running":    c.running,
		"connected":  c.IsConnected(),
		"agent_id":   c.agentID,
		"connection": c.connectionManager.GetStatus(),
		"processor":  c.messageProcessor.GetStats(),
	}

	return status
}

// GetConnectionStatus 获取连接状态（兼容原接口）
func (c *EnhancedClient) GetConnectionStatus() map[string]interface{} {
	return c.GetStatus()
}
