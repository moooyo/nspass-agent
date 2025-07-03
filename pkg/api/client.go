package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/sirupsen/logrus"
)

// Client API客户端
type Client struct {
	config     config.APIConfig
	httpClient *http.Client
}

// NewClient 创建新的API客户端
func NewClient(cfg config.APIConfig) *Client {
	client := &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
	}

	logger.LogStartup("api-client", "1.0", map[string]interface{}{
		"base_url":    cfg.BaseURL,
		"timeout":     cfg.Timeout,
		"retry_count": cfg.RetryCount,
		"retry_delay": cfg.RetryDelay,
	})

	return client
}

// Configuration API返回的配置结构
type Configuration struct {
	Proxies       []ProxyConfig `json:"proxies"`
	IPTablesRules []IPTableRule `json:"iptables_rules"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// ProxyConfig 代理配置
type ProxyConfig struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"` // shadowsocks, trojan, snell
	Name    string                 `json:"name"`
	Config  map[string]interface{} `json:"config"`
	Enabled bool                   `json:"enabled"`
}

// IPTableRule iptables规则
type IPTableRule struct {
	ID      string `json:"id"`
	Table   string `json:"table"`  // filter, nat, mangle
	Chain   string `json:"chain"`  // INPUT, OUTPUT, FORWARD, etc.
	Rule    string `json:"rule"`   // 完整的iptables规则
	Action  string `json:"action"` // add, delete
	Enabled bool   `json:"enabled"`
}

// GetConfiguration 从API获取配置
func (c *Client) GetConfiguration() (*Configuration, error) {
	startTime := time.Now()
	log := logger.GetAPILogger()

	url := fmt.Sprintf("%s/api/v1/agent/config", c.config.BaseURL)

	log.WithField("url", url).Debug("开始获取配置")

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.LogError(err, "创建API请求失败", logrus.Fields{
			"url": url,
		})
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.config.Token)
	req.Header.Set("Content-Type", "application/json")

	var resp *http.Response
	var lastErr error

	// 重试机制
	for i := 0; i < c.config.RetryCount; i++ {
		attemptStart := time.Now()
		resp, lastErr = c.httpClient.Do(req)
		attemptDuration := time.Since(attemptStart)

		if lastErr == nil && resp.StatusCode == http.StatusOK {
			log.WithFields(logrus.Fields{
				"attempt":     i + 1,
				"duration_ms": attemptDuration.Milliseconds(),
			}).Debug("API请求成功")
			break
		}

		if resp != nil {
			resp.Body.Close()
		}

		if i < c.config.RetryCount-1 {
			retryDelay := time.Duration(c.config.RetryDelay) * time.Second
			log.WithFields(logrus.Fields{
				"attempt":      i + 1,
				"max_attempts": c.config.RetryCount,
				"error":        lastErr,
				"retry_delay":  retryDelay,
			}).Warn("API请求失败，准备重试")
			time.Sleep(retryDelay)
		}
	}

	if lastErr != nil {
		logger.LogError(lastErr, "API请求最终失败", logrus.Fields{
			"url":            url,
			"retry_count":    c.config.RetryCount,
			"total_duration": time.Since(startTime).Milliseconds(),
		})
		return nil, fmt.Errorf("API请求失败: %w", lastErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.LogError(fmt.Errorf("API返回错误状态码: %d", resp.StatusCode),
			"API响应错误", logrus.Fields{
				"status_code": resp.StatusCode,
				"response":    string(body),
				"url":         url,
			})
		return nil, fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var config Configuration
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		logger.LogError(err, "解析API响应失败", logrus.Fields{
			"url": url,
		})
		return nil, fmt.Errorf("解析API响应失败: %w", err)
	}

	duration := time.Since(startTime)

	// 记录性能指标
	logger.LogPerformance("api_get_configuration", duration, logrus.Fields{
		"proxies_count": len(config.Proxies),
		"rules_count":   len(config.IPTablesRules),
		"url":           url,
	})

	log.WithFields(logrus.Fields{
		"proxies_count": len(config.Proxies),
		"rules_count":   len(config.IPTablesRules),
		"updated_at":    config.UpdatedAt,
		"duration_ms":   duration.Milliseconds(),
	}).Info("成功获取配置")

	return &config, nil
}

// ReportStatus 向API报告状态
func (c *Client) ReportStatus(status AgentStatus) error {
	startTime := time.Now()
	log := logger.GetAPILogger()

	url := fmt.Sprintf("%s/api/v1/agent/status", c.config.BaseURL)

	log.WithFields(logrus.Fields{
		"url":      url,
		"hostname": status.Hostname,
		"status":   status.Status,
	}).Debug("开始报告状态")

	data, err := json.Marshal(status)
	if err != nil {
		logger.LogError(err, "序列化状态数据失败", logrus.Fields{
			"hostname": status.Hostname,
			"status":   status.Status,
		})
		return fmt.Errorf("序列化状态数据失败: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		logger.LogError(err, "创建状态报告请求失败", logrus.Fields{
			"url": url,
		})
		return fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.config.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.LogError(err, "发送状态报告失败", logrus.Fields{
			"url":      url,
			"hostname": status.Hostname,
		})
		return fmt.Errorf("发送状态报告失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.LogError(fmt.Errorf("状态报告失败，状态码: %d", resp.StatusCode),
			"状态报告响应错误", logrus.Fields{
				"status_code": resp.StatusCode,
				"response":    string(body),
				"url":         url,
				"hostname":    status.Hostname,
			})
		return fmt.Errorf("状态报告失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	duration := time.Since(startTime)

	// 记录性能指标
	logger.LogPerformance("api_report_status", duration, logrus.Fields{
		"hostname":       status.Hostname,
		"status":         status.Status,
		"active_proxies": len(status.ActiveProxies),
		"errors_count":   len(status.Errors),
	})

	log.WithFields(logrus.Fields{
		"hostname":    status.Hostname,
		"status":      status.Status,
		"duration_ms": duration.Milliseconds(),
	}).Info("状态报告成功")

	return nil
}

// GetBaseURL 获取API基础URL
func (c *Client) GetBaseURL() string {
	return c.config.BaseURL
}

// AgentStatus Agent状态
type AgentStatus struct {
	Hostname      string                 `json:"hostname"`
	Version       string                 `json:"version"`
	Status        string                 `json:"status"` // online, offline, error
	LastUpdate    time.Time              `json:"last_update"`
	ActiveProxies []string               `json:"active_proxies"`
	SystemInfo    map[string]interface{} `json:"system_info"`
	Errors        []string               `json:"errors"`
}
