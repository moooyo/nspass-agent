package cert

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/cert/provider"
	"github.com/nspass/nspass-agent/pkg/interfaces"
	"github.com/nspass/nspass-agent/pkg/logging"
)

var (
	globalDNSConfigManager *DNSConfigManager
	once                   sync.Once
)

// DNSConfigManager DNS配置管理器
// 管理全局的DNS配置和提供商配置，供各个证书管理器实例使用
type DNSConfigManager struct {
	mu              sync.RWMutex
	dnsConfigs      map[uint32]*model.DnsConfig
	providerConfigs map[uint32]*model.DnsProviderConfig
	logger          interfaces.Logger
}

// NewDNSConfigManager 创建DNS配置管理器
func NewDNSConfigManager() *DNSConfigManager {
	return &DNSConfigManager{
		dnsConfigs:      make(map[uint32]*model.DnsConfig),
		providerConfigs: make(map[uint32]*model.DnsProviderConfig),
		logger:          logging.GetComponentLogger("dns-config-manager"),
	}
}

// UpdateConfigs 更新DNS配置和提供商配置
func (dcm *DNSConfigManager) UpdateConfigs(dnsConfigs []*model.DnsConfig, providerConfigs []*model.DnsProviderConfig) {
	dcm.mu.Lock()
	defer dcm.mu.Unlock()

	// 更新DNS配置
	dcm.dnsConfigs = make(map[uint32]*model.DnsConfig)
	for _, config := range dnsConfigs {
		dcm.dnsConfigs[config.Id] = config
	}

	// 更新提供商配置
	dcm.providerConfigs = make(map[uint32]*model.DnsProviderConfig)
	for _, config := range providerConfigs {
		dcm.providerConfigs[config.Id] = config
	}

	dcm.logger.Info("更新DNS配置", logging.StandardFields{
		Custom: map[string]interface{}{
			"dns_configs":      len(dcm.dnsConfigs),
			"provider_configs": len(dcm.providerConfigs),
		},
	})
}

// GetDNSProvider 根据DNS配置ID获取对应的DNS提供商
func (dcm *DNSConfigManager) GetDNSProvider(dnsConfigID uint32) (provider.DNSProvider, error) {
	dcm.mu.RLock()
	defer dcm.mu.RUnlock()

	// 获取DNS配置
	dnsConfig, exists := dcm.dnsConfigs[dnsConfigID]
	if !exists {
		return nil, ErrDNSConfigNotFound{ConfigID: dnsConfigID}
	}

	// 获取提供商配置
	providerConfig, exists := dcm.providerConfigs[dnsConfig.ProviderId]
	if !exists {
		return nil, ErrDNSProviderNotFound{ProviderID: dnsConfig.ProviderId}
	}

	// 创建DNS提供商
	return dcm.createDNSProvider(providerConfig)
}

// GetDNSConfig 获取DNS配置
func (dcm *DNSConfigManager) GetDNSConfig(dnsConfigID uint32) (*model.DnsConfig, error) {
	dcm.mu.RLock()
	defer dcm.mu.RUnlock()

	config, exists := dcm.dnsConfigs[dnsConfigID]
	if !exists {
		return nil, ErrDNSConfigNotFound{ConfigID: dnsConfigID}
	}

	return config, nil
}

// createDNSProvider 创建DNS提供商
func (dcm *DNSConfigManager) createDNSProvider(config *model.DnsProviderConfig) (provider.DNSProvider, error) {
	switch config.Provider {
	case model.DnsProvider_DNS_PROVIDER_CLOUDFLARE:
		var cfConfig provider.CloudflareConfig
		if err := json.Unmarshal([]byte(config.Config), &cfConfig); err != nil {
			return nil, fmt.Errorf("解析Cloudflare配置失败: %w", err)
		}
		return provider.NewCloudflareProvider(&cfConfig)
	default:
		return nil, fmt.Errorf("不支持的DNS提供商: %s", config.Provider)
	}
}

// ErrDNSConfigNotFound DNS配置未找到错误
type ErrDNSConfigNotFound struct {
	ConfigID uint32
}

func (e ErrDNSConfigNotFound) Error() string {
	return fmt.Sprintf("DNS配置未找到: %d", e.ConfigID)
}

// ErrDNSProviderNotFound DNS提供商未找到错误
type ErrDNSProviderNotFound struct {
	ProviderID uint32
}

func (e ErrDNSProviderNotFound) Error() string {
	return fmt.Sprintf("DNS提供商未找到: %d", e.ProviderID)
}

// GetGlobalDNSConfigManager 获取全局DNS配置管理器
func GetGlobalDNSConfigManager() *DNSConfigManager {
	once.Do(func() {
		globalDNSConfigManager = NewDNSConfigManager()
	})
	return globalDNSConfigManager
}
