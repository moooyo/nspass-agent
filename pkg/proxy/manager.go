package proxy

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/nspass/nspass-agent/pkg/proxy/shadowsocks"
	"github.com/nspass/nspass-agent/pkg/proxy/snell"
	"github.com/nspass/nspass-agent/pkg/proxy/trojan"
	"github.com/sirupsen/logrus"
)

// ProxyInterface 代理接口
type ProxyInterface interface {
	Configure(config *model.EgressItem) error
	Start() error
	Stop() error
	Status() (string, error)
	IsInstalled() bool
	IsRunning() bool
}

// Manager 代理管理器
type Manager struct {
	proxies map[string]ProxyInterface
	monitor *ProxyMonitor // 进程监控器
	mu      sync.RWMutex
}

// NewManager 创建新的代理管理器
func NewManager(cfg config.ProxyConfig) *Manager {
	manager := &Manager{
		proxies: make(map[string]ProxyInterface),
		monitor: NewProxyMonitor(cfg.Monitor), // 初始化监控器
	}

	logger.LogStartup("proxy-manager", "1.0", map[string]interface{}{
		"bin_path":        cfg.BinPath,
		"config_path":     cfg.ConfigPath,
		"enabled_types":   cfg.EnabledTypes,
		"auto_start":      cfg.AutoStart,
		"restart_on_fail": cfg.RestartOnFail,
		"monitor_config":  cfg.Monitor,
	})

	// 确保必要的目录存在
	if err := os.MkdirAll(cfg.ConfigPath, 0755); err != nil {
		logger.LogError(err, "创建代理配置目录失败", logrus.Fields{
			"config_path": cfg.ConfigPath,
		})
	}

	// 启动监控器
	if err := manager.monitor.Start(); err != nil {
		logger.LogError(err, "启动代理监控器失败", nil)
	}

	return manager
}

// getProxyInstance 获取代理实例
func (m *Manager) getProxyInstance(config *model.EgressItem) (ProxyInterface, error) {
	log := logger.GetProxyLogger()

	// 检查类型是否支持
	supported := false
	if config.EgressMode == model.EgressMode_EGRESS_MODE_SS2022 {
		supported = true
	}

	if !supported {
		log.WithFields(logrus.Fields{
			"proxy_type": config.EgressMode,
		}).Warn("不支持的代理类型")
		return nil, fmt.Errorf("不支持的代理类型: %s", config.EgressMode)
	}

	// 创建代理实例
	switch config.EgressMode {
	case model.EgressMode_EGRESS_MODE_SS2022:
		return shadowsocks.New(config), nil
	case model.EgressMode_EGRESS_MODE_TROJAN:
		return trojan.New(config), nil
	case model.EgressMode_EGRESS_MODE_SNELL:
		return snell.New(config), nil
	default:
		log.WithField("proxy_type", config.EgressMode).Warn("不支持的代理类型")
		return nil, fmt.Errorf("不支持的代理类型: %s", config.EgressMode)
	}
}

// UpdateProxies 更新代理配置
func (m *Manager) UpdateProxies(configs []*model.EgressItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	startTime := time.Now()
	log := logger.GetProxyLogger()

	log.WithField("config_count", len(configs)).Info("开始更新代理配置")

	successCount := 0
	errorCount := 0
	var errors []string

	// 记录当前配置的代理ID
	configuredProxyIDs := make(map[string]bool)

	for _, cfg := range configs {
		proxyLog := log.WithFields(logrus.Fields{
			"proxy_id":   cfg.EgressId,
			"proxy_type": cfg.EgressMode,
			"proxy_name": cfg.EgressName,
		})

		configuredProxyIDs[cfg.EgressId] = true

		proxyLog.Info("开始配置代理")

		if err := m.configureProxy(cfg); err != nil {
			errorCount++
			errorMsg := fmt.Sprintf("配置代理 %s(%s) 失败: %v", cfg.EgressMode, cfg.EgressId, err)
			errors = append(errors, errorMsg)
			logger.LogError(err, "配置代理失败", logrus.Fields{
				"proxy_id":   cfg.EgressId,
				"proxy_type": cfg.EgressMode,
				"proxy_name": cfg.EgressName,
			})
		} else {
			successCount++
			proxyLog.Info("代理配置完成")
		}
	}

	// 移除不在配置中的代理
	for proxyID := range m.proxies {
		if !configuredProxyIDs[proxyID] {
			log.WithField("proxy_id", proxyID).Info("移除不在配置中的代理")
			if proxy := m.proxies[proxyID]; proxy != nil {
				if err := proxy.Stop(); err != nil {
					logger.LogError(err, "停止移除的代理失败", logrus.Fields{
						"proxy_id": proxyID,
					})
				}
			}
			delete(m.proxies, proxyID)
			// 从监控器中取消注册
			if m.monitor != nil {
				m.monitor.UnregisterProxy(proxyID)
			}
		}
	}

	duration := time.Since(startTime)

	// 记录性能指标
	logger.LogPerformance("proxy_update", duration, logrus.Fields{
		"total_proxies": len(configs),
		"success_count": successCount,
		"error_count":   errorCount,
	})

	log.WithFields(logrus.Fields{
		"total_proxies": len(configs),
		"success_count": successCount,
		"error_count":   errorCount,
		"duration_ms":   duration.Milliseconds(),
	}).Info("代理配置更新完成")

	if errorCount > 0 {
		return fmt.Errorf("部分代理配置失败，成功: %d, 失败: %d, 错误: %v",
			successCount, errorCount, errors)
	}

	return nil
}

// configureProxy 配置单个代理
func (m *Manager) configureProxy(cfg *model.EgressItem) error {
	log := logger.GetProxyLogger().WithFields(logrus.Fields{
		"proxy_id":   cfg.EgressId,
		"proxy_type": cfg.EgressMode,
	})

	// 停止已存在的代理
	if existing, exists := m.proxies[cfg.EgressId]; exists {
		log.Debug("停止现有代理")
		if err := existing.Stop(); err != nil {
			logger.LogError(err, "停止现有代理失败", logrus.Fields{
				"proxy_id":   cfg.EgressId,
				"proxy_type": cfg.EgressMode,
			})
		}
	}

	// 获取代理实例
	proxy, err := m.getProxyInstance(cfg)
	if err != nil {
		return err
	}

	// 检查是否已安装
	if !proxy.IsInstalled() {
		logger.LogError(fmt.Errorf("代理软件未安装"), "代理软件未安装", logrus.Fields{
			"proxy_type": cfg.EgressMode,
		})
		return fmt.Errorf("%s 代理软件未安装，请检查安装脚本是否正确执行", cfg.EgressMode)
	}

	// 配置代理
	log.Debug("开始配置代理")
	if err := proxy.Configure(cfg); err != nil {
		logger.LogError(err, "配置代理失败", logrus.Fields{
			"proxy_id":   cfg.EgressId,
			"proxy_type": cfg.EgressMode,
		})
		return fmt.Errorf("配置 %s 代理失败: %w", cfg.EgressMode, err)
	}

	// 启动代理服务
	log.Debug("启动代理服务")
	if err := proxy.Start(); err != nil {
		logger.LogError(err, "启动代理服务失败", logrus.Fields{
			"proxy_id":   cfg.EgressId,
			"proxy_type": cfg.EgressMode,
		})
		return fmt.Errorf("启动 %s 代理服务失败: %w", cfg.EgressMode, err)
	}

	// 保存代理实例
	m.proxies[cfg.EgressId] = proxy

	// 注册到监控器
	m.monitor.RegisterProxy(cfg, proxy)

	// 记录状态变更
	logger.LogStateChange("proxy", "unconfigured", "configured",
		fmt.Sprintf("代理 %s(%s) 配置完成", cfg.EgressMode, cfg.EgressId))

	log.Info("代理配置和启动完成")
	return nil
}

// GetStatus 获取所有代理状态
func (m *Manager) GetStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	log := logger.GetProxyLogger()
	statuses := make(map[string]interface{})

	for id, proxy := range m.proxies {
		status, err := proxy.Status()
		if err != nil {
			logger.LogError(err, "获取代理状态失败", logrus.Fields{
				"proxy_id": id,
			})
			statuses[id] = "error"
		} else {
			statuses[id] = status
		}
	}

	summary := map[string]interface{}{
		"total_proxies": len(m.proxies),
		"statuses":      statuses,
	}

	log.WithField("summary", summary).Debug("代理状态获取完成")
	return summary
}

// RestartAll 重启所有代理服务
func (m *Manager) RestartAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	startTime := time.Now()
	log := logger.GetProxyLogger()

	log.Info("开始重启所有代理服务")

	successCount := 0
	errorCount := 0
	var errors []string

	for proxyType, proxy := range m.proxies {
		proxyLog := log.WithField("proxy_type", proxyType)

		proxyLog.Debug("重启代理服务")
		if err := proxy.Stop(); err != nil {
			errorMsg := fmt.Sprintf("停止 %s 代理失败: %v", proxyType, err)
			errors = append(errors, errorMsg)
			logger.LogError(err, "停止代理失败", logrus.Fields{
				"proxy_type": proxyType,
			})
		}

		if err := proxy.Start(); err != nil {
			errorCount++
			errorMsg := fmt.Sprintf("启动 %s 代理失败: %v", proxyType, err)
			errors = append(errors, errorMsg)
			logger.LogError(err, "启动代理失败", logrus.Fields{
				"proxy_type": proxyType,
			})
		} else {
			successCount++
			proxyLog.Info("代理重启成功")
		}
	}

	duration := time.Since(startTime)

	// 记录性能指标
	logger.LogPerformance("proxy_restart_all", duration, logrus.Fields{
		"total_proxies": len(m.proxies),
		"success_count": successCount,
		"error_count":   errorCount,
	})

	log.WithFields(logrus.Fields{
		"success_count": successCount,
		"error_count":   errorCount,
		"duration_ms":   duration.Milliseconds(),
	}).Info("代理服务重启完成")

	if errorCount > 0 {
		return fmt.Errorf("部分代理重启失败，成功: %d, 失败: %d, 错误: %v",
			successCount, errorCount, errors)
	}

	return nil
}

// StopAll 停止所有代理服务
func (m *Manager) StopAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	startTime := time.Now()
	log := logger.GetProxyLogger()

	log.Info("开始停止所有代理服务")

	successCount := 0
	errorCount := 0
	var errors []string

	for proxyType, proxy := range m.proxies {
		proxyLog := log.WithField("proxy_type", proxyType)

		proxyLog.Debug("停止代理服务")
		if err := proxy.Stop(); err != nil {
			errorCount++
			errorMsg := fmt.Sprintf("停止 %s 代理失败: %v", proxyType, err)
			errors = append(errors, errorMsg)
			logger.LogError(err, "停止代理失败", logrus.Fields{
				"proxy_type": proxyType,
			})
		} else {
			successCount++
			proxyLog.Info("代理停止成功")
		}
	}

	duration := time.Since(startTime)

	// 记录性能指标
	logger.LogPerformance("proxy_stop_all", duration, logrus.Fields{
		"total_proxies": len(m.proxies),
		"success_count": successCount,
		"error_count":   errorCount,
	})

	log.WithFields(logrus.Fields{
		"success_count": successCount,
		"error_count":   errorCount,
		"duration_ms":   duration.Milliseconds(),
	}).Info("代理服务停止完成")

	if errorCount > 0 {
		return fmt.Errorf("部分代理停止失败，成功: %d, 失败: %d, 错误: %v",
			successCount, errorCount, errors)
	}

	return nil
}

// GetMonitorStatus 获取监控器状态
func (m *Manager) GetMonitorStatus() map[string]interface{} {
	if m.monitor == nil {
		return map[string]interface{}{
			"enabled": false,
			"running": false,
		}
	}

	return m.monitor.GetMonitorSummary()
}

// GetProxyMonitorState 获取指定代理的监控状态
func (m *Manager) GetProxyMonitorState(proxyID string) (*ProxyState, bool) {
	if m.monitor == nil {
		return nil, false
	}

	return m.monitor.GetProxyState(proxyID)
}

// EnableProxyMonitor 启用指定代理的监控
func (m *Manager) EnableProxyMonitor(proxyID string) {
	if m.monitor != nil {
		m.monitor.EnableProxy(proxyID)
	}
}

// DisableProxyMonitor 禁用指定代理的监控
func (m *Manager) DisableProxyMonitor(proxyID string) {
	if m.monitor != nil {
		m.monitor.DisableProxy(proxyID)
	}
}

// StopMonitor 停止监控器
func (m *Manager) StopMonitor() error {
	if m.monitor != nil {
		return m.monitor.Stop()
	}
	return nil
}

// RemoveProxy 移除代理（包括从监控器中移除）
func (m *Manager) RemoveProxy(proxyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 从代理管理器中移除
	if proxy, exists := m.proxies[proxyID]; exists {
		// 先停止代理
		if err := proxy.Stop(); err != nil {
			logger.LogError(err, "停止代理失败", logrus.Fields{
				"proxy_id": proxyID,
			})
		}

		delete(m.proxies, proxyID)

		// 从监控器中移除
		if m.monitor != nil {
			m.monitor.UnregisterProxy(proxyID)
		}

		logger.GetProxyLogger().WithField("proxy_id", proxyID).Info("代理已移除")
		return nil
	}

	return fmt.Errorf("代理 %s 不存在", proxyID)
}
