package logging

import (
	"time"

	"github.com/sirupsen/logrus"
)

// StandardLogLevel 标准日志级别枚举
type StandardLogLevel string

const (
	StandardLevelDebug StandardLogLevel = "debug"
	StandardLevelInfo  StandardLogLevel = "info"
	StandardLevelWarn  StandardLogLevel = "warn"
	StandardLevelError StandardLogLevel = "error"
	StandardLevelFatal StandardLogLevel = "fatal"
)

// StandardFields 标准日志字段
type StandardFields struct {
	Component  string                 `json:"component"`
	Operation  string                 `json:"operation,omitempty"`
	Duration   time.Duration          `json:"duration_ms,omitempty"`
	Error      error                  `json:"error,omitempty"`
	UserID     string                 `json:"user_id,omitempty"`
	RequestID  string                 `json:"request_id,omitempty"`
	ProxyType  string                 `json:"proxy_type,omitempty"`
	ProcessID  int                    `json:"process_id,omitempty"`
	ConfigPath string                 `json:"config_path,omitempty"`
	BinaryPath string                 `json:"binary_path,omitempty"`
	Custom     map[string]interface{} `json:"custom,omitempty"`
}

// ToLogrusFields 转换为logrus字段
func (sf StandardFields) ToLogrusFields() logrus.Fields {
	fields := logrus.Fields{}

	if sf.Component != "" {
		fields["component"] = sf.Component
	}
	if sf.Operation != "" {
		fields["operation"] = sf.Operation
	}
	if sf.Duration > 0 {
		fields["duration_ms"] = sf.Duration.Milliseconds()
	}
	if sf.Error != nil {
		fields["error"] = sf.Error.Error()
	}
	if sf.UserID != "" {
		fields["user_id"] = sf.UserID
	}
	if sf.RequestID != "" {
		fields["request_id"] = sf.RequestID
	}
	if sf.ProxyType != "" {
		fields["proxy_type"] = sf.ProxyType
	}
	if sf.ProcessID > 0 {
		fields["process_id"] = sf.ProcessID
	}
	if sf.ConfigPath != "" {
		fields["config_path"] = sf.ConfigPath
	}
	if sf.BinaryPath != "" {
		fields["binary_path"] = sf.BinaryPath
	}

	// 添加自定义字段
	for k, v := range sf.Custom {
		fields[k] = v
	}

	return fields
}

// StandardLogger 标准化日志记录器
type StandardLogger struct {
	logger    *logrus.Logger
	component string
}

// NewStandardLogger 创建标准化日志记录器
func NewStandardLogger(component string) *StandardLogger {
	// 使用全局配置的logger实例，而不是创建新的实例
	if globalLogger == nil {
		// 如果全局logger未初始化，使用默认配置初始化
		config := DefaultConfig()
		Initialize(config)
	}

	return &StandardLogger{
		logger:    globalLogger,
		component: component,
	}
}

// Debug 记录调试信息
func (sl *StandardLogger) Debug(message string, fields StandardFields) {
	fields.Component = sl.component
	sl.logger.WithFields(fields.ToLogrusFields()).Debug(message)
}

// Info 记录信息
func (sl *StandardLogger) Info(message string, fields StandardFields) {
	fields.Component = sl.component
	sl.logger.WithFields(fields.ToLogrusFields()).Info(message)
}

// Warn 记录警告
func (sl *StandardLogger) Warn(message string, fields StandardFields) {
	fields.Component = sl.component
	sl.logger.WithFields(fields.ToLogrusFields()).Warn(message)
}

// Error 记录错误
func (sl *StandardLogger) Error(message string, fields StandardFields) {
	fields.Component = sl.component
	sl.logger.WithFields(fields.ToLogrusFields()).Error(message)
}

// Fatal 记录致命错误
func (sl *StandardLogger) Fatal(message string, fields StandardFields) {
	fields.Component = sl.component
	sl.logger.WithFields(fields.ToLogrusFields()).Fatal(message)
}

// WithOperation 添加操作上下文
func (sl *StandardLogger) WithOperation(operation string) *OperationLogger {
	return &OperationLogger{
		standardLogger: sl,
		operation:      operation,
		startTime:      time.Now(),
	}
}

// OperationLogger 操作日志记录器
type OperationLogger struct {
	standardLogger *StandardLogger
	operation      string
	startTime      time.Time
}

// Debug 记录调试信息
func (ol *OperationLogger) Debug(message string, fields StandardFields) {
	fields.Operation = ol.operation
	fields.Duration = time.Since(ol.startTime)
	ol.standardLogger.Debug(message, fields)
}

// Info 记录信息
func (ol *OperationLogger) Info(message string, fields StandardFields) {
	fields.Operation = ol.operation
	fields.Duration = time.Since(ol.startTime)
	ol.standardLogger.Info(message, fields)
}

// Warn 记录警告
func (ol *OperationLogger) Warn(message string, fields StandardFields) {
	fields.Operation = ol.operation
	fields.Duration = time.Since(ol.startTime)
	ol.standardLogger.Warn(message, fields)
}

// Error 记录错误
func (ol *OperationLogger) Error(message string, fields StandardFields) {
	fields.Operation = ol.operation
	fields.Duration = time.Since(ol.startTime)
	ol.standardLogger.Error(message, fields)
}

// Success 记录操作成功
func (ol *OperationLogger) Success(message string, fields StandardFields) {
	fields.Operation = ol.operation
	fields.Duration = time.Since(ol.startTime)
	ol.standardLogger.Info(message+" (成功)", fields)
}

// Failure 记录操作失败
func (ol *OperationLogger) Failure(message string, err error, fields StandardFields) {
	fields.Operation = ol.operation
	fields.Duration = time.Since(ol.startTime)
	fields.Error = err
	ol.standardLogger.Error(message+" (失败)", fields)
}

// ProxyLogger 代理专用日志记录器
type ProxyLogger struct {
	*StandardLogger
	proxyType string
}

// NewProxyLogger 创建代理日志记录器
func NewProxyLogger(proxyType string) *ProxyLogger {
	return &ProxyLogger{
		StandardLogger: NewStandardLogger("proxy"),
		proxyType:      proxyType,
	}
}

// LogProxyOperation 记录代理操作
func (pl *ProxyLogger) LogProxyOperation(operation, message string, fields StandardFields) {
	fields.ProxyType = pl.proxyType
	fields.Operation = operation
	pl.Info(message, fields)
}

// LogProxyError 记录代理错误
func (pl *ProxyLogger) LogProxyError(operation, message string, err error, fields StandardFields) {
	fields.ProxyType = pl.proxyType
	fields.Operation = operation
	fields.Error = err
	pl.Error(message, fields)
}

// LogProcessOperation 记录进程操作
func (pl *ProxyLogger) LogProcessOperation(operation, message string, pid int, fields StandardFields) {
	fields.ProxyType = pl.proxyType
	fields.Operation = operation
	fields.ProcessID = pid
	pl.Info(message, fields)
}

// 全局标准化日志记录器实例
var (
	agentLogger   *StandardLogger
	proxyLogger   *StandardLogger
	processLogger *StandardLogger
	configLogger  *StandardLogger
)

// InitStandardLoggers 初始化标准化日志记录器
func InitStandardLoggers() {
	agentLogger = NewStandardLogger("agent")
	proxyLogger = NewStandardLogger("proxy")
	processLogger = NewStandardLogger("process")
	configLogger = NewStandardLogger("config")
}

// GetAgentLogger 获取Agent日志记录器
func GetAgentLogger() *StandardLogger {
	if agentLogger == nil {
		InitStandardLoggers()
	}
	return agentLogger
}

// GetStandardProxyLogger 获取代理日志记录器
func GetStandardProxyLogger() *StandardLogger {
	if proxyLogger == nil {
		InitStandardLoggers()
	}
	return proxyLogger
}

// GetStandardProcessLogger 获取进程日志记录器
func GetStandardProcessLogger() *StandardLogger {
	if processLogger == nil {
		InitStandardLoggers()
	}
	return processLogger
}

// GetStandardConfigLogger 获取配置日志记录器
func GetStandardConfigLogger() *StandardLogger {
	if configLogger == nil {
		InitStandardLoggers()
	}
	return configLogger
}
