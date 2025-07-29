package errors

import (
	"fmt"
	"runtime"
	"strings"
)

// ErrorType 错误类型
type ErrorType string

const (
	// 配置相关错误
	ErrorTypeConfig ErrorType = "CONFIG"
	
	// 网络相关错误
	ErrorTypeNetwork ErrorType = "NETWORK"
	
	// 代理相关错误
	ErrorTypeProxy ErrorType = "PROXY"
	
	// IPTables相关错误
	ErrorTypeIPTables ErrorType = "IPTABLES"
	
	// WebSocket相关错误
	ErrorTypeWebSocket ErrorType = "WEBSOCKET"
	
	// 任务相关错误
	ErrorTypeTask ErrorType = "TASK"
	
	// 系统相关错误
	ErrorTypeSystem ErrorType = "SYSTEM"
	
	// 未知错误
	ErrorTypeUnknown ErrorType = "UNKNOWN"
)

// NSPassError NSPass自定义错误类型
type NSPassError struct {
	Type      ErrorType
	Code      string
	Message   string
	Cause     error
	Context   map[string]interface{}
	StackTrace string
}

// Error 实现error接口
func (e *NSPassError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s:%s] %s: %v", e.Type, e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s:%s] %s", e.Type, e.Code, e.Message)
}

// Unwrap 支持errors.Unwrap
func (e *NSPassError) Unwrap() error {
	return e.Cause
}

// WithContext 添加上下文信息
func (e *NSPassError) WithContext(key string, value interface{}) *NSPassError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// WithContextMap 添加多个上下文信息
func (e *NSPassError) WithContextMap(context map[string]interface{}) *NSPassError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	for k, v := range context {
		e.Context[k] = v
	}
	return e
}

// New 创建新的NSPassError
func New(errorType ErrorType, code, message string) *NSPassError {
	return &NSPassError{
		Type:       errorType,
		Code:       code,
		Message:    message,
		StackTrace: getStackTrace(),
	}
}

// Wrap 包装现有错误
func Wrap(err error, errorType ErrorType, code, message string) *NSPassError {
	return &NSPassError{
		Type:       errorType,
		Code:       code,
		Message:    message,
		Cause:      err,
		StackTrace: getStackTrace(),
	}
}

// getStackTrace 获取调用栈信息
func getStackTrace() string {
	const depth = 32
	var pcs [depth]uintptr
	n := runtime.Callers(3, pcs[:])
	
	var sb strings.Builder
	frames := runtime.CallersFrames(pcs[:n])
	
	for {
		frame, more := frames.Next()
		if !strings.Contains(frame.File, "nspass-agent") {
			if !more {
				break
			}
			continue
		}
		
		sb.WriteString(fmt.Sprintf("%s:%d %s\n", frame.File, frame.Line, frame.Function))
		
		if !more {
			break
		}
	}
	
	return sb.String()
}

// 预定义的常见错误

// 配置错误
var (
	ErrConfigNotFound     = New(ErrorTypeConfig, "CONFIG_NOT_FOUND", "配置文件未找到")
	ErrConfigInvalid      = New(ErrorTypeConfig, "CONFIG_INVALID", "配置文件格式无效")
	ErrConfigValidation   = New(ErrorTypeConfig, "CONFIG_VALIDATION", "配置验证失败")
)

// 网络错误
var (
	ErrNetworkTimeout     = New(ErrorTypeNetwork, "NETWORK_TIMEOUT", "网络请求超时")
	ErrNetworkConnection  = New(ErrorTypeNetwork, "NETWORK_CONNECTION", "网络连接失败")
	ErrNetworkUnreachable = New(ErrorTypeNetwork, "NETWORK_UNREACHABLE", "网络不可达")
)

// 代理错误
var (
	ErrProxyNotFound      = New(ErrorTypeProxy, "PROXY_NOT_FOUND", "代理未找到")
	ErrProxyStartFailed   = New(ErrorTypeProxy, "PROXY_START_FAILED", "代理启动失败")
	ErrProxyStopFailed    = New(ErrorTypeProxy, "PROXY_STOP_FAILED", "代理停止失败")
	ErrProxyConfigInvalid = New(ErrorTypeProxy, "PROXY_CONFIG_INVALID", "代理配置无效")
)

// IPTables错误
var (
	ErrIPTablesRuleInvalid = New(ErrorTypeIPTables, "IPTABLES_RULE_INVALID", "IPTables规则无效")
	ErrIPTablesApplyFailed = New(ErrorTypeIPTables, "IPTABLES_APPLY_FAILED", "IPTables规则应用失败")
	ErrIPTablesBackupFailed = New(ErrorTypeIPTables, "IPTABLES_BACKUP_FAILED", "IPTables备份失败")
)

// WebSocket错误
var (
	ErrWebSocketConnection = New(ErrorTypeWebSocket, "WEBSOCKET_CONNECTION", "WebSocket连接失败")
	ErrWebSocketSendFailed = New(ErrorTypeWebSocket, "WEBSOCKET_SEND_FAILED", "WebSocket消息发送失败")
	ErrWebSocketAuthFailed = New(ErrorTypeWebSocket, "WEBSOCKET_AUTH_FAILED", "WebSocket认证失败")
)

// 任务错误
var (
	ErrTaskNotFound       = New(ErrorTypeTask, "TASK_NOT_FOUND", "任务未找到")
	ErrTaskExecutionFailed = New(ErrorTypeTask, "TASK_EXECUTION_FAILED", "任务执行失败")
	ErrTaskTimeout        = New(ErrorTypeTask, "TASK_TIMEOUT", "任务执行超时")
)

// 系统错误
var (
	ErrSystemResourceExhausted = New(ErrorTypeSystem, "SYSTEM_RESOURCE_EXHAUSTED", "系统资源耗尽")
	ErrSystemPermissionDenied  = New(ErrorTypeSystem, "SYSTEM_PERMISSION_DENIED", "系统权限不足")
	ErrSystemServiceUnavailable = New(ErrorTypeSystem, "SYSTEM_SERVICE_UNAVAILABLE", "系统服务不可用")
)

// IsType 检查错误是否为指定类型
func IsType(err error, errorType ErrorType) bool {
	if nsErr, ok := err.(*NSPassError); ok {
		return nsErr.Type == errorType
	}
	return false
}

// GetType 获取错误类型
func GetType(err error) ErrorType {
	if nsErr, ok := err.(*NSPassError); ok {
		return nsErr.Type
	}
	return ErrorTypeUnknown
}

// GetCode 获取错误代码
func GetCode(err error) string {
	if nsErr, ok := err.(*NSPassError); ok {
		return nsErr.Code
	}
	return "UNKNOWN"
}

// GetContext 获取错误上下文
func GetContext(err error) map[string]interface{} {
	if nsErr, ok := err.(*NSPassError); ok {
		return nsErr.Context
	}
	return nil
}
