package agent

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/nspass/nspass-agent/pkg/api"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/iptables"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/nspass/nspass-agent/pkg/proxy"
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

	logger.LogStartup("agent-service", "1.0", map[string]interface{}{
		"server_id":        serverID,
		"update_interval":  cfg.UpdateInterval,
		"api_base_url":     cfg.API.BaseURL,
		"proxy_enabled":    len(cfg.Proxy.EnabledTypes) > 0,
		"iptables_enabled": cfg.IPTables.Enable,
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

	// 启动主循环
	s.wg.Add(1)
	go s.mainLoop()

	// 启动状态上报循环
	s.wg.Add(1)
	go s.statusReportLoop()

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

// mainLoop 主循环 - 定期获取配置和更新服务
func (s *Service) mainLoop() {
	defer s.wg.Done()

	log := logger.GetComponentLogger("agent-service")
	ticker := time.NewTicker(time.Duration(s.config.UpdateInterval) * time.Second)
	defer ticker.Stop()

	// 启动时立即执行一次
	if err := s.updateConfiguration(); err != nil {
		logger.LogError(err, "初始配置更新失败", logrus.Fields{
			"server_id": s.serverID,
		})
	}

	for {
		select {
		case <-s.ctx.Done():
			log.Info("主循环退出")
			return
		case <-ticker.C:
			if err := s.updateConfiguration(); err != nil {
				logger.LogError(err, "定期配置更新失败", logrus.Fields{
					"server_id": s.serverID,
				})
			}
		}
	}
}

// statusReportLoop 状态上报循环
func (s *Service) statusReportLoop() {
	defer s.wg.Done()

	log := logger.GetComponentLogger("agent-service")
	// 状态上报间隔为更新间隔的一半，但不少于30秒
	reportInterval := s.config.UpdateInterval / 2
	if reportInterval < 30 {
		reportInterval = 30
	}

	ticker := time.NewTicker(time.Duration(reportInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			log.Info("状态上报循环退出")
			return
		case <-ticker.C:
			if err := s.reportStatus(); err != nil {
				logger.LogError(err, "状态上报失败", logrus.Fields{
					"server_id": s.serverID,
				})
			}
		}
	}
}

// updateConfiguration 更新配置
func (s *Service) updateConfiguration() error {
	startTime := time.Now()
	log := logger.GetComponentLogger("agent-service")

	log.WithField("server_id", s.serverID).Debug("开始更新配置")

	// 获取服务器配置
	serverConfig, err := s.apiClient.GetServerConfig(s.serverID)
	if err != nil {
		return fmt.Errorf("获取服务器配置失败: %w", err)
	}

	// 检查配置是否有变化
	configHash := s.calculateConfigHash(serverConfig)
	if configHash == s.lastConfigHash {
		log.Debug("配置无变化，跳过更新")
		return nil
	}

	log.WithFields(logrus.Fields{
		"routes_count":  len(serverConfig.Routes),
		"egress_count":  len(serverConfig.Egress),
		"forward_rules": len(serverConfig.ForwardRules),
	}).Info("检测到配置变化，开始更新")

	var updateErrors []string

	// 更新proxy配置
	if err := s.updateProxyServices(serverConfig); err != nil {
		updateErrors = append(updateErrors, fmt.Sprintf("更新proxy服务失败: %v", err))
		logger.LogError(err, "更新proxy服务失败", logrus.Fields{
			"server_id": s.serverID,
		})
	}

	// 更新iptables规则
	if err := s.updateIPTablesRules(serverConfig); err != nil {
		updateErrors = append(updateErrors, fmt.Sprintf("更新iptables规则失败: %v", err))
		logger.LogError(err, "更新iptables规则失败", logrus.Fields{
			"server_id": s.serverID,
		})
	}

	// 记录更新结果
	if len(updateErrors) == 0 {
		s.lastConfigHash = configHash

		duration := time.Since(startTime)
		logger.LogPerformance("agent_config_update", duration, logrus.Fields{
			"server_id":     s.serverID,
			"routes_count":  len(serverConfig.Routes),
			"egress_count":  len(serverConfig.Egress),
			"forward_rules": len(serverConfig.ForwardRules),
		})

		log.WithFields(logrus.Fields{
			"duration_ms": duration.Milliseconds(),
			"config_hash": configHash,
		}).Info("配置更新完成")

		return nil
	}

	return fmt.Errorf("部分配置更新失败: %v", updateErrors)
}

// updateProxyServices 更新proxy服务
func (s *Service) updateProxyServices(serverConfig *api.ServerConfigData) error {
	// 转换配置格式
	var proxyConfigs []api.ProxyConfig

	for _, route := range serverConfig.Routes {
		if route.Status != "active" {
			continue
		}

		proxyConfig := api.ProxyConfig{
			ID:      route.ID,
			Type:    route.Protocol,
			Name:    route.RouteName,
			Enabled: route.Status == "active",
			Config: map[string]interface{}{
				"server":      route.EntryPoint,
				"server_port": route.Port,
				"method":      "aes-256-gcm", // 默认加密方法
			},
		}

		// 添加协议特定的参数
		for key, value := range route.ProtocolParams {
			proxyConfig.Config[key] = value
		}

		proxyConfigs = append(proxyConfigs, proxyConfig)
	}

	return s.proxyManager.UpdateProxies(proxyConfigs)
}

// updateIPTablesRules 更新iptables规则
func (s *Service) updateIPTablesRules(serverConfig *api.ServerConfigData) error {
	// 转换配置格式
	var iptableRules []api.IPTableRule

	for _, rule := range serverConfig.ForwardRules {
		if rule.Status != "active" {
			continue
		}

		// 根据转发规则生成iptables规则
		iptableRule := api.IPTableRule{
			ID:      fmt.Sprintf("forward_%d", rule.ID),
			Table:   "nat",
			Chain:   "PREROUTING",
			Action:  "add",
			Enabled: rule.Status == "active",
		}

		// 生成规则内容
		if rule.ForwardType == "tcp" || rule.ForwardType == "all" {
			iptableRule.Rule = fmt.Sprintf("-p tcp --dport %d -j DNAT --to-destination %s:%d",
				rule.SourcePort, rule.TargetAddress, rule.TargetPort)
		} else if rule.ForwardType == "udp" {
			iptableRule.Rule = fmt.Sprintf("-p udp --dport %d -j DNAT --to-destination %s:%d",
				rule.SourcePort, rule.TargetAddress, rule.TargetPort)
		}

		if iptableRule.Rule != "" {
			iptableRules = append(iptableRules, iptableRule)
		}
	}

	return s.iptablesManager.UpdateRules(iptableRules)
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

// getNetworkAddresses 获取网络地址
func (s *Service) getNetworkAddresses() (ipv4, ipv6 string, err error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", "", err
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
				} else if ipnet.IP.To16() != nil && ipv6 == "" {
					ipv6 = ipnet.IP.String()
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
		TotalBytesSent:     0,                         // TODO: 实现流量统计
		TotalBytesReceived: 0,                         // TODO: 实现流量统计
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
				Port:            0, // TODO: 从配置中获取端口
				ConnectionCount: 0, // TODO: 实现连接数统计
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
