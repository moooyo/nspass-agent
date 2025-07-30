package cert

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/nspass/nspass-agent/pkg/cert/provider"
	"github.com/nspass/nspass-agent/pkg/interfaces"
	"github.com/nspass/nspass-agent/pkg/logging"
)

// DNSChallengeProvider lego DNS challenge provider适配器
type DNSChallengeProvider struct {
	provider provider.DNSProvider
	logger   interfaces.Logger
}

// NewDNSChallengeProvider 创建DNS challenge provider
func NewDNSChallengeProvider(dnsProvider provider.DNSProvider) *DNSChallengeProvider {
	return &DNSChallengeProvider{
		provider: dnsProvider,
		logger:   logging.GetComponentLogger("dns-challenge"),
	}
}

// Present 创建DNS TXT记录用于challenge验证
func (p *DNSChallengeProvider) Present(domain, token, keyAuth string) error {
	p.logger.Info("开始DNS challenge验证", logging.StandardFields{
		Custom: map[string]interface{}{
			"domain": domain,
			"token":  token,
		},
	})

	// 计算challenge值
	info := dns01.GetChallengeInfo(domain, keyAuth)
	
	// 创建TXT记录
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err := p.provider.CreateTXTRecord(ctx, domain, info.FQDN, info.Value)
	if err != nil {
		p.logger.Error("创建DNS TXT记录失败", logging.StandardFields{
			Error: err,
			Custom: map[string]interface{}{
				"domain": domain,
				"fqdn":   info.FQDN,
				"value":  info.Value,
			},
		})
		return fmt.Errorf("创建DNS TXT记录失败: %w", err)
	}

	// 等待DNS记录传播
	p.logger.Info("等待DNS记录传播", logging.StandardFields{
		Custom: map[string]interface{}{
			"domain": domain,
			"fqdn":   info.FQDN,
		},
	})

	propagationCtx, propagationCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer propagationCancel()

	err = p.provider.WaitForPropagation(propagationCtx, domain, info.FQDN, info.Value, 5*time.Minute)
	if err != nil {
		p.logger.Warn("等待DNS记录传播超时，继续验证", logging.StandardFields{
			Error: err,
			Custom: map[string]interface{}{
				"domain": domain,
				"fqdn":   info.FQDN,
			},
		})
		// 不返回错误，让ACME服务器尝试验证
	}

	p.logger.Info("DNS challenge记录创建完成", logging.StandardFields{
		Custom: map[string]interface{}{
			"domain": domain,
			"fqdn":   info.FQDN,
			"value":  info.Value,
		},
	})

	return nil
}

// CleanUp 清理DNS TXT记录
func (p *DNSChallengeProvider) CleanUp(domain, token, keyAuth string) error {
	p.logger.Info("清理DNS challenge记录", logging.StandardFields{
		Custom: map[string]interface{}{
			"domain": domain,
			"token":  token,
		},
	})

	// 计算challenge值
	info := dns01.GetChallengeInfo(domain, keyAuth)
	
	// 删除TXT记录
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err := p.provider.DeleteTXTRecord(ctx, domain, info.FQDN, info.Value)
	if err != nil {
		p.logger.Warn("删除DNS TXT记录失败", logging.StandardFields{
			Error: err,
			Custom: map[string]interface{}{
				"domain": domain,
				"fqdn":   info.FQDN,
				"value":  info.Value,
			},
		})
		// 不返回错误，因为清理失败不应该影响证书申请
	} else {
		p.logger.Info("DNS challenge记录清理完成", logging.StandardFields{
			Custom: map[string]interface{}{
				"domain": domain,
				"fqdn":   info.FQDN,
			},
		})
	}

	return nil
}

// Timeout 返回DNS传播超时时间
func (p *DNSChallengeProvider) Timeout() (timeout, interval time.Duration) {
	return 5 * time.Minute, 10 * time.Second
}

// extractRootDomain 提取根域名
func extractRootDomain(domain string) string {
	// 移除前缀的点
	domain = strings.TrimPrefix(domain, ".")
	
	// 简单的根域名提取逻辑
	// 对于更复杂的情况，可能需要使用公共后缀列表
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return domain
}
