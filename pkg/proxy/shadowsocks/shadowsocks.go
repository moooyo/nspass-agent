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
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultConfigPath 默认代理配置文件路径
	DefaultConfigPath = "/etc/nspass-agent"
	// DefaultBinPath 默认代理软件安装路径
	DefaultBinPath = "/usr/local/bin/proxy"
)

// Shadowsocks shadowsocks代理实现
type Shadowsocks struct {
	egressItem *model.EgressItem // 出口配置
	configPath string
	pidFile    string
}

// New 创建新的Shadowsocks实例
func New(egressItem *model.EgressItem) *Shadowsocks {
	ss := &Shadowsocks{
		egressItem: egressItem,
		configPath: filepath.Join(DefaultConfigPath, fmt.Sprintf("shadowsocks-%s.json", egressItem.EgressId)),
		pidFile:    filepath.Join(DefaultConfigPath, fmt.Sprintf("shadowsocks-%s.pid", egressItem.EgressId)),
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

	// 从EgressItem中解析配置
	egressConfig := make(map[string]interface{})
	if cfg.EgressConfig != "" {
		if err := json.Unmarshal([]byte(cfg.EgressConfig), &egressConfig); err != nil {
			log.WithError(err).Error("解析出口配置失败")
			return fmt.Errorf("解析出口配置失败: %w", err)
		}
	}

	// 生成shadowsocks配置
	config := map[string]interface{}{
		"server":      egressConfig["server"],
		"server_port": *cfg.Port,     // 使用protobuf字段（指针解引用）
		"password":    *cfg.Password, // 使用protobuf字段（指针解引用）
		"method":      egressConfig["method"],
		"timeout":     egressConfig["timeout"],
		"fast_open":   true,
	}

	// 如果有本地配置
	if localPort, ok := egressConfig["local_port"]; ok {
		config["local_port"] = localPort
	} else {
		config["local_port"] = 1080
	}

	if localAddr, ok := egressConfig["local_address"]; ok {
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

	// 读取配置文件以构建命令行参数
	configData, err := os.ReadFile(s.configPath)
	if err != nil {
		logger.LogError(err, "读取配置文件失败", logrus.Fields{
			"config_path": s.configPath,
		})
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configData, &config); err != nil {
		logger.LogError(err, "解析配置文件失败", logrus.Fields{
			"config_path": s.configPath,
		})
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 构建shadowsocks URL格式
	// ss://method:password@server:port
	server := config["server"].(string)
	serverPort := fmt.Sprintf("%.0f", config["server_port"].(float64))
	password := config["password"].(string)
	method := config["method"].(string)
	localAddr := config["local_address"].(string)
	localPort := fmt.Sprintf("%.0f", config["local_port"].(float64))

	shadowsocksURL := fmt.Sprintf("ss://%s:%s@%s:%s", method, password, server, serverPort)
	socksAddr := fmt.Sprintf("%s:%s", localAddr, localPort)

	// 启动go-shadowsocks2客户端
	binaryPath := filepath.Join(DefaultBinPath, "go-shadowsocks2")
	cmd := exec.Command(binaryPath, "-c", shadowsocksURL, "-socks", socksAddr)

	// 设置环境变量以便后台运行
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		logger.LogError(err, "启动shadowsocks失败", logrus.Fields{
			"binary_path":     binaryPath,
			"shadowsocks_url": shadowsocksURL,
			"socks_addr":      socksAddr,
		})
		return fmt.Errorf("启动shadowsocks失败: %w", err)
	}

	// 写入PID文件
	if err := os.WriteFile(s.pidFile, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0644); err != nil {
		logger.LogError(err, "写入PID文件失败", logrus.Fields{
			"pid_file": s.pidFile,
			"pid":      cmd.Process.Pid,
		})
		// 不要因为PID文件写入失败而返回错误，服务已经启动了
	}

	duration := time.Since(startTime)
	logger.LogPerformance("shadowsocks_start", duration, logrus.Fields{
		"pid": cmd.Process.Pid,
	})

	// 记录状态变更
	logger.LogStateChange("shadowsocks", "stopped", "running", "正常启动")

	log.WithFields(logrus.Fields{
		"pid":         cmd.Process.Pid,
		"socks_addr":  socksAddr,
		"duration_ms": duration.Milliseconds(),
	}).Info("shadowsocks服务已启动")

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
	log := logger.GetProxyLogger().WithField("proxy_type", "shadowsocks")

	if err := s.Stop(); err != nil {
		log.WithError(err).Warn("停止shadowsocks失败")
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
	binaryPath := filepath.Join(DefaultBinPath, "go-shadowsocks2")
	_, err := os.Stat(binaryPath)
	installed := err == nil

	logger.GetProxyLogger().WithFields(logrus.Fields{
		"proxy_type":  "shadowsocks",
		"binary_path": binaryPath,
		"installed":   installed,
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
