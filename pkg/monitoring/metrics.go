package monitoring

import (
	"context"
	"sync"
	"time"

	"github.com/nspass/nspass-agent/pkg/logging"
)

// MetricType 指标类型
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// Metric 指标接口
type Metric interface {
	Name() string
	Type() MetricType
	Value() interface{}
	Labels() map[string]string
	Timestamp() time.Time
}

// Counter 计数器指标
type Counter struct {
	name      string
	value     int64
	labels    map[string]string
	timestamp time.Time
	mu        sync.RWMutex
}

// NewCounter 创建计数器
func NewCounter(name string, labels map[string]string) *Counter {
	return &Counter{
		name:      name,
		labels:    labels,
		timestamp: time.Now(),
	}
}

func (c *Counter) Name() string                 { return c.name }
func (c *Counter) Type() MetricType             { return MetricTypeCounter }
func (c *Counter) Labels() map[string]string   { return c.labels }
func (c *Counter) Timestamp() time.Time        { return c.timestamp }

func (c *Counter) Value() interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value
}

// Inc 增加计数
func (c *Counter) Inc() {
	c.Add(1)
}

// Add 增加指定值
func (c *Counter) Add(delta int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value += delta
	c.timestamp = time.Now()
}

// Gauge 仪表盘指标
type Gauge struct {
	name      string
	value     float64
	labels    map[string]string
	timestamp time.Time
	mu        sync.RWMutex
}

// NewGauge 创建仪表盘指标
func NewGauge(name string, labels map[string]string) *Gauge {
	return &Gauge{
		name:      name,
		labels:    labels,
		timestamp: time.Now(),
	}
}

func (g *Gauge) Name() string                 { return g.name }
func (g *Gauge) Type() MetricType             { return MetricTypeGauge }
func (g *Gauge) Labels() map[string]string   { return g.labels }
func (g *Gauge) Timestamp() time.Time        { return g.timestamp }

func (g *Gauge) Value() interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.value
}

// Set 设置值
func (g *Gauge) Set(value float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value = value
	g.timestamp = time.Now()
}

// Inc 增加1
func (g *Gauge) Inc() {
	g.Add(1)
}

// Dec 减少1
func (g *Gauge) Dec() {
	g.Add(-1)
}

// Add 增加指定值
func (g *Gauge) Add(delta float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value += delta
	g.timestamp = time.Now()
}

// Histogram 直方图指标
type Histogram struct {
	name      string
	buckets   []float64
	counts    []int64
	sum       float64
	count     int64
	labels    map[string]string
	timestamp time.Time
	mu        sync.RWMutex
}

// NewHistogram 创建直方图指标
func NewHistogram(name string, buckets []float64, labels map[string]string) *Histogram {
	return &Histogram{
		name:      name,
		buckets:   buckets,
		counts:    make([]int64, len(buckets)+1),
		labels:    labels,
		timestamp: time.Now(),
	}
}

func (h *Histogram) Name() string                 { return h.name }
func (h *Histogram) Type() MetricType             { return MetricTypeHistogram }
func (h *Histogram) Labels() map[string]string   { return h.labels }
func (h *Histogram) Timestamp() time.Time        { return h.timestamp }

func (h *Histogram) Value() interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return map[string]interface{}{
		"buckets": h.buckets,
		"counts":  h.counts,
		"sum":     h.sum,
		"count":   h.count,
	}
}

// Observe 观察值
func (h *Histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum += value
	h.count++

	for i, bucket := range h.buckets {
		if value <= bucket {
			h.counts[i]++
		}
	}
	// 最后一个桶包含所有值
	h.counts[len(h.buckets)]++
	h.timestamp = time.Now()
}

// MetricsCollector 指标收集器
type MetricsCollector struct {
	metrics map[string]Metric
	logger  *logging.StandardLogger
	mu      sync.RWMutex
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		metrics: make(map[string]Metric),
		logger:  logging.NewStandardLogger("metrics"),
	}
}

// RegisterCounter 注册计数器
func (mc *MetricsCollector) RegisterCounter(name string, labels map[string]string) *Counter {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := mc.buildKey(name, labels)
	if existing, exists := mc.metrics[key]; exists {
		if counter, ok := existing.(*Counter); ok {
			return counter
		}
	}

	counter := NewCounter(name, labels)
	mc.metrics[key] = counter
	return counter
}

// RegisterGauge 注册仪表盘指标
func (mc *MetricsCollector) RegisterGauge(name string, labels map[string]string) *Gauge {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := mc.buildKey(name, labels)
	if existing, exists := mc.metrics[key]; exists {
		if gauge, ok := existing.(*Gauge); ok {
			return gauge
		}
	}

	gauge := NewGauge(name, labels)
	mc.metrics[key] = gauge
	return gauge
}

// RegisterHistogram 注册直方图指标
func (mc *MetricsCollector) RegisterHistogram(name string, buckets []float64, labels map[string]string) *Histogram {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := mc.buildKey(name, labels)
	if existing, exists := mc.metrics[key]; exists {
		if histogram, ok := existing.(*Histogram); ok {
			return histogram
		}
	}

	histogram := NewHistogram(name, buckets, labels)
	mc.metrics[key] = histogram
	return histogram
}

// GetMetrics 获取所有指标
func (mc *MetricsCollector) GetMetrics() map[string]Metric {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	result := make(map[string]Metric)
	for k, v := range mc.metrics {
		result[k] = v
	}
	return result
}

// GetMetric 获取指定指标
func (mc *MetricsCollector) GetMetric(name string, labels map[string]string) (Metric, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	key := mc.buildKey(name, labels)
	metric, exists := mc.metrics[key]
	return metric, exists
}

// buildKey 构建指标键
func (mc *MetricsCollector) buildKey(name string, labels map[string]string) string {
	key := name
	for k, v := range labels {
		key += ":" + k + "=" + v
	}
	return key
}

// ProxyMetrics 代理指标
type ProxyMetrics struct {
	collector     *MetricsCollector
	proxyType     string
	startCounter  *Counter
	stopCounter   *Counter
	errorCounter  *Counter
	statusGauge   *Gauge
	uptimeGauge   *Gauge
	startTime     time.Time
}

// NewProxyMetrics 创建代理指标
func NewProxyMetrics(collector *MetricsCollector, proxyType string) *ProxyMetrics {
	labels := map[string]string{"proxy_type": proxyType}

	return &ProxyMetrics{
		collector:    collector,
		proxyType:    proxyType,
		startCounter: collector.RegisterCounter("proxy_starts_total", labels),
		stopCounter:  collector.RegisterCounter("proxy_stops_total", labels),
		errorCounter: collector.RegisterCounter("proxy_errors_total", labels),
		statusGauge:  collector.RegisterGauge("proxy_status", labels),
		uptimeGauge:  collector.RegisterGauge("proxy_uptime_seconds", labels),
		startTime:    time.Now(),
	}
}

// RecordStart 记录启动
func (pm *ProxyMetrics) RecordStart() {
	pm.startCounter.Inc()
	pm.statusGauge.Set(1) // 1表示运行中
	pm.startTime = time.Now()
}

// RecordStop 记录停止
func (pm *ProxyMetrics) RecordStop() {
	pm.stopCounter.Inc()
	pm.statusGauge.Set(0) // 0表示已停止
}

// RecordError 记录错误
func (pm *ProxyMetrics) RecordError() {
	pm.errorCounter.Inc()
}

// UpdateUptime 更新运行时间
func (pm *ProxyMetrics) UpdateUptime() {
	if pm.statusGauge.Value().(float64) == 1 {
		uptime := time.Since(pm.startTime).Seconds()
		pm.uptimeGauge.Set(uptime)
	}
}

// Monitor 监控器
type Monitor struct {
	collector *MetricsCollector
	logger    *logging.StandardLogger
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewMonitor 创建监控器
func NewMonitor() *Monitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Monitor{
		collector: NewMetricsCollector(),
		logger:    logging.NewStandardLogger("monitor"),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// GetCollector 获取指标收集器
func (m *Monitor) GetCollector() *MetricsCollector {
	return m.collector
}

// Start 启动监控
func (m *Monitor) Start() {
	go m.collectMetrics()
}

// Stop 停止监控
func (m *Monitor) Stop() {
	m.cancel()
}

// collectMetrics 收集指标
func (m *Monitor) collectMetrics() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.logger.Debug("收集指标", logging.StandardFields{})
			// 这里可以添加定期指标收集逻辑
		}
	}
}

// 全局监控器实例
var globalMonitor *Monitor

// GetGlobalMonitor 获取全局监控器
func GetGlobalMonitor() *Monitor {
	if globalMonitor == nil {
		globalMonitor = NewMonitor()
	}
	return globalMonitor
}
