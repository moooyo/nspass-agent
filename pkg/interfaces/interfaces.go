package interfaces

import (
	"context"

	"github.com/moooyo/nspass-proto/generated/model"
)

// ProxyManager 代理管理器接口
type ProxyManager interface {
	// 配置管理
	UpdateProxies(configs []*model.EgressItem) error
	GetProxyStatus(proxyID string) (string, error)

	// 生命周期管理
	StartProxy(proxyID string) error
	StopProxy(proxyID string) error
	RestartProxy(proxyID string) error
	RestartAll() error
	RestartProxyForDomain(domain string) error

	// 监控相关
	StartMonitor() error
	StopMonitor() error
	GetStatus() map[string]interface{}

	// 统计信息 - 兼容原有接口
	GetProxyStats() map[string]interface{}
}

// IPTablesManager iptables管理器接口
type IPTablesManager interface {
	// 规则管理
	ApplyRules(rules []*model.IptablesConfig) error
	RemoveRules(ruleIDs []string) error

	// 状态查询
	GetRulesSummary() map[string]interface{}
	IsRuleActive(ruleID string) bool

	// 备份恢复
	BackupRules() error
	RestoreRules() error
}

// TaskHandler 任务处理器接口
type TaskHandler interface {
	HandleTask(ctx context.Context, task *model.TaskMessage) (*model.TaskResult, error)
	CheckTaskStatus(taskID string, taskType model.TaskType) (shouldExecute bool, existingResult *model.TaskResult)
	GetTaskStats() map[string]int
}

// MetricsCollector 监控数据收集器接口
type MetricsCollector interface {
	CollectSystemMetrics() (*model.SystemMetrics, error)
	CollectTrafficMetrics() (*model.TrafficMetrics, error)
	CollectConnectionMetrics() (*model.ConnectionMetrics, error)
	CollectPerformanceMetrics() (*model.PerformanceMetrics, error)
	CollectErrorMetrics() (*model.ErrorMetrics, error)
}

// WebSocketClient WebSocket客户端接口
type WebSocketClient interface {
	// 连接管理
	Start() error
	Stop() error
	IsConnected() bool

	// 消息发送
	SendMessage(message *model.WebSocketMessage) error
	SendHeartbeat() error
	SendMetrics(metricsType model.MetricsType) error

	// 状态查询
	GetConnectionStatus() map[string]interface{}
}

// Logger 日志记录器接口
type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})

	WithField(key string, value interface{}) Logger
	WithFields(fields map[string]interface{}) Logger
	WithError(err error) Logger
}

// AgentService Agent服务接口
type AgentService interface {
	// 生命周期管理
	Start() error
	Stop() error
	IsRunning() bool

	// 状态查询
	GetServerID() string
	GetStatus() map[string]interface{}
}

// ConfigManager 配置管理器接口
type ConfigManager interface {
	LoadConfig(path string) error
	SaveConfig(path string) error
	GetConfig() interface{}
	ValidateConfig() error
	ReloadConfig() error
}

// HealthChecker 健康检查器接口
type HealthChecker interface {
	CheckHealth() (bool, error)
	GetHealthStatus() map[string]interface{}
}

// Upgrader 升级器接口
type Upgrader interface {
	UpgradeAgent(version string) error
	UpgradeProxy(proxyType, version string) error
	GetUpgradeStatus() map[string]interface{}
}
