package shadowsocks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/logging"
	"github.com/nspass/nspass-agent/pkg/process"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultBinPath 默认代理软件安装路径
	DefaultBinPath = "/usr/local/bin/proxy"
)

// Shadowsocks shadowsocks代理实现
type Shadowsocks struct {
	egressItem     *model.EgressItem
	config         config.ProxyConfig
	configPath     string
	processManager *process.Manager
}

// New 创建新的Shadowsocks实例
func New(egressItem *model.EgressItem) *Shadowsocks {
	configPath := filepath.Join("/etc/nspass-agent", fmt.Sprintf("shadowsocks-%d.json", egressItem.Id))
	pidFile := filepath.Join("/etc/nspass-agent", fmt.Sprintf("shadowsocks-%d.pid", egressItem.Id))

	processManager := process.NewManager("shadowsocks", "go-shadowsocks2", DefaultBinPath, pidFile)

	ss := &Shadowsocks{
		egressItem:     egressItem,
		configPath:     configPath,
		processManager: processManager,
	}

	logging.LogStartup("shadowsocks-proxy", "1.0", map[string]any{
		"config_path": ss.configPath,
		"pid_file":    pidFile,
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
	log := logging.GetProxyLogger().WithField("proxy_type", "shadowsocks")

	log.WithField("config_path", s.configPath).Debug("开始配置shadowsocks")

	// 确保配置目录存在
	configDir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		logging.LogError(err, "创建配置目录失败", logrus.Fields{
			"config_dir": configDir,
		})
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 从EgressItem中解析配置
	// 通用字段：Port和Password从EgressItem直接获取
	// 特定配置：从EgressConfig JSON解析
	egressConfig := make(map[string]any)
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

	// 生成shadowsocks服务端配置
	config := map[string]any{
		"server":      "0.0.0.0",               // 监听外网地址
		"server_port": *cfg.Port,               // 从通用字段获取
		"password":    *cfg.Password,           // 从通用字段获取
		"method":      egressConfig["method"],  // 从特定配置获取
		"timeout":     egressConfig["timeout"], // 从特定配置获取（可选）
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
		logging.LogError(err, "序列化配置失败", logrus.Fields{
			"config": cfg,
		})
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(s.configPath, data, 0600); err != nil {
		logging.LogError(err, "写入配置文件失败", logrus.Fields{
			"config_path": s.configPath,
		})
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	duration := time.Since(startTime)
	logging.LogPerformance("shadowsocks_configure", duration, logrus.Fields{
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

	// 构建启动参数
	args, err := s.buildStartArgs()
	if err != nil {
		return fmt.Errorf("构建启动参数失败: %w", err)
	}

	// 使用进程管理器启动进程
	cmd, err := s.processManager.StartProcess(args)
	if err != nil {
		return err
	}

	if cmd != nil {
		duration := time.Since(startTime)
		s.processManager.LogPerformance("start", duration, map[string]any{
			"pid": cmd.Process.Pid,
		})

		// 记录状态变更
		s.processManager.LogStateChange("stopped", "running", map[string]any{
			"reason": "正常启动",
		})
	}

	return nil
}

// buildStartArgs 构建shadowsocks启动参数
func (s *Shadowsocks) buildStartArgs() ([]string, error) {
	// 读取配置文件以构建命令行参数
	configData, err := os.ReadFile(s.configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config map[string]any
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 构建shadowsocks URL格式
	// ss://method:password@server:port
	// go-shadowsocks2 -s 'ss://AEAD_CHACHA20_POLY1305:your-password@:8488' -verbose

	serverPort := *s.egressItem.Port
	password := *s.egressItem.Password // 解引用指针

	method := "AEAD_AES_128_GCM" // 默认加密方法

	shadowsocksURL := fmt.Sprintf("ss://%s:%s@%s:%d", method, password, "", serverPort)

	return []string{"-c", shadowsocksURL}, nil
}

// Stop 停止shadowsocks
func (s *Shadowsocks) Stop() error {
	startTime := time.Now()

	// 使用进程管理器停止进程
	if err := s.processManager.StopProcess(); err != nil {
		return err
	}

	duration := time.Since(startTime)
	s.processManager.LogPerformance("stop", duration, nil)

	// 记录状态变更
	s.processManager.LogStateChange("running", "stopped", map[string]any{
		"reason": "正常停止",
	})

	return nil
}

// Restart 重启shadowsocks
func (s *Shadowsocks) Restart() error {
	// 构建启动参数
	args, err := s.buildStartArgs()
	if err != nil {
		return fmt.Errorf("构建启动参数失败: %w", err)
	}

	// 使用进程管理器重启进程
	_, err = s.processManager.RestartProcess(args)
	return err
}

// Status 获取shadowsocks状态
func (s *Shadowsocks) Status() (string, error) {
	return s.processManager.GetStatus()
}

// IsInstalled 检查是否已安装
func (s *Shadowsocks) IsInstalled() bool {
	return s.processManager.IsInstalled()
}

// IsRunning 检查是否正在运行
func (s *Shadowsocks) IsRunning() bool {
	return s.processManager.IsRunning()
}

// Cleanup 清理配置文件和PID文件
func (s *Shadowsocks) Cleanup() error {
	log := logging.GetProxyLogger().WithField("proxy_type", "shadowsocks")
	log.Debug("开始清理shadowsocks配置文件和PID文件")

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

	// 删除PID文件（通过进程管理器）
	if err := s.processManager.RemovePIDFile(); err != nil {
		log.WithError(err).Error("删除PID文件失败")
		errors = append(errors, fmt.Errorf("删除PID文件失败: %w", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("清理过程中发生错误: %v", errors)
	}

	log.Info("shadowsocks清理完成")
	return nil
}

// GetConfigPath 获取配置文件路径
func (s *Shadowsocks) GetConfigPath() string {
	return s.configPath
}

// GetPIDPath 获取PID文件路径
func (s *Shadowsocks) GetPIDPath() string {
	return s.processManager.GetPIDFile()
}
