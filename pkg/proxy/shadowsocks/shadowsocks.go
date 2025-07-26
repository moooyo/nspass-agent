package shadowsocks

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/sirupsen/logrus"
)

// Shadowsocks shadowsocks代理实现
type Shadowsocks struct {
	config     config.ProxyConfig
	configPath string
	pidFile    string
}

// New 创建新的Shadowsocks实例
func New(cfg *model.EgressItem) *Shadowsocks {
	ss := &Shadowsocks{
		config:     cfg,
		configPath: filepath.Join(cfg.ConfigPath, "shadowsocks.json"),
		pidFile:    filepath.Join(cfg.ConfigPath, "shadowsocks.pid"),
	}

	logger.LogStartup("shadowsocks-proxy", "1.0", map[string]interface{}{
		"config_path": ss.configPath,
		"pid_file":    ss.pidFile,
	})

	return ss
}

// Type 返回代理类型
func (s *Shadowsocks) Type() string {
	return "shadowsocks"
}

// Install 安装shadowsocks
func (s *Shadowsocks) Install() error {
	startTime := time.Now()
	log := logger.GetProxyLogger().WithField("proxy_type", "shadowsocks")

	// 检查是否已安装
	if s.IsInstalled() {
		log.Debug("shadowsocks已安装，跳过安装")
		return nil
	}

	log.Info("开始安装shadowsocks-libev")

	// 使用包管理器安装
	var cmd *exec.Cmd
	var pkgManager string

	if _, err := exec.LookPath("apt-get"); err == nil {
		// Debian/Ubuntu
		pkgManager = "apt-get"
		log.Debug("使用apt-get包管理器")
		cmd = exec.Command("apt-get", "update")
		if err := cmd.Run(); err != nil {
			logger.LogError(err, "更新包列表失败", logrus.Fields{
				"pkg_manager": pkgManager,
			})
			return fmt.Errorf("更新包列表失败: %w", err)
		}
		cmd = exec.Command("apt-get", "install", "-y", "shadowsocks-libev")
	} else if _, err := exec.LookPath("yum"); err == nil {
		// CentOS/RHEL
		pkgManager = "yum"
		log.Debug("使用yum包管理器")
		cmd = exec.Command("yum", "install", "-y", "shadowsocks-libev")
	} else if _, err := exec.LookPath("pacman"); err == nil {
		// Arch Linux
		pkgManager = "pacman"
		log.Debug("使用pacman包管理器")
		cmd = exec.Command("pacman", "-S", "--noconfirm", "shadowsocks-libev")
	} else {
		logger.LogError(fmt.Errorf("未找到支持的包管理器"),
			"不支持的系统，无法自动安装shadowsocks", nil)
		return fmt.Errorf("不支持的系统，无法自动安装shadowsocks")
	}

	if err := cmd.Run(); err != nil {
		logger.LogError(err, "安装shadowsocks失败", logrus.Fields{
			"pkg_manager": pkgManager,
		})
		return fmt.Errorf("安装shadowsocks失败: %w", err)
	}

	duration := time.Since(startTime)
	logger.LogPerformance("shadowsocks_install", duration, logrus.Fields{
		"pkg_manager": pkgManager,
	})

	log.WithField("duration_ms", duration.Milliseconds()).Info("shadowsocks-libev安装完成")
	return nil
}

// Uninstall 卸载shadowsocks
func (s *Shadowsocks) Uninstall() error {
	// 先停止服务
	if s.IsRunning() {
		if err := s.Stop(); err != nil {
			logrus.Warnf("停止shadowsocks服务失败: %v", err)
		}
	}

	// 使用包管理器卸载
	var cmd *exec.Cmd
	if _, err := exec.LookPath("apt-get"); err == nil {
		cmd = exec.Command("apt-get", "remove", "-y", "shadowsocks-libev")
	} else if _, err := exec.LookPath("yum"); err == nil {
		cmd = exec.Command("yum", "remove", "-y", "shadowsocks-libev")
	} else if _, err := exec.LookPath("pacman"); err == nil {
		cmd = exec.Command("pacman", "-R", "--noconfirm", "shadowsocks-libev")
	}

	if cmd != nil {
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("卸载shadowsocks失败: %w", err)
		}
	}

	// 清理配置文件
	os.Remove(s.configPath)
	os.Remove(s.pidFile)

	return nil
}

// Configure 配置shadowsocks
func (s *Shadowsocks) Configure(cfg *model.EgressItem) error {
	startTime := time.Now()
	log := logger.GetProxyLogger().WithField("proxy_type", "shadowsocks")

	log.WithField("config_path", s.configPath).Debug("开始配置shadowsocks")

	// 确保配置目录存在
	configDir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		logger.LogError(err, "创建配置目录失败", logrus.Fields{
			"config_dir": configDir,
		})
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 生成shadowsocks配置
	config := map[string]interface{}{
		"server":      cfg["server"],
		"server_port": cfg["port"],
		"password":    cfg["password"],
		"method":      cfg["method"],
		"timeout":     cfg["timeout"],
		"fast_open":   true,
	}

	// 如果有本地配置
	if localPort, ok := cfg["local_port"]; ok {
		config["local_port"] = localPort
	} else {
		config["local_port"] = 1080
	}

	if localAddr, ok := cfg["local_address"]; ok {
		config["local_address"] = localAddr
	} else {
		config["local_address"] = "0.0.0.0"
	}

	// 写入配置文件
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		logger.LogError(err, "序列化配置失败", logrus.Fields{
			"config": cfg,
		})
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(s.configPath, data, 0600); err != nil {
		logger.LogError(err, "写入配置文件失败", logrus.Fields{
			"config_path": s.configPath,
		})
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	duration := time.Since(startTime)
	logger.LogPerformance("shadowsocks_configure", duration, logrus.Fields{
		"config_size": len(data),
	})

	log.WithFields(logrus.Fields{
		"config_path": s.configPath,
		"duration_ms": duration.Milliseconds(),
	}).Info("shadowsocks配置已更新")

	return nil
}

// Start 启动shadowsocks
func (s *Shadowsocks) Start() error {
	startTime := time.Now()
	log := logger.GetProxyLogger().WithField("proxy_type", "shadowsocks")

	if s.IsRunning() {
		log.Debug("shadowsocks已在运行")
		return nil
	}

	if !s.IsInstalled() {
		logger.LogError(fmt.Errorf("shadowsocks未安装"), "无法启动未安装的shadowsocks", nil)
		return fmt.Errorf("shadowsocks未安装")
	}

	log.Debug("启动shadowsocks服务")

	// 启动ss-local
	cmd := exec.Command("ss-local", "-c", s.configPath, "-f", s.pidFile)
	if err := cmd.Start(); err != nil {
		logger.LogError(err, "启动shadowsocks失败", logrus.Fields{
			"config_path": s.configPath,
			"pid_file":    s.pidFile,
		})
		return fmt.Errorf("启动shadowsocks失败: %w", err)
	}

	duration := time.Since(startTime)
	logger.LogPerformance("shadowsocks_start", duration, nil)

	// 记录状态变更
	logger.LogStateChange("shadowsocks", "stopped", "running", "正常启动")

	log.WithField("duration_ms", duration.Milliseconds()).Info("shadowsocks服务已启动")
	return nil
}

// Stop 停止shadowsocks
func (s *Shadowsocks) Stop() error {
	startTime := time.Now()
	log := logger.GetProxyLogger().WithField("proxy_type", "shadowsocks")

	log.Debug("停止shadowsocks服务")

	// 读取PID文件
	pidData, err := os.ReadFile(s.pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("PID文件不存在，shadowsocks可能已停止")
			return nil // PID文件不存在，说明已经停止
		}
		logger.LogError(err, "读取PID文件失败", logrus.Fields{
			"pid_file": s.pidFile,
		})
		return fmt.Errorf("读取PID文件失败: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err != nil {
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
	logger.LogPerformance("shadowsocks_stop", duration, logrus.Fields{
		"pid": pid,
	})

	// 记录状态变更
	logger.LogStateChange("shadowsocks", "running", "stopped", "正常停止")

	log.WithFields(logrus.Fields{
		"pid":         pid,
		"duration_ms": duration.Milliseconds(),
	}).Info("shadowsocks服务已停止")

	return nil
}

// Restart 重启shadowsocks
func (s *Shadowsocks) Restart() error {
	if err := s.Stop(); err != nil {
		logrus.Warnf("停止shadowsocks失败: %v", err)
	}

	return s.Start()
}

// Status 获取shadowsocks状态
func (s *Shadowsocks) Status() (string, error) {
	log := logger.GetProxyLogger().WithField("proxy_type", "shadowsocks")

	if !s.IsInstalled() {
		log.Debug("shadowsocks未安装")
		return "not_installed", nil
	}

	if s.IsRunning() {
		log.Debug("shadowsocks正在运行")
		return "running", nil
	}

	log.Debug("shadowsocks已停止")
	return "stopped", nil
}

// IsInstalled 检查是否已安装
func (s *Shadowsocks) IsInstalled() bool {
	_, err := exec.LookPath("ss-local")
	installed := err == nil

	logger.GetProxyLogger().WithFields(logrus.Fields{
		"proxy_type": "shadowsocks",
		"installed":  installed,
	}).Debug("检查安装状态")

	return installed
}

// IsRunning 检查是否正在运行
func (s *Shadowsocks) IsRunning() bool {
	log := logger.GetProxyLogger().WithField("proxy_type", "shadowsocks")

	// 检查PID文件
	pidData, err := os.ReadFile(s.pidFile)
	if err != nil {
		log.WithField("pid_file", s.pidFile).Debug("PID文件不存在或读取失败")
		return false
	}

	var pid int
	if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err != nil {
		log.WithField("pid_data", string(pidData)).Debug("解析PID失败")
		return false
	}

	// 检查进程是否存在
	if err := syscall.Kill(pid, 0); err != nil {
		log.WithField("pid", pid).Debug("进程不存在")
		return false
	}

	log.WithField("pid", pid).Debug("shadowsocks进程运行中")
	return true
}
