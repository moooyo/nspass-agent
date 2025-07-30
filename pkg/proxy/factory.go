package proxy

import (
	"fmt"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/logging"
	"github.com/nspass/nspass-agent/pkg/proxy/shadowsocks"
	"github.com/nspass/nspass-agent/pkg/proxy/snell"
	"github.com/nspass/nspass-agent/pkg/proxy/trojan"
)

// egressModeToProxyType 将EgressMode转换为代理类型字符串
func egressModeToProxyType(mode model.EgressMode) string {
	switch mode {
	case model.EgressMode_EGRESS_MODE_SS2022:
		return ProxyTypeShadowsocks
	case model.EgressMode_EGRESS_MODE_TROJAN:
		return ProxyTypeTrojan
	case model.EgressMode_EGRESS_MODE_SNELL:
		return ProxyTypeSnell
	default:
		return ""
	}
}

// SimpleProxyFactory 简单代理工厂
type SimpleProxyFactory struct {
	config config.ProxyConfig
	logger *logging.StandardLogger
}

// NewSimpleProxyFactory 创建简单代理工厂
func NewSimpleProxyFactory(cfg config.ProxyConfig) *SimpleProxyFactory {
	return &SimpleProxyFactory{
		config: cfg,
		logger: logging.NewStandardLogger("proxy-factory"),
	}
}

// CreateProxy 创建代理实例
func (pf *SimpleProxyFactory) CreateProxy(egressItem *model.EgressItem) (ProxyInterface, error) {
	if egressItem == nil {
		return nil, fmt.Errorf("出口配置不能为空")
	}

	// 根据EgressMode确定代理类型
	proxyType := egressModeToProxyType(egressItem.EgressMode)

	// 验证代理类型
	if !IsValidProxyType(proxyType) {
		pf.logger.Error("不支持的代理类型", logging.StandardFields{
			ProxyType: proxyType,
			Custom: map[string]interface{}{
				"supported_types": GetSupportedProxyTypes(),
			},
		})
		return nil, fmt.Errorf("不支持的代理类型: %s", proxyType)
	}

	pf.logger.Info("创建代理实例", logging.StandardFields{
		ProxyType: proxyType,
		Custom: map[string]interface{}{
			"egress_id": egressItem.EgressId,
		},
	})

	// 根据代理类型创建实例
	switch proxyType {
	case ProxyTypeShadowsocks:
		return shadowsocks.New(egressItem), nil
	case ProxyTypeTrojan:
		return trojan.New(egressItem), nil
	case ProxyTypeSnell:
		return snell.New(egressItem), nil
	default:
		return nil, fmt.Errorf("未实现的代理类型: %s", proxyType)
	}
}

// CreateProxyWithConfig 使用自定义配置创建代理实例
func (pf *SimpleProxyFactory) CreateProxyWithConfig(egressItem *model.EgressItem, customConfig config.ProxyConfig) (ProxyInterface, error) {
	// 临时保存原配置
	originalConfig := pf.config

	// 使用自定义配置
	pf.config = customConfig

	// 创建代理
	proxy, err := pf.CreateProxy(egressItem)

	// 恢复原配置
	pf.config = originalConfig

	return proxy, err
}

// GetSupportedTypes 获取支持的代理类型
func (pf *SimpleProxyFactory) GetSupportedTypes() []string {
	return GetSupportedProxyTypes()
}

// ValidateProxyType 验证代理类型
func (pf *SimpleProxyFactory) ValidateProxyType(proxyType string) error {
	if !IsValidProxyType(proxyType) {
		return fmt.Errorf("不支持的代理类型: %s, 支持的类型: %v", proxyType, GetSupportedProxyTypes())
	}
	return nil
}

// ProxyManager 代理管理器
type ProxyManager struct {
	factory *SimpleProxyFactory
	proxies map[string]ProxyInterface
	logger  *logging.StandardLogger
}

// NewProxyManager 创建代理管理器
func NewProxyManager(cfg config.ProxyConfig) *ProxyManager {
	return &ProxyManager{
		factory: NewSimpleProxyFactory(cfg),
		proxies: make(map[string]ProxyInterface),
		logger:  logging.NewStandardLogger("proxy-manager"),
	}
}

// CreateProxy 创建并管理代理实例
func (pm *ProxyManager) CreateProxy(egressItem *model.EgressItem) (ProxyInterface, error) {
	if egressItem == nil {
		return nil, fmt.Errorf("出口配置不能为空")
	}

	egressID := egressItem.EgressId

	// 检查是否已存在
	if existing, exists := pm.proxies[egressID]; exists {
		pm.logger.Warn("代理实例已存在", logging.StandardFields{
			Custom: map[string]interface{}{
				"egress_id": egressID,
			},
		})
		return existing, nil
	}

	// 创建新实例
	proxy, err := pm.factory.CreateProxy(egressItem)
	if err != nil {
		pm.logger.Error("创建代理实例失败", logging.StandardFields{
			Error: err,
			Custom: map[string]interface{}{
				"egress_id": egressID,
			},
		})
		return nil, err
	}

	// 保存实例
	pm.proxies[egressID] = proxy

	pm.logger.Info("代理实例创建成功", logging.StandardFields{
		Custom: map[string]interface{}{
			"egress_id":   egressID,
			"egress_mode": egressItem.EgressMode.String(),
		},
	})

	return proxy, nil
}

// GetProxy 获取代理实例
func (pm *ProxyManager) GetProxy(egressID string) (ProxyInterface, bool) {
	proxy, exists := pm.proxies[egressID]
	return proxy, exists
}

// RemoveProxy 移除代理实例
func (pm *ProxyManager) RemoveProxy(egressID string) error {
	proxy, exists := pm.proxies[egressID]
	if !exists {
		return fmt.Errorf("代理实例不存在: %s", egressID)
	}

	// 停止代理
	if proxy.IsRunning() {
		if err := proxy.Stop(); err != nil {
			pm.logger.Warn("停止代理失败", logging.StandardFields{
				Error: err,
				Custom: map[string]interface{}{
					"egress_id": egressID,
				},
			})
		}
	}

	// 从管理器中移除
	delete(pm.proxies, egressID)

	pm.logger.Info("代理实例已移除", logging.StandardFields{
		Custom: map[string]interface{}{
			"egress_id": egressID,
		},
	})

	return nil
}

// ListProxies 列出所有代理实例
func (pm *ProxyManager) ListProxies() map[string]ProxyInterface {
	result := make(map[string]ProxyInterface)
	for k, v := range pm.proxies {
		result[k] = v
	}
	return result
}

// GetProxyStatus 获取代理状态
func (pm *ProxyManager) GetProxyStatus(egressID string) (string, error) {
	proxy, exists := pm.proxies[egressID]
	if !exists {
		return "", fmt.Errorf("代理实例不存在: %s", egressID)
	}

	return proxy.Status()
}

// StartProxy 启动代理
func (pm *ProxyManager) StartProxy(egressID string) error {
	proxy, exists := pm.proxies[egressID]
	if !exists {
		return fmt.Errorf("代理实例不存在: %s", egressID)
	}

	return proxy.Start()
}

// StopProxy 停止代理
func (pm *ProxyManager) StopProxy(egressID string) error {
	proxy, exists := pm.proxies[egressID]
	if !exists {
		return fmt.Errorf("代理实例不存在: %s", egressID)
	}

	return proxy.Stop()
}

// RestartProxy 重启代理
func (pm *ProxyManager) RestartProxy(egressID string) error {
	proxy, exists := pm.proxies[egressID]
	if !exists {
		return fmt.Errorf("代理实例不存在: %s", egressID)
	}

	return proxy.Restart()
}

// StopAllProxies 停止所有代理
func (pm *ProxyManager) StopAllProxies() error {
	var errors []error

	for egressID, proxy := range pm.proxies {
		if proxy.IsRunning() {
			if err := proxy.Stop(); err != nil {
				pm.logger.Error("停止代理失败", logging.StandardFields{
					Error: err,
					Custom: map[string]interface{}{
						"egress_id": egressID,
					},
				})
				errors = append(errors, fmt.Errorf("停止代理 %s 失败: %w", egressID, err))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("停止部分代理失败: %v", errors)
	}

	return nil
}

// GetFactory 获取代理工厂
func (pm *ProxyManager) GetFactory() *SimpleProxyFactory {
	return pm.factory
}
