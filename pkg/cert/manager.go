package cert

import (
	"fmt"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/interfaces"
	"github.com/nspass/nspass-agent/pkg/logging"
)

const (
	// DefaultExpiryThreshold 默认过期阈值（7天）
	DefaultExpiryThreshold = 7 * 24 * time.Hour
	// ACMEProductionURL Let's Encrypt生产环境URL
	ACMEProductionURL = "https://acme-v02.api.letsencrypt.org/directory"
	// ACMEStagingURL Let's Encrypt测试环境URL
	ACMEStagingURL = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// Manager 证书管理器
type Manager struct {
	store           CertificateStore
	logger          interfaces.Logger
	expiryThreshold time.Duration
	email           string
	useStaging      bool
	acmeUser        *ACMEUser
	acmeClient      *lego.Client
	dnsConfigMgr    *DNSConfigManager // DNS配置管理器
}

// Config 证书管理器配置
type Config struct {
	StorePath       string        // 证书存储路径
	Email           string        // ACME账户邮箱
	ExpiryThreshold time.Duration // 过期阈值
	UseStaging      bool          // 是否使用测试环境
}

// NewManager 创建证书管理器
func NewManager(config *Config) (*Manager, error) {
	if config.Email == "" {
		return nil, fmt.Errorf("ACME账户邮箱不能为空")
	}

	// 创建证书存储
	store, err := NewFileCertificateStore(config.StorePath)
	if err != nil {
		return nil, fmt.Errorf("创建证书存储失败: %w", err)
	}

	expiryThreshold := config.ExpiryThreshold
	if expiryThreshold == 0 {
		expiryThreshold = DefaultExpiryThreshold
	}

	manager := &Manager{
		store:           store,
		logger:          logging.GetComponentLogger("cert-manager"),
		expiryThreshold: expiryThreshold,
		email:           config.Email,
		useStaging:      config.UseStaging,
		dnsConfigMgr:    GetGlobalDNSConfigManager(),
	}

	// 初始化ACME客户端
	if err := manager.initACMEClient(); err != nil {
		return nil, fmt.Errorf("初始化ACME客户端失败: %w", err)
	}

	return manager, nil
}

// 删除SetExpiryChecker方法，不再需要

// UpdateDNSProviders 更新DNS提供商配置
// 现在委托给全局DNS配置管理器
func (m *Manager) UpdateDNSProviders(dnsConfigs []*model.DnsConfig, providerConfigs []*model.DnsProviderConfig) error {
	m.logger.Info("更新DNS提供商配置", logging.StandardFields{
		Custom: map[string]interface{}{
			"dns_configs":      len(dnsConfigs),
			"provider_configs": len(providerConfigs),
		},
	})

	// 委托给全局DNS配置管理器
	m.dnsConfigMgr.UpdateConfigs(dnsConfigs, providerConfigs)

	return nil
}

// EnsureCertificate 确保域名有有效的证书
func (m *Manager) EnsureCertificate(domain string, dnsConfigID uint32) (*CertificateInfo, error) {
	m.logger.Info("确保证书有效", logging.StandardFields{
		Custom: map[string]interface{}{
			"domain":        domain,
			"dns_config_id": dnsConfigID,
		},
	})

	// 检查证书是否存在且有效
	isExpiring, err := m.store.IsCertificateExpiring(domain, m.expiryThreshold)
	if err != nil {
		return nil, fmt.Errorf("检查证书过期状态失败: %w", err)
	}

	if !isExpiring {
		// 证书存在且未过期，直接返回
		info, err := m.store.GetCertificate(domain)
		if err != nil {
			return nil, fmt.Errorf("获取证书信息失败: %w", err)
		}
		m.logger.Info("证书有效，无需更新", logging.StandardFields{
			Custom: map[string]interface{}{
				"domain": domain,
			},
		})
		return info, nil
	}

	// 需要申请新证书
	m.logger.Info("证书不存在或即将过期，开始申请新证书", logging.StandardFields{
		Custom: map[string]interface{}{
			"domain": domain,
		},
	})
	return m.obtainCertificate(domain, dnsConfigID)
}

// obtainCertificate 申请证书
func (m *Manager) obtainCertificate(domain string, dnsConfigID uint32) (*CertificateInfo, error) {
	// 从全局DNS配置管理器获取DNS提供商
	dnsProvider, err := m.dnsConfigMgr.GetDNSProvider(dnsConfigID)
	if err != nil {
		return nil, fmt.Errorf("获取DNS提供商失败: %w", err)
	}

	m.logger.Info("开始申请证书", logging.StandardFields{
		Custom: map[string]interface{}{
			"domain":        domain,
			"dns_config_id": dnsConfigID,
		},
	})

	// 创建DNS challenge provider适配器
	challengeProvider := NewDNSChallengeProvider(dnsProvider)

	// 设置DNS challenge
	if err = m.acmeClient.Challenge.SetDNS01Provider(challengeProvider); err != nil {
		return nil, fmt.Errorf("设置DNS challenge失败: %w", err)
	}

	// 申请证书
	request := certificate.ObtainRequest{
		Domains: []string{domain},
		Bundle:  true,
	}

	certificates, err := m.acmeClient.Certificate.Obtain(request)
	if err != nil {
		return nil, fmt.Errorf("申请证书失败: %w", err)
	}

	// 保存证书
	info, err := m.store.SaveCertificate(domain, certificates.Certificate, certificates.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("保存证书失败: %w", err)
	}

	m.logger.Info("证书申请成功", logging.StandardFields{
		Custom: map[string]interface{}{
			"domain":     domain,
			"expires_at": info.ExpiresAt,
			"issuer":     info.Issuer,
		},
	})

	return info, nil
}

// CheckExpiring 检查即将过期的证书
func (m *Manager) CheckExpiring() ([]*CertificateInfo, error) {
	m.logger.Info("检查即将过期的证书", logging.StandardFields{})

	certificates, err := m.store.ListCertificates()
	if err != nil {
		return nil, fmt.Errorf("获取证书列表失败: %w", err)
	}

	var expiring []*CertificateInfo
	for _, cert := range certificates {
		timeUntilExpiry := time.Until(cert.ExpiresAt)
		if timeUntilExpiry <= m.expiryThreshold {
			expiring = append(expiring, cert)
			m.logger.Warn("证书即将过期", logging.StandardFields{
				Custom: map[string]interface{}{
					"domain":            cert.Domain,
					"expires_at":        cert.ExpiresAt,
					"time_until_expiry": timeUntilExpiry,
				},
			})
		}
	}

	m.logger.Info("过期检查完成", logging.StandardFields{
		Custom: map[string]interface{}{
			"expiring_count": len(expiring),
		},
	})
	return expiring, nil
}

// GetCertificate 获取证书信息
func (m *Manager) GetCertificate(domain string) (*CertificateInfo, error) {
	return m.store.GetCertificate(domain)
}

// createDNSProvider方法已移动到DNSConfigManager中

// initACMEClient 初始化ACME客户端
func (m *Manager) initACMEClient() error {
	// 创建ACME用户
	acmeUser, err := NewACMEUser(m.email)
	if err != nil {
		return fmt.Errorf("创建ACME用户失败: %w", err)
	}

	// 选择ACME服务器URL
	acmeURL := ACMEProductionURL
	if m.useStaging {
		acmeURL = ACMEStagingURL
	}

	// 创建ACME客户端配置
	config := lego.NewConfig(acmeUser)
	config.CADirURL = acmeURL
	// 使用默认的RSA密钥类型

	// 创建客户端
	client, err := lego.NewClient(config)
	if err != nil {
		return fmt.Errorf("创建ACME客户端失败: %w", err)
	}

	// 注册用户
	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return fmt.Errorf("注册ACME用户失败: %w", err)
	}

	acmeUser.Registration = reg
	m.acmeUser = acmeUser
	m.acmeClient = client

	m.logger.Info("ACME客户端初始化成功", logging.StandardFields{
		Custom: map[string]interface{}{
			"email":       m.email,
			"acme_url":    acmeURL,
			"use_staging": m.useStaging,
		},
	})

	return nil
}
