package websocket

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/nspass/nspass-agent/pkg/proxy"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DefaultMetricsCollector 默认监控数据收集器
type DefaultMetricsCollector struct {
	proxyManager *proxy.Manager
	taskProvider TaskStatsProvider
	log          *logrus.Entry

	// 缓存上次的数据用于计算差值
	lastTrafficData  *TrafficData
	lastNetworkStats *NetworkStats
	lastUpdateTime   time.Time
}

// TaskStatsProvider interface for task statistics
type TaskStatsProvider interface {
	GetTaskStats() map[string]int
}

// TrafficData 流量数据结构
type TrafficData struct {
	BytesIn    int64
	BytesOut   int64
	PacketsIn  int64
	PacketsOut int64
	Timestamp  time.Time
}

// NetworkStats 网络统计数据结构
type NetworkStats struct {
	BytesRecv   uint64
	BytesSent   uint64
	PacketsRecv uint64
	PacketsSent uint64
	Timestamp   time.Time
}

// NewDefaultMetricsCollector 创建默认监控数据收集器
func NewDefaultMetricsCollector(proxyManager *proxy.Manager) *DefaultMetricsCollector {
	return &DefaultMetricsCollector{
		proxyManager:   proxyManager,
		log:            logger.GetComponentLogger("metrics-collector"),
		lastUpdateTime: time.Now(),
	}
}

// SetTaskStatsProvider 设置任务统计提供者
func (c *DefaultMetricsCollector) SetTaskStatsProvider(provider TaskStatsProvider) {
	c.taskProvider = provider
}

// CollectSystemMetrics 收集系统监控数据
func (c *DefaultMetricsCollector) CollectSystemMetrics() (*model.SystemMetrics, error) {
	c.log.Debug("收集系统监控数据")

	// 获取CPU使用率
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil {
		c.log.WithError(err).Error("获取CPU使用率失败")
		return nil, fmt.Errorf("获取CPU使用率失败: %w", err)
	}

	var cpuUsage float64
	if len(cpuPercent) > 0 {
		cpuUsage = cpuPercent[0]
	}

	// 获取内存使用情况
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		c.log.WithError(err).Error("获取内存使用情况失败")
		return nil, fmt.Errorf("获取内存使用情况失败: %w", err)
	}

	// 获取SWAP使用情况
	swapInfo, err := mem.SwapMemory()
	if err != nil {
		c.log.WithError(err).Error("获取SWAP使用情况失败")
		return nil, fmt.Errorf("获取SWAP使用情况失败: %w", err)
	}

	// 获取磁盘使用情况
	diskInfo, err := disk.Usage("/")
	if err != nil {
		c.log.WithError(err).Error("获取磁盘使用情况失败")
		return nil, fmt.Errorf("获取磁盘使用情况失败: %w", err)
	}

	// 获取负载平均值
	loadInfo, err := load.Avg()
	if err != nil {
		c.log.WithError(err).Error("获取负载平均值失败")
		return nil, fmt.Errorf("获取负载平均值失败: %w", err)
	}

	// 获取TCP连接数
	tcpConnections, err := c.getTCPConnectionCount()
	if err != nil {
		c.log.WithError(err).Warn("获取TCP连接数失败")
		tcpConnections = 0 // 设置默认值
	}

	// 获取网络速度
	downloadSpeed, uploadSpeed, err := c.getNetworkSpeed()
	if err != nil {
		c.log.WithError(err).Warn("获取网络速度失败")
		downloadSpeed, uploadSpeed = 0, 0 // 设置默认值
	}

	// 获取进程数量
	processes, err := process.Processes()
	if err != nil {
		c.log.WithError(err).Error("获取进程数量失败")
		processes = []*process.Process{} // 设置为空切片以避免崩溃
	}

	// 获取运行时间（简化实现，使用程序运行时间）
	uptime := time.Since(c.lastUpdateTime).Seconds()

	return &model.SystemMetrics{
		CpuUsage:       cpuUsage,
		MemoryUsage:    memInfo.UsedPercent,
		DiskUsage:      diskInfo.UsedPercent,
		LoadAverage:    loadInfo.Load1,
		Uptime:         int64(uptime),
		ProcessCount:   int32(len(processes)),
		MemoryTotal:    int64(memInfo.Total),
		MemoryUsed:     int64(memInfo.Used),
		DiskTotal:      int64(diskInfo.Total),
		DiskUsed:       int64(diskInfo.Used),
		TcpConnections: int32(tcpConnections),
		DownloadSpeed:  downloadSpeed,
		UploadSpeed:    uploadSpeed,
		SwapTotal:      int64(swapInfo.Total),
		SwapUsed:       int64(swapInfo.Used),
	}, nil
}

// CollectTrafficMetrics 收集流量监控数据
func (c *DefaultMetricsCollector) CollectTrafficMetrics() (*model.TrafficMetrics, error) {
	c.log.Debug("收集流量监控数据")

	now := time.Now()
	periodStart := c.lastUpdateTime

	// 获取网络接口统计数据
	netStats, err := net.IOCounters(false)
	if err != nil {
		c.log.WithError(err).Error("获取网络统计数据失败")
		return nil, fmt.Errorf("获取网络统计数据失败: %w", err)
	}

	if len(netStats) == 0 {
		c.log.Error("没有网络接口数据")
		return nil, fmt.Errorf("没有网络接口数据")
	}

	// 汇总所有接口的数据
	var currentTraffic TrafficData
	for _, stat := range netStats {
		currentTraffic.BytesIn += int64(stat.BytesRecv)
		currentTraffic.BytesOut += int64(stat.BytesSent)
		currentTraffic.PacketsIn += int64(stat.PacketsRecv)
		currentTraffic.PacketsOut += int64(stat.PacketsSent)
	}
	currentTraffic.Timestamp = now

	var bytesIn, bytesOut, packetsIn, packetsOut int64
	var bandwidthIn, bandwidthOut float64

	if c.lastTrafficData != nil {
		// 计算差值
		bytesIn = currentTraffic.BytesIn - c.lastTrafficData.BytesIn
		bytesOut = currentTraffic.BytesOut - c.lastTrafficData.BytesOut
		packetsIn = currentTraffic.PacketsIn - c.lastTrafficData.PacketsIn
		packetsOut = currentTraffic.PacketsOut - c.lastTrafficData.PacketsOut

		// 计算带宽 (bytes per second)
		duration := now.Sub(c.lastTrafficData.Timestamp).Seconds()
		if duration > 0 {
			bandwidthIn = float64(bytesIn) / duration
			bandwidthOut = float64(bytesOut) / duration
		}
	} else {
		// 首次收集，使用当前值
		bytesIn = currentTraffic.BytesIn
		bytesOut = currentTraffic.BytesOut
		packetsIn = currentTraffic.PacketsIn
		packetsOut = currentTraffic.PacketsOut
	}

	// 更新缓存
	c.lastTrafficData = &currentTraffic

	// 获取实际的TCP连接数
	tcpConnections, err := c.getTCPConnectionCount()
	if err != nil {
		c.log.WithError(err).Warn("获取TCP连接数失败，使用默认值")
		tcpConnections = 0
	}

	c.log.WithFields(logrus.Fields{
		"bytes_in":          bytesIn,
		"bytes_out":         bytesOut,
		"packets_in":        packetsIn,
		"packets_out":       packetsOut,
		"bandwidth_in_bps":  bandwidthIn,
		"bandwidth_out_bps": bandwidthOut,
		"tcp_connections":   tcpConnections,
	}).Debug("流量监控数据统计")

	return &model.TrafficMetrics{
		BytesIn:         bytesIn,
		BytesOut:        bytesOut,
		PacketsIn:       packetsIn,
		PacketsOut:      packetsOut,
		ConnectionCount: int32(tcpConnections),
		BandwidthIn:     bandwidthIn,
		BandwidthOut:    bandwidthOut,
		PeriodStart:     timestamppb.New(periodStart),
		PeriodEnd:       timestamppb.New(now),
	}, nil
}

// CollectConnectionMetrics 收集连接监控数据
func (c *DefaultMetricsCollector) CollectConnectionMetrics() (*model.ConnectionMetrics, error) {
	c.log.Debug("收集连接监控数据")

	// 获取所有TCP连接
	connections, err := net.Connections("tcp")
	if err != nil {
		c.log.WithError(err).Error("获取TCP连接失败")
		return nil, fmt.Errorf("获取TCP连接失败: %w", err)
	}

	// 统计连接状态
	var activeConnections, totalConnections, failedConnections int32
	connectionByProtocol := make(map[string]int32)
	destinationMap := make(map[string]int)

	for _, conn := range connections {
		totalConnections++

		// 统计协议类型
		connectionByProtocol["tcp"]++

		switch conn.Status {
		case "ESTABLISHED":
			activeConnections++
		case "CLOSE_WAIT", "CLOSING", "LAST_ACK", "TIME_WAIT":
			// 这些状态可能表示连接问题
		}

		// 统计目标地址（仅统计外部连接）
		if conn.Raddr.IP != "" && !isLocalAddress(conn.Raddr.IP) {
			destinationMap[conn.Raddr.IP]++
		}
	}

	// 获取UDP连接
	udpConnections, err := net.Connections("udp")
	if err == nil {
		connectionByProtocol["udp"] = int32(len(udpConnections))
	}

	// 获取热门目标地址（前3个）
	topDestinations := getTopDestinations(destinationMap, 3)

	// 计算平均响应时间（简化实现，使用模拟值）
	averageResponseTime := 50.0 // ms

	// 并发用户数（简化为活跃连接数）
	concurrentUsers := activeConnections

	c.log.WithFields(logrus.Fields{
		"active_connections":     activeConnections,
		"total_connections":      totalConnections,
		"failed_connections":     failedConnections,
		"concurrent_users":       concurrentUsers,
		"connection_by_protocol": connectionByProtocol,
		"top_destinations":       topDestinations,
	}).Debug("连接监控数据统计")

	return &model.ConnectionMetrics{
		ActiveConnections:    activeConnections,
		TotalConnections:     totalConnections,
		FailedConnections:    failedConnections,
		AverageResponseTime:  averageResponseTime,
		ConcurrentUsers:      concurrentUsers,
		TopDestinations:      topDestinations,
		ConnectionByProtocol: connectionByProtocol,
	}, nil
}

// CollectPerformanceMetrics 收集性能监控数据
func (c *DefaultMetricsCollector) CollectPerformanceMetrics() (*model.PerformanceMetrics, error) {
	c.log.Debug("收集性能监控数据")

	// 获取内存使用情况
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 获取CPU使用率
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil {
		c.log.WithError(err).Error("获取CPU使用率失败")
		cpuPercent = []float64{0.0}
	}

	var cpuUsage float64
	if len(cpuPercent) > 0 {
		cpuUsage = cpuPercent[0]
	}

	// 计算内存使用率
	memoryUsage := float64(memStats.Alloc) / float64(memStats.Sys) * 100

	// 创建自定义指标映射
	customMetrics := map[string]float64{
		"goroutines": float64(runtime.NumGoroutine()),
		"gc_cycles":  float64(memStats.NumGC),
		"heap_alloc": float64(memStats.HeapAlloc),
		"heap_sys":   float64(memStats.HeapSys),
	}

	// 获取任务统计信息
	if c.taskProvider != nil {
		taskStats := c.taskProvider.GetTaskStats()
		for status, count := range taskStats {
			customMetrics["task_"+strings.ToLower(strings.ReplaceAll(status, "TASK_STATUS_", ""))] = float64(count)
		}

		c.log.WithField("task_stats", taskStats).Debug("Added task statistics to performance metrics")
	}

	return &model.PerformanceMetrics{
		ResponseTime:  50.0,  // 50ms 模拟值
		Throughput:    100.0, // 100 requests/sec 模拟值
		ErrorRate:     0.05,  // 5% 模拟值
		QueueSize:     5,     // 模拟值
		MemoryUsage:   memoryUsage,
		CpuUsage:      cpuUsage,
		CustomMetrics: customMetrics,
	}, nil
}

// CollectErrorMetrics 收集错误监控数据
func (c *DefaultMetricsCollector) CollectErrorMetrics() (*model.ErrorMetrics, error) {
	c.log.Debug("收集错误监控数据")

	// 这里应该从实际的错误监控中获取数据
	// 简化实现，返回模拟数据

	return &model.ErrorMetrics{
		TotalErrors:    10,
		CriticalErrors: 2,
		WarningErrors:  5,
		ErrorTypes:     []string{"connection_error", "timeout_error", "config_error"},
		ErrorCountByType: map[string]int32{
			"connection_error": 3,
			"timeout_error":    4,
			"config_error":     3,
		},
		LastErrorTime: timestamppb.New(time.Now().Add(-time.Hour)), // 1小时前
	}, nil
}

// isLocalAddress 检查IP地址是否为本地地址
func isLocalAddress(ip string) bool {
	if ip == "" || ip == "127.0.0.1" || ip == "::1" || ip == "localhost" {
		return true
	}

	// 检查私有网络地址
	if strings.HasPrefix(ip, "192.168.") ||
		strings.HasPrefix(ip, "10.") ||
		strings.HasPrefix(ip, "172.16.") ||
		strings.HasPrefix(ip, "172.17.") ||
		strings.HasPrefix(ip, "172.18.") ||
		strings.HasPrefix(ip, "172.19.") ||
		strings.HasPrefix(ip, "172.20.") ||
		strings.HasPrefix(ip, "172.21.") ||
		strings.HasPrefix(ip, "172.22.") ||
		strings.HasPrefix(ip, "172.23.") ||
		strings.HasPrefix(ip, "172.24.") ||
		strings.HasPrefix(ip, "172.25.") ||
		strings.HasPrefix(ip, "172.26.") ||
		strings.HasPrefix(ip, "172.27.") ||
		strings.HasPrefix(ip, "172.28.") ||
		strings.HasPrefix(ip, "172.29.") ||
		strings.HasPrefix(ip, "172.30.") ||
		strings.HasPrefix(ip, "172.31.") {
		return true
	}

	return false
}

// getTopDestinations 获取前N个热门目标地址
func getTopDestinations(destinations map[string]int, topN int) []string {
	type destination struct {
		ip    string
		count int
	}

	var dests []destination
	for ip, count := range destinations {
		dests = append(dests, destination{ip: ip, count: count})
	}

	// 按连接数排序
	for i := 0; i < len(dests)-1; i++ {
		for j := i + 1; j < len(dests); j++ {
			if dests[j].count > dests[i].count {
				dests[i], dests[j] = dests[j], dests[i]
			}
		}
	}

	// 返回前N个
	var result []string
	limit := topN
	if len(dests) < limit {
		limit = len(dests)
	}

	for i := 0; i < limit; i++ {
		result = append(result, dests[i].ip)
	}

	return result
}

// getTCPConnectionCount 获取TCP连接数
func (c *DefaultMetricsCollector) getTCPConnectionCount() (int, error) {
	connections, err := net.Connections("tcp")
	if err != nil {
		return 0, fmt.Errorf("获取TCP连接失败: %w", err)
	}

	// 统计不同状态的连接数
	established := 0
	for _, conn := range connections {
		if conn.Status == "ESTABLISHED" {
			established++
		}
	}

	c.log.WithFields(logrus.Fields{
		"total_connections":       len(connections),
		"established_connections": established,
	}).Debug("TCP连接统计")

	return established, nil
}

// getNetworkSpeed 获取网络速度（下载/上传速度）
func (c *DefaultMetricsCollector) getNetworkSpeed() (downloadSpeed, uploadSpeed float64, err error) {
	// 获取当前网络统计
	netStats, err := net.IOCounters(false)
	if err != nil {
		return 0, 0, fmt.Errorf("获取网络统计失败: %w", err)
	}

	if len(netStats) == 0 {
		return 0, 0, fmt.Errorf("没有网络接口数据")
	}

	// 使用所有接口的总和
	var currentStats NetworkStats
	for _, stat := range netStats {
		currentStats.BytesRecv += stat.BytesRecv
		currentStats.BytesSent += stat.BytesSent
		currentStats.PacketsRecv += stat.PacketsRecv
		currentStats.PacketsSent += stat.PacketsSent
	}
	currentStats.Timestamp = time.Now()

	// 如果有上次的数据，计算速度
	if c.lastNetworkStats != nil {
		timeDiff := currentStats.Timestamp.Sub(c.lastNetworkStats.Timestamp).Seconds()
		if timeDiff > 0 {
			downloadSpeed = float64(currentStats.BytesRecv-c.lastNetworkStats.BytesRecv) / timeDiff
			uploadSpeed = float64(currentStats.BytesSent-c.lastNetworkStats.BytesSent) / timeDiff
		}
	}

	// 更新缓存
	c.lastNetworkStats = &currentStats

	c.log.WithFields(logrus.Fields{
		"download_speed_bps": downloadSpeed,
		"upload_speed_bps":   uploadSpeed,
		"bytes_recv":         currentStats.BytesRecv,
		"bytes_sent":         currentStats.BytesSent,
	}).Debug("网络速度统计")

	return downloadSpeed, uploadSpeed, nil
}

// getTCPConnectionsFromProc 从/proc/net/tcp读取TCP连接数（备用方法）
func (c *DefaultMetricsCollector) getTCPConnectionsFromProc() (int, error) {
	file, err := os.Open("/proc/net/tcp")
	if err != nil {
		return 0, fmt.Errorf("打开/proc/net/tcp失败: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	connectionCount := 0
	established := 0

	// 跳过头行
	if scanner.Scan() {
		// 读取数据行
		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				connectionCount++
				// 第4个字段是状态，01表示ESTABLISHED
				if len(fields) > 3 {
					state := fields[3]
					if state == "01" { // ESTABLISHED状态
						established++
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("读取/proc/net/tcp失败: %w", err)
	}

	c.log.WithFields(logrus.Fields{
		"total_tcp_connections":       connectionCount,
		"established_tcp_connections": established,
	}).Debug("从/proc/net/tcp获取TCP连接统计")

	return established, nil
}
