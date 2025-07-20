package utils

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/sirupsen/logrus"
)

// ProcessManager 进程管理工具
type ProcessManager struct {
	logger    *logrus.Entry
	fileUtils *FileUtils
}

// NewProcessManager 创建进程管理工具实例
func NewProcessManager(component string) *ProcessManager {
	return &ProcessManager{
		logger:    logger.GetComponentLogger(component + "-process"),
		fileUtils: NewFileUtils(component),
	}
}

// IsProcessRunning 检查进程是否在运行
func (p *ProcessManager) IsProcessRunning(pidFile string) bool {
	pid := p.GetPIDFromFile(pidFile)
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

// GetPIDFromFile 从PID文件获取进程ID
func (p *ProcessManager) GetPIDFromFile(pidFile string) int {
	if !p.fileUtils.FileExists(pidFile) {
		return 0
	}

	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		p.logger.WithError(err).Warnf("读取PID文件失败: %s", pidFile)
		return 0
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		p.logger.WithError(err).Warnf("解析PID失败: %s", pidFile)
		return 0
	}

	return pid
}

// WritePIDFile 写入PID到文件
func (p *ProcessManager) WritePIDFile(pidFile string, pid int) error {
	return p.fileUtils.WritePidFile(pidFile, pid)
}

// RemovePIDFile 删除PID文件
func (p *ProcessManager) RemovePIDFile(pidFile string) error {
	return p.fileUtils.RemoveFileIfExists(pidFile)
}

// StopProcess 停止进程
func (p *ProcessManager) StopProcess(pidFile string, processName string) error {
	pid := p.GetPIDFromFile(pidFile)
	if pid == 0 {
		return fmt.Errorf("进程未运行")
	}

	log := p.logger.WithFields(logrus.Fields{
		"process_name": processName,
		"pid":         pid,
	})

	process, err := os.FindProcess(pid)
	if err != nil {
		log.WithError(err).Error("找不到进程")
		return fmt.Errorf("找不到进程: %w", err)
	}

	// 先尝试SIGTERM
	if err := process.Signal(syscall.SIGTERM); err != nil {
		log.WithError(err).Warn("发送SIGTERM信号失败，尝试SIGKILL")
		if err := process.Signal(syscall.SIGKILL); err != nil {
			log.WithError(err).Error("发送SIGKILL信号失败")
			return fmt.Errorf("停止进程失败: %w", err)
		}
	}

	// 等待进程退出
	done := make(chan bool, 1)
	go func() {
		process.Wait()
		done <- true
	}()

	select {
	case <-done:
		log.Info("进程已成功停止")
	case <-time.After(10 * time.Second):
		log.Warn("等待进程退出超时，强制终止")
		process.Signal(syscall.SIGKILL)
	}

	// 清理PID文件
	return p.RemovePIDFile(pidFile)
}

// GetProcessStatus 获取进程状态
func (p *ProcessManager) GetProcessStatus(pidFile string) (string, error) {
	if p.IsProcessRunning(pidFile) {
		return "running", nil
	}
	return "stopped", nil
}
