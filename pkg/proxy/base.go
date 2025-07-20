package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/sirupsen/logrus"
)

// BaseProxy 代理基础结构，封装通用功能
type BaseProxy struct {
	proxyType  string
	config     config.ProxyConfig
	configPath string
	pidFile    string
}

// NewBaseProxy 创建基础代理实例
func NewBaseProxy(proxyType string, cfg config.ProxyConfig, configFileName string) *BaseProxy {
	base := &BaseProxy{
		proxyType:  proxyType,
		config:     cfg,
		configPath: filepath.Join(cfg.ConfigPath, configFileName),
		pidFile:    filepath.Join(cfg.ConfigPath, proxyType+".pid"),
	}

	logger.LogStartup(proxyType+"-proxy", "1.0", map[string]interface{}{
		"config_path": base.configPath,
		"pid_file":    base.pidFile,
	})

	return base
}

// Type 返回代理类型
func (b *BaseProxy) Type() string {
	return b.proxyType
}

// GetConfigPath 获取配置文件路径
func (b *BaseProxy) GetConfigPath() string {
	return b.configPath
}

// GetPidFile 获取PID文件路径
func (b *BaseProxy) GetPidFile() string {
	return b.pidFile
}

// EnsureConfigDirectory 确保配置目录存在
func (b *BaseProxy) EnsureConfigDirectory() error {
	configDir := filepath.Dir(b.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		logger.LogError(err, "创建配置目录失败", logrus.Fields{
			"proxy_type":  b.proxyType,
			"config_dir":  configDir,
		})
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	return nil
}

// InstallPackage 使用系统包管理器安装软件包
func (b *BaseProxy) InstallPackage(packageName string) error {
	startTime := time.Now()
	log := logger.GetProxyLogger().WithField("proxy_type", b.proxyType)

	var cmd *exec.Cmd
	var pkgManager string

	if _, err := exec.LookPath("apt-get"); err == nil {
		// Debian/Ubuntu
		pkgManager = "apt-get"
		log.Debug("使用apt-get包管理器")
		// 更新包列表
		cmd = exec.Command("apt-get", "update")
		if err := cmd.Run(); err != nil {
			logger.LogError(err, "更新包列表失败", logrus.Fields{
				"pkg_manager": pkgManager,
			})
			return fmt.Errorf("更新包列表失败: %w", err)
		}
		cmd = exec.Command("apt-get", "install", "-y", packageName)
	} else if _, err := exec.LookPath("yum"); err == nil {
		// CentOS/RHEL
		pkgManager = "yum"
		log.Debug("使用yum包管理器")
		cmd = exec.Command("yum", "install", "-y", packageName)
	} else if _, err := exec.LookPath("pacman"); err == nil {
		// Arch Linux
		pkgManager = "pacman"
		log.Debug("使用pacman包管理器")
		cmd = exec.Command("pacman", "-S", "--noconfirm", packageName)
	} else {
		logger.LogError(fmt.Errorf("未找到支持的包管理器"),
			"不支持的系统，无法自动安装"+b.proxyType, nil)
		return fmt.Errorf("不支持的系统，无法自动安装%s", b.proxyType)
	}

	if err := cmd.Run(); err != nil {
		logger.LogError(err, "安装软件包失败", logrus.Fields{
			"pkg_manager": pkgManager,
			"package":     packageName,
		})
		return fmt.Errorf("安装%s失败: %w", packageName, err)
	}

	duration := time.Since(startTime)
	logger.LogPerformance(b.proxyType+"_install", duration, logrus.Fields{
		"pkg_manager": pkgManager,
		"package":     packageName,
	})

	log.WithFields(logrus.Fields{
		"duration_ms": duration.Milliseconds(),
		"package":     packageName,
	}).Info(b.proxyType + "安装完成")

	return nil
}

// CreateBinaryPlaceholder 创建二进制文件占位符（用于测试）
func (b *BaseProxy) CreateBinaryPlaceholder(installDir, binaryName string) error {
	if err := os.MkdirAll(installDir, 0755); err != nil {
		logger.LogError(err, "创建安装目录失败", logrus.Fields{
			"install_dir": installDir,
		})
		return fmt.Errorf("创建安装目录失败: %w", err)
	}

	binaryPath := filepath.Join(installDir, binaryName)
	content := fmt.Sprintf("#!/bin/bash\necho '%s placeholder'\n", b.proxyType)
	if err := os.WriteFile(binaryPath, []byte(content), 0755); err != nil {
		logger.LogError(err, "创建二进制文件失败", logrus.Fields{
			"binary_path": binaryPath,
		})
		return fmt.Errorf("创建二进制文件失败: %w", err)
	}

	// 创建符号链接到系统PATH
	systemBin := "/usr/local/bin/" + binaryName
	if err := os.Symlink(binaryPath, systemBin); err != nil && !os.IsExist(err) {
		logger.LogError(err, "创建符号链接失败", logrus.Fields{
			"source": binaryPath,
			"target": systemBin,
		})
		return fmt.Errorf("创建符号链接失败: %w", err)
	}

	return nil
}

// IsRunning 检查进程是否在运行
func (b *BaseProxy) IsRunning() bool {
	pid := b.GetPID()
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
func (b *BaseProxy) GetPID() int {
	if _, err := os.Stat(b.pidFile); os.IsNotExist(err) {
		return 0
	}

	pidData, err := os.ReadFile(b.pidFile)
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
func (b *BaseProxy) WritePID(pid int) error {
	return os.WriteFile(b.pidFile, []byte(fmt.Sprintf("%d", pid)), 0644)
}

// RemovePIDFile 删除PID文件
func (b *BaseProxy) RemovePIDFile() error {
	if _, err := os.Stat(b.pidFile); os.IsNotExist(err) {
		return nil // 文件不存在，无需删除
	}
	return os.Remove(b.pidFile)
}

// StopProcess 停止进程
func (b *BaseProxy) StopProcess() error {
	pid := b.GetPID()
	if pid == 0 {
		return fmt.Errorf("进程未运行")
	}

	log := logger.GetProxyLogger().WithFields(logrus.Fields{
		"proxy_type": b.proxyType,
		"pid":        pid,
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
	return b.RemovePIDFile()
}

// GetStatus 获取状态
func (b *BaseProxy) GetStatus() (string, error) {
	if b.IsRunning() {
		return "running", nil
	}
	return "stopped", nil
}
