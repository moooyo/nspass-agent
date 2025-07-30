package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/cert"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/interfaces"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MessageHandler 消息处理函数类型
type MessageHandler func(*MessageProcessor, *model.WebSocketMessage)

// messageHandlerTable 消息处理跳表
var messageHandlerTable = map[model.WebSocketMessageType]MessageHandler{
	model.WebSocketMessageType_WEBSOCKET_MESSAGE_SERVER_TYPE_HEARTBEAT:       (*MessageProcessor).handleHeartbeat,
	model.WebSocketMessageType_WEBSOCKET_MESSAGE_SERVER_TYPE_TASK:            (*MessageProcessor).handleTask,
	model.WebSocketMessageType_WEBSOCKET_MESSAGE_SERVER_TYPE_EGRESS_CONFIG:   (*MessageProcessor).handleEgressConfig,
	model.WebSocketMessageType_WEBSOCKET_MESSAGE_SERVER_TYPE_IPTABLES_CONFIG: (*MessageProcessor).handleIPTablesConfig,
	model.WebSocketMessageType_WEBSOCKET_MESSAGE_SERVER_TYPE_AGENT_UPGRADE:   (*MessageProcessor).handleAgentUpgrade,
	model.WebSocketMessageType_WEBSOCKET_MESSAGE_SERVER_TYPE_PROXY_UPGRADE:   (*MessageProcessor).handleProxyUpgrade,
}

// EgressConfigParser 出口配置解析器
type EgressConfigParser struct{}

// ParsedEgressConfig 解析后的出口配置
type ParsedEgressConfig struct {
	// 通用字段（从EgressItem直接获取）
	Port     int32  // 端口号
	Password string // 密码

	// 特定配置（从EgressConfig JSON解析）
	SpecificConfig map[string]interface{} // 代理特定配置
}

// ParseEgressConfig 解析出口配置，分离通用字段和特定配置
func (p *EgressConfigParser) ParseEgressConfig(item *model.EgressItem) (*ParsedEgressConfig, error) {
	parsed := &ParsedEgressConfig{
		SpecificConfig: make(map[string]interface{}),
	}

	// 获取通用字段
	if item.Port != nil {
		parsed.Port = *item.Port
	}
	if item.Password != nil {
		parsed.Password = *item.Password
	}

	// 解析特定配置JSON
	if item.EgressConfig != "" {
		if err := json.Unmarshal([]byte(item.EgressConfig), &parsed.SpecificConfig); err != nil {
			return nil, fmt.Errorf("解析出口配置JSON失败: %w", err)
		}
	}

	// 验证配置是否包含必要字段
	if err := p.validateConfigByMode(item.EgressMode, parsed); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return parsed, nil
}

// validateConfigByMode 根据模式验证配置
func (p *EgressConfigParser) validateConfigByMode(mode model.EgressMode, parsed *ParsedEgressConfig) error {
	switch mode {
	case model.EgressMode_EGRESS_MODE_SS2022:
		// 验证Shadowsocks2022必需字段
		// 密码从通用字段获取
		if parsed.Password == "" {
			return fmt.Errorf("Shadowsocks2022配置缺少password字段")
		}
		// 端口从通用字段获取
		if parsed.Port == 0 {
			return fmt.Errorf("Shadowsocks2022配置缺少port字段")
		}
		// 加密方法从特定配置获取
		if _, ok := parsed.SpecificConfig["method"]; !ok {
			return fmt.Errorf("Shadowsocks2022配置缺少method字段")
		}
		// 注意：不需要server字段，因为代理监听0.0.0.0

	case model.EgressMode_EGRESS_MODE_TROJAN:
		// 验证Trojan必需字段
		// 密码从通用字段获取
		if parsed.Password == "" {
			return fmt.Errorf("Trojan配置缺少password字段")
		}
		// 端口从通用字段获取
		if parsed.Port == 0 {
			return fmt.Errorf("Trojan配置缺少port字段")
		}
		// 注意：不需要server字段，因为代理监听0.0.0.0

	case model.EgressMode_EGRESS_MODE_SNELL:
		// 验证Snell必需字段
		// 端口从通用字段获取
		if parsed.Port == 0 {
			return fmt.Errorf("Snell配置缺少port字段")
		}
		// 密码从通用字段获取（Snell使用password作为PSK）
		if parsed.Password == "" {
			return fmt.Errorf("Snell配置缺少password字段")
		}
		// 版本从特定配置获取（可选）
		// 注意：不需要server字段，因为代理监听0.0.0.0

	case model.EgressMode_EGRESS_MODE_DIRECT:
		// 验证Direct必需字段
		// 目标地址从特定配置获取
		if _, ok := parsed.SpecificConfig["target_address"]; !ok {
			return fmt.Errorf("Direct配置缺少target_address字段")
		}

	case model.EgressMode_EGRESS_MODE_IPTABLES:
		// IPTables配置通常在IPTables配置消息中处理
		return fmt.Errorf("IPTables模式不支持在出口配置中解析")

	default:
		return fmt.Errorf("不支持的出口模式: %s", mode)
	}

	return nil
}

// ValidateEgressConfig 验证出口配置
func (p *EgressConfigParser) ValidateEgressConfig(item *model.EgressItem) error {
	_, err := p.ParseEgressConfig(item)
	return err
}

// ProcessEgressItems 处理出口配置项，解析和验证JSON配置
func (p *EgressConfigParser) ProcessEgressItems(items []*model.EgressItem) ([]*model.EgressItem, error) {
	processedItems := make([]*model.EgressItem, 0, len(items))

	for _, item := range items {
		// 验证配置
		if err := p.ValidateEgressConfig(item); err != nil {
			return nil, fmt.Errorf("出口配置验证失败 [%s]: %w", item.EgressId, err)
		}

		// 解析配置（这里主要是验证，实际的配置解析由代理管理器处理）
		parsedConfig, err := p.ParseEgressConfig(item)
		if err != nil {
			return nil, fmt.Errorf("出口配置解析失败 [%s]: %w", item.EgressId, err)
		}

		// 记录解析结果用于调试（简化版本，避免依赖问题）
		fmt.Printf("出口配置解析完成: egress_id=%s, mode=%s, port=%d, has_password=%t, specific_fields=%d\n",
			item.EgressId, item.EgressMode, parsedConfig.Port, parsedConfig.Password != "", len(parsedConfig.SpecificConfig))

		processedItems = append(processedItems, item)
	}

	return processedItems, nil
}

// MessageProcessor 消息处理器
type MessageProcessor struct {
	agentID          string
	config           *config.Config // 配置信息
	logger           interfaces.Logger
	taskHandler      interfaces.TaskHandler
	metricsCollector interfaces.MetricsCollector
	proxyManager     interfaces.ProxyManager    // 代理管理器
	iptablesManager  interfaces.IPTablesManager // IPTables管理器
	certManager      *cert.Manager              // 证书管理器

	// 消息队列
	incomingMessages chan []byte
	outgoingMessages chan *model.WebSocketMessage

	// 控制相关
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 统计信息
	stats struct {
		mu               sync.RWMutex
		messagesReceived int64
		messagesSent     int64
		tasksProcessed   int64
		metricsCollected int64
		errors           int64
	}
}

// NewMessageProcessor 创建消息处理器
func NewMessageProcessor(
	agentID string,
	config *config.Config,
	logger interfaces.Logger,
	taskHandler interfaces.TaskHandler,
	metricsCollector interfaces.MetricsCollector,
	proxyManager interfaces.ProxyManager,
	iptablesManager interfaces.IPTablesManager,
	certManager *cert.Manager,
) *MessageProcessor {
	ctx, cancel := context.WithCancel(context.Background())

	return &MessageProcessor{
		agentID:          agentID,
		config:           config,
		logger:           logger,
		taskHandler:      taskHandler,
		metricsCollector: metricsCollector,
		proxyManager:     proxyManager,
		iptablesManager:  iptablesManager,
		certManager:      certManager,
		incomingMessages: make(chan []byte, 100),
		outgoingMessages: make(chan *model.WebSocketMessage, 100),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start 启动消息处理器
func (mp *MessageProcessor) Start() error {
	mp.logger.Info("启动WebSocket消息处理器")

	// 启动消息处理协程
	mp.wg.Add(2)
	go mp.processIncomingMessages()
	go mp.processOutgoingMessages()

	return nil
}

// Stop 停止消息处理器
func (mp *MessageProcessor) Stop() error {
	mp.logger.Info("停止WebSocket消息处理器")

	mp.cancel()
	mp.wg.Wait()

	close(mp.incomingMessages)
	close(mp.outgoingMessages)

	return nil
}

// ProcessMessage 处理接收到的消息
func (mp *MessageProcessor) ProcessMessage(data []byte) {
	select {
	case mp.incomingMessages <- data:
	case <-mp.ctx.Done():
	default:
		mp.logger.Warn("消息队列已满，丢弃消息")
		mp.incrementErrorCount()
	}
}

// SendMessage 发送消息
func (mp *MessageProcessor) SendMessage(message *model.WebSocketMessage) {
	select {
	case mp.outgoingMessages <- message:
	case <-mp.ctx.Done():
	default:
		mp.logger.Warn("发送队列已满，丢弃消息")
		mp.incrementErrorCount()
	}
}

// GetOutgoingChannel 获取发送消息通道
func (mp *MessageProcessor) GetOutgoingChannel() <-chan *model.WebSocketMessage {
	return mp.outgoingMessages
}

// processIncomingMessages 处理接收消息
func (mp *MessageProcessor) processIncomingMessages() {
	defer mp.wg.Done()

	for {
		select {
		case <-mp.ctx.Done():
			return
		case data := <-mp.incomingMessages:
			mp.handleIncomingMessage(data)
		}
	}
}

// processOutgoingMessages 处理发送消息
func (mp *MessageProcessor) processOutgoingMessages() {
	defer mp.wg.Done()

	for {
		select {
		case <-mp.ctx.Done():
			return
		case message := <-mp.outgoingMessages:
			mp.handleOutgoingMessage(message)
		}
	}
}

// handleIncomingMessage 处理接收到的消息
func (mp *MessageProcessor) handleIncomingMessage(data []byte) {
	mp.incrementReceivedCount()

	// 解析WebSocket消息
	var wsMessage model.WebSocketMessage
	if err := proto.Unmarshal(data, &wsMessage); err != nil {
		mp.logger.WithError(err).Error("解析WebSocket消息失败")
		mp.incrementErrorCount()
		return
	}

	mp.logger.WithField("message_type", wsMessage.MessageType).
		WithField("message_id", wsMessage.MessageId).
		Debug("收到WebSocket消息")

	// 使用跳表处理消息
	if handler, exists := messageHandlerTable[wsMessage.MessageType]; exists {
		handler(mp, &wsMessage)
	} else {
		mp.logger.WithField("message_type", wsMessage.MessageType).
			Warn("未知的消息类型")
		mp.incrementErrorCount()
	}
}

// handleOutgoingMessage 处理发送消息
func (mp *MessageProcessor) handleOutgoingMessage(message *model.WebSocketMessage) {
	mp.incrementSentCount()

	mp.logger.WithField("message_type", message.MessageType).
		WithField("message_id", message.MessageId).
		Debug("发送WebSocket消息")
}

// handleHeartbeat 处理心跳消息
func (mp *MessageProcessor) handleHeartbeat(message *model.WebSocketMessage) {
	mp.logger.Debug("收到心跳消息")

	// 发送心跳响应
	response := &model.WebSocketMessage{
		MessageId:     generateMessageID(),
		MessageType:   model.WebSocketMessageType_WEBSOCKET_MESSAGE_AGENT_TYPE_HEARTBEAT,
		Timestamp:     timestamppb.Now(),
		CorrelationId: message.MessageId,
	}

	// 创建心跳消息载荷
	heartbeat := &model.HeartbeatMessage{
		AgentId:   mp.agentID,
		Timestamp: timestamppb.Now(),
		Status:    "healthy",
	}

	payload, err := anypb.New(heartbeat)
	if err != nil {
		mp.logger.WithError(err).Error("创建心跳载荷失败")
		return
	}

	response.Payload = payload
	mp.SendMessage(response)
}

// handleTask 处理任务消息
func (mp *MessageProcessor) handleTask(message *model.WebSocketMessage) {
	mp.logger.WithField("correlation_id", message.CorrelationId).Info("收到任务消息")

	// 解析任务消息
	var taskMessage model.TaskMessage
	if err := message.Payload.UnmarshalTo(&taskMessage); err != nil {
		mp.logger.WithError(err).Error("解析任务消息失败")
		mp.sendErrorAck(message.MessageId, "解析任务消息失败", err.Error())
		return
	}

	// 处理任务
	ctx, cancel := context.WithTimeout(mp.ctx, 5*time.Minute)
	defer cancel()

	_, err := mp.taskHandler.HandleTask(ctx, &taskMessage)
	if err != nil {
		mp.logger.WithError(err).Error("任务执行失败")
		mp.sendErrorAck(message.MessageId, "任务执行失败", err.Error())
		return
	}

	mp.incrementTasksProcessedCount()

	// 发送成功确认
	mp.sendSuccessAck(message.MessageId, "任务执行成功")
}

// handleEgressConfig 处理出口配置消息
func (mp *MessageProcessor) handleEgressConfig(message *model.WebSocketMessage) {
	mp.logger.WithField("message_id", message.MessageId).Info("收到出口配置消息")

	// 解析AgentEgressConfigs消息
	var agentEgressConfigs model.AgentEgressConfigs
	if err := message.Payload.UnmarshalTo(&agentEgressConfigs); err != nil {
		mp.logger.WithError(err).Error("解析出口配置消息失败")
		mp.sendErrorAck(message.MessageId, "解析出口配置消息失败", err.Error())
		return
	}

	mp.logger.WithFields(map[string]interface{}{
		"egress_count":         len(agentEgressConfigs.EgressItems),
		"dns_configs":          len(agentEgressConfigs.DnsConfigs),
		"dns_provider_configs": len(agentEgressConfigs.DnsProviderConfigs),
	}).Info("解析出口配置完成")

	// 处理DNS配置 - 更新全局DNS配置管理器
	if len(agentEgressConfigs.DnsConfigs) > 0 || len(agentEgressConfigs.DnsProviderConfigs) > 0 {
		mp.logger.Info("更新DNS提供商配置")
		// 使用全局DNS配置管理器
		dnsConfigMgr := cert.GetGlobalDNSConfigManager()
		dnsConfigMgr.UpdateConfigs(agentEgressConfigs.DnsConfigs, agentEgressConfigs.DnsProviderConfigs)
	}

	// 使用配置解析器处理和验证配置
	parser := &EgressConfigParser{}
	processedItems, err := parser.ProcessEgressItems(agentEgressConfigs.EgressItems)
	if err != nil {
		mp.logger.WithError(err).Error("处理出口配置失败")
		mp.sendErrorAck(message.MessageId, "处理出口配置失败", err.Error())
		return
	}

	// 直接处理出口配置（不再使用TASK概念）
	mp.logger.Info("开始应用出口配置")

	// 调用代理管理器应用配置
	if err := mp.proxyManager.UpdateProxies(processedItems); err != nil {
		mp.logger.WithError(err).Error("应用出口配置失败")
		mp.sendErrorAck(message.MessageId, "应用出口配置失败", err.Error())
		return
	}

	mp.logger.Info("出口配置处理成功")
	mp.sendSuccessAck(message.MessageId, "出口配置应用成功")
}

// handleIPTablesConfig 处理IPTables配置消息
func (mp *MessageProcessor) handleIPTablesConfig(message *model.WebSocketMessage) {
	mp.logger.WithField("message_id", message.MessageId).Info("收到IPTables配置消息")

	// 解析AgentIptablesConfigs消息
	var agentIptablesConfigs model.AgentIptablesConfigs
	if err := message.Payload.UnmarshalTo(&agentIptablesConfigs); err != nil {
		mp.logger.WithError(err).Error("解析IPTables配置消息失败")
		mp.sendErrorAck(message.MessageId, "解析IPTables配置消息失败", err.Error())
		return
	}

	mp.logger.WithField("rules_count", len(agentIptablesConfigs.IptablesRules)).Info("解析IPTables配置完成")

	// 直接处理IPTables配置（不再使用TASK概念）
	mp.logger.Info("开始应用IPTables规则")

	// 调用IPTables管理器应用规则
	if err := mp.iptablesManager.ApplyRules(agentIptablesConfigs.IptablesRules); err != nil {
		mp.logger.WithError(err).Error("应用IPTables规则失败")
		mp.sendErrorAck(message.MessageId, "应用IPTables规则失败", err.Error())
		return
	}

	mp.logger.Info("IPTables配置处理成功")
	mp.sendSuccessAck(message.MessageId, "IPTables规则应用成功")
}

// handleAgentUpgrade 处理Agent升级消息
func (mp *MessageProcessor) handleAgentUpgrade(message *model.WebSocketMessage) {
	mp.logger.WithField("message_id", message.MessageId).Info("收到Agent升级消息")

	// 解析AgentUpgradeMessage消息
	var agentUpgradeMessage model.AgentUpgradeMessage
	if err := message.Payload.UnmarshalTo(&agentUpgradeMessage); err != nil {
		mp.logger.WithError(err).Error("解析Agent升级消息失败")
		mp.sendErrorAck(message.MessageId, "解析Agent升级消息失败", err.Error())
		return
	}

	mp.logger.WithFields(map[string]interface{}{
		"target_version": agentUpgradeMessage.TargetVersion,
		"script_url":     mp.config.Upgrade.AgentScriptURL,
	}).Info("开始Agent升级")

	// 异步执行升级
	go mp.executeAgentUpgrade(message.MessageId, &agentUpgradeMessage)
}

// handleProxyUpgrade 处理代理升级消息
func (mp *MessageProcessor) handleProxyUpgrade(message *model.WebSocketMessage) {
	mp.logger.WithField("message_id", message.MessageId).Info("收到代理升级消息")

	// 解析ProxyUpgradeMessage消息
	var proxyUpgradeMessage model.ProxyUpgradeMessage
	if err := message.Payload.UnmarshalTo(&proxyUpgradeMessage); err != nil {
		mp.logger.WithError(err).Error("解析代理升级消息失败")
		mp.sendErrorAck(message.MessageId, "解析代理升级消息失败", err.Error())
		return
	}

	mp.logger.WithFields(map[string]interface{}{
		"target_version": proxyUpgradeMessage.TargetVersion,
		"metadata":       proxyUpgradeMessage.Metadata,
		"script_url":     mp.config.Upgrade.ProxyScriptURL,
	}).Info("开始代理升级")

	// 异步执行升级
	go mp.executeProxyUpgrade(message.MessageId, &proxyUpgradeMessage)
}

// sendSuccessAck 发送成功确认
func (mp *MessageProcessor) sendSuccessAck(messageID, message string) {
	ackMessage := &model.AckMessage{
		MessageId: messageID,
		Success:   true,
	}

	mp.sendAckMessage(ackMessage)
}

// sendErrorAck 发送错误确认
func (mp *MessageProcessor) sendErrorAck(messageID, errorMessage, details string) {
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

	mp.sendAckMessage(ackMessage)
}

// sendAckMessage 发送确认消息
func (mp *MessageProcessor) sendAckMessage(ackMessage *model.AckMessage) {
	payload, err := anypb.New(ackMessage)
	if err != nil {
		mp.logger.WithError(err).Error("创建确认消息载荷失败")
		return
	}

	wsMessage := &model.WebSocketMessage{
		MessageId:     generateMessageID(),
		MessageType:   model.WebSocketMessageType_WEBSOCKET_MESSAGE_AGENT_TYPE_ACK,
		Timestamp:     timestamppb.Now(),
		Payload:       payload,
		CorrelationId: ackMessage.MessageId,
	}

	mp.SendMessage(wsMessage)
}

// 统计方法
func (mp *MessageProcessor) incrementReceivedCount() {
	mp.stats.mu.Lock()
	mp.stats.messagesReceived++
	mp.stats.mu.Unlock()
}

func (mp *MessageProcessor) incrementSentCount() {
	mp.stats.mu.Lock()
	mp.stats.messagesSent++
	mp.stats.mu.Unlock()
}

func (mp *MessageProcessor) incrementTasksProcessedCount() {
	mp.stats.mu.Lock()
	mp.stats.tasksProcessed++
	mp.stats.mu.Unlock()
}

func (mp *MessageProcessor) incrementErrorCount() {
	mp.stats.mu.Lock()
	mp.stats.errors++
	mp.stats.mu.Unlock()
}

// GetStats 获取统计信息
func (mp *MessageProcessor) GetStats() map[string]interface{} {
	mp.stats.mu.RLock()
	defer mp.stats.mu.RUnlock()

	return map[string]interface{}{
		"messages_received": mp.stats.messagesReceived,
		"messages_sent":     mp.stats.messagesSent,
		"tasks_processed":   mp.stats.tasksProcessed,
		"metrics_collected": mp.stats.metricsCollected,
		"errors":            mp.stats.errors,
	}
}

// generateMessageID 生成消息ID
func generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

// executeAgentUpgrade 执行Agent升级
func (mp *MessageProcessor) executeAgentUpgrade(messageId string, upgradeMsg *model.AgentUpgradeMessage) {
	mp.logger.Info("开始执行Agent升级")

	// 检查升级功能是否启用
	if !mp.config.Upgrade.Enabled {
		mp.logger.Error("Agent升级功能未启用")
		mp.sendErrorAck(messageId, "Agent升级功能未启用", "请在配置中启用upgrade.enabled")
		return
	}

	// 设置升级超时
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(mp.config.Upgrade.Timeout)*time.Second)
	defer cancel()

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "agent_upgrade_*")
	if err != nil {
		mp.logger.WithError(err).Error("创建临时目录失败")
		mp.sendErrorAck(messageId, "创建临时目录失败", err.Error())
		return
	}
	defer os.RemoveAll(tempDir)

	// 下载升级脚本（使用配置中的安全URL）
	scriptPath := filepath.Join(tempDir, "agent_upgrade.sh")
	if err := mp.downloadScript(mp.config.Upgrade.AgentScriptURL, scriptPath); err != nil {
		mp.logger.WithError(err).Error("下载Agent升级脚本失败")
		mp.sendErrorAck(messageId, "下载升级脚本失败", err.Error())
		return
	}

	// 设置脚本权限
	if err := os.Chmod(scriptPath, 0755); err != nil {
		mp.logger.WithError(err).Error("设置脚本权限失败")
		mp.sendErrorAck(messageId, "设置脚本权限失败", err.Error())
		return
	}

	// 构建升级参数
	args := []string{}

	// 如果消息中指定了目标版本，则传递给脚本
	if upgradeMsg.TargetVersion != "" {
		args = append(args, "--target-version="+upgradeMsg.TargetVersion)
	}

	// 根据配置设置备份选项
	if !mp.config.Upgrade.BackupEnabled {
		args = append(args, "--no-backup")
	}

	// 设置超时参数
	args = append(args, fmt.Sprintf("--timeout=%d", mp.config.Upgrade.Timeout))

	// 执行升级脚本
	cmd := exec.CommandContext(ctx, scriptPath, args...)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		mp.logger.WithError(err).WithField("output", string(output)).Error("Agent升级失败")
		mp.sendErrorAck(messageId, "Agent升级失败", fmt.Sprintf("%s\n输出: %s", err.Error(), string(output)))
		return
	}

	mp.logger.Info("Agent升级成功完成")
	mp.sendSuccessAck(messageId, fmt.Sprintf("Agent升级成功，输出: %s", string(output)))
}

// executeProxyUpgrade 执行Proxy升级
func (mp *MessageProcessor) executeProxyUpgrade(messageId string, upgradeMsg *model.ProxyUpgradeMessage) {
	mp.logger.Info("开始执行Proxy升级")

	// 检查升级功能是否启用
	if !mp.config.Upgrade.Enabled {
		mp.logger.Error("Proxy升级功能未启用")
		mp.sendErrorAck(messageId, "Proxy升级功能未启用", "请在配置中启用upgrade.enabled")
		return
	}

	// 设置升级超时
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(mp.config.Upgrade.Timeout)*time.Second)
	defer cancel()

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "proxy_upgrade_*")
	if err != nil {
		mp.logger.WithError(err).Error("创建临时目录失败")
		mp.sendErrorAck(messageId, "创建临时目录失败", err.Error())
		return
	}
	defer os.RemoveAll(tempDir)

	// 下载升级脚本（使用配置中的安全URL）
	scriptPath := filepath.Join(tempDir, "proxy_upgrade.sh")
	if err := mp.downloadScript(mp.config.Upgrade.ProxyScriptURL, scriptPath); err != nil {
		mp.logger.WithError(err).Error("下载Proxy升级脚本失败")
		mp.sendErrorAck(messageId, "下载升级脚本失败", err.Error())
		return
	}

	// 设置脚本权限
	if err := os.Chmod(scriptPath, 0755); err != nil {
		mp.logger.WithError(err).Error("设置脚本权限失败")
		mp.sendErrorAck(messageId, "设置脚本权限失败", err.Error())
		return
	}

	// 构建升级参数
	args := []string{}

	// 如果消息中指定了目标版本，则传递给脚本
	if upgradeMsg.TargetVersion != "" {
		args = append(args, "--target-version="+upgradeMsg.TargetVersion)
	}

	// 根据配置设置备份选项
	if !mp.config.Upgrade.BackupEnabled {
		args = append(args, "--no-backup")
	}

	// 设置超时参数
	args = append(args, fmt.Sprintf("--timeout=%d", mp.config.Upgrade.Timeout))

	// 执行升级脚本
	cmd := exec.CommandContext(ctx, scriptPath, args...)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		mp.logger.WithError(err).WithField("output", string(output)).Error("Proxy升级失败")
		mp.sendErrorAck(messageId, "Proxy升级失败", fmt.Sprintf("%s\n输出: %s", err.Error(), string(output)))
		return
	}

	mp.logger.Info("Proxy升级成功完成")
	mp.sendSuccessAck(messageId, fmt.Sprintf("Proxy升级成功，输出: %s", string(output)))
}

// downloadScript 下载脚本文件
func (mp *MessageProcessor) downloadScript(downloadURL, scriptPath string) error {
	mp.logger.WithFields(map[string]interface{}{
		"download_url": downloadURL,
		"script_path":  scriptPath,
	}).Info("下载升级脚本")

	// 创建HTTP请求
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，HTTP状态码: %d", resp.StatusCode)
	}

	// 创建文件
	file, err := os.Create(scriptPath)
	if err != nil {
		return fmt.Errorf("创建脚本文件失败: %w", err)
	}
	defer file.Close()

	// 复制内容
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("写入脚本文件失败: %w", err)
	}

	mp.logger.Info("升级脚本下载完成")
	return nil
}
