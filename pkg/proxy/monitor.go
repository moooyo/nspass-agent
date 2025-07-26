package proxy

import (
	"context"
	"sync"
	"time"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/sirupsen/logrus"
)

// ProxyState 代理状态信息
type ProxyState struct {
	ID             string            // 代理ID
	Type           model.EgressMode  // 代理类型
	Instance       ProxyInterface    // 代理实例
	Config         *model.EgressItem // 代理配置
	LastCheck      time.Time         // 上次检查时间
	LastRestart    time.Time         // 上次重启时间
	Status         string            // 当前状态: running, stopped, crashed, restarting
	RestartCount   int               // 重启次数
	RestartHistory []RestartRecord   // 重启历史记录
	Enabled        bool              // 是否启用
	mu             sync.RWMutex      // 状态锁
}

// RestartRecord 重启记录
type RestartRecord struct {
	Timestamp time.Time `json:"timestamp"` // 重启时间
	Reason    string    `json:"reason"`    // 重启原因
	Success   bool      `json:"success"`   // 是否成功
	Duration  int64     `json:"duration"`  // 重启耗时(毫秒)
}

// GetStatus 获取代理状态（线程安全）
func (ps *ProxyState) GetStatus() string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.Status
}

// SetStatus 设置代理状态（线程安全）
func (ps *ProxyState) SetStatus(status string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.Status != status {
		oldStatus := ps.Status
		ps.Status = status
		ps.LastCheck = time.Now()

		// 记录状态变更
		logger.LogStateChange(
			ps.Type,
			oldStatus,
			status,
			"代理状态监控检测到变化",
		)
	}
}

// AddRestartRecord 添加重启记录（线程安全）
func (ps *ProxyState) AddRestartRecord(reason string, success bool, duration time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	record := RestartRecord{
		Timestamp: time.Now(),
		Reason:    reason,
		Success:   success,
		Duration:  duration.Milliseconds(),
	}

	ps.RestartHistory = append(ps.RestartHistory, record)
	ps.RestartCount++
	ps.LastRestart = record.Timestamp

	// 保持重启历史记录在合理范围内（最多100条）
	if len(ps.RestartHistory) > 100 {
		ps.RestartHistory = ps.RestartHistory[1:]
	}
}

// GetRecentRestarts 获取最近一小时的重启次数（线程安全）
func (ps *ProxyState) GetRecentRestarts() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	oneHourAgo := time.Now().Add(-time.Hour)
	count := 0

	for _, record := range ps.RestartHistory {
		if record.Timestamp.After(oneHourAgo) {
			count++
		}
	}

	return count
}

// CanRestart 检查是否可以重启（线程安全）
func (ps *ProxyState) CanRestart(maxRestarts int, cooldownSeconds int) bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	// 检查是否在冷却期内
	if !ps.LastRestart.IsZero() {
		cooldownDuration := time.Duration(cooldownSeconds) * time.Second
		if time.Since(ps.LastRestart) < cooldownDuration {
			return false
		}
	}

	// 检查最近一小时的重启次数
	recentRestarts := ps.GetRecentRestarts()
	return recentRestarts < maxRestarts
}

// ProxyMonitor 代理监控器
type ProxyMonitor struct {
	config  config.MonitorConfig
	states  map[string]*ProxyState // 代理状态映射 proxyID -> state
	mu      sync.RWMutex           // 状态映射锁
	ctx     context.Context        // 上下文
	cancel  context.CancelFunc     // 取消函数
	ticker  *time.Ticker           // 定时器
	running bool                   // 是否运行中
	log     *logrus.Entry          // 日志记录器

	// 统计信息
	stats ProxyMonitorStats
}

// ProxyMonitorStats 监控统计信息
type ProxyMonitorStats struct {
	TotalChecks     int64        `json:"total_checks"`     // 总检查次数
	TotalRestarts   int64        `json:"total_restarts"`   // 总重启次数
	SuccessRestarts int64        `json:"success_restarts"` // 成功重启次数
	FailedRestarts  int64        `json:"failed_restarts"`  // 失败重启次数
	LastCheckTime   time.Time    `json:"last_check_time"`  // 最后检查时间
	mu              sync.RWMutex // 统计锁
}

// NewProxyMonitor 创建新的代理监控器
func NewProxyMonitor(config config.MonitorConfig) *ProxyMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	monitor := &ProxyMonitor{
		config:  config,
		states:  make(map[string]*ProxyState),
		ctx:     ctx,
		cancel:  cancel,
		running: false,
		log:     logger.GetProxyLogger().WithField("component", "monitor"),
		stats:   ProxyMonitorStats{},
	}

	logger.LogStartup("proxy-monitor", "1.0", map[string]interface{}{
		"check_interval":   config.CheckInterval,
		"restart_cooldown": config.RestartCooldown,
		"max_restarts":     config.MaxRestarts,
		"health_timeout":   config.HealthTimeout,
	})

	return monitor
}

// RegisterProxy 注册代理进行监控
func (pm *ProxyMonitor) RegisterProxy(config *model.EgressItem, instance ProxyInterface) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	state := &ProxyState{
		ID:             config.EgressId,
		Type:           config.EgressMode,
		Instance:       instance,
		Config:         config,
		LastCheck:      time.Now(),
		Status:         "unknown",
		RestartCount:   0,
		RestartHistory: make([]RestartRecord, 0),
		Enabled:        true,
	}

	pm.states[config.EgressId] = state

	pm.log.WithFields(logrus.Fields{
		"proxy_id":   config.EgressId,
		"proxy_type": config.EgressMode,
	}).Info("代理已注册到监控器")
}

// UnregisterProxy 取消注册代理
func (pm *ProxyMonitor) UnregisterProxy(id string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if state, exists := pm.states[id]; exists {
		delete(pm.states, id)
		pm.log.WithFields(logrus.Fields{
			"proxy_id":   id,
			"proxy_type": state.Type,
		}).Info("代理已从监控器取消注册")
	}
}

// EnableProxy 启用代理监控
func (pm *ProxyMonitor) EnableProxy(id string) {
	pm.mu.RLock()
	state, exists := pm.states[id]
	pm.mu.RUnlock()

	if exists {
		state.mu.Lock()
		state.Enabled = true
		state.mu.Unlock()

		pm.log.WithField("proxy_id", id).Info("代理监控已启用")
	}
}

// DisableProxy 禁用代理监控
func (pm *ProxyMonitor) DisableProxy(id string) {
	pm.mu.RLock()
	state, exists := pm.states[id]
	pm.mu.RUnlock()

	if exists {
		state.mu.Lock()
		state.Enabled = false
		state.mu.Unlock()

		pm.log.WithField("proxy_id", id).Info("代理监控已禁用")
	}
}

// Start 启动监控器
func (pm *ProxyMonitor) Start() error {
	if !pm.config.Enable {
		pm.log.Info("代理监控已禁用，跳过启动")
		return nil
	}

	if pm.running {
		pm.log.Warn("监控器已在运行")
		return nil
	}

	pm.running = true
	interval := time.Duration(pm.config.CheckInterval) * time.Second
	pm.ticker = time.NewTicker(interval)

	pm.log.WithField("check_interval", interval).Info("代理监控器已启动")

	// 启动监控循环
	go pm.monitorLoop()

	return nil
}

// Stop 停止监控器
func (pm *ProxyMonitor) Stop() error {
	if !pm.running {
		return nil
	}

	pm.running = false
	pm.cancel()

	if pm.ticker != nil {
		pm.ticker.Stop()
	}

	pm.log.Info("代理监控器已停止")
	return nil
}

// GetStats 获取监控统计信息
func (pm *ProxyMonitor) GetStats() ProxyMonitorStats {
	pm.stats.mu.RLock()
	defer pm.stats.mu.RUnlock()
	return pm.stats
}

// GetProxyState 获取指定代理的状态
func (pm *ProxyMonitor) GetProxyState(id string) (*ProxyState, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	state, exists := pm.states[id]
	return state, exists
}

// GetAllStates 获取所有代理状态
func (pm *ProxyMonitor) GetAllStates() map[string]*ProxyState {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// 创建副本以避免并发问题
	states := make(map[string]*ProxyState)
	for id, state := range pm.states {
		states[id] = state
	}

	return states
}

// monitorLoop 监控循环
func (pm *ProxyMonitor) monitorLoop() {
	pm.log.Info("监控循环已启动")

	for {
		select {
		case <-pm.ctx.Done():
			pm.log.Info("监控循环已停止")
			return
		case <-pm.ticker.C:
			pm.performHealthCheck()
		}
	}
}

// performHealthCheck 执行健康检查
func (pm *ProxyMonitor) performHealthCheck() {
	startTime := time.Now()
	pm.log.Debug("开始执行代理健康检查")

	// 更新统计信息
	pm.stats.mu.Lock()
	pm.stats.TotalChecks++
	pm.stats.LastCheckTime = startTime
	pm.stats.mu.Unlock()

	pm.mu.RLock()
	states := make([]*ProxyState, 0, len(pm.states))
	for _, state := range pm.states {
		states = append(states, state)
	}
	pm.mu.RUnlock()

	// 并发检查所有代理
	var wg sync.WaitGroup
	for _, state := range states {
		wg.Add(1)
		go func(s *ProxyState) {
			defer wg.Done()
			pm.checkProxyHealth(s)
		}(state)
	}

	wg.Wait()

	duration := time.Since(startTime)
	logger.LogPerformance("proxy_health_check", duration, logrus.Fields{
		"checked_proxies": len(states),
	})

	pm.log.WithFields(logrus.Fields{
		"checked_proxies": len(states),
		"duration_ms":     duration.Milliseconds(),
	}).Debug("代理健康检查完成")
}

// checkProxyHealth 检查单个代理的健康状态
func (pm *ProxyMonitor) checkProxyHealth(state *ProxyState) {
	state.mu.RLock()
	enabled := state.Enabled
	proxyID := state.ID
	proxyType := state.Type
	instance := state.Instance
	state.mu.RUnlock()

	if !enabled {
		return
	}

	log := pm.log.WithFields(logrus.Fields{
		"proxy_id":   proxyID,
		"proxy_type": proxyType,
	})

	log.Debug("检查代理健康状态")

	// 使用超时检查进程状态
	checkCtx, cancel := context.WithTimeout(pm.ctx, time.Duration(pm.config.HealthTimeout)*time.Second)
	defer cancel()

	// 在独立的goroutine中检查状态
	statusChan := make(chan bool, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.WithField("panic", r).Error("检查代理状态时发生panic")
				statusChan <- false
			}
		}()

		statusChan <- instance.IsRunning()
	}()

	var isRunning bool
	select {
	case <-checkCtx.Done():
		log.Warn("代理健康检查超时")
		isRunning = false
	case isRunning = <-statusChan:
	}

	// 更新状态
	oldStatus := state.GetStatus()
	var newStatus string

	if isRunning {
		newStatus = "running"
	} else {
		// 判断是否是异常退出
		if oldStatus == "running" || oldStatus == "unknown" {
			newStatus = "crashed"
			log.Warn("检测到代理进程异常退出")
		} else {
			newStatus = "stopped"
		}
	}

	state.SetStatus(newStatus)

	// 如果检测到崩溃且启用了自动重启，则尝试重启
	if newStatus == "crashed" && pm.config.Enable {
		log.Info("代理进程崩溃，尝试自动重启")
		pm.attemptRestart(state, "进程崩溃检测")
	}
}

// attemptRestart 尝试重启代理
func (pm *ProxyMonitor) attemptRestart(state *ProxyState, reason string) {
	state.mu.RLock()
	proxyID := state.ID
	proxyType := state.Type
	instance := state.Instance
	config := state.Config
	state.mu.RUnlock()

	log := pm.log.WithFields(logrus.Fields{
		"proxy_id":   proxyID,
		"proxy_type": proxyType,
		"reason":     reason,
	})

	// 检查是否可以重启
	if !state.CanRestart(pm.config.MaxRestarts, pm.config.RestartCooldown) {
		log.Warn("代理重启被限制（达到最大重启次数或在冷却期内）")
		return
	}

	log.Info("开始重启代理")
	startTime := time.Now()

	// 设置重启状态
	state.SetStatus("restarting")

	// 更新统计信息
	pm.stats.mu.Lock()
	pm.stats.TotalRestarts++
	pm.stats.mu.Unlock()

	// 执行重启操作
	success := pm.performRestart(instance, config, log)
	duration := time.Since(startTime)

	// 记录重启结果
	state.AddRestartRecord(reason, success, duration)

	// 更新统计信息
	pm.stats.mu.Lock()
	if success {
		pm.stats.SuccessRestarts++
	} else {
		pm.stats.FailedRestarts++
	}
	pm.stats.mu.Unlock()

	// 记录性能指标
	logger.LogPerformance("proxy_restart", duration, logrus.Fields{
		"proxy_id":   proxyID,
		"proxy_type": proxyType,
		"success":    success,
		"reason":     reason,
	})

	if success {
		state.SetStatus("running")
		log.WithField("duration_ms", duration.Milliseconds()).Info("代理重启成功")

		// 记录审计日志
		logger.LogAudit("proxy_auto_restart", "system", logrus.Fields{
			"proxy_id":   proxyID,
			"proxy_type": proxyType,
			"reason":     reason,
			"duration":   duration.Milliseconds(),
		})
	} else {
		state.SetStatus("crashed")
		log.WithField("duration_ms", duration.Milliseconds()).Error("代理重启失败")
	}
}

// performRestart 执行重启操作
func (pm *ProxyMonitor) performRestart(instance ProxyInterface, config map[string]interface{}, log *logrus.Entry) bool {
	// 1. 尝试停止进程（如果还在运行）
	log.Debug("停止代理进程")
	if err := instance.Stop(); err != nil {
		log.WithError(err).Warn("停止代理进程失败，继续重启流程")
	}

	// 2. 等待一小段时间确保进程完全停止
	time.Sleep(2 * time.Second)

	// 3. 重新配置
	log.Debug("重新配置代理")
	if err := instance.Configure(config); err != nil {
		log.WithError(err).Error("重新配置代理失败")
		return false
	}

	// 4. 启动代理
	log.Debug("启动代理进程")
	if err := instance.Start(); err != nil {
		log.WithError(err).Error("启动代理进程失败")
		return false
	}

	// 5. 验证启动是否成功（等待几秒后检查）
	time.Sleep(3 * time.Second)
	if !instance.IsRunning() {
		log.Error("代理启动后验证失败，进程未运行")
		return false
	}

	return true
}

// GetMonitorSummary 获取监控摘要信息
func (pm *ProxyMonitor) GetMonitorSummary() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	summary := map[string]interface{}{
		"config":        pm.config,
		"running":       pm.running,
		"total_proxies": len(pm.states),
		"stats":         pm.GetStats(),
	}

	// 按状态统计代理数量
	statusCount := make(map[string]int)
	enabledCount := 0

	for _, state := range pm.states {
		status := state.GetStatus()
		statusCount[status]++

		state.mu.RLock()
		if state.Enabled {
			enabledCount++
		}
		state.mu.RUnlock()
	}

	summary["status_count"] = statusCount
	summary["enabled_proxies"] = enabledCount

	return summary
}

// IsRunning 检查监控器是否正在运行
func (pm *ProxyMonitor) IsRunning() bool {
	return pm.running
}
