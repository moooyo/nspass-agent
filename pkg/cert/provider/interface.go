package provider

import (
	"context"
	"fmt"
	"time"
)

// DNSProvider DNS提供商接口，用于DNS-01 challenge验证
type DNSProvider interface {
	// CreateTXTRecord 创建TXT记录用于ACME DNS-01 challenge
	// domain: 要验证的域名
	// name: TXT记录名称（通常是 _acme-challenge.domain）
	// value: TXT记录值
	CreateTXTRecord(ctx context.Context, domain, name, value string) error

	// DeleteTXTRecord 删除TXT记录
	// domain: 要验证的域名
	// name: TXT记录名称
	// value: TXT记录值
	DeleteTXTRecord(ctx context.Context, domain, name, value string) error

	// GetTXTRecord 获取TXT记录（用于验证记录是否已生效）
	// domain: 要验证的域名
	// name: TXT记录名称
	GetTXTRecord(ctx context.Context, domain, name string) ([]string, error)

	// WaitForPropagation 等待DNS记录传播
	// domain: 要验证的域名
	// name: TXT记录名称
	// value: 期望的TXT记录值
	// timeout: 等待超时时间
	WaitForPropagation(ctx context.Context, domain, name, value string, timeout time.Duration) error

	// GetProviderName 获取提供商名称
	GetProviderName() string
}

// DNSProviderConfig DNS提供商配置接口
type DNSProviderConfig interface {
	// Validate 验证配置是否有效
	Validate() error

	// GetProviderType 获取提供商类型
	GetProviderType() string
}

// CloudflareConfig Cloudflare DNS提供商配置
type CloudflareConfig struct {
	APIToken string `json:"api_token"` // Cloudflare API Token
	Email    string `json:"email"`     // Cloudflare账户邮箱（可选，使用API Token时不需要）
	APIKey   string `json:"api_key"`   // Cloudflare Global API Key（可选，使用API Token时不需要）
}

// Validate 验证Cloudflare配置
func (c *CloudflareConfig) Validate() error {
	if c.APIToken == "" && (c.Email == "" || c.APIKey == "") {
		return fmt.Errorf("必须提供API Token或者Email+API Key组合")
	}
	return nil
}

// GetProviderType 获取提供商类型
func (c *CloudflareConfig) GetProviderType() string {
	return "cloudflare"
}
