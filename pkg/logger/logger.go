package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config 日志配置
type Config struct {
	Level      string `yaml:"level" json:"level"`             // 日志级别: debug, info, warn, error
	Format     string `yaml:"format" json:"format"`           // 日志格式: json, text
	Output     string `yaml:"output" json:"output"`           // 输出方式: stdout, file, both
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
		Output:     "stdout",
		File:       "/var/log/nspass/agent.log",
		MaxSize:    100,
		MaxBackups: 5,
		MaxAge:     30,
		Compress:   true,
	}
}

var (
	// 全局logger实例
	globalLogger *logrus.Logger
	// 组件专用logger映射
	componentLoggers = make(map[string]*logrus.Entry)
)

// Initialize 初始化全局日志器
func Initialize(config Config) error {
	// 创建新的logger实例
	logger := logrus.New()

	// 设置日志级别
	level, err := logrus.ParseLevel(config.Level)
	if err != nil {
		return fmt.Errorf("无效的日志级别 '%s': %w", config.Level, err)
	}
	logger.SetLevel(level)

	// 设置日志格式
	switch strings.ToLower(config.Format) {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
				logrus.FieldKeyFunc:  "function",
				logrus.FieldKeyFile:  "file",
			},
		})
	case "text":
		logger.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: time.RFC3339,
			FullTimestamp:   true,
		})
	default:
		return fmt.Errorf("不支持的日志格式: %s", config.Format)
	}

	// 设置日志输出
	switch strings.ToLower(config.Output) {
	case "stdout":
		logger.SetOutput(os.Stdout)
	case "file":
		output, err := setupFileOutput(config)
		if err != nil {
			return fmt.Errorf("设置文件输出失败: %w", err)
		}
		logger.SetOutput(output)
	case "both":
		fileOutput, err := setupFileOutput(config)
		if err != nil {
			return fmt.Errorf("设置文件输出失败: %w", err)
		}
		logger.SetOutput(io.MultiWriter(os.Stdout, fileOutput))
	default:
		return fmt.Errorf("不支持的输出方式: %s", config.Output)
	}

	// 设置调用信息（在debug级别时显示）
	logger.SetReportCaller(level == logrus.DebugLevel)

	globalLogger = logger
	return nil
}

// setupFileOutput 设置文件输出
func setupFileOutput(config Config) (io.Writer, error) {
	// 确保日志目录存在
	logDir := filepath.Dir(config.File)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 使用lumberjack进行日志轮转
	return &lumberjack.Logger{
		Filename:   config.File,
		MaxSize:    config.MaxSize,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAge,
		Compress:   config.Compress,
		LocalTime:  true,
	}, nil
}

// GetLogger 获取全局logger实例
func GetLogger() *logrus.Logger {
	if globalLogger == nil {
		// 如果未初始化，使用默认配置
		config := DefaultConfig()
		config.Output = "stdout" // 默认输出到stdout
		if err := Initialize(config); err != nil {
			// 初始化失败，使用基础配置
			globalLogger = logrus.New()
		}
	}
	return globalLogger
}

// GetComponentLogger 获取组件专用logger
func GetComponentLogger(component string) *logrus.Entry {
	if componentLoggers[component] == nil {
		logger := GetLogger()
		componentLoggers[component] = logger.WithField("component", component)
	}
	return componentLoggers[component]
}

// 便捷方法 - 获取各个组件的logger
func GetAPILogger() *logrus.Entry      { return GetComponentLogger("api") }
func GetProxyLogger() *logrus.Entry    { return GetComponentLogger("proxy") }
func GetIPTablesLogger() *logrus.Entry { return GetComponentLogger("iptables") }
func GetConfigLogger() *logrus.Entry   { return GetComponentLogger("config") }
func GetSystemLogger() *logrus.Entry   { return GetComponentLogger("system") }

// 辅助方法 - 用于创建带有额外上下文的logger
func WithField(key string, value interface{}) *logrus.Entry {
	return GetLogger().WithField(key, value)
}

func WithFields(fields logrus.Fields) *logrus.Entry {
	return GetLogger().WithFields(fields)
}

func WithError(err error) *logrus.Entry {
	return GetLogger().WithError(err)
}

// 性能指标日志
func LogPerformance(operation string, duration time.Duration, fields logrus.Fields) {
	if fields == nil {
		fields = logrus.Fields{}
	}
	fields["operation"] = operation
	fields["duration_ms"] = duration.Milliseconds()
	fields["performance"] = true

	GetLogger().WithFields(fields).Info("性能指标")
}

// 审计日志
func LogAudit(action string, user string, fields logrus.Fields) {
	if fields == nil {
		fields = logrus.Fields{}
	}
	fields["action"] = action
	fields["user"] = user
	fields["audit"] = true

	GetLogger().WithFields(fields).Info("审计日志")
}

// 错误日志增强
func LogError(err error, message string, fields logrus.Fields) {
	if fields == nil {
		fields = logrus.Fields{}
	}
	fields["error_type"] = fmt.Sprintf("%T", err)

	GetLogger().WithError(err).WithFields(fields).Error(message)
}

// 启动/关闭日志
func LogStartup(component string, version string, config interface{}) {
	GetLogger().WithFields(logrus.Fields{
		"component": component,
		"version":   version,
		"config":    config,
		"lifecycle": "startup",
	}).Info("组件启动")
}

func LogShutdown(component string, duration time.Duration) {
	GetLogger().WithFields(logrus.Fields{
		"component":   component,
		"duration_ms": duration.Milliseconds(),
		"lifecycle":   "shutdown",
	}).Info("组件关闭")
}

// 状态变更日志
func LogStateChange(component string, from string, to string, reason string) {
	GetLogger().WithFields(logrus.Fields{
		"component":    component,
		"state_from":   from,
		"state_to":     to,
		"reason":       reason,
		"state_change": true,
	}).Info("状态变更")
}
