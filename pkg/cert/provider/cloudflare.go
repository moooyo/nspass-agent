package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nspass/nspass-agent/pkg/interfaces"
	"github.com/nspass/nspass-agent/pkg/logging"
	"github.com/sirupsen/logrus"
)

const (
	cloudflareAPIBase = "https://api.cloudflare.com/client/v4"
)

// CloudflareProvider Cloudflare DNS提供商实现
type CloudflareProvider struct {
	config *CloudflareConfig
	client *http.Client
	logger interfaces.Logger
}

// CloudflareZone Cloudflare Zone信息
type CloudflareZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CloudflareDNSRecord Cloudflare DNS记录
type CloudflareDNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

// CloudflareAPIResponse Cloudflare API响应
type CloudflareAPIResponse struct {
	Success bool                 `json:"success"`
	Errors  []CloudflareAPIError `json:"errors"`
	Result  json.RawMessage      `json:"result"`
}

// CloudflareAPIError Cloudflare API错误
type CloudflareAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewCloudflareProvider 创建Cloudflare DNS提供商
func NewCloudflareProvider(config *CloudflareConfig) (*CloudflareProvider, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("Cloudflare配置验证失败: %w", err)
	}

	return &CloudflareProvider{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logging.GetComponentLogger("cloudflare-dns"),
	}, nil
}

// GetProviderName 获取提供商名称
func (p *CloudflareProvider) GetProviderName() string {
	return "cloudflare"
}

// CreateTXTRecord 创建TXT记录
func (p *CloudflareProvider) CreateTXTRecord(ctx context.Context, domain, name, value string) error {
	p.logger.WithFields(logrus.Fields{
		"domain": domain,
		"name":   name,
		"value":  value,
	}).Info("创建TXT记录")

	// 获取Zone ID
	zoneID, err := p.getZoneID(ctx, domain)
	if err != nil {
		return fmt.Errorf("获取Zone ID失败: %w", err)
	}

	// 创建DNS记录
	record := CloudflareDNSRecord{
		Type:    "TXT",
		Name:    name,
		Content: value,
		TTL:     120, // 2分钟TTL，加快传播速度
	}

	recordData, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("序列化DNS记录失败: %w", err)
	}

	url := fmt.Sprintf("%s/zones/%s/dns_records", cloudflareAPIBase, zoneID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(recordData))
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	p.setAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("发送HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	if _, err := p.checkAPIResponse(resp); err != nil {
		return fmt.Errorf("创建TXT记录失败: %w", err)
	}

	p.logger.WithField("domain", domain).Info("TXT记录创建成功")
	return nil
}

// DeleteTXTRecord 删除TXT记录
func (p *CloudflareProvider) DeleteTXTRecord(ctx context.Context, domain, name, value string) error {
	p.logger.WithFields(logrus.Fields{
		"domain": domain,
		"name":   name,
		"value":  value,
	}).Info("删除TXT记录")

	// 获取Zone ID
	zoneID, err := p.getZoneID(ctx, domain)
	if err != nil {
		return fmt.Errorf("获取Zone ID失败: %w", err)
	}

	// 查找要删除的记录
	records, err := p.getDNSRecords(ctx, zoneID, "TXT", name)
	if err != nil {
		return fmt.Errorf("查找DNS记录失败: %w", err)
	}

	// 删除匹配的记录
	for _, record := range records {
		if record.Content == value {
			if err := p.deleteDNSRecord(ctx, zoneID, record.ID); err != nil {
				return fmt.Errorf("删除DNS记录失败: %w", err)
			}
		}
	}

	p.logger.WithField("domain", domain).Info("TXT记录删除成功")
	return nil
}

// GetTXTRecord 获取TXT记录
func (p *CloudflareProvider) GetTXTRecord(ctx context.Context, domain, name string) ([]string, error) {
	// 获取Zone ID
	zoneID, err := p.getZoneID(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("获取Zone ID失败: %w", err)
	}

	// 查找TXT记录
	records, err := p.getDNSRecords(ctx, zoneID, "TXT", name)
	if err != nil {
		return nil, fmt.Errorf("查找DNS记录失败: %w", err)
	}

	var values []string
	for _, record := range records {
		values = append(values, record.Content)
	}

	return values, nil
}

// WaitForPropagation 等待DNS记录传播
func (p *CloudflareProvider) WaitForPropagation(ctx context.Context, domain, name, value string, timeout time.Duration) error {
	p.logger.WithFields(logrus.Fields{
		"domain":  domain,
		"name":    name,
		"value":   value,
		"timeout": timeout,
	}).Info("等待DNS记录传播")

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待DNS记录传播超时")
		case <-ticker.C:
			values, err := p.GetTXTRecord(ctx, domain, name)
			if err != nil {
				p.logger.WithError(err).Warn("检查DNS记录传播失败")
				continue
			}

			for _, v := range values {
				if v == value {
					p.logger.Info("DNS记录传播完成")
					return nil
				}
			}

			p.logger.Debug("DNS记录尚未传播，继续等待")
		}
	}
}

// getZoneID 获取域名对应的Zone ID
func (p *CloudflareProvider) getZoneID(ctx context.Context, domain string) (string, error) {
	// 提取根域名
	rootDomain := p.extractRootDomain(domain)

	url := fmt.Sprintf("%s/zones?name=%s", cloudflareAPIBase, rootDomain)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	p.setAuthHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := p.checkAPIResponse(resp)
	if err != nil {
		return "", fmt.Errorf("获取Zone信息失败: %w", err)
	}

	var apiResp CloudflareAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("解析API响应失败: %w", err)
	}

	var zones []CloudflareZone
	if err := json.Unmarshal(apiResp.Result, &zones); err != nil {
		return "", fmt.Errorf("解析Zone信息失败: %w", err)
	}

	if len(zones) == 0 {
		return "", fmt.Errorf("未找到域名 %s 对应的Zone", rootDomain)
	}

	return zones[0].ID, nil
}

// getDNSRecords 获取DNS记录
func (p *CloudflareProvider) getDNSRecords(ctx context.Context, zoneID, recordType, name string) ([]CloudflareDNSRecord, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records?type=%s&name=%s", cloudflareAPIBase, zoneID, recordType, name)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	p.setAuthHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := p.checkAPIResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("获取DNS记录失败: %w", err)
	}

	var apiResp CloudflareAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("解析API响应失败: %w", err)
	}

	var records []CloudflareDNSRecord
	if err := json.Unmarshal(apiResp.Result, &records); err != nil {
		return nil, fmt.Errorf("解析DNS记录失败: %w", err)
	}

	return records, nil
}

// deleteDNSRecord 删除DNS记录
func (p *CloudflareProvider) deleteDNSRecord(ctx context.Context, zoneID, recordID string) error {
	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", cloudflareAPIBase, zoneID, recordID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	p.setAuthHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("发送HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	_, err = p.checkAPIResponse(resp)
	return err
}

// setAuthHeaders 设置认证头
func (p *CloudflareProvider) setAuthHeaders(req *http.Request) {
	if p.config.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.APIToken)
	}
}

// checkAPIResponse 检查API响应并返回body内容
func (p *CloudflareProvider) checkAPIResponse(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP错误 %d: %s", resp.StatusCode, string(body))
	}

	var apiResp CloudflareAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("解析API响应失败: %w", err)
	}

	if !apiResp.Success {
		var errorMsgs []string
		for _, apiErr := range apiResp.Errors {
			errorMsgs = append(errorMsgs, fmt.Sprintf("代码%d: %s", apiErr.Code, apiErr.Message))
		}
		return nil, fmt.Errorf("API错误: %s", strings.Join(errorMsgs, "; "))
	}

	return body, nil
}

// extractRootDomain 提取根域名
func (p *CloudflareProvider) extractRootDomain(domain string) string {
	// 简单的根域名提取逻辑
	// 对于更复杂的情况，可能需要使用公共后缀列表
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return domain
}
