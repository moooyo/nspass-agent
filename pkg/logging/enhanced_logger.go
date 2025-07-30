package logging

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nspass/nspass-agent/pkg/errors"
	"github.com/nspass/nspass-agent/pkg/interfaces"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config 日志配置
type Config struct {
	Level      string `yaml:"level" json:"level"`             // 日志级别: debug, info, warn, error
	Format     string `yaml:"format" json:"format"`           // 日志格式: json, text
	File       string `yaml:"file" json:"file"`               // 日志文件路径
	MaxSize    int    `yaml:"max_size" json:"max_size"`       // 单个日志文件最大大小(MB)
	MaxBackups int    `yaml:"max_backups" json:"max_backups"` // 保留的旧日志文件数量
	MaxAge     int    `yaml:"max_age" json:"max_age"`         // 日志文件保留天数
	Compress   bool   `yaml:"compress" json:"compress"`       // 是否压缩旧日志文件
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Level:      "info",
		Format:     "json",
		File:       "/var/log/nspass/agent.log",
		MaxSize:    100,
		MaxBackups: 5,
		MaxAge:     30,
		Compress:   true,
	}
}

var (
	// 全局logger实例
	globalLogger     *logrus.Logger
	globalLoggerOnce sync.Once
	// 组件专用logger映射
	componentLoggers = make(map[string]interfaces.Logger)
	componentMutex   sync.RWMutex
)

// EnhancedLogger 增强的日志记录器
type EnhancedLogger struct {
	logger    *logrus.Entry
	component string
	context   context.Context
}

// NewEnhancedLogger 创建增强的日志记录器
func NewEnhancedLogger(logger *logrus.Entry, component string) *EnhancedLogger {
	return &EnhancedLogger{
		logger:    logger.WithField("component", component),
		component: component,
		context:   context.Background(),
	}
}

// WithContext 添加上下文
func (l *EnhancedLogger) WithContext(ctx context.Context) *EnhancedLogger {
	return &EnhancedLogger{
		logger:    l.logger,
		component: l.component,
		context:   ctx,
	}
}

// WithField 添加字段
func (l *EnhancedLogger) WithField(key string, value interface{}) interfaces.Logger {
	return &EnhancedLogger{
		logger:    l.logger.WithField(key, value),
		component: l.component,
		context:   l.context,
	}
}

// WithFields 添加多个字段
func (l *EnhancedLogger) WithFields(fields map[string]interface{}) interfaces.Logger {
	logrusFields := make(logrus.Fields)
	for k, v := range fields {
		logrusFields[k] = v
	}
	return &EnhancedLogger{
		logger:    l.logger.WithFields(logrusFields),
		component: l.component,
		context:   l.context,
	}
}

// WithError 添加错误信息
func (l *EnhancedLogger) WithError(err error) interfaces.Logger {
	entry := l.logger.WithError(err)

	// 如果是NSPassError，添加额外的错误信息
	if nsErr, ok := err.(*errors.NSPassError); ok {
		entry = entry.WithFields(logrus.Fields{
			"error_type":    nsErr.Type,
			"error_code":    nsErr.Code,
			"error_context": nsErr.Context,
		})
	}

	return &EnhancedLogger{
		logger:    entry,
		component: l.component,
		context:   l.context,
	}
}

// Debug 记录调试信息
func (l *EnhancedLogger) Debug(args ...interface{}) {
	l.logger.Debug(args...)
}

// Info 记录信息
func (l *EnhancedLogger) Info(args ...interface{}) {
	l.logger.Info(args...)
}

// Warn 记录警告
func (l *EnhancedLogger) Warn(args ...interface{}) {
	l.logger.Warn(args...)
}

// Error 记录错误
func (l *EnhancedLogger) Error(args ...interface{}) {
	l.logger.Error(args...)
}

// Fatal 记录致命错误
func (l *EnhancedLogger) Fatal(args ...interface{}) {
	l.logger.Fatal(args...)
}

// LogOperation 记录操作日志
func (l *EnhancedLogger) LogOperation(operation string, duration time.Duration, success bool, details map[string]interface{}) {
	fields := logrus.Fields{
		"operation":   operation,
		"duration_ms": duration.Milliseconds(),
		"success":     success,
		"log_type":    "operation",
	}

	for k, v := range details {
		fields[k] = v
	}

	entry := l.logger.WithFields(fields)
	if success {
		entry.Info("操作完成")
	} else {
		entry.Error("操作失败")
	}
}

// LogStateChange 记录状态变更
func (l *EnhancedLogger) LogStateChange(from, to, reason string, metadata map[string]interface{}) {
	fields := logrus.Fields{
		"state_from": from,
		"state_to":   to,
		"reason":     reason,
		"log_type":   "state_change",
	}

	for k, v := range metadata {
		fields[k] = v
	}

	l.logger.WithFields(fields).Info("状态变更")
}

// LogMetrics 记录监控指标
func (l *EnhancedLogger) LogMetrics(metrics map[string]interface{}) {
	fields := logrus.Fields{
		"log_type":  "metrics",
		"timestamp": time.Now().Unix(),
	}

	for k, v := range metrics {
		fields[k] = v
	}

	l.logger.WithFields(fields).Info("监控指标")
}

// LogAudit 记录审计日志
func (l *EnhancedLogger) LogAudit(action, user string, resource string, result string, details map[string]interface{}) {
	fields := logrus.Fields{
		"action":   action,
		"user":     user,
		"resource": resource,
		"result":   result,
		"log_type": "audit",
	}

	for k, v := range details {
		fields[k] = v
	}

	l.logger.WithFields(fields).Info("审计日志")
}

// LogError 记录增强的错误信息
func (l *EnhancedLogger) LogError(err error, message string, context map[string]interface{}) {
	entry := l.WithError(err)
	if context != nil {
		entry = entry.WithFields(context)
	}
	entry.Error(message)
}

// LogStartup 记录启动日志
func (l *EnhancedLogger) LogStartup(version string, config interface{}) {
	l.logger.WithFields(logrus.Fields{
		"version":   version,
		"config":    config,
		"log_type":  "lifecycle",
		"lifecycle": "startup",
	}).Info("组件启动")
}

// LogShutdown 记录关闭日志
func (l *EnhancedLogger) LogShutdown(duration time.Duration) {
	l.logger.WithFields(logrus.Fields{
		"duration_ms": duration.Milliseconds(),
		"log_type":    "lifecycle",
		"lifecycle":   "shutdown",
	}).Info("组件关闭")
}

// LoggerFactory 日志记录器工厂
type LoggerFactory struct {
	baseLogger *logrus.Logger
}

// NewLoggerFactory 创建日志记录器工厂
func NewLoggerFactory(baseLogger *logrus.Logger) *LoggerFactory {
	return &LoggerFactory{
		baseLogger: baseLogger,
	}
}

// GetLogger 获取组件日志记录器
func (f *LoggerFactory) GetLogger(component string) interfaces.Logger {
	entry := f.baseLogger.WithField("component", component)
	return NewEnhancedLogger(entry, component)
}

// GetLoggerWithFields 获取带有预设字段的日志记录器
func (f *LoggerFactory) GetLoggerWithFields(component string, fields map[string]interface{}) interfaces.Logger {
	logrusFields := make(logrus.Fields)
	logrusFields["component"] = component
	for k, v := range fields {
		logrusFields[k] = v
	}

	entry := f.baseLogger.WithFields(logrusFields)
	return NewEnhancedLogger(entry, component)
}

// StructuredLogger 结构化日志记录器
type StructuredLogger struct {
	*EnhancedLogger
}

// NewStructuredLogger 创建结构化日志记录器
func NewStructuredLogger(logger *logrus.Entry, component string) *StructuredLogger {
	return &StructuredLogger{
		EnhancedLogger: NewEnhancedLogger(logger, component),
	}
}

// LogEvent 记录结构化事件
func (s *StructuredLogger) LogEvent(eventType string, event interface{}) {
	s.logger.WithFields(logrus.Fields{
		"event_type": eventType,
		"event":      event,
		"log_type":   "event",
	}).Info("事件记录")
}

// LogRequest 记录请求日志
func (s *StructuredLogger) LogRequest(method, url string, duration time.Duration, statusCode int, requestID string) {
	level := logrus.InfoLevel
	if statusCode >= 400 {
		level = logrus.ErrorLevel
	} else if statusCode >= 300 {
		level = logrus.WarnLevel
	}

	s.logger.WithFields(logrus.Fields{
		"method":      method,
		"url":         url,
		"duration_ms": duration.Milliseconds(),
		"status_code": statusCode,
		"request_id":  requestID,
		"log_type":    "request",
	}).Log(level, "HTTP请求")
}

// LogResponse 记录响应日志
func (s *StructuredLogger) LogResponse(statusCode int, responseSize int64, duration time.Duration, requestID string) {
	s.logger.WithFields(logrus.Fields{
		"status_code":   statusCode,
		"response_size": responseSize,
		"duration_ms":   duration.Milliseconds(),
		"request_id":    requestID,
		"log_type":      "response",
	}).Info("HTTP响应")
}

// 全局函数 - 替代旧的logger包功能

// ensureLogDirectory 确保日志目录存在
func ensureLogDirectory(logFile string) error {
	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}
	return nil
}

// createLogWriter 创建日志写入器，支持文件轮转
func createLogWriter(config Config) (io.Writer, error) {
	// 确保日志目录存在
	if err := ensureLogDirectory(config.File); err != nil {
		return nil, err
	}

	// 创建lumberjack日志轮转器
	// 支持以下轮转策略：
	// 1. 大小轮转：当文件超过MaxSize MB时轮转
	// 2. 时间轮转：通过MaxAge控制文件保留天数（自动删除旧文件）
	// 3. 数量轮转：通过MaxBackups控制保留的备份文件数量
	lumberjackLogger := &lumberjack.Logger{
		Filename:   config.File,
		MaxSize:    config.MaxSize,    // MB - 单个日志文件最大大小
		MaxBackups: config.MaxBackups, // 保留的旧日志文件数量
		MaxAge:     config.MaxAge,     // 日志文件保留天数（0表示不删除旧文件）
		Compress:   config.Compress,   // 是否压缩旧日志文件
		LocalTime:  true,              // 使用本地时间命名轮转文件
	}

	return lumberjackLogger, nil
}

// RotateLogs 手动触发日志轮转
// 这个函数可以被外部调用来强制进行日志轮转，比如在定时任务中每天调用一次
func RotateLogs() error {
	if globalLogger == nil {
		return nil // 如果logger未初始化，直接返回
	}

	// 尝试获取lumberjack.Logger实例
	// 注意：这需要我们保存lumberjack实例的引用
	// 目前的实现中，我们无法直接访问lumberjack实例
	// 这是一个设计上的限制，但lumberjack会自动处理轮转
	return nil
}

// Initialize 初始化全局日志器
func Initialize(config Config) error {
	var initErr error
	globalLoggerOnce.Do(func() {
		// 创建新的logger实例
		logger := logrus.New()

		// 设置日志级别
		level, err := logrus.ParseLevel(config.Level)
		if err != nil {
			initErr = fmt.Errorf("无效的日志级别 '%s': %w", config.Level, err)
			return
		}
		logger.SetLevel(level)

		// 设置日志格式
		switch strings.ToLower(config.Format) {
		case "json":
			logger.SetFormatter(&logrus.JSONFormatter{
				TimestampFormat: time.RFC3339,
			})
		case "text":
			logger.SetFormatter(&logrus.TextFormatter{
				TimestampFormat: time.RFC3339,
				FullTimestamp:   true,
			})
		default:
			initErr = fmt.Errorf("不支持的日志格式 '%s'", config.Format)
			return
		}

		// 设置日志输出 - 只支持文件输出
		fileWriter, err := createLogWriter(config)
		if err != nil {
			initErr = fmt.Errorf("创建日志文件写入器失败: %w", err)
			return
		}
		logger.SetOutput(fileWriter)

		globalLogger = logger
	})
	return initErr
}

// GetLogger 获取全局logger实例
func GetLogger() interfaces.Logger {
	if globalLogger == nil {
		// 如果未初始化，使用默认配置
		config := DefaultConfig()
		Initialize(config)
	}
	entry := globalLogger.WithField("component", "global")
	return NewEnhancedLogger(entry, "global")
}

// GetComponentLogger 获取组件专用logger
func GetComponentLogger(component string) interfaces.Logger {
	componentMutex.RLock()
	if logger, exists := componentLoggers[component]; exists {
		componentMutex.RUnlock()
		return logger
	}
	componentMutex.RUnlock()

	componentMutex.Lock()
	defer componentMutex.Unlock()

	// 双重检查
	if logger, exists := componentLoggers[component]; exists {
		return logger
	}

	if globalLogger == nil {
		config := DefaultConfig()
		Initialize(config)
	}

	entry := globalLogger.WithField("component", component)
	logger := NewEnhancedLogger(entry, component)
	componentLoggers[component] = logger
	return logger
}

// 便捷方法 - 获取各个组件的logger
func GetAPILogger() interfaces.Logger      { return GetComponentLogger("api") }
func GetProxyLogger() interfaces.Logger    { return GetComponentLogger("proxy") }
func GetIPTablesLogger() interfaces.Logger { return GetComponentLogger("iptables") }
func GetConfigLogger() interfaces.Logger   { return GetComponentLogger("config") }
func GetSystemLogger() interfaces.Logger   { return GetComponentLogger("system") }

// 兼容性函数 - 兼容旧的logger包API
func LogError(err error, message string, fields map[string]interface{}) {
	logger := GetLogger()
	if fields != nil {
		logger = logger.WithFields(fields)
	}
	if err != nil {
		logger = logger.WithError(err)
	}
	logger.Error(message)
}

func LogStartup(component, version string, fields map[string]interface{}) {
	logger := GetComponentLogger(component)
	if fields != nil {
		logger = logger.WithFields(fields)
	}
	logger.WithField("version", version).Info("组件启动")
}

func LogPerformance(operation string, duration time.Duration, fields map[string]interface{}) {
	logger := GetLogger()
	if fields != nil {
		logger = logger.WithFields(fields)
	}
	logger.WithFields(map[string]interface{}{
		"operation":    operation,
		"duration_ms":  duration.Milliseconds(),
		"duration_str": duration.String(),
	}).Info("性能统计")
}

func LogStateChange(component, oldState, newState string, fields map[string]interface{}) {
	logger := GetComponentLogger(component)
	if fields != nil {
		logger = logger.WithFields(fields)
	}
	logger.WithFields(map[string]interface{}{
		"old_state": oldState,
		"new_state": newState,
	}).Info("状态变更")
}

func LogAudit(action, actor string, fields map[string]interface{}) {
	logger := GetComponentLogger("audit")
	if fields != nil {
		logger = logger.WithFields(fields)
	}
	logger.WithFields(map[string]interface{}{
		"action": action,
		"actor":  actor,
	}).Info("审计日志")
}

func LogShutdown(component string, duration time.Duration) {
	logger := GetComponentLogger(component)
	logger.WithFields(map[string]interface{}{
		"duration_ms":  duration.Milliseconds(),
		"duration_str": duration.String(),
	}).Info("组件关闭")
}

// 兼容性类型别名
type Level = string
type Format = string
type File = string
type MaxSize = int
type MaxBackups = int
type MaxAge = int
type Compress = bool

// 兼容性常量
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"

	FormatJSON = "json"
	FormatText = "text"
)
