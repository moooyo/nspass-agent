package snell

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/logging"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultConfigPath 默认代理配置文件路径
	DefaultConfigPath = "/etc/nspass-agent"
	// DefaultBinPath 默认代理软件安装路径
	DefaultBinPath = "/usr/local/bin/proxy"
	// SnellServerBinPath Snell服务器二进制文件路径
	SnellServerBinPath = DefaultBinPath + "/snell-server"
)

// Snell snell代理实现
type Snell struct {
	egressItem *model.EgressItem // 出口配置
	configPath string
	pidFile    string
}

// New 创建新的Snell实例
func New(egressItem *model.EgressItem) *Snell {
	s := &Snell{
		egressItem: egressItem,
		configPath: filepath.Join(DefaultConfigPath, fmt.Sprintf("snell-%d.conf", egressItem.Id)),
		pidFile:    filepath.Join(DefaultConfigPath, fmt.Sprintf("snell-%d.pid", egressItem.Id)),
	}

	logging.LogStartup("snell-proxy", "1.0", map[string]interface{}{
		"config_path": s.configPath,
		"pid_file":    s.pidFile,
	})

	return s
}

// Type 返回代理类型
func (s *Snell) Type() string {
	return "snell"
}

// Configure 配置snell
func (s *Snell) Configure(cfg *model.EgressItem) error {
	startTime := time.Now()
	log := logging.GetProxyLogger().WithField("proxy_type", "snell")

	log.WithField("config_path", s.configPath).Debug("开始配置snell")

	// 确保配置目录存在
	configDir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		logging.LogError(err, "创建配置目录失败", logrus.Fields{
			"config_dir": configDir,
		})
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 先停止现有服务
	if s.IsRunning() {
		log.Debug("停止现有snell服务以更新配置")
		if err := s.Stop(); err != nil {
			logging.LogError(err, "停止snell服务失败", nil)
		}
	}

	// 从EgressItem中解析配置
	// 通用字段：Port和Password从EgressItem直接获取
	// 特定配置：从EgressConfig JSON解析
	egressConfig := make(map[string]interface{})
	if cfg.EgressConfig != "" {
		if err := json.Unmarshal([]byte(cfg.EgressConfig), &egressConfig); err != nil {
			log.WithError(err).Error("解析出口配置失败")
			return fmt.Errorf("解析出口配置失败: %w", err)
		}
	}

	// 验证通用字段
	if cfg.Port == nil {
		return fmt.Errorf("端口号不能为空")
	}
	if cfg.Password == nil {
		return fmt.Errorf("密码不能为空")
	}

	// 生成snell配置
	var configLines []string
	configLines = append(configLines, "[snell-server]")
	configLines = append(configLines, fmt.Sprintf("listen = 0.0.0.0:%d", *cfg.Port)) // 从通用字段获取
	configLines = append(configLines, fmt.Sprintf("psk = %s", *cfg.Password))        // 从通用字段获取
	configLines = append(configLines, "ipv6 = false")

	// 从特定配置获取UDP支持设置
	if supportUdp, ok := egressConfig["support_udp"]; ok {
		if udp, ok := supportUdp.(bool); ok && udp {
			configLines = append(configLines, "udp = true")
		}
	}

	// 从特定配置获取版本设置
	if version, ok := egressConfig["version"]; ok {
		configLines = append(configLines, fmt.Sprintf("version = %v", version))
	}

	configContent := strings.Join(configLines, "\n")

	// 写入配置文件
	if err := os.WriteFile(s.configPath, []byte(configContent), 0600); err != nil {
		logging.LogError(err, "写入配置文件失败", logrus.Fields{
			"config_path": s.configPath,
		})
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	duration := time.Since(startTime)
	logging.LogPerformance("snell_configure", duration, logrus.Fields{
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
	log := logging.GetProxyLogger().WithField("proxy_type", "snell")

	if s.IsRunning() {
		log.Debug("snell已在运行")
		return nil
	}

	if !s.IsInstalled() {
		logging.LogError(fmt.Errorf("snell未安装"), "无法启动未安装的snell", nil)
		return fmt.Errorf("snell未安装")
	}

	log.Debug("启动snell服务")

	// 启动snell-server
	snellBinaryPath := filepath.Join(DefaultBinPath, "snell-server")
	cmd := exec.Command(snellBinaryPath, "-c", s.configPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logging.LogError(err, "启动snell失败", logrus.Fields{
			"config_path": s.configPath,
		})
		return fmt.Errorf("启动snell失败: %w", err)
	}

	// 写入PID文件
	pid := cmd.Process.Pid
	if err := os.WriteFile(s.pidFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		logging.LogError(err, "写入PID文件失败", logrus.Fields{
			"pid":      pid,
			"pid_file": s.pidFile,
		})
	}

	duration := time.Since(startTime)
	logging.LogPerformance("snell_start", duration, logrus.Fields{
		"pid": pid,
	})

	// 记录状态变更
	logging.LogStateChange("snell", "stopped", "running", map[string]interface{}{"reason": "正常启动"})

	log.WithFields(logrus.Fields{
		"pid":         pid,
		"duration_ms": duration.Milliseconds(),
	}).Info("snell-server服务已启动")

	return nil
}

// Stop 停止snell
func (s *Snell) Stop() error {
	startTime := time.Now()
	log := logging.GetProxyLogger().WithField("proxy_type", "snell")

	log.Debug("停止snell服务")

	// 读取PID文件
	pidData, err := os.ReadFile(s.pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("PID文件不存在，snell可能已停止")
			return nil
		}
		logging.LogError(err, "读取PID文件失败", logrus.Fields{
			"pid_file": s.pidFile,
		})
		return fmt.Errorf("读取PID文件失败: %w", err)
	}

	pid, err := strconv.Atoi(string(pidData))
	if err != nil {
		logging.LogError(err, "解析PID失败", logrus.Fields{
			"pid_data": string(pidData),
		})
		return fmt.Errorf("解析PID失败: %w", err)
	}

	// 发送TERM信号
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		logging.LogError(err, "停止进程失败", logrus.Fields{
			"pid": pid,
		})
		return fmt.Errorf("停止进程失败: %w", err)
	}

	// 删除PID文件
	os.Remove(s.pidFile)

	duration := time.Since(startTime)
	logging.LogPerformance("snell_stop", duration, logrus.Fields{
		"pid": pid,
	})

	// 记录状态变更
	logging.LogStateChange("snell", "running", "stopped", map[string]interface{}{"reason": "正常停止"})

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
	log := logging.GetProxyLogger().WithField("proxy_type", "snell")

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
	binaryPath := filepath.Join(DefaultBinPath, "snell-server")
	_, err := os.Stat(binaryPath)
	installed := err == nil

	logging.GetProxyLogger().WithFields(logrus.Fields{
		"proxy_type":  "snell",
		"binary_path": binaryPath,
		"installed":   installed,
	}).Debug("检查安装状态")

	return installed
}

// IsRunning 检查是否正在运行
func (s *Snell) IsRunning() bool {
	log := logging.GetProxyLogger().WithField("proxy_type", "snell")

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

// Cleanup 清理配置文件和PID文件
func (s *Snell) Cleanup() error {
	log := logging.GetProxyLogger().WithField("proxy_type", "snell")
	log.Debug("开始清理snell配置文件和PID文件")

	var errors []error

	// 确保进程已停止
	if s.IsRunning() {
		if err := s.Stop(); err != nil {
			log.WithError(err).Warn("停止进程失败，继续清理文件")
			errors = append(errors, fmt.Errorf("停止进程失败: %w", err))
		}
	}

	// 删除配置文件
	if _, err := os.Stat(s.configPath); err == nil {
		if err := os.Remove(s.configPath); err != nil {
			log.WithError(err).WithField("config_path", s.configPath).Error("删除配置文件失败")
			errors = append(errors, fmt.Errorf("删除配置文件失败: %w", err))
		} else {
			log.WithField("config_path", s.configPath).Info("配置文件已删除")
		}
	}

	// 删除PID文件
	if _, err := os.Stat(s.pidFile); err == nil {
		if err := os.Remove(s.pidFile); err != nil {
			log.WithError(err).WithField("pid_file", s.pidFile).Error("删除PID文件失败")
			errors = append(errors, fmt.Errorf("删除PID文件失败: %w", err))
		} else {
			log.WithField("pid_file", s.pidFile).Info("PID文件已删除")
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("清理过程中发生错误: %v", errors)
	}

	log.Info("snell清理完成")
	return nil
}

// GetConfigPath 获取配置文件路径
func (s *Snell) GetConfigPath() string {
	return s.configPath
}

// GetPIDPath 获取PID文件路径
func (s *Snell) GetPIDPath() string {
	return s.pidFile
}
