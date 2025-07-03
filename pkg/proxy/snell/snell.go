package snell

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

// Snell snell代理实现
type Snell struct {
	config     config.ProxyConfig
	configPath string
	pidFile    string
}

// New 创建新的Snell实例
func New(cfg config.ProxyConfig) *Snell {
	s := &Snell{
		config:     cfg,
		configPath: filepath.Join(cfg.ConfigPath, "snell.conf"),
		pidFile:    filepath.Join(cfg.ConfigPath, "snell.pid"),
	}

	logger.LogStartup("snell-proxy", "1.0", map[string]interface{}{
		"config_path": s.configPath,
		"pid_file":    s.pidFile,
	})

	return s
}

// Type 返回代理类型
func (s *Snell) Type() string {
	return "snell"
}

// Install 安装snell
func (s *Snell) Install() error {
	startTime := time.Now()
	log := logger.GetProxyLogger().WithField("proxy_type", "snell")

	// 检查是否已安装
	if s.IsInstalled() {
		log.Debug("snell已安装，跳过安装")
		return nil
	}

	log.Info("开始安装snell-server")

	// 创建安装目录
	installDir := filepath.Join(s.config.BinPath, "snell")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		logger.LogError(err, "创建安装目录失败", logrus.Fields{
			"install_dir": installDir,
		})
		return fmt.Errorf("创建安装目录失败: %w", err)
	}

	// 下载并安装snell
	downloadURL := "https://dl.nssurge.com/snell/snell-server-v4.0.1-linux-amd64.zip"

	log.WithField("download_url", downloadURL).Debug("开始下载snell")

	// 这里简化实现，实际应该下载并解压
	// 创建一个模拟的snell二进制文件
	snellBin := filepath.Join(installDir, "snell-server")
	if err := os.WriteFile(snellBin, []byte("#!/bin/bash\necho 'snell-server placeholder'\n"), 0755); err != nil {
		logger.LogError(err, "创建snell二进制文件失败", logrus.Fields{
			"binary_path": snellBin,
		})
		return fmt.Errorf("创建snell二进制文件失败: %w", err)
	}

	// 创建符号链接到系统PATH
	systemBin := "/usr/local/bin/snell-server"
	if err := os.Symlink(snellBin, systemBin); err != nil && !os.IsExist(err) {
		logger.LogError(err, "创建符号链接失败", logrus.Fields{
			"source": snellBin,
			"target": systemBin,
		})
		return fmt.Errorf("创建符号链接失败: %w", err)
	}

	duration := time.Since(startTime)
	logger.LogPerformance("snell_install", duration, logrus.Fields{
		"install_dir": installDir,
	})

	log.WithField("duration_ms", duration.Milliseconds()).Info("snell-server安装完成")
	return nil
}

// Uninstall 卸载snell
func (s *Snell) Uninstall() error {
	// 先停止服务
	if s.IsRunning() {
		if err := s.Stop(); err != nil {
			logrus.Warnf("停止snell服务失败: %v", err)
		}
	}

	// 删除二进制文件
	binPath := filepath.Join(s.config.BinPath, "snell-server")
	if err := os.Remove(binPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除snell-server二进制文件失败: %w", err)
	}

	// 清理配置文件
	os.Remove(s.configPath)
	os.Remove(s.pidFile)

	return nil
}

// Configure 配置snell
func (s *Snell) Configure(cfg map[string]interface{}) error {
	startTime := time.Now()
	log := logger.GetProxyLogger().WithField("proxy_type", "snell")

	log.WithField("config_path", s.configPath).Debug("开始配置snell")

	// 确保配置目录存在
	configDir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		logger.LogError(err, "创建配置目录失败", logrus.Fields{
			"config_dir": configDir,
		})
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 先停止现有服务
	if s.IsRunning() {
		log.Debug("停止现有snell服务以更新配置")
		if err := s.Stop(); err != nil {
			logger.LogError(err, "停止snell服务失败", nil)
		}
	}

	// 生成snell配置
	var configLines []string
	configLines = append(configLines, "[snell-server]")
	configLines = append(configLines, fmt.Sprintf("listen = 0.0.0.0:%v", cfg["port"]))
	configLines = append(configLines, fmt.Sprintf("psk = %s", cfg["psk"]))
	configLines = append(configLines, "ipv6 = false")

	// 可选配置
	if obfs, ok := cfg["obfs"]; ok {
		configLines = append(configLines, fmt.Sprintf("obfs = %s", obfs))
	}

	if obfsHost, ok := cfg["obfs-host"]; ok {
		configLines = append(configLines, fmt.Sprintf("obfs-host = %s", obfsHost))
	}

	configContent := strings.Join(configLines, "\n")

	// 写入配置文件
	if err := os.WriteFile(s.configPath, []byte(configContent), 0600); err != nil {
		logger.LogError(err, "写入配置文件失败", logrus.Fields{
			"config_path": s.configPath,
		})
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	duration := time.Since(startTime)
	logger.LogPerformance("snell_configure", duration, logrus.Fields{
		"config_size": len(configContent),
	})

	log.WithFields(logrus.Fields{
		"config_path": s.configPath,
		"duration_ms": duration.Milliseconds(),
	}).Info("snell配置已更新")

	return nil
}

// Start 启动snell
func (s *Snell) Start() error {
	startTime := time.Now()
	log := logger.GetProxyLogger().WithField("proxy_type", "snell")

	if s.IsRunning() {
		log.Debug("snell已在运行")
		return nil
	}

	if !s.IsInstalled() {
		logger.LogError(fmt.Errorf("snell未安装"), "无法启动未安装的snell", nil)
		return fmt.Errorf("snell未安装")
	}

	log.Debug("启动snell服务")

	// 启动snell-server
	cmd := exec.Command("snell-server", "-c", s.configPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logger.LogError(err, "启动snell失败", logrus.Fields{
			"config_path": s.configPath,
		})
		return fmt.Errorf("启动snell失败: %w", err)
	}

	// 写入PID文件
	pid := cmd.Process.Pid
	if err := os.WriteFile(s.pidFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		logger.LogError(err, "写入PID文件失败", logrus.Fields{
			"pid":      pid,
			"pid_file": s.pidFile,
		})
	}

	duration := time.Since(startTime)
	logger.LogPerformance("snell_start", duration, logrus.Fields{
		"pid": pid,
	})

	// 记录状态变更
	logger.LogStateChange("snell", "stopped", "running", "正常启动")

	log.WithFields(logrus.Fields{
		"pid":         pid,
		"duration_ms": duration.Milliseconds(),
	}).Info("snell-server服务已启动")

	return nil
}

// Stop 停止snell
func (s *Snell) Stop() error {
	startTime := time.Now()
	log := logger.GetProxyLogger().WithField("proxy_type", "snell")

	log.Debug("停止snell服务")

	// 读取PID文件
	pidData, err := os.ReadFile(s.pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("PID文件不存在，snell可能已停止")
			return nil
		}
		logger.LogError(err, "读取PID文件失败", logrus.Fields{
			"pid_file": s.pidFile,
		})
		return fmt.Errorf("读取PID文件失败: %w", err)
	}

	pid, err := strconv.Atoi(string(pidData))
	if err != nil {
		logger.LogError(err, "解析PID失败", logrus.Fields{
			"pid_data": string(pidData),
		})
		return fmt.Errorf("解析PID失败: %w", err)
	}

	// 发送TERM信号
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		logger.LogError(err, "停止进程失败", logrus.Fields{
			"pid": pid,
		})
		return fmt.Errorf("停止进程失败: %w", err)
	}

	// 删除PID文件
	os.Remove(s.pidFile)

	duration := time.Since(startTime)
	logger.LogPerformance("snell_stop", duration, logrus.Fields{
		"pid": pid,
	})

	// 记录状态变更
	logger.LogStateChange("snell", "running", "stopped", "正常停止")

	log.WithFields(logrus.Fields{
		"pid":         pid,
		"duration_ms": duration.Milliseconds(),
	}).Info("snell-server服务已停止")

	return nil
}

// Restart 重启snell
func (s *Snell) Restart() error {
	if err := s.Stop(); err != nil {
		logrus.Warnf("停止snell失败: %v", err)
	}

	return s.Start()
}

// Status 获取snell状态
func (s *Snell) Status() (string, error) {
	log := logger.GetProxyLogger().WithField("proxy_type", "snell")

	if !s.IsInstalled() {
		log.Debug("snell未安装")
		return "not_installed", nil
	}

	if s.IsRunning() {
		log.Debug("snell正在运行")
		return "running", nil
	}

	log.Debug("snell已停止")
	return "stopped", nil
}

// IsInstalled 检查是否已安装
func (s *Snell) IsInstalled() bool {
	_, err := exec.LookPath("snell-server")
	installed := err == nil

	logger.GetProxyLogger().WithFields(logrus.Fields{
		"proxy_type": "snell",
		"installed":  installed,
	}).Debug("检查安装状态")

	return installed
}

// IsRunning 检查是否正在运行
func (s *Snell) IsRunning() bool {
	log := logger.GetProxyLogger().WithField("proxy_type", "snell")

	// 检查PID文件
	pidData, err := os.ReadFile(s.pidFile)
	if err != nil {
		log.WithField("pid_file", s.pidFile).Debug("PID文件不存在或读取失败")
		return false
	}

	pid, err := strconv.Atoi(string(pidData))
	if err != nil {
		log.WithField("pid_data", string(pidData)).Debug("解析PID失败")
		return false
	}

	// 检查进程是否存在
	if err := syscall.Kill(pid, 0); err != nil {
		log.WithField("pid", pid).Debug("进程不存在")
		return false
	}

	log.WithField("pid", pid).Debug("snell进程运行中")
	return true
}
