package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
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
	// 新增清理方法
	Cleanup() error        // 清理配置文件和PID文件
	GetConfigPath() string // 获取配置文件路径
	GetPIDPath() string    // 获取PID文件路径
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
	// 新增字段用于配置变更事件
	OldConfig *model.EgressItem // 旧配置（用于配置变更事件）
	NewConfig *model.EgressItem // 新配置（用于配置变更事件）
}

// ProxyFactory 代理工厂接口
type ProxyFactory interface {
	CreateProxy(config *model.EgressItem) (ProxyInterface, error)
	SupportedTypes() []model.EgressMode
}

// DefaultProxyFactory 默认代理工厂
type DefaultProxyFactory struct {
	certConfig *cert.Config
	logger     interfaces.Logger
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
		// trojan必须有证书配置，如果没有静态配置，尝试动态创建
		certConfig := f.certConfig
		if certConfig == nil {
			// 尝试动态创建证书配置
			dynamicCertConfig, err := f.createDynamicCertConfig(config)
			if err != nil {
				return nil, fmt.Errorf("trojan代理必须配置证书管理器: %w", err)
			}
			certConfig = dynamicCertConfig
		}
		trojanProxy.SetCertConfig(certConfig)
		return trojanProxy, nil
	case model.EgressMode_EGRESS_MODE_SNELL:
		// 使用现有的snell包
		return snell.New(config), nil
	default:
		return nil, errors.New(errors.ErrorTypeProxy, "PROXY_TYPE_UNSUPPORTED",
			fmt.Sprintf("不支持的代理类型: %s", config.EgressMode))
	}
}

// createDynamicCertConfig 动态创建证书配置
func (f *DefaultProxyFactory) createDynamicCertConfig(config *model.EgressItem) (*cert.Config, error) {
	// 检查是否有DNS配置ID
	if config.DnsConfigId == nil {
		return nil, fmt.Errorf("trojan代理需要dns_config_id来申请证书")
	}

	// 从全局DNS配置管理器获取DNS配置
	dnsConfigMgr := cert.GetGlobalDNSConfigManager()
	dnsConfig, err := dnsConfigMgr.GetDNSConfig(*config.DnsConfigId)
	if err != nil {
		return nil, fmt.Errorf("获取DNS配置失败: %w", err)
	}

	// 使用默认的证书配置，但使用一个通用的email
	// 这里可以使用域名相关的email或者系统默认email
	defaultEmail := fmt.Sprintf("admin@%s", dnsConfig.Domain)

	certConfig := &cert.Config{
		StorePath:       "/etc/nspass-agent/certs", // 使用默认路径
		Email:           defaultEmail,
		ExpiryThreshold: 7 * 24 * time.Hour, // 7天
		UseStaging:      false,              // 生产环境
	}

	if f.logger != nil {
		f.logger.Info("动态创建证书配置", map[string]interface{}{
			"email":         certConfig.Email,
			"domain":        dnsConfig.Domain,
			"dns_config_id": *config.DnsConfigId,
		})
	}

	return certConfig, nil
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
		config:      cfg,
		logger:      logger,
		certManager: nil, // 不再需要全局的certManager
		instances:   make(map[string]*ProxyInstance),
		proxyFactory: &DefaultProxyFactory{
			certConfig: certConfig,
			logger:     logger,
		},
		ctx:        ctx,
		cancel:     cancel,
		eventsChan: make(chan ProxyEvent, 100),
	}

	// 创建监控器
	manager.monitor = NewProxyMonitor(cfg.Monitor)

	// 启动事件处理
	manager.wg.Add(1)
	go manager.eventLoop()

	// 执行启动时的状态同步
	manager.wg.Add(1)
	go manager.syncExistingProxiesOnStartup()

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
		proxyID := fmt.Sprintf("%d", config.Id)
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
		ID:     fmt.Sprintf("%d", config.Id),
		Type:   config.EgressMode,
		Config: config,
		Proxy:  proxy,
		State:  InstanceStateStopped,
	}

	em.instances[fmt.Sprintf("%d", config.Id)] = instance

	// 配置代理
	if err := proxy.Configure(config); err != nil {
		instance.SetError(err)
		return errors.Wrap(err, errors.ErrorTypeProxy, "PROXY_CONFIGURE_FAILED", "配置代理失败")
	}

	em.logger.WithField("proxy_id", fmt.Sprintf("%d", config.Id)).
		WithField("proxy_type", config.EgressMode).
		Info("代理实例创建成功")

	em.emitEvent("created", fmt.Sprintf("%d", config.Id), InstanceStateStopped, nil)

	// 自动启动代理
	if err := em.startProxyInstance(instance); err != nil {
		em.logger.WithError(err).WithField("proxy_id", fmt.Sprintf("%d", config.Id)).
			Error("自动启动代理失败")
		// 不返回错误，因为代理已经创建和配置成功，只是启动失败
		// 监控器会检测到这个问题并尝试重启
	}

	return nil
}

// updateProxyInstance 更新代理实例
func (em *Manager) updateProxyInstance(instance *ProxyInstance, config *model.EgressItem) error {
	// 检查配置是否有变化
	if em.configEquals(instance.Config, config) {
		// 配置没有变化，但确保代理正在运行
		if instance.GetState() != InstanceStateRunning {
			if err := em.startProxyInstance(instance); err != nil {
				em.logger.WithError(err).WithField("proxy_id", fmt.Sprintf("%d", config.Id)).
					Error("启动现有代理失败")
			}
		}
		return nil
	}

	em.logger.WithField("proxy_id", fmt.Sprintf("%d", config.Id)).Info("代理配置有变化，重新配置")

	// 停止代理
	if instance.GetState() == InstanceStateRunning {
		if err := em.stopProxyInstance(instance); err != nil {
			return err
		}
	}

	// 清理旧的配置文件（保留PID文件，因为可能需要重用）
	oldConfigPath := instance.Proxy.GetConfigPath()
	if _, err := os.Stat(oldConfigPath); err == nil {
		if err := os.Remove(oldConfigPath); err != nil {
			em.logger.WithError(err).WithField("config_path", oldConfigPath).Warn("删除旧配置文件失败")
		} else {
			em.logger.WithField("config_path", oldConfigPath).Debug("旧配置文件已删除")
		}
	}

	// 保存旧配置用于事件
	oldConfig := instance.Config

	// 更新配置
	instance.Config = config
	if err := instance.Proxy.Configure(config); err != nil {
		instance.SetError(err)
		return errors.Wrap(err, errors.ErrorTypeProxy, "PROXY_RECONFIGURE_FAILED", "重新配置代理失败")
	}

	// 发送配置变更事件
	em.emitEventWithConfig("config_changed", fmt.Sprintf("%d", config.Id), instance.GetState(), nil, oldConfig, config)

	// 重新启动代理
	if err := em.startProxyInstance(instance); err != nil {
		em.logger.WithError(err).WithField("proxy_id", fmt.Sprintf("%d", config.Id)).
			Error("重新启动代理失败")
		// 不返回错误，因为代理已经重新配置成功，只是启动失败
		// 监控器会检测到这个问题并尝试重启
	}

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

	// 清理配置文件和PID文件
	if err := instance.Proxy.Cleanup(); err != nil {
		em.logger.WithError(err).WithField("proxy_id", proxyID).Warn("清理代理文件失败")
	}

	// 从监控器中注销代理
	if em.monitor != nil {
		em.monitor.UnregisterProxy(proxyID)
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
	em.emitEventWithConfig(eventType, proxyID, state, err, nil, nil)
}

// emitEventWithConfig 发送带配置信息的事件
func (em *Manager) emitEventWithConfig(eventType, proxyID string, state InstanceState, err error, oldConfig, newConfig *model.EgressItem) {
	event := ProxyEvent{
		Type:      eventType,
		ProxyID:   proxyID,
		State:     state,
		Error:     err,
		Timestamp: time.Now(),
		OldConfig: oldConfig,
		NewConfig: newConfig,
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

	// 处理特定事件类型
	switch event.Type {
	case "config_changed":
		em.handleConfigChangeEvent(event)
	case "started":
		em.handleProxyStartedEvent(event)
	case "stopped":
		em.handleProxyStoppedEvent(event)
	case "start_failed", "stop_failed":
		em.handleProxyErrorEvent(event)
	}

	// 更新统计信息
	em.updateStats()

	// 如果是错误事件，可以在这里添加告警逻辑
	if event.Error != nil {
		em.logger.WithError(event.Error).
			WithField("proxy_id", event.ProxyID).
			Error("代理事件包含错误")
	}
}

// handleConfigChangeEvent 处理配置变更事件
func (em *Manager) handleConfigChangeEvent(event ProxyEvent) {
	if event.OldConfig == nil || event.NewConfig == nil {
		return
	}

	em.logger.WithField("proxy_id", event.ProxyID).
		WithField("old_mode", event.OldConfig.EgressMode).
		WithField("new_mode", event.NewConfig.EgressMode).
		Info("代理配置已变更")

	// 记录配置变更的详细信息
	configChanges := em.detectConfigChanges(event.OldConfig, event.NewConfig)
	if len(configChanges) > 0 {
		em.logger.WithField("proxy_id", event.ProxyID).
			WithField("changes", configChanges).
			Info("配置变更详情")
	}
}

// handleProxyStartedEvent 处理代理启动事件
func (em *Manager) handleProxyStartedEvent(event ProxyEvent) {
	em.logger.WithField("proxy_id", event.ProxyID).Info("代理启动成功")

	// 可以在这里添加启动后的额外处理逻辑
	// 例如：注册到监控系统、更新负载均衡器等
}

// handleProxyStoppedEvent 处理代理停止事件
func (em *Manager) handleProxyStoppedEvent(event ProxyEvent) {
	em.logger.WithField("proxy_id", event.ProxyID).Info("代理已停止")

	// 可以在这里添加停止后的额外处理逻辑
	// 例如：从监控系统注销、更新负载均衡器等
}

// handleProxyErrorEvent 处理代理错误事件
func (em *Manager) handleProxyErrorEvent(event ProxyEvent) {
	em.logger.WithError(event.Error).
		WithField("proxy_id", event.ProxyID).
		WithField("event_type", event.Type).
		Error("代理操作失败")

	// 可以在这里添加错误处理逻辑
	// 例如：发送告警、记录到错误日志、尝试恢复等
}

// detectConfigChanges 检测配置变更
func (em *Manager) detectConfigChanges(oldConfig, newConfig *model.EgressItem) []string {
	var changes []string

	if oldConfig.EgressMode != newConfig.EgressMode {
		changes = append(changes, fmt.Sprintf("模式: %s -> %s", oldConfig.EgressMode, newConfig.EgressMode))
	}

	if !em.compareOptionalInt32(oldConfig.Port, newConfig.Port) {
		oldPort := "nil"
		newPort := "nil"
		if oldConfig.Port != nil {
			oldPort = fmt.Sprintf("%d", *oldConfig.Port)
		}
		if newConfig.Port != nil {
			newPort = fmt.Sprintf("%d", *newConfig.Port)
		}
		changes = append(changes, fmt.Sprintf("端口: %s -> %s", oldPort, newPort))
	}

	if !em.compareOptionalString(oldConfig.Password, newConfig.Password) {
		changes = append(changes, "密码已变更")
	}

	if oldConfig.EgressConfig != newConfig.EgressConfig {
		changes = append(changes, "代理配置已变更")
	}

	if !em.compareOptionalUint32(oldConfig.DnsConfigId, newConfig.DnsConfigId) {
		changes = append(changes, "DNS配置ID已变更")
	}

	return changes
}

// configEquals 比较配置是否相等
func (em *Manager) configEquals(config1, config2 *model.EgressItem) bool {
	if config1 == nil || config2 == nil {
		return config1 == config2
	}

	// 比较基本字段
	if config1.Id != config2.Id {
		return false
	}

	if config1.EgressMode != config2.EgressMode {
		return false
	}

	if config1.EgressConfig != config2.EgressConfig {
		return false
	}

	// 比较端口
	if !em.compareOptionalInt32(config1.Port, config2.Port) {
		return false
	}

	// 比较密码
	if !em.compareOptionalString(config1.Password, config2.Password) {
		return false
	}

	// 比较DNS配置ID
	if !em.compareOptionalUint32(config1.DnsConfigId, config2.DnsConfigId) {
		return false
	}

	// 比较服务器ID
	if config1.ServerId != config2.ServerId {
		return false
	}

	// 比较出口名称
	if config1.EgressName != config2.EgressName {
		return false
	}

	return true
}

// compareOptionalInt32 比较可选的int32指针
func (em *Manager) compareOptionalInt32(a, b *int32) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// compareOptionalString 比较可选的string指针
func (em *Manager) compareOptionalString(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// compareOptionalUint32 比较可选的uint32指针
func (em *Manager) compareOptionalUint32(a, b *uint32) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// syncExistingProxiesOnStartup 启动时同步现有的proxy状态
func (em *Manager) syncExistingProxiesOnStartup() {
	defer em.wg.Done()

	em.logger.Info("开始同步现有proxy状态")

	// 扫描配置目录，查找现有的配置文件和PID文件
	configDir := em.config.ConfigPath
	if configDir == "" {
		configDir = "/etc/nspass-agent" // 默认配置目录
	}

	// 检查配置目录是否存在
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		em.logger.WithField("config_dir", configDir).Debug("配置目录不存在，跳过状态同步")
		return
	}

	// 读取配置目录中的文件
	files, err := os.ReadDir(configDir)
	if err != nil {
		em.logger.WithError(err).WithField("config_dir", configDir).Error("读取配置目录失败")
		return
	}

	orphanedFiles := make([]string, 0)
	orphanedPIDs := make([]string, 0)

	// 扫描孤立的配置文件和PID文件
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileName := file.Name()

		// 检查是否是代理配置文件
		if em.isProxyConfigFile(fileName) {
			proxyID := em.extractProxyIDFromConfigFile(fileName)
			if proxyID != "" {
				em.mu.RLock()
				_, exists := em.instances[proxyID]
				em.mu.RUnlock()

				if !exists {
					orphanedFiles = append(orphanedFiles, filepath.Join(configDir, fileName))
					em.logger.WithField("config_file", fileName).WithField("proxy_id", proxyID).
						Debug("发现孤立的配置文件")
				}
			}
		}

		// 检查是否是PID文件
		if em.isProxyPIDFile(fileName) {
			proxyID := em.extractProxyIDFromPIDFile(fileName)
			if proxyID != "" {
				em.mu.RLock()
				_, exists := em.instances[proxyID]
				em.mu.RUnlock()

				if !exists {
					pidFilePath := filepath.Join(configDir, fileName)
					// 检查PID文件对应的进程是否还在运行
					if em.isProcessRunning(pidFilePath) {
						em.logger.WithField("pid_file", fileName).WithField("proxy_id", proxyID).
							Warn("发现孤立的运行中进程")
						// 尝试停止孤立进程
						em.stopOrphanedProcess(pidFilePath)
					}
					orphanedPIDs = append(orphanedPIDs, pidFilePath)
				}
			}
		}
	}

	// 清理孤立的文件
	em.cleanupOrphanedFiles(orphanedFiles, orphanedPIDs)

	em.logger.WithField("orphaned_configs", len(orphanedFiles)).
		WithField("orphaned_pids", len(orphanedPIDs)).
		Info("proxy状态同步完成")
}

// isProxyConfigFile 检查是否是代理配置文件
func (em *Manager) isProxyConfigFile(fileName string) bool {
	// 匹配格式：shadowsocks-123.json, trojan-456.json, snell-789.json
	matched, _ := regexp.MatchString(`^(shadowsocks|trojan|snell)-\d+\.(json|conf)$`, fileName)
	return matched
}

// extractProxyIDFromConfigFile 从配置文件名提取代理ID
func (em *Manager) extractProxyIDFromConfigFile(fileName string) string {
	// 使用正则表达式提取ID
	re := regexp.MustCompile(`^(shadowsocks|trojan|snell)-(\d+)\.(json|conf)$`)
	matches := re.FindStringSubmatch(fileName)
	if len(matches) >= 3 {
		return matches[2] // 返回ID部分
	}
	return ""
}

// isProxyPIDFile 检查是否是代理PID文件
func (em *Manager) isProxyPIDFile(fileName string) bool {
	// 匹配格式：shadowsocks-123.pid, trojan-456.pid, snell-789.pid
	matched, _ := regexp.MatchString(`^(shadowsocks|trojan|snell)-\d+\.pid$`, fileName)
	return matched
}

// extractProxyIDFromPIDFile 从PID文件名提取代理ID
func (em *Manager) extractProxyIDFromPIDFile(fileName string) string {
	// 使用正则表达式提取ID
	re := regexp.MustCompile(`^(shadowsocks|trojan|snell)-(\d+)\.pid$`)
	matches := re.FindStringSubmatch(fileName)
	if len(matches) >= 3 {
		return matches[2] // 返回ID部分
	}
	return ""
}

// isProcessRunning 检查PID文件对应的进程是否在运行
func (em *Manager) isProcessRunning(pidFilePath string) bool {
	pidData, err := os.ReadFile(pidFilePath)
	if err != nil {
		return false
	}

	pidStr := strings.TrimSpace(string(pidData))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false
	}

	// 使用kill(pid, 0)检查进程是否存在
	err = syscall.Kill(pid, 0)
	return err == nil
}

// stopOrphanedProcess 停止孤立进程
func (em *Manager) stopOrphanedProcess(pidFilePath string) {
	pidData, err := os.ReadFile(pidFilePath)
	if err != nil {
		return
	}

	pidStr := strings.TrimSpace(string(pidData))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return
	}

	em.logger.WithField("pid", pid).Info("尝试停止孤立进程")

	// 发送TERM信号
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		em.logger.WithError(err).WithField("pid", pid).Warn("停止孤立进程失败")
		return
	}

	// 等待进程退出
	time.Sleep(2 * time.Second)

	// 检查进程是否还在运行
	if err := syscall.Kill(pid, 0); err == nil {
		// 进程仍在运行，发送KILL信号
		em.logger.WithField("pid", pid).Warn("进程未响应TERM信号，发送KILL信号")
		syscall.Kill(pid, syscall.SIGKILL)
	}
}

// cleanupOrphanedFiles 清理孤立的文件
func (em *Manager) cleanupOrphanedFiles(configFiles, pidFiles []string) {
	// 清理配置文件
	for _, configFile := range configFiles {
		if err := os.Remove(configFile); err != nil {
			em.logger.WithError(err).WithField("config_file", configFile).Warn("删除孤立配置文件失败")
		} else {
			em.logger.WithField("config_file", configFile).Info("已删除孤立配置文件")
		}
	}

	// 清理PID文件
	for _, pidFile := range pidFiles {
		if err := os.Remove(pidFile); err != nil {
			em.logger.WithError(err).WithField("pid_file", pidFile).Warn("删除孤立PID文件失败")
		} else {
			em.logger.WithField("pid_file", pidFile).Info("已删除孤立PID文件")
		}
	}
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
				"egress_id": fmt.Sprintf("%d", config.Id),
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
