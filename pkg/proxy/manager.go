package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/cert"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/errors"
	"github.com/nspass/nspass-agent/pkg/interfaces"
	"github.com/nspass/nspass-agent/pkg/logging"
	"github.com/nspass/nspass-agent/pkg/proxy/shadowsocks"
	"github.com/nspass/nspass-agent/pkg/proxy/snell"
	"github.com/nspass/nspass-agent/pkg/proxy/trojan"
)

// ProxyInterface 代理接口
type ProxyInterface interface {
	Configure(config *model.EgressItem) error
	Start() error
	Stop() error
	Restart() error
	Status() (string, error)
	IsInstalled() bool
	IsRunning() bool
}

// InstanceState 代理实例状态
type InstanceState int

const (
	InstanceStateStopped InstanceState = iota
	InstanceStateStarting
	InstanceStateRunning
	InstanceStateStopping
	InstanceStateError
)

func (s InstanceState) String() string {
	switch s {
	case InstanceStateStopped:
		return "stopped"
	case InstanceStateStarting:
		return "starting"
	case InstanceStateRunning:
		return "running"
	case InstanceStateStopping:
		return "stopping"
	case InstanceStateError:
		return "error"
	default:
		return "unknown"
	}
}

// ProxyInstance 代理实例
type ProxyInstance struct {
	ID        string
	Type      model.EgressMode
	Config    *model.EgressItem
	Proxy     ProxyInterface
	State     InstanceState
	LastError error
	StartTime time.Time
	mu        sync.RWMutex
}

// GetState 获取代理状态
func (pi *ProxyInstance) GetState() InstanceState {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return pi.State
}

// SetState 设置代理状态
func (pi *ProxyInstance) SetState(state InstanceState) {
	pi.mu.Lock()
	pi.State = state
	pi.mu.Unlock()
}

// SetError 设置错误
func (pi *ProxyInstance) SetError(err error) {
	pi.mu.Lock()
	pi.LastError = err
	pi.State = InstanceStateError
	pi.mu.Unlock()
}

// GetStatus 获取状态信息
func (pi *ProxyInstance) GetStatus() map[string]interface{} {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	status := map[string]interface{}{
		"id":         pi.ID,
		"type":       pi.Type.String(),
		"state":      pi.State.String(),
		"start_time": pi.StartTime,
	}

	if pi.LastError != nil {
		status["last_error"] = pi.LastError.Error()
	}

	if pi.Proxy != nil {
		if proxyStatus, err := pi.Proxy.Status(); err == nil {
			status["proxy_status"] = proxyStatus
		}
		status["is_running"] = pi.Proxy.IsRunning()
		status["is_installed"] = pi.Proxy.IsInstalled()
	}

	return status
}

// Manager 代理管理器
type Manager struct {
	config      config.ProxyConfig
	logger      interfaces.Logger
	monitor     *ProxyMonitor
	certManager *cert.Manager // 证书管理器

	// 代理实例管理
	instances map[string]*ProxyInstance
	mu        sync.RWMutex

	// 工厂方法
	proxyFactory ProxyFactory

	// 控制相关
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 事件通道
	eventsChan chan ProxyEvent

	// 统计信息
	stats struct {
		mu             sync.RWMutex
		totalProxies   int
		runningProxies int
		stoppedProxies int
		errorProxies   int
		restartCount   int64
		lastUpdateTime time.Time
	}
}

// ProxyEvent 代理事件
type ProxyEvent struct {
	Type      string
	ProxyID   string
	State     InstanceState
	Error     error
	Timestamp time.Time
}

// ProxyFactory 代理工厂接口
type ProxyFactory interface {
	CreateProxy(config *model.EgressItem) (ProxyInterface, error)
	SupportedTypes() []model.EgressMode
}

// DefaultProxyFactory 默认代理工厂
type DefaultProxyFactory struct {
	certConfig *cert.Config
}

// CreateProxy 创建代理实例
func (f *DefaultProxyFactory) CreateProxy(config *model.EgressItem) (ProxyInterface, error) {
	switch config.EgressMode {
	case model.EgressMode_EGRESS_MODE_SS2022:
		// 使用现有的shadowsocks包
		return shadowsocks.New(config), nil
	case model.EgressMode_EGRESS_MODE_TROJAN:
		// 使用现有的trojan包
		trojanProxy := trojan.New(config)
		// trojan必须有证书配置
		if f.certConfig == nil {
			return nil, fmt.Errorf("trojan代理必须配置证书管理器")
		}
		trojanProxy.SetCertConfig(f.certConfig)
		return trojanProxy, nil
	case model.EgressMode_EGRESS_MODE_SNELL:
		// 使用现有的snell包
		return snell.New(config), nil
	default:
		return nil, errors.New(errors.ErrorTypeProxy, "PROXY_TYPE_UNSUPPORTED",
			fmt.Sprintf("不支持的代理类型: %s", config.EgressMode))
	}
}

// SupportedTypes 支持的代理类型
func (f *DefaultProxyFactory) SupportedTypes() []model.EgressMode {
	return []model.EgressMode{
		model.EgressMode_EGRESS_MODE_SS2022,
		model.EgressMode_EGRESS_MODE_TROJAN,
		model.EgressMode_EGRESS_MODE_SNELL,
	}
}

// NewManager 创建代理管理器
func NewManager(cfg config.ProxyConfig, logger interfaces.Logger, certConfig *cert.Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	manager := &Manager{
		config:       cfg,
		logger:       logger,
		certManager:  nil, // 不再需要全局的certManager
		instances:    make(map[string]*ProxyInstance),
		proxyFactory: &DefaultProxyFactory{certConfig: certConfig},
		ctx:          ctx,
		cancel:       cancel,
		eventsChan:   make(chan ProxyEvent, 100),
	}

	// 创建监控器
	manager.monitor = NewProxyMonitor(cfg.Monitor)

	// 启动事件处理
	manager.wg.Add(1)
	go manager.eventLoop()

	return manager
}

// SetProxyFactory 设置代理工厂
func (em *Manager) SetProxyFactory(factory ProxyFactory) {
	em.proxyFactory = factory
}

// UpdateProxies 更新代理配置
func (em *Manager) UpdateProxies(configs []*model.EgressItem) error {
	em.logger.WithField("count", len(configs)).Info("更新代理配置")

	em.mu.Lock()
	defer em.mu.Unlock()

	// 记录现有代理ID
	existingIDs := make(map[string]bool)
	for id := range em.instances {
		existingIDs[id] = true
	}

	// 处理新配置
	for _, config := range configs {
		proxyID := config.EgressId
		delete(existingIDs, proxyID)

		if instance, exists := em.instances[proxyID]; exists {
			// 更新现有代理
			if err := em.updateProxyInstance(instance, config); err != nil {
				em.logger.WithError(err).WithField("proxy_id", proxyID).Error("更新代理失败")
				continue
			}
		} else {
			// 创建新代理
			if err := em.createProxyInstance(config); err != nil {
				em.logger.WithError(err).WithField("proxy_id", proxyID).Error("创建代理失败")
				continue
			}
		}
	}

	// 删除不再需要的代理
	for proxyID := range existingIDs {
		if err := em.removeProxyInstance(proxyID); err != nil {
			em.logger.WithError(err).WithField("proxy_id", proxyID).Error("删除代理失败")
		}
	}

	em.updateStats()
	return nil
}

// createProxyInstance 创建代理实例
func (em *Manager) createProxyInstance(config *model.EgressItem) error {
	proxy, err := em.proxyFactory.CreateProxy(config)
	if err != nil {
		return errors.Wrap(err, errors.ErrorTypeProxy, "PROXY_CREATE_FAILED", "创建代理失败")
	}

	instance := &ProxyInstance{
		ID:     config.EgressId,
		Type:   config.EgressMode,
		Config: config,
		Proxy:  proxy,
		State:  InstanceStateStopped,
	}

	em.instances[config.EgressId] = instance

	// 配置代理
	if err := proxy.Configure(config); err != nil {
		instance.SetError(err)
		return errors.Wrap(err, errors.ErrorTypeProxy, "PROXY_CONFIGURE_FAILED", "配置代理失败")
	}

	em.logger.WithField("proxy_id", config.EgressId).
		WithField("proxy_type", config.EgressMode).
		Info("代理实例创建成功")

	em.emitEvent("created", config.EgressId, InstanceStateStopped, nil)
	return nil
}

// updateProxyInstance 更新代理实例
func (em *Manager) updateProxyInstance(instance *ProxyInstance, config *model.EgressItem) error {
	// 检查配置是否有变化
	if em.configEquals(instance.Config, config) {
		return nil
	}

	em.logger.WithField("proxy_id", config.EgressId).Info("代理配置有变化，重新配置")

	// 停止代理
	if instance.GetState() == InstanceStateRunning {
		if err := em.stopProxyInstance(instance); err != nil {
			return err
		}
	}

	// 更新配置
	instance.Config = config
	if err := instance.Proxy.Configure(config); err != nil {
		instance.SetError(err)
		return errors.Wrap(err, errors.ErrorTypeProxy, "PROXY_RECONFIGURE_FAILED", "重新配置代理失败")
	}

	em.emitEvent("updated", config.EgressId, instance.GetState(), nil)
	return nil
}

// removeProxyInstance 删除代理实例
func (em *Manager) removeProxyInstance(proxyID string) error {
	instance, exists := em.instances[proxyID]
	if !exists {
		return nil
	}

	// 停止代理
	if instance.GetState() == InstanceStateRunning {
		if err := em.stopProxyInstance(instance); err != nil {
			em.logger.WithError(err).WithField("proxy_id", proxyID).Warn("停止代理失败")
		}
	}

	delete(em.instances, proxyID)
	em.logger.WithField("proxy_id", proxyID).Info("代理实例已删除")

	em.emitEvent("removed", proxyID, InstanceStateStopped, nil)
	return nil
}

// StartProxy 启动代理
func (em *Manager) StartProxy(proxyID string) error {
	em.mu.RLock()
	instance, exists := em.instances[proxyID]
	em.mu.RUnlock()

	if !exists {
		return errors.New(errors.ErrorTypeProxy, "PROXY_NOT_FOUND", "代理未找到")
	}

	return em.startProxyInstance(instance)
}

// StopProxy 停止代理
func (em *Manager) StopProxy(proxyID string) error {
	em.mu.RLock()
	instance, exists := em.instances[proxyID]
	em.mu.RUnlock()

	if !exists {
		return errors.New(errors.ErrorTypeProxy, "PROXY_NOT_FOUND", "代理未找到")
	}

	return em.stopProxyInstance(instance)
}

// RestartProxy 重启代理
func (em *Manager) RestartProxy(proxyID string) error {
	if err := em.StopProxy(proxyID); err != nil {
		return err
	}

	// 等待一小段时间确保完全停止
	time.Sleep(1 * time.Second)

	return em.StartProxy(proxyID)
}

// RestartAll 重启所有代理
func (em *Manager) RestartAll() error {
	em.mu.RLock()
	proxyIDs := make([]string, 0, len(em.instances))
	for proxyID := range em.instances {
		proxyIDs = append(proxyIDs, proxyID)
	}
	em.mu.RUnlock()

	em.logger.Info("开始重启所有代理服务")

	var lastError error
	successCount := 0
	errorCount := 0

	for _, proxyID := range proxyIDs {
		em.logger.WithField("proxy_id", proxyID).Debug("重启代理服务")

		if err := em.RestartProxy(proxyID); err != nil {
			errorCount++
			lastError = err
			em.logger.WithError(err).WithField("proxy_id", proxyID).Error("重启代理失败")
		} else {
			successCount++
			em.logger.WithField("proxy_id", proxyID).Info("代理重启成功")
		}
	}

	em.logger.WithFields(map[string]interface{}{
		"success_count": successCount,
		"error_count":   errorCount,
		"total_count":   len(proxyIDs),
	}).Info("代理重启完成")

	if errorCount > 0 {
		return fmt.Errorf("重启代理失败，成功: %d, 失败: %d, 最后错误: %v", successCount, errorCount, lastError)
	}

	return nil
}

// startProxyInstance 启动代理实例
func (em *Manager) startProxyInstance(instance *ProxyInstance) error {
	if instance.GetState() == InstanceStateRunning {
		return nil
	}

	instance.SetState(InstanceStateStarting)
	em.emitEvent("starting", instance.ID, InstanceStateStarting, nil)

	if err := instance.Proxy.Start(); err != nil {
		instance.SetError(err)
		em.emitEvent("start_failed", instance.ID, InstanceStateError, err)
		return errors.Wrap(err, errors.ErrorTypeProxy, "PROXY_START_FAILED", "启动代理失败")
	}

	instance.SetState(InstanceStateRunning)
	instance.StartTime = time.Now()
	em.emitEvent("started", instance.ID, InstanceStateRunning, nil)

	em.logger.WithField("proxy_id", instance.ID).Info("代理启动成功")
	return nil
}

// stopProxyInstance 停止代理实例
func (em *Manager) stopProxyInstance(instance *ProxyInstance) error {
	if instance.GetState() == InstanceStateStopped {
		return nil
	}

	instance.SetState(InstanceStateStopping)
	em.emitEvent("stopping", instance.ID, InstanceStateStopping, nil)

	if err := instance.Proxy.Stop(); err != nil {
		instance.SetError(err)
		em.emitEvent("stop_failed", instance.ID, InstanceStateError, err)
		return errors.Wrap(err, errors.ErrorTypeProxy, "PROXY_STOP_FAILED", "停止代理失败")
	}

	instance.SetState(InstanceStateStopped)
	em.emitEvent("stopped", instance.ID, InstanceStateStopped, nil)

	em.logger.WithField("proxy_id", instance.ID).Info("代理停止成功")
	return nil
}

// emitEvent 发送事件
func (em *Manager) emitEvent(eventType, proxyID string, state InstanceState, err error) {
	event := ProxyEvent{
		Type:      eventType,
		ProxyID:   proxyID,
		State:     state,
		Error:     err,
		Timestamp: time.Now(),
	}

	select {
	case em.eventsChan <- event:
	default:
		em.logger.Warn("事件通道已满，丢弃事件")
	}
}

// eventLoop 事件处理循环
func (em *Manager) eventLoop() {
	defer em.wg.Done()

	for {
		select {
		case <-em.ctx.Done():
			return
		case event := <-em.eventsChan:
			em.handleEvent(event)
		}
	}
}

// handleEvent 处理事件
func (em *Manager) handleEvent(event ProxyEvent) {
	em.logger.WithField("event_type", event.Type).
		WithField("proxy_id", event.ProxyID).
		WithField("state", event.State.String()).
		Debug("处理代理事件")

	// 更新统计信息
	em.updateStats()

	// 如果是错误事件，可以在这里添加告警逻辑
	if event.Error != nil {
		em.logger.WithError(event.Error).
			WithField("proxy_id", event.ProxyID).
			Error("代理事件包含错误")
	}
}

// configEquals 比较配置是否相等
func (em *Manager) configEquals(config1, config2 *model.EgressItem) bool {
	// 简化的配置比较，实际应该比较所有相关字段
	return config1.EgressConfig == config2.EgressConfig &&
		config1.EgressMode == config2.EgressMode
}

// updateStats 更新统计信息
func (em *Manager) updateStats() {
	em.stats.mu.Lock()
	defer em.stats.mu.Unlock()

	em.stats.totalProxies = len(em.instances)
	em.stats.runningProxies = 0
	em.stats.stoppedProxies = 0
	em.stats.errorProxies = 0

	for _, instance := range em.instances {
		switch instance.GetState() {
		case InstanceStateRunning:
			em.stats.runningProxies++
		case InstanceStateStopped:
			em.stats.stoppedProxies++
		case InstanceStateError:
			em.stats.errorProxies++
		}
	}

	em.stats.lastUpdateTime = time.Now()
}

// GetStatus 获取管理器状态
func (em *Manager) GetStatus() map[string]interface{} {
	em.mu.RLock()
	defer em.mu.RUnlock()

	em.stats.mu.RLock()
	defer em.stats.mu.RUnlock()

	proxies := make(map[string]interface{})
	for id, instance := range em.instances {
		proxies[id] = instance.GetStatus()
	}

	return map[string]interface{}{
		"total_proxies":    em.stats.totalProxies,
		"running_proxies":  em.stats.runningProxies,
		"stopped_proxies":  em.stats.stoppedProxies,
		"error_proxies":    em.stats.errorProxies,
		"restart_count":    em.stats.restartCount,
		"last_update_time": em.stats.lastUpdateTime,
		"proxies":          proxies,
	}
}

// GetProxyStats 获取代理统计信息（兼容原接口）
func (em *Manager) GetProxyStats() map[string]interface{} {
	return em.GetStatus()
}

// GetProxyStatus 获取指定代理状态（兼容原接口）
func (em *Manager) GetProxyStatus(proxyID string) (string, error) {
	em.mu.RLock()
	instance, exists := em.instances[proxyID]
	em.mu.RUnlock()

	if !exists {
		return "", errors.New(errors.ErrorTypeProxy, "PROXY_NOT_FOUND", "代理未找到")
	}

	return instance.GetState().String(), nil
}

// StartMonitor 启动监控（兼容原接口）
func (em *Manager) StartMonitor() error {
	em.logger.Info("启动代理监控")
	if em.monitor != nil {
		return em.monitor.Start()
	}
	return nil
}

// StopMonitor 停止监控（兼容原接口）
func (em *Manager) StopMonitor() error {
	em.logger.Info("停止代理监控")
	if em.monitor != nil {
		return em.monitor.Stop()
	}
	return nil
}

// Stop 停止管理器
func (em *Manager) Stop() error {
	em.logger.Info("停止代理管理器")

	em.cancel()

	// 停止所有代理
	em.mu.RLock()
	for _, instance := range em.instances {
		if instance.GetState() == InstanceStateRunning {
			em.stopProxyInstance(instance)
		}
	}
	em.mu.RUnlock()

	// 停止监控器
	if em.monitor != nil {
		em.monitor.Stop()
	}

	em.wg.Wait()
	close(em.eventsChan)

	return nil
}

// RestartProxyForDomain 重启使用指定域名证书的代理
func (em *Manager) RestartProxyForDomain(domain string) error {
	em.mu.RLock()
	defer em.mu.RUnlock()

	var restartedProxies []string
	var errors []error

	// 遍历所有代理实例，找到使用该域名的代理
	for egressID, instance := range em.instances {
		// 检查代理配置中是否包含该域名
		if em.proxyUsesDomain(instance, domain) {
			em.logger.Info("重启使用域名证书的代理", logging.StandardFields{
				Custom: map[string]interface{}{
					"egress_id": egressID,
					"domain":    domain,
				},
			})

			// 重启代理
			if err := em.stopProxyInstance(instance); err != nil {
				errors = append(errors, fmt.Errorf("停止代理 %s 失败: %w", egressID, err))
				continue
			}

			if err := em.startProxyInstance(instance); err != nil {
				errors = append(errors, fmt.Errorf("启动代理 %s 失败: %w", egressID, err))
				continue
			}

			restartedProxies = append(restartedProxies, egressID)
		}
	}

	if len(errors) > 0 {
		em.logger.Error("部分代理重启失败", logging.StandardFields{
			Custom: map[string]interface{}{
				"domain":            domain,
				"restarted_proxies": restartedProxies,
				"failed_count":      len(errors),
			},
		})
		// 返回第一个错误
		return errors[0]
	}

	em.logger.Info("域名相关代理重启完成", logging.StandardFields{
		Custom: map[string]interface{}{
			"domain":            domain,
			"restarted_proxies": restartedProxies,
		},
	})

	return nil
}

// proxyUsesDomain 检查代理是否使用指定域名
func (em *Manager) proxyUsesDomain(instance *ProxyInstance, domain string) bool {
	// 检查代理配置中是否包含指定域名
	if instance.Config == nil {
		return false
	}

	// 根据代理类型检查域名使用情况
	switch instance.Config.EgressMode {
	case model.EgressMode_EGRESS_MODE_TROJAN:
		// 对于trojan代理，检查配置中的域名字段
		return em.checkTrojanDomain(instance.Config, domain)
	default:
		// 其他代理类型暂不支持域名检查
		return false
	}
}

// checkTrojanDomain 检查trojan代理是否使用指定域名
func (em *Manager) checkTrojanDomain(config *model.EgressItem, domain string) bool {
	if config.EgressConfig == "" {
		return false
	}

	// 解析trojan配置
	var trojanConfig map[string]interface{}
	if err := json.Unmarshal([]byte(config.EgressConfig), &trojanConfig); err != nil {
		em.logger.Warn("解析trojan配置失败", logging.StandardFields{
			Error: err,
			Custom: map[string]interface{}{
				"egress_id": config.EgressId,
			},
		})
		return false
	}

	// 检查域名字段
	if configDomain, ok := trojanConfig["domain"].(string); ok {
		return configDomain == domain
	}

	// 检查SNI字段（可能也包含域名）
	if sni, ok := trojanConfig["sni"].(string); ok {
		return sni == domain
	}

	return false
}
