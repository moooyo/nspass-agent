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

	"github.com/nspass/nspass-agent/pkg/api"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/iptables"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/nspass/nspass-agent/pkg/proxy"
	"github.com/nspass/nspass-agent/pkg/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/sirupsen/logrus"
)

// Service Agent核心服务
type Service struct {
	config          *config.Config
	apiClient       *api.Client
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

	// 创建API客户端
	apiClient := api.NewClient(cfg.API, serverID)

	// 创建proxy管理器
	proxyManager := proxy.NewManager(cfg.Proxy)

	// 创建iptables管理器
	iptablesManager := iptables.NewManager(cfg.IPTables)

	ctx, cancel := context.WithCancel(context.Background())

	service := &Service{
		config:          cfg,
		apiClient:       apiClient,
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

// updateIPTablesRulesFromProto 使用proto配置更新iptables规则
func (s *Service) updateIPTablesRulesFromProto() error {
	log := logger.GetComponentLogger("agent-service")

	// 从API获取proto格式的iptables配置
	iptablesConfigs, err := s.apiClient.GetServerIptablesConfigsProto(s.serverID)
	if err != nil {
		log.WithError(err).Error("获取iptables配置失败")
		return fmt.Errorf("获取iptables配置失败: %w", err)
	}

	log.WithField("configs_count", len(iptablesConfigs)).Info("获取到iptables配置(proto)")

	// 直接使用proto配置
	return s.iptablesManager.UpdateRulesFromProto(iptablesConfigs)
}

// reportStatus 上报状态
func (s *Service) reportStatus() error {
	log := logger.GetComponentLogger("agent-service")

	// 获取网络地址
	ipv4, ipv6, err := s.getNetworkAddresses()
	if err != nil {
		logger.LogError(err, "获取网络地址失败", nil)
	}

	// 收集系统资源信息
	activity, err := s.collectActivityInfo()
	if err != nil {
		logger.LogError(err, "收集活动信息失败", nil)
		// 创建一个基本的活动信息
		activity = &api.AgentActivity{
			ActiveConnections:  0,
			TotalBytesSent:     0,
			TotalBytesReceived: 0,
			ProxyServices:      []api.ProxyServiceStatus{},
			LastActivity:       time.Now(),
			CPUUsage:           0,
			MemoryUsage:        0,
			DiskUsage:          0,
		}
	}

	// 构建状态报告
	statusReport := api.AgentStatusReport{
		ServerID:    s.serverID,
		IPv4Address: ipv4,
		IPv6Address: ipv6,
		Activity:    *activity,
		ReportTime:  time.Now(),
	}

	// 发送状态报告
	configUpdate, err := s.apiClient.ReportAgentStatus(statusReport)
	if err != nil {
		return fmt.Errorf("发送状态报告失败: %w", err)
	}

	// 检查是否有配置更新
	if configUpdate != nil && configUpdate.HasUpdate {
		log.WithFields(logrus.Fields{
			"config_version": configUpdate.ConfigVersion,
			"update_message": configUpdate.UpdateMessage,
		}).Info("检测到服务器配置更新，将在下次循环中获取新配置")

		// 清除配置hash以强制在下次循环中更新
		s.lastConfigHash = ""
	}

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

// getNetworkAddresses 获取网络地址
func (s *Service) getNetworkAddresses() (ipv4, ipv6 string, err error) {
	log := logger.GetComponentLogger("agent-service")

	// 首先尝试通过API获取真实的外网IPv4地址
	publicIPv4, err := getPublicIP()
	if err != nil {
		log.WithError(err).Warn("通过API获取公网IP失败，将使用本地网络接口IP")
	} else {
		log.WithField("public_ip", publicIPv4).Info("成功获取公网IP地址")
		ipv4 = publicIPv4
	}

	// 如果API获取失败，则回退到本地网络接口获取
	if ipv4 == "" {
		interfaces, err := net.Interfaces()
		if err != nil {
			return "", "", fmt.Errorf("获取网络接口失败: %w", err)
		}

		for _, iface := range interfaces {
			// 跳过回环接口和未启用的接口
			if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
				continue
			}

			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}

			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil && ipv4 == "" {
						ipv4 = ipnet.IP.String()
					}
				}
			}
		}
	}

	// 获取IPv6地址（仍然从本地接口获取，因为大多数外网IP查询服务不提供IPv6）
	interfaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			// 跳过回环接口和未启用的接口
			if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
				continue
			}

			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}

			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To16() != nil && ipnet.IP.To4() == nil && ipv6 == "" {
						ipv6 = ipnet.IP.String()
					}
				}
			}
		}
	}

	return ipv4, ipv6, nil
}

// collectActivityInfo 收集活动信息
func (s *Service) collectActivityInfo() (*api.AgentActivity, error) {
	// 获取系统资源信息
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil {
		logger.LogError(err, "获取CPU使用率失败", nil)
		cpuPercent = []float64{0}
	}

	memInfo, err := mem.VirtualMemory()
	if err != nil {
		logger.LogError(err, "获取内存使用率失败", nil)
		memInfo = &mem.VirtualMemoryStat{UsedPercent: 0}
	}

	diskInfo, err := disk.Usage("/")
	if err != nil {
		logger.LogError(err, "获取磁盘使用率失败", nil)
		diskInfo = &disk.UsageStat{UsedPercent: 0}
	}

	// 获取proxy服务状态
	proxyStatuses := s.collectProxyStatuses()

	activity := &api.AgentActivity{
		ActiveConnections:  int32(len(proxyStatuses)), // 简化的连接数统计
		TotalBytesSent:     0,                         // 流量统计功能待实现
		TotalBytesReceived: 0,                         // 流量统计功能待实现
		ProxyServices:      proxyStatuses,
		LastActivity:       time.Now(),
		CPUUsage:           float32(cpuPercent[0]),
		MemoryUsage:        float32(memInfo.UsedPercent),
		DiskUsage:          float32(diskInfo.UsedPercent),
	}

	return activity, nil
}

// collectProxyStatuses 收集proxy服务状态
func (s *Service) collectProxyStatuses() []api.ProxyServiceStatus {
	var statuses []api.ProxyServiceStatus

	proxyStatus := s.proxyManager.GetStatus()
	if statusMap, ok := proxyStatus["statuses"].(map[string]interface{}); ok {
		for proxyID, status := range statusMap {
			serviceStatus := api.ProxyServiceStatus{
				ServiceName:     proxyID,
				ServiceStatus:   fmt.Sprintf("%v", status),
				Port:            8080, // 使用默认端口，实际应从配置获取
				ConnectionCount: 0,    // 连接数统计功能待实现
				LastCheck:       time.Now(),
			}

			if status != "running" {
				serviceStatus.ErrorMessage = fmt.Sprintf("服务状态: %v", status)
			}

			statuses = append(statuses, serviceStatus)
		}
	}

	return statuses
}

// calculateConfigHash 计算配置哈希值
func (s *Service) calculateConfigHash(config *api.ServerConfigData) string {
	// 简化的哈希计算 - 实际应用中可以使用更复杂的哈希算法
	hash := fmt.Sprintf("%s_%d_%d_%d_%v",
		config.ServerID,
		len(config.Routes),
		len(config.Egress),
		len(config.ForwardRules),
		config.LastUpdated.Unix(),
	)
	return hash
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
