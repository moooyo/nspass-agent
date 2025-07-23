package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nspass/nspass-agent/generated/model"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/sirupsen/logrus"
)

// Client API客户端
type Client struct {
	config     config.APIConfig
	serverID   string
	httpClient *http.Client
}

// NewClient 创建新的API客户端
func NewClient(cfg config.APIConfig, serverID string) *Client {
	client := &Client{
		config:   cfg,
		serverID: serverID,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
	}

	logger.LogStartup("api-client", "1.0", map[string]interface{}{
		"base_url":    cfg.BaseURL,
		"server_id":   serverID,
		"timeout":     cfg.Timeout,
		"retry_count": cfg.RetryCount,
		"retry_delay": cfg.RetryDelay,
	})

	return client
}

// setAuthHeaders 设置鉴权Headers
func (c *Client) setAuthHeaders(req *http.Request) {
	req.Header.Set("Server-ID", c.serverID)
	req.Header.Set("Server-Token", c.config.Token)
	req.Header.Set("Content-Type", "application/json")
}

// GetServerConfig 获取服务器配置
func (c *Client) GetServerConfig(serverID string) (*ServerConfigData, error) {
	startTime := time.Now()
	log := logger.GetAPILogger()

	url := fmt.Sprintf("%s/v1/agent/config/%s", c.config.BaseURL, serverID)

	log.WithFields(logrus.Fields{
		"url":       url,
		"server_id": serverID,
	}).Debug("开始获取服务器配置")

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.LogError(err, "创建API请求失败", logrus.Fields{
			"url":       url,
			"server_id": serverID,
		})
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	c.setAuthHeaders(req)

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
			}).Debug("获取服务器配置成功")
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
			}).Warn("获取服务器配置失败，准备重试")
			time.Sleep(retryDelay)
		}
	}

	if lastErr != nil {
		logger.LogError(lastErr, "获取服务器配置最终失败", logrus.Fields{
			"url":            url,
			"server_id":      serverID,
			"retry_count":    c.config.RetryCount,
			"total_duration": time.Since(startTime).Milliseconds(),
		})
		return nil, fmt.Errorf("获取服务器配置失败: %w", lastErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.LogError(fmt.Errorf("API返回错误状态码: %d", resp.StatusCode),
			"API响应错误", logrus.Fields{
				"status_code": resp.StatusCode,
				"response":    string(body),
				"url":         url,
				"server_id":   serverID,
			})
		return nil, fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	type GetServerConfigResponse struct {
		Status struct {
			Success   bool   `json:"success"`
			Message   string `json:"message,omitempty"`
			ErrorCode string `json:"error_code,omitempty"`
		} `json:"status"`
		Data *ServerConfigData `json:"data,omitempty"`
	}

	var response GetServerConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		logger.LogError(err, "解析API响应失败", logrus.Fields{
			"url":       url,
			"server_id": serverID,
		})
		return nil, fmt.Errorf("解析API响应失败: %w", err)
	}

	if !response.Status.Success {
		err := fmt.Errorf("API返回错误: %s", response.Status.Message)
		logger.LogError(err, "获取服务器配置API返回错误", logrus.Fields{
			"server_id":     serverID,
			"error_code":    response.Status.ErrorCode,
			"error_message": response.Status.Message,
		})
		return nil, err
	}

	if response.Data == nil {
		err := fmt.Errorf("服务器配置数据为空")
		logger.LogError(err, "服务器配置数据为空", logrus.Fields{
			"server_id": serverID,
		})
		return nil, err
	}

	duration := time.Since(startTime)

	// 记录性能指标
	logger.LogPerformance("api_get_server_config", duration, logrus.Fields{
		"server_id":     serverID,
		"routes_count":  len(response.Data.Routes),
		"egress_count":  len(response.Data.Egress),
		"forward_rules": len(response.Data.ForwardRules),
	})

	log.WithFields(logrus.Fields{
		"server_id":     serverID,
		"server_name":   response.Data.ServerName,
		"routes_count":  len(response.Data.Routes),
		"egress_count":  len(response.Data.Egress),
		"forward_rules": len(response.Data.ForwardRules),
		"last_updated":  response.Data.LastUpdated,
		"duration_ms":   duration.Milliseconds(),
	}).Info("成功获取服务器配置")

	return response.Data, nil
}

// ReportAgentStatus 上报Agent状态
func (c *Client) ReportAgentStatus(status AgentStatusReport) (*ServerConfigUpdateInfo, error) {
	startTime := time.Now()
	log := logger.GetAPILogger()

	url := fmt.Sprintf("%s/v1/agent/status", c.config.BaseURL)

	log.WithFields(logrus.Fields{
		"url":                url,
		"server_id":          status.ServerID,
		"active_connections": status.Activity.ActiveConnections,
		"proxy_services":     len(status.Activity.ProxyServices),
	}).Debug("开始上报Agent状态")

	data, err := json.Marshal(status)
	if err != nil {
		logger.LogError(err, "序列化状态数据失败", logrus.Fields{
			"server_id": status.ServerID,
		})
		return nil, fmt.Errorf("序列化状态数据失败: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		logger.LogError(err, "创建状态报告请求失败", logrus.Fields{
			"url":       url,
			"server_id": status.ServerID,
		})
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	c.setAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.LogError(err, "发送状态报告失败", logrus.Fields{
			"url":       url,
			"server_id": status.ServerID,
		})
		return nil, fmt.Errorf("发送状态报告失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.LogError(fmt.Errorf("状态报告失败，状态码: %d", resp.StatusCode),
			"状态报告响应错误", logrus.Fields{
				"status_code": resp.StatusCode,
				"response":    string(body),
				"url":         url,
				"server_id":   status.ServerID,
			})
		return nil, fmt.Errorf("状态报告失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	type ReportAgentStatusResponse struct {
		Status struct {
			Success   bool   `json:"success"`
			Message   string `json:"message,omitempty"`
			ErrorCode string `json:"error_code,omitempty"`
		} `json:"status"`
		Acknowledgment string                  `json:"acknowledgment,omitempty"`
		ConfigUpdate   *ServerConfigUpdateInfo `json:"config_update,omitempty"`
	}

	var response ReportAgentStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		logger.LogError(err, "解析状态报告响应失败", logrus.Fields{
			"url":       url,
			"server_id": status.ServerID,
		})
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if !response.Status.Success {
		err := fmt.Errorf("API返回错误: %s", response.Status.Message)
		logger.LogError(err, "上报Agent状态API返回错误", logrus.Fields{
			"server_id":     status.ServerID,
			"error_code":    response.Status.ErrorCode,
			"error_message": response.Status.Message,
		})
		return nil, err
	}

	duration := time.Since(startTime)

	// 记录性能指标
	logger.LogPerformance("api_report_agent_status", duration, logrus.Fields{
		"server_id":          status.ServerID,
		"active_connections": status.Activity.ActiveConnections,
		"proxy_services":     len(status.Activity.ProxyServices),
	})

	log.WithFields(logrus.Fields{
		"server_id":      status.ServerID,
		"acknowledgment": response.Acknowledgment,
		"has_update":     response.ConfigUpdate != nil && response.ConfigUpdate.HasUpdate,
		"duration_ms":    duration.Milliseconds(),
	}).Info("成功上报Agent状态")

	return response.ConfigUpdate, nil
}

// GetServerIptablesConfigsProto 获取服务器的iptables配置（proto格式）
func (c *Client) GetServerIptablesConfigsProto(serverID string) ([]*model.IptablesConfig, error) {
	startTime := time.Now()
	log := logger.GetAPILogger()

	url := fmt.Sprintf("%s/v1/servers/%s/iptables/configs", c.config.BaseURL, serverID)

	log.WithFields(logrus.Fields{
		"url":       url,
		"server_id": serverID,
	}).Debug("开始获取服务器iptables配置(proto)")

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.LogError(err, "创建iptables配置请求失败", logrus.Fields{
			"url":       url,
			"server_id": serverID,
		})
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	c.setAuthHeaders(req)

	var resp *http.Response
	var lastErr error

	// 重试机制
	for i := 0; i < c.config.RetryCount; i++ {
		resp, lastErr = c.httpClient.Do(req)
		if lastErr == nil && resp.StatusCode < 500 {
			break
		}

		if resp != nil {
			resp.Body.Close()
		}

		if i < c.config.RetryCount-1 {
			retryDelay := time.Duration(c.config.RetryDelay) * time.Second
			log.WithFields(logrus.Fields{
				"retry":     i + 1,
				"max_retry": c.config.RetryCount,
				"error":     lastErr,
				"delay":     retryDelay,
			}).Debug("iptables配置请求失败，准备重试")
			time.Sleep(retryDelay)
		}
	}

	if lastErr != nil {
		logger.LogError(lastErr, "iptables配置请求最终失败", logrus.Fields{
			"url":         url,
			"server_id":   serverID,
			"retry_count": c.config.RetryCount,
		})
		return nil, fmt.Errorf("iptables配置请求失败: %w", lastErr)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.LogError(fmt.Errorf("iptables配置API返回错误状态码: %d", resp.StatusCode),
			"iptables配置API响应错误", logrus.Fields{
				"status_code": resp.StatusCode,
				"response":    string(body),
				"url":         url,
				"server_id":   serverID,
			})
		return nil, fmt.Errorf("iptables配置API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// API返回proto格式的数据结构
	var response struct {
		Data []*model.IptablesConfig `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		logger.LogError(err, "解析iptables配置响应失败", logrus.Fields{
			"url":       url,
			"server_id": serverID,
		})
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	duration := time.Since(startTime)

	log.WithFields(logrus.Fields{
		"server_id":     serverID,
		"configs_count": len(response.Data),
		"duration_ms":   duration.Milliseconds(),
	}).Info("成功获取iptables配置(proto)")

	return response.Data, nil
}

// GetBaseURL 获取API基础URL
func (c *Client) GetBaseURL() string {
	return c.config.BaseURL
}
