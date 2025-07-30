package trojan

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
	// DefaultConfigPath 默认代理配置文件路径
	DefaultConfigPath = "/etc/nspass-agent"
	// DefaultBinPath 默认代理软件安装路径
	DefaultBinPath = "/usr/local/bin/proxy"
)

// Trojan trojan代理实现
type Trojan struct {
	egressItem     *model.EgressItem // 出口配置
	config         config.ProxyConfig
	configPath     string
	processManager *process.Manager
}

// New 创建新的Trojan实例
func New(egressItem *model.EgressItem) *Trojan {
	configPath := filepath.Join(DefaultConfigPath, fmt.Sprintf("trojan-%s.json", egressItem.EgressId))
	pidFile := filepath.Join(DefaultConfigPath, fmt.Sprintf("trojan-%s.pid", egressItem.EgressId))

	processManager := process.NewManager("trojan", "trojan-go", DefaultBinPath, pidFile)

	t := &Trojan{
		egressItem:     egressItem,
		configPath:     configPath,
		processManager: processManager,
	}

	logging.LogStartup("trojan-proxy", "1.0", map[string]any{
		"config_path": t.configPath,
		"pid_file":    pidFile,
	})

	return t
}

// Type 返回代理类型
func (t *Trojan) Type() string {
	return "trojan"
}

// Configure 配置trojan
func (t *Trojan) Configure(cfg *model.EgressItem) error {
	startTime := time.Now()
	log := logging.GetProxyLogger().WithField("proxy_type", "trojan")

	log.WithField("config_path", t.configPath).Debug("开始配置trojan")

	// 确保配置目录存在
	configDir := filepath.Dir(t.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		logging.LogError(err, "创建配置目录失败", logrus.Fields{
			"config_dir": configDir,
		})
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 先停止现有服务
	if t.IsRunning() {
		log.Debug("停止现有trojan服务以更新配置")
		if err := t.Stop(); err != nil {
			logging.LogError(err, "停止trojan服务失败", nil)
		}
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

	// 生成trojan服务端配置
	config := map[string]any{
		"run_type":   "server",                // 运行为服务端模式
		"local_addr": "0.0.0.0",               // 监听外网地址
		"local_port": *cfg.Port,               // 从通用字段获取监听端口
		"password":   []string{*cfg.Password}, // 从通用字段获取
		"log_level":  1,
		"ssl": map[string]any{
			"verify":          true,
			"verify_hostname": true,
			"cert":            "",
			"cipher":          "ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384",
			"cipher_tls13":    "TLS_AES_128_GCM_SHA256:TLS_CHACHA20_POLY1305_SHA256:TLS_AES_256_GCM_SHA384",
			"sni":             egressConfig["sni"],
		},
		"tcp": map[string]any{
			"no_delay":       true,
			"keep_alive":     true,
			"reuse_port":     false,
			"fast_open":      false,
			"fast_open_qlen": 20,
		},
	}

	// 如果有自定义本地端口
	if localPort, ok := egressConfig["local_port"]; ok {
		config["local_port"] = localPort
	}

	if localAddr, ok := egressConfig["local_addr"]; ok {
		config["local_addr"] = localAddr
	}

	// 写入配置文件
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		logging.LogError(err, "序列化配置失败", logrus.Fields{
			"config": cfg,
		})
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(t.configPath, data, 0600); err != nil {
		logging.LogError(err, "写入配置文件失败", logrus.Fields{
			"config_path": t.configPath,
		})
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	duration := time.Since(startTime)
	logging.LogPerformance("trojan_configure", duration, logrus.Fields{
		"config_size": len(data),
	})

	log.WithFields(logrus.Fields{
		"config_path": t.configPath,
		"duration_ms": duration.Milliseconds(),
	}).Info("trojan配置已更新")

	return nil
}

// Start 启动trojan
func (t *Trojan) Start() error {
	startTime := time.Now()

	// 构建启动参数
	args := []string{"-c", t.configPath}

	// 使用进程管理器启动进程
	cmd, err := t.processManager.StartProcess(args)
	if err != nil {
		return err
	}

	if cmd != nil {
		duration := time.Since(startTime)
		t.processManager.LogPerformance("start", duration, map[string]any{
			"pid": cmd.Process.Pid,
		})

		// 记录状态变更
		t.processManager.LogStateChange("stopped", "running", map[string]any{
			"reason": "正常启动",
		})
	}

	return nil
}

// Stop 停止trojan
func (t *Trojan) Stop() error {
	startTime := time.Now()

	// 使用进程管理器停止进程
	if err := t.processManager.StopProcess(); err != nil {
		return err
	}

	duration := time.Since(startTime)
	t.processManager.LogPerformance("stop", duration, nil)

	// 记录状态变更
	t.processManager.LogStateChange("running", "stopped", map[string]any{
		"reason": "正常停止",
	})

	return nil
}

// Restart 重启trojan
func (t *Trojan) Restart() error {
	// 构建启动参数
	args := []string{"-c", t.configPath}

	// 使用进程管理器重启进程
	_, err := t.processManager.RestartProcess(args)
	return err
}

// Status 获取trojan状态
func (t *Trojan) Status() (string, error) {
	return t.processManager.GetStatus()
}

// IsInstalled 检查是否已安装
func (t *Trojan) IsInstalled() bool {
	return t.processManager.IsInstalled()
}

// IsRunning 检查是否正在运行
func (t *Trojan) IsRunning() bool {
	return t.processManager.IsRunning()
}
