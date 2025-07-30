package cert

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nspass/nspass-agent/pkg/interfaces"
	"github.com/nspass/nspass-agent/pkg/logging"
)

// ProxyRestarter 代理重启器接口
type ProxyRestarter interface {
	// RestartProxyForDomain 重启使用指定域名证书的代理
	RestartProxyForDomain(domain string) error
}

// ExpiryChecker 证书过期检查器
type ExpiryChecker struct {
	certManager    *Manager
	logger         interfaces.Logger
	interval       time.Duration
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	running        bool
	mu             sync.RWMutex
	domainDNSMap   map[string]uint32 // 域名到DNS配置ID的映射
	proxyRestarter ProxyRestarter    // 代理重启器接口
}

// ExpiryCheckerConfig 过期检查器配置
type ExpiryCheckerConfig struct {
	CheckInterval time.Duration // 检查间隔，默认24小时
}

// NewExpiryChecker 创建证书过期检查器
func NewExpiryChecker(certManager *Manager, config *ExpiryCheckerConfig, proxyRestarter ProxyRestarter) *ExpiryChecker {
	interval := 24 * time.Hour // 默认每天检查一次
	if config != nil && config.CheckInterval > 0 {
		interval = config.CheckInterval
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ExpiryChecker{
		certManager:    certManager,
		logger:         logging.GetComponentLogger("cert-expiry-checker"),
		interval:       interval,
		ctx:            ctx,
		cancel:         cancel,
		domainDNSMap:   make(map[string]uint32),
		proxyRestarter: proxyRestarter,
	}
}

// Start 启动过期检查器
func (ec *ExpiryChecker) Start() error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if ec.running {
		return nil
	}

	ec.logger.Info("启动证书过期检查器", logging.StandardFields{
		Custom: map[string]interface{}{
			"check_interval": ec.interval,
		},
	})

	ec.running = true
	ec.wg.Add(1)
	go ec.checkLoop()

	return nil
}

// Stop 停止过期检查器
func (ec *ExpiryChecker) Stop() error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if !ec.running {
		return nil
	}

	ec.logger.Info("停止证书过期检查器", logging.StandardFields{})

	ec.cancel()
	ec.running = false

	// 等待检查循环结束
	ec.wg.Wait()

	ec.logger.Info("证书过期检查器已停止", logging.StandardFields{})
	return nil
}

// IsRunning 检查是否正在运行
func (ec *ExpiryChecker) IsRunning() bool {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.running
}

// CheckNow 立即执行一次检查
func (ec *ExpiryChecker) CheckNow() error {
	ec.logger.Info("执行立即证书过期检查", logging.StandardFields{})
	return ec.performCheck()
}

// checkLoop 检查循环
func (ec *ExpiryChecker) checkLoop() {
	defer ec.wg.Done()

	// 启动时立即执行一次检查
	if err := ec.performCheck(); err != nil {
		ec.logger.Error("初始证书过期检查失败", logging.StandardFields{
			Error: err,
		})
	}

	ticker := time.NewTicker(ec.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ec.ctx.Done():
			ec.logger.Debug("证书过期检查循环退出", logging.StandardFields{})
			return
		case <-ticker.C:
			if err := ec.performCheck(); err != nil {
				ec.logger.Error("证书过期检查失败", logging.StandardFields{
					Error: err,
				})
			}
		}
	}
}

// performCheck 执行检查
func (ec *ExpiryChecker) performCheck() error {
	ec.logger.Debug("开始执行证书过期检查", logging.StandardFields{})

	// 获取即将过期的证书
	expiringCerts, err := ec.certManager.CheckExpiring()
	if err != nil {
		return err
	}

	if len(expiringCerts) == 0 {
		ec.logger.Info("没有即将过期的证书", logging.StandardFields{})
		return nil
	}

	ec.logger.Warn("发现即将过期的证书", logging.StandardFields{
		Custom: map[string]interface{}{
			"expiring_count": len(expiringCerts),
		},
	})

	// 尝试续期每个即将过期的证书
	var renewalErrors []error
	successCount := 0

	for _, certInfo := range expiringCerts {
		ec.logger.Info("尝试续期证书", logging.StandardFields{
			Custom: map[string]interface{}{
				"domain":     certInfo.Domain,
				"expires_at": certInfo.ExpiresAt,
			},
		})

		// 获取域名对应的DNS配置ID
		ec.mu.RLock()
		dnsConfigID, exists := ec.domainDNSMap[certInfo.Domain]
		ec.mu.RUnlock()

		if !exists {
			renewalErrors = append(renewalErrors, fmt.Errorf("未找到域名 %s 对应的DNS配置", certInfo.Domain))
			ec.logger.Warn("跳过证书续期", logging.StandardFields{
				Custom: map[string]interface{}{
					"domain": certInfo.Domain,
					"reason": "未找到对应的DNS配置ID",
				},
			})
			continue
		}

		// 尝试续期证书
		newCertInfo, err := ec.certManager.EnsureCertificate(certInfo.Domain, dnsConfigID)
		if err != nil {
			renewalErrors = append(renewalErrors, fmt.Errorf("续期域名 %s 的证书失败: %w", certInfo.Domain, err))
			ec.logger.Error("证书续期失败", logging.StandardFields{
				Error: err,
				Custom: map[string]interface{}{
					"domain":        certInfo.Domain,
					"dns_config_id": dnsConfigID,
				},
			})
			continue
		}

		if newCertInfo != nil {
			successCount++
			ec.logger.Info("证书续期成功", logging.StandardFields{
				Custom: map[string]interface{}{
					"domain":         certInfo.Domain,
					"old_expires_at": certInfo.ExpiresAt,
					"new_expires_at": newCertInfo.ExpiresAt,
				},
			})

			// 通知代理管理器重启相关代理
			ec.notifyProxyRestart(certInfo.Domain)
		}

	}

	// 记录续期结果
	ec.logger.Info("证书过期检查完成", logging.StandardFields{
		Custom: map[string]interface{}{
			"total_expiring": len(expiringCerts),
			"success_count":  successCount,
			"error_count":    len(renewalErrors),
		},
	})

	// 如果有续期错误，记录详细信息
	for _, err := range renewalErrors {
		ec.logger.Error("证书续期错误", logging.StandardFields{
			Error: err,
		})
	}

	return nil
}

// notifyProxyRestart 通知代理管理器重启相关代理
func (ec *ExpiryChecker) notifyProxyRestart(domain string) {
	if ec.proxyRestarter != nil {
		if err := ec.proxyRestarter.RestartProxyForDomain(domain); err != nil {
			ec.logger.Error("重启代理失败", logging.StandardFields{
				Error: err,
				Custom: map[string]interface{}{
					"domain": domain,
				},
			})
		} else {
			ec.logger.Info("代理重启成功", logging.StandardFields{
				Custom: map[string]interface{}{
					"domain": domain,
				},
			})
		}
	} else {
		ec.logger.Warn("代理重启器未设置", logging.StandardFields{
			Custom: map[string]interface{}{
				"domain": domain,
			},
		})
	}
}

// UpdateDomainDNSMapping 更新域名到DNS配置ID的映射
func (ec *ExpiryChecker) UpdateDomainDNSMapping(domainDNSMap map[string]uint32) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.domainDNSMap = make(map[string]uint32)
	for domain, dnsConfigID := range domainDNSMap {
		ec.domainDNSMap[domain] = dnsConfigID
	}

	ec.logger.Info("更新域名DNS映射", logging.StandardFields{
		Custom: map[string]interface{}{
			"mapping_count": len(ec.domainDNSMap),
		},
	})
}

// AddDomainDNSMapping 添加域名到DNS配置ID的映射
func (ec *ExpiryChecker) AddDomainDNSMapping(domain string, dnsConfigID uint32) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.domainDNSMap[domain] = dnsConfigID

	ec.logger.Debug("添加域名DNS映射", logging.StandardFields{
		Custom: map[string]interface{}{
			"domain":        domain,
			"dns_config_id": dnsConfigID,
		},
	})
}
