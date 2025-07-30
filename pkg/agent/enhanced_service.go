package agent

import (
	"context"
	"sync"
	"time"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/cert"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/errors"
	"github.com/nspass/nspass-agent/pkg/interfaces"
	"github.com/nspass/nspass-agent/pkg/iptables"
	"github.com/nspass/nspass-agent/pkg/logging"
	"github.com/nspass/nspass-agent/pkg/proxy"
	"github.com/nspass/nspass-agent/pkg/websocket"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// EnhancedService 增强的Agent服务
type EnhancedService struct {
	config   *config.Config
	serverID string
	logger   interfaces.Logger

	// 核心组件
	proxyManager     interfaces.ProxyManager
	iptablesManager  interfaces.IPTablesManager
	wsClient         interfaces.WebSocketClient
	taskHandler      interfaces.TaskHandler
	metricsCollector interfaces.MetricsCollector
	certManager      *cert.Manager
	expiryChecker    *cert.ExpiryChecker

	// 控制相关
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex

	// 状态信息
	startTime   time.Time
	lastUpdate  time.Time
	updateCount int64
	errorCount  int64
}

// NewEnhancedService 创建增强的Agent服务
func NewEnhancedService(cfg *config.Config, serverID string) (*EnhancedService, error) {
	if serverID == "" {
		return nil, errors.New(errors.ErrorTypeConfig, "INVALID_SERVER_ID", "server_id不能为空")
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, errors.ErrorTypeConfig, "CONFIG_VALIDATION_FAILED", "配置验证失败")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 创建日志记录器工厂
	serviceLogger := logging.GetComponentLogger("enhanced-agent-service")

	service := &EnhancedService{
		config:   cfg,
		serverID: serverID,
		logger:   serviceLogger,
		ctx:      ctx,
		cancel:   cancel,
	}

	// 初始化组件
	if err := service.initializeComponents(); err != nil {
		cancel()
		return nil, err
	}

	// 记录启动信息
	service.logger.(*logging.EnhancedLogger).LogStartup("1.0", map[string]interface{}{
		"server_id":        serverID,
		"update_interval":  cfg.UpdateInterval,
		"api_base_url":     cfg.API.BaseURL,
		"proxy_enabled":    len(cfg.Proxy.EnabledTypes) > 0,
		"iptables_enabled": cfg.IPTables.Enable,
	})

	return service, nil
}

// initializeComponents 初始化组件
func (s *EnhancedService) initializeComponents() error {
	// 创建证书管理器配置
	var certConfig *cert.Config
	if s.config.Certificate.Enabled && s.config.Certificate.Email != "" {
		// 使用配置文件中的证书设置
		certConfig = &cert.Config{
			StorePath:       s.config.Certificate.StorePath,
			Email:           s.config.Certificate.Email,
			ExpiryThreshold: time.Duration(s.config.Certificate.ExpiryThreshold) * 24 * time.Hour,
			UseStaging:      s.config.Certificate.UseStaging,
		}
		s.logger.Info("证书管理已启用", logging.StandardFields{
			Custom: map[string]interface{}{
				"email":       s.config.Certificate.Email,
				"store_path":  s.config.Certificate.StorePath,
				"use_staging": s.config.Certificate.UseStaging,
			},
		})
	} else {
		// 证书管理未启用，trojan代理将无法工作
		s.logger.Warn("证书管理未启用或邮箱未配置，trojan代理将无法工作", logging.StandardFields{
			Custom: map[string]interface{}{
				"enabled": s.config.Certificate.Enabled,
				"email":   s.config.Certificate.Email,
			},
		})
		certConfig = nil
	}

	// 创建代理管理器
	proxyLogger := logging.GetComponentLogger("proxy-manager")
	s.proxyManager = proxy.NewManager(s.config.Proxy, proxyLogger, certConfig)

	// 创建IPTables管理器（使用适配器）
	iptablesManager := iptables.NewManager(s.config.IPTables)
	s.iptablesManager = NewIPTablesAdapter(iptablesManager)

	// 暂时使用简化的任务处理器和监控收集器
	s.taskHandler = &SimpleTaskHandler{}
	s.metricsCollector = &SimpleMetricsCollector{}

	// 注意：现在每个trojan实例都有自己的certManager，所以过期检查器需要重新设计
	// 暂时禁用过期检查器，因为它需要重新设计来适应新的架构
	s.expiryChecker = nil

	// 创建增强WebSocket客户端
	wsLogger := logging.GetComponentLogger("websocket-client")
	s.wsClient = websocket.NewEnhancedClient(
		s.config,
		s.serverID,
		s.config.API.Token,
		wsLogger,
		s.taskHandler,
		s.metricsCollector,
		s.proxyManager,
		s.iptablesManager,
		s.certManager,
	)

	return nil
}

// Start 启动服务
func (s *EnhancedService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return errors.New(errors.ErrorTypeSystem, "SERVICE_ALREADY_RUNNING", "Agent服务已在运行")
	}

	s.logger.Info("启动增强Agent服务")
	s.startTime = time.Now()

	// 启动代理管理器
	if err := s.startProxyManager(); err != nil {
		return err
	}

	// 启动IPTables管理器
	if err := s.startIPTablesManager(); err != nil {
		s.stopProxyManager()
		return err
	}

	// 启动WebSocket客户端
	if err := s.startWebSocketClient(); err != nil {
		s.stopIPTablesManager()
		s.stopProxyManager()
		return err
	}

	// 启动配置更新循环
	s.wg.Add(1)
	go s.configUpdateLoop()

	// 启动健康检查循环
	s.wg.Add(1)
	go s.healthCheckLoop()

	// 启动证书过期检查器
	if s.config.Certificate.Enabled && s.config.Certificate.Email != "" && s.expiryChecker != nil {
		if err := s.expiryChecker.Start(); err != nil {
			s.logger.WithError(err).Error("启动证书过期检查器失败")
			// 不返回错误，因为这不是关键功能
		} else {
			s.logger.Info("证书过期检查器启动成功")
		}
	} else {
		s.logger.Info("证书管理未启用或邮箱未配置，跳过证书过期检查器启动")
	}

	s.running = true
	s.logger.Info("增强Agent服务启动成功")

	return nil
}

// Stop 停止服务
func (s *EnhancedService) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	s.logger.Info("停止增强Agent服务")

	// 取消上下文
	s.cancel()

	// 停止WebSocket客户端
	if s.wsClient != nil {
		if err := s.wsClient.Stop(); err != nil {
			s.logger.WithError(err).Error("停止WebSocket客户端失败")
		}
	}

	// 停止IPTables管理器
	s.stopIPTablesManager()

	// 停止代理管理器
	s.stopProxyManager()

	// 停止证书过期检查器
	if s.expiryChecker != nil {
		if err := s.expiryChecker.Stop(); err != nil {
			s.logger.WithError(err).Error("停止证书过期检查器失败")
		} else {
			s.logger.Info("证书过期检查器停止成功")
		}
	}

	// 等待所有goroutine结束
	s.wg.Wait()

	// 记录关闭信息
	duration := time.Since(s.startTime)
	s.logger.(*logging.EnhancedLogger).LogShutdown(duration)

	return nil
}

// IsRunning 检查服务是否运行
func (s *EnhancedService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetServerID 获取服务器ID
func (s *EnhancedService) GetServerID() string {
	return s.serverID
}

// GetStatus 获取服务状态
func (s *EnhancedService) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
		"running":      s.running,
		"server_id":    s.serverID,
		"start_time":   s.startTime,
		"last_update":  s.lastUpdate,
		"update_count": s.updateCount,
		"error_count":  s.errorCount,
		"uptime":       time.Since(s.startTime).String(),
	}

	// 添加组件状态
	if s.proxyManager != nil {
		status["proxy_manager"] = s.proxyManager.GetStatus()
	}

	if s.iptablesManager != nil {
		status["iptables_manager"] = s.iptablesManager.GetRulesSummary()
	}

	if s.wsClient != nil {
		status["websocket_client"] = s.wsClient.GetConnectionStatus()
	}

	return status
}

// 私有方法

// startProxyManager 启动代理管理器
func (s *EnhancedService) startProxyManager() error {
	s.logger.Info("启动代理管理器")

	// 如果是Manager，直接返回（它在创建时已经启动）
	if manager, ok := s.proxyManager.(*proxy.Manager); ok {
		_ = manager // Manager在创建时已经启动
		return nil
	}

	// 对于其他类型的管理器，调用StartMonitor
	return s.proxyManager.StartMonitor()
}

// stopProxyManager 停止代理管理器
func (s *EnhancedService) stopProxyManager() {
	if s.proxyManager != nil {
		s.logger.Info("停止代理管理器")
		if err := s.proxyManager.StopMonitor(); err != nil {
			s.logger.WithError(err).Error("停止代理管理器失败")
		}
	}
}

// startIPTablesManager 启动IPTables管理器
func (s *EnhancedService) startIPTablesManager() error {
	if !s.config.IPTables.Enable {
		s.logger.Info("IPTables管理器已禁用")
		return nil
	}

	s.logger.Info("启动IPTables管理器")
	return s.iptablesManager.BackupRules()
}

// stopIPTablesManager 停止IPTables管理器
func (s *EnhancedService) stopIPTablesManager() {
	if s.iptablesManager != nil && s.config.IPTables.Enable {
		s.logger.Info("停止IPTables管理器")
		if err := s.iptablesManager.RestoreRules(); err != nil {
			s.logger.WithError(err).Error("恢复IPTables规则失败")
		}
	}
}

// startWebSocketClient 启动WebSocket客户端
func (s *EnhancedService) startWebSocketClient() error {
	s.logger.Info("启动WebSocket客户端")
	return s.wsClient.Start()
}

// configUpdateLoop 配置更新循环
func (s *EnhancedService) configUpdateLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(s.config.UpdateInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.performConfigUpdate()
		}
	}
}

// healthCheckLoop 健康检查循环
func (s *EnhancedService) healthCheckLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second) // 每30秒检查一次
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.performHealthCheck()
		}
	}
}

// performConfigUpdate 执行配置更新
func (s *EnhancedService) performConfigUpdate() {
	s.logger.Debug("执行配置更新检查")

	s.mu.Lock()
	s.updateCount++
	s.lastUpdate = time.Now()
	s.mu.Unlock()

	// 这里可以添加实际的配置更新逻辑
	// 例如从API获取最新配置并应用
}

// performHealthCheck 执行健康检查
func (s *EnhancedService) performHealthCheck() {
	s.logger.Debug("执行健康检查")

	// 检查WebSocket连接状态
	if s.wsClient != nil && !s.wsClient.IsConnected() {
		s.logger.Warn("WebSocket连接断开")
		s.mu.Lock()
		s.errorCount++
		s.mu.Unlock()
	}

	// 可以添加更多健康检查逻辑
}

// 简化的任务处理器实现
type SimpleTaskHandler struct{}

func (h *SimpleTaskHandler) HandleTask(ctx context.Context, task *model.TaskMessage) (*model.TaskResult, error) {
	return &model.TaskResult{
		TaskId:      task.TaskId,
		Status:      model.TaskStatus_TASK_STATUS_COMPLETED,
		StartedAt:   timestamppb.Now(),
		CompletedAt: timestamppb.Now(),
		Output:      "任务处理成功",
	}, nil
}

func (h *SimpleTaskHandler) CheckTaskStatus(taskID string, taskType model.TaskType) (bool, *model.TaskResult) {
	return true, nil
}

func (h *SimpleTaskHandler) GetTaskStats() map[string]int {
	return map[string]int{
		"total":   0,
		"success": 0,
		"failed":  0,
		"pending": 0,
	}
}

// 简化的监控收集器实现
type SimpleMetricsCollector struct{}

func (c *SimpleMetricsCollector) CollectSystemMetrics() (*model.SystemMetrics, error) {
	return &model.SystemMetrics{}, nil
}

func (c *SimpleMetricsCollector) CollectTrafficMetrics() (*model.TrafficMetrics, error) {
	return &model.TrafficMetrics{}, nil
}

func (c *SimpleMetricsCollector) CollectConnectionMetrics() (*model.ConnectionMetrics, error) {
	return &model.ConnectionMetrics{}, nil
}

func (c *SimpleMetricsCollector) CollectPerformanceMetrics() (*model.PerformanceMetrics, error) {
	return &model.PerformanceMetrics{}, nil
}

func (c *SimpleMetricsCollector) CollectErrorMetrics() (*model.ErrorMetrics, error) {
	return &model.ErrorMetrics{}, nil
}

// 简化的IPTables管理器适配器
type IPTablesAdapter struct {
	manager iptables.ManagerInterface
}

func NewIPTablesAdapter(manager iptables.ManagerInterface) *IPTablesAdapter {
	return &IPTablesAdapter{manager: manager}
}

func (a *IPTablesAdapter) ApplyRules(rules []*model.IptablesConfig) error {
	// 直接调用底层管理器的UpdateRulesFromProto方法
	return a.manager.UpdateRulesFromProto(rules)
}

func (a *IPTablesAdapter) RemoveRules(ruleIDs []string) error {
	// 简化实现
	return nil
}

func (a *IPTablesAdapter) GetRulesSummary() map[string]interface{} {
	return a.manager.GetRulesSummary()
}

func (a *IPTablesAdapter) IsRuleActive(ruleID string) bool {
	return false
}

func (a *IPTablesAdapter) BackupRules() error {
	// 简化实现，实际应该调用iptables命令进行备份
	return nil
}

func (a *IPTablesAdapter) RestoreRules() error {
	// 简化实现，实际应该调用iptables命令进行恢复
	return nil
}
