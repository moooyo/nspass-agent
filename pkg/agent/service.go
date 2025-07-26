package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/iptables"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/nspass/nspass-agent/pkg/proxy"
	"github.com/nspass/nspass-agent/pkg/websocket"
)

// Service Agent核心服务
type Service struct {
	config          *config.Config
	proxyManager    *proxy.Manager
	iptablesManager iptables.ManagerInterface
	wsClient        *websocket.Client

	serverID       string
	lastConfigHash string

	// 控制相关
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex
}

// NewService 创建新的Agent服务
func NewService(cfg *config.Config, serverID string) (*Service, error) {
	if serverID == "" {
		return nil, fmt.Errorf("server_id不能为空")
	}

	// 创建proxy管理器
	proxyManager := proxy.NewManager(cfg.Proxy)

	// 创建iptables管理器
	iptablesManager := iptables.NewManager(cfg.IPTables)

	ctx, cancel := context.WithCancel(context.Background())

	service := &Service{
		config:          cfg,
		proxyManager:    proxyManager,
		iptablesManager: iptablesManager,
		serverID:        serverID,
		ctx:             ctx,
		cancel:          cancel,
	}

	// 创建任务处理器
	taskHandler := websocket.NewDefaultTaskHandler(cfg, proxyManager, iptablesManager)

	// 创建监控数据收集器
	metricsCollector := websocket.NewDefaultMetricsCollector(proxyManager)

	// 创建WebSocket客户端
	wsClient := websocket.NewClient(cfg, serverID, cfg.API.Token, taskHandler, metricsCollector, iptablesManager, proxyManager)

	// 设置任务统计提供者，用于监控数据收集
	wsClient.SetTaskStatsProvider()

	service.wsClient = wsClient

	logger.LogStartup("agent-service", "1.0", map[string]interface{}{
		"server_id":         serverID,
		"update_interval":   cfg.UpdateInterval,
		"api_base_url":      cfg.API.BaseURL,
		"proxy_enabled":     len(cfg.Proxy.EnabledTypes) > 0,
		"iptables_enabled":  cfg.IPTables.Enable,
		"websocket_enabled": true,
	})

	return service, nil
}

// Start 启动Agent服务
func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("Agent服务已在运行")
	}

	log := logger.GetComponentLogger("agent-service")
	log.Info("启动Agent服务")

	// 启动WebSocket客户端
	if s.wsClient != nil {
		if err := s.wsClient.Start(); err != nil {
			log.WithError(err).Error("启动WebSocket客户端失败")
		}
	}

	s.running = true

	log.Info("Agent服务启动完成")
	return nil
}

// Stop 停止Agent服务
func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	log := logger.GetComponentLogger("agent-service")
	log.Info("停止Agent服务")

	// 停止WebSocket客户端
	if s.wsClient != nil {
		if err := s.wsClient.Stop(); err != nil {
			log.WithError(err).Error("停止WebSocket客户端失败")
		}
	}

	// 取消上下文
	s.cancel()

	// 等待所有goroutine完成
	s.wg.Wait()

	// 停止proxy监控器
	if err := s.proxyManager.StopMonitor(); err != nil {
		logger.LogError(err, "停止proxy监控器失败", nil)
	}

	s.running = false

	log.Info("Agent服务已停止")
	return nil
}

// getPublicIP 通过API获取真实的外网IP地址
func getPublicIP() (string, error) {
	// 定义多个IP查询API，按优先级排序
	ipAPIs := []string{
		"https://ipapi.co/ip/",
		"https://ip.sb",
		"http://ip-api.com/line/?fields=query",
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for _, apiURL := range ipAPIs {
		ip, err := queryIPAPI(client, apiURL)
		if err == nil && ip != "" {
			// 验证IP地址格式
			if net.ParseIP(ip) != nil {
				return ip, nil
			}
		}
	}

	return "", fmt.Errorf("无法从任何API获取有效的公网IP地址")
}

// queryIPAPI 查询指定的IP API
func queryIPAPI(client *http.Client, apiURL string) (string, error) {
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("请求API失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API返回错误状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	// 清理返回的IP地址（去除换行符和空格）
	ip := strings.TrimSpace(string(body))

	// 处理ip-api.com的JSON响应（可能返回JSON格式）
	if strings.Contains(apiURL, "ip-api.com") && strings.Contains(ip, "{") {
		// 如果是JSON响应，提取query字段的值
		if strings.Contains(ip, `"query":"`) {
			start := strings.Index(ip, `"query":"`) + 9
			end := strings.Index(ip[start:], `"`)
			if end > 0 {
				ip = ip[start : start+end]
			}
		}
	}

	return ip, nil
}

// IsRunning 检查服务是否在运行
func (s *Service) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetServerID 获取服务器ID
func (s *Service) GetServerID() string {
	return s.serverID
}

// GetStatus 获取服务状态
func (s *Service) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"running":          s.running,
		"server_id":        s.serverID,
		"last_config_hash": s.lastConfigHash,
		"proxy_status":     s.proxyManager.GetStatus(),
		"iptables_status":  s.iptablesManager.GetRulesSummary(),
		"memory_usage":     m.Alloc / 1024 / 1024, // MB
		"goroutines":       runtime.NumGoroutine(),
	}
}
