// Package process 提供通用的进程管理功能
//
// 这个包封装了进程生命周期管理的常用操作，包括：
// - 进程启动、停止、重启
// - PID文件管理
// - 进程状态检查
// - 二进制文件安装检查
// - 性能日志记录
//
// 主要用于管理代理服务进程，但设计为通用的进程管理器，
// 可以用于管理任何需要PID文件管理的长期运行进程。
package process

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nspass/nspass-agent/pkg/interfaces"
	"github.com/nspass/nspass-agent/pkg/logging"
	"github.com/nspass/nspass-agent/pkg/monitoring"
)

// Manager 进程管理器，提供通用的进程管理功能
//
// Manager 封装了进程管理的常用操作，包括进程的启动、停止、重启，
// PID文件的管理，以及进程状态的检查。它被设计为可重用的组件，
// 可以用于管理任何需要PID文件管理的长期运行进程。
//
// 主要特性：
// - 自动PID文件管理
// - 进程状态检查
// - 二进制文件安装检查
// - 结构化日志记录
// - 性能指标记录
// - 优雅的进程停止（SIGTERM -> SIGKILL）
type Manager struct {
	proxyType  string                   // 进程类型标识，用于日志和错误消息
	binaryName string                   // 二进制文件名
	binPath    string                   // 二进制文件所在目录路径
	pidFile    string                   // PID文件的完整路径
	logger     interfaces.Logger        // 结构化日志记录器
	metrics    *monitoring.ProxyMetrics // 监控指标（可选）
}

// NewManager 创建进程管理器
//
// 参数：
//   - proxyType: 进程类型标识，用于日志记录和错误消息，如 "shadowsocks", "trojan"
//   - binaryName: 二进制文件名，如 "go-shadowsocks2", "trojan-go"
//   - binPath: 二进制文件所在目录的完整路径，如 "/usr/local/bin/proxy"
//   - pidFile: PID文件的完整路径，如 "/etc/nspass-agent/shadowsocks.pid"
//
// 返回：
//   - *Manager: 配置好的进程管理器实例
//
// 示例：
//
//	manager := NewManager("shadowsocks", "go-shadowsocks2", "/usr/local/bin/proxy", "/etc/nspass-agent/shadowsocks.pid")
func NewManager(proxyType, binaryName, binPath, pidFile string) *Manager {
	manager := &Manager{
		proxyType:  proxyType,
		binaryName: binaryName,
		binPath:    binPath,
		pidFile:    pidFile,
		logger:     logging.GetComponentLogger(proxyType + "-process"),
	}

	// 尝试初始化监控指标（如果监控系统可用）
	if monitor := monitoring.GetGlobalMonitor(); monitor != nil {
		manager.metrics = monitoring.NewProxyMetrics(monitor.GetCollector(), proxyType)
	}

	return manager
}

// IsInstalled 检查二进制文件是否已安装
func (pm *Manager) IsInstalled() bool {
	binaryPath := filepath.Join(pm.binPath, pm.binaryName)
	_, err := os.Stat(binaryPath)
	installed := err == nil

	pm.logger.WithFields(map[string]any{
		"binary_path": binaryPath,
		"installed":   installed,
	}).Debug("检查安装状态")

	return installed
}

// IsRunning 检查进程是否在运行
func (pm *Manager) IsRunning() bool {
	pid := pm.GetPID()
	if pid == 0 {
		return false
	}

	// 检查进程是否存在
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// 发送信号0检查进程是否存活
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// GetPID 从PID文件获取进程ID
func (pm *Manager) GetPID() int {
	if _, err := os.Stat(pm.pidFile); os.IsNotExist(err) {
		return 0
	}

	pidData, err := os.ReadFile(pm.pidFile)
	if err != nil {
		return 0
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return 0
	}

	return pid
}

// WritePID 写入PID到文件
func (pm *Manager) WritePID(pid int) error {
	pidData := fmt.Sprintf("%d", pid)
	return os.WriteFile(pm.pidFile, []byte(pidData), 0644)
}

// RemovePIDFile 删除PID文件
func (pm *Manager) RemovePIDFile() error {
	if _, err := os.Stat(pm.pidFile); os.IsNotExist(err) {
		return nil // 文件不存在，无需删除
	}
	return os.Remove(pm.pidFile)
}

// StartProcess 启动进程
func (pm *Manager) StartProcess(args []string) (*exec.Cmd, error) {
	if pm.IsRunning() {
		pm.logger.Debug("进程已在运行")
		return nil, nil
	}

	if !pm.IsInstalled() {
		return nil, fmt.Errorf("%s未安装", pm.proxyType)
	}

	pm.logger.Debug("启动进程")

	// 构建完整的二进制路径
	binaryPath := filepath.Join(pm.binPath, pm.binaryName)
	cmd := exec.Command(binaryPath, args...)

	// 设置进程组，便于管理
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		pm.logger.WithError(err).Error("启动进程失败")
		return nil, fmt.Errorf("启动%s失败: %w", pm.proxyType, err)
	}

	// 写入PID文件
	if err := pm.WritePID(cmd.Process.Pid); err != nil {
		pm.logger.WithError(err).Warn("写入PID文件失败")
		// 不要因为PID文件写入失败而返回错误，服务已经启动了
	}

	pm.logger.WithField("pid", cmd.Process.Pid).Info("进程已启动")

	// 记录监控指标
	if pm.metrics != nil {
		pm.metrics.RecordStart()
	}

	return cmd, nil
}

// StopProcess 停止进程
func (pm *Manager) StopProcess() error {
	pm.logger.Debug("停止进程")

	pid := pm.GetPID()
	if pid == 0 {
		pm.logger.Debug("PID文件不存在，进程可能已停止")
		return nil
	}

	// 发送TERM信号
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		pm.logger.WithError(err).WithField("pid", pid).Error("停止进程失败")
		return fmt.Errorf("停止进程失败: %w", err)
	}

	// 删除PID文件
	pm.RemovePIDFile()

	pm.logger.WithField("pid", pid).Info("进程已停止")

	// 记录监控指标
	if pm.metrics != nil {
		pm.metrics.RecordStop()
	}

	return nil
}

// RestartProcess 重启进程
func (pm *Manager) RestartProcess(args []string) (*exec.Cmd, error) {
	if err := pm.StopProcess(); err != nil {
		pm.logger.WithError(err).Warn("停止进程失败")
	}

	// 等待一小段时间确保完全停止
	time.Sleep(1 * time.Second)

	return pm.StartProcess(args)
}

// GetStatus 获取进程状态
func (pm *Manager) GetStatus() (string, error) {
	if !pm.IsInstalled() {
		return "not_installed", nil
	}
	if pm.IsRunning() {
		return "running", nil
	}
	return "stopped", nil
}

// LogPerformance 记录性能指标
//
// 记录进程操作的性能指标，包括操作类型、持续时间和自定义字段。
// 这些指标可用于监控和性能分析。
//
// 参数：
//   - operation: 操作类型，如 "start", "stop", "restart"
//   - duration: 操作持续时间
//   - fields: 额外的上下文字段，可以为nil
func (pm *Manager) LogPerformance(operation string, duration time.Duration, fields map[string]any) {
	if fields == nil {
		fields = make(map[string]any)
	}
	fields["operation"] = operation
	fields["duration_ms"] = duration.Milliseconds()

	logging.LogPerformance(pm.proxyType+"_"+operation, duration, fields)
}

// LogStateChange 记录状态变更
//
// 记录进程状态的变更，用于审计和故障排查。
//
// 参数：
//   - from: 原始状态，如 "stopped", "running"
//   - to: 目标状态，如 "running", "stopped"
//   - reason: 状态变更的原因和上下文信息
func (pm *Manager) LogStateChange(from, to string, reason map[string]any) {
	logging.LogStateChange(pm.proxyType, from, to, reason)
}

// EnsureConfigDirectory 确保配置目录存在
func (pm *Manager) EnsureConfigDirectory(configPath string) error {
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		pm.logger.WithError(err).WithField("config_dir", configDir).Error("创建配置目录失败")
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	return nil
}

// GetPIDFile 获取PID文件路径
func (pm *Manager) GetPIDFile() string {
	return pm.pidFile
}
