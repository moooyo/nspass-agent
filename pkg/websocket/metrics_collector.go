package websocket

import (
	"fmt"
	"runtime"
	"time"

	"github.com/nspass/nspass-agent/generated/model"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/nspass/nspass-agent/pkg/proxy"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DefaultMetricsCollector 默认监控数据收集器
type DefaultMetricsCollector struct {
	proxyManager *proxy.Manager
	log          *logrus.Entry
	
	// 缓存上次的数据用于计算差值
	lastTrafficData *TrafficData
	lastUpdateTime  time.Time
}

// TrafficData 流量数据结构
type TrafficData struct {
	BytesIn    int64
	BytesOut   int64
	PacketsIn  int64
	PacketsOut int64
	Timestamp  time.Time
}

// NewDefaultMetricsCollector 创建默认监控数据收集器
func NewDefaultMetricsCollector(proxyManager *proxy.Manager) *DefaultMetricsCollector {
	return &DefaultMetricsCollector{
		proxyManager: proxyManager,
		log:          logger.GetComponentLogger("metrics-collector"),
		lastUpdateTime: time.Now(),
	}
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
	
	// 获取进程数量
	processes, err := process.Processes()
	if err != nil {
		c.log.WithError(err).Error("获取进程数量失败")
		processes = []*process.Process{} // 设置为空切片以避免崩溃
	}
	
	// 获取运行时间（简化实现，使用程序运行时间）
	uptime := time.Since(c.lastUpdateTime).Seconds()
	
	return &model.SystemMetrics{
		CpuUsage:     cpuUsage,
		MemoryUsage:  memInfo.UsedPercent,
		DiskUsage:    diskInfo.UsedPercent,
		LoadAverage:  loadInfo.Load1,
		Uptime:       int64(uptime),
		ProcessCount: int32(len(processes)),
		MemoryTotal:  int64(memInfo.Total),
		MemoryUsed:   int64(memInfo.Used),
		DiskTotal:    int64(diskInfo.Total),
		DiskUsed:     int64(diskInfo.Used),
	}, nil
}

// CollectTrafficMetrics 收集流量监控数据
func (c *DefaultMetricsCollector) CollectTrafficMetrics() (*model.TrafficMetrics, error) {
	c.log.Debug("收集流量监控数据")
	
	now := time.Now()
	periodStart := c.lastUpdateTime
	
	// 这里应该从实际的网络接口或代理监控中获取流量数据
	// 简化实现，返回模拟数据
	
	// 模拟流量数据
	currentTraffic := &TrafficData{
		BytesIn:    1024 * 1024,  // 1MB
		BytesOut:   2048 * 1024,  // 2MB
		PacketsIn:  1000,
		PacketsOut: 1500,
		Timestamp:  now,
	}
	
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
	c.lastTrafficData = currentTraffic
	
	// 获取连接数（简化实现）
	connectionCount := int32(10) // 模拟值
	
	return &model.TrafficMetrics{
		BytesIn:         bytesIn,
		BytesOut:        bytesOut,
		PacketsIn:       packetsIn,
		PacketsOut:      packetsOut,
		ConnectionCount: connectionCount,
		BandwidthIn:     bandwidthIn,
		BandwidthOut:    bandwidthOut,
		PeriodStart:     timestamppb.New(periodStart),
		PeriodEnd:       timestamppb.New(now),
	}, nil
}

// CollectConnectionMetrics 收集连接监控数据
func (c *DefaultMetricsCollector) CollectConnectionMetrics() (*model.ConnectionMetrics, error) {
	c.log.Debug("收集连接监控数据")
	
	// 这里应该从实际的连接监控中获取数据
	// 简化实现，返回模拟数据
	
	return &model.ConnectionMetrics{
		ActiveConnections:      10,
		TotalConnections:       100,
		FailedConnections:      5,
		AverageResponseTime:    50.0, // 50ms
		ConcurrentUsers:        8,
		TopDestinations:        []string{"google.com", "github.com", "stackoverflow.com"},
		ConnectionByProtocol: map[string]int32{
			"tcp":  8,
			"udp":  2,
			"http": 5,
		},
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
	
	return &model.PerformanceMetrics{
		ResponseTime:  50.0,  // 50ms 模拟值
		Throughput:    100.0, // 100 requests/sec 模拟值
		ErrorRate:     0.05,  // 5% 模拟值
		QueueSize:     5,     // 模拟值
		MemoryUsage:   memoryUsage,
		CpuUsage:      cpuUsage,
		CustomMetrics: map[string]float64{
			"goroutines": float64(runtime.NumGoroutine()),
			"gc_cycles":  float64(memStats.NumGC),
		},
	}, nil
}

// CollectErrorMetrics 收集错误监控数据
func (c *DefaultMetricsCollector) CollectErrorMetrics() (*model.ErrorMetrics, error) {
	c.log.Debug("收集错误监控数据")
	
	// 这里应该从实际的错误监控中获取数据
	// 简化实现，返回模拟数据
	
	return &model.ErrorMetrics{
		TotalErrors:        10,
		CriticalErrors:     2,
		WarningErrors:      5,
		ErrorTypes:         []string{"connection_error", "timeout_error", "config_error"},
		ErrorCountByType: map[string]int32{
			"connection_error": 3,
			"timeout_error":    4,
			"config_error":     3,
		},
		LastErrorTime: timestamppb.New(time.Now().Add(-time.Hour)), // 1小时前
	}, nil
}
