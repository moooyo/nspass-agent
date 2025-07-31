package trojan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/cert"
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
	configPath     string
	processManager *process.Manager
	certManager    *cert.Manager // 证书管理器
	certConfig     *cert.Config  // 证书配置
}

// New 创建新的Trojan实例
func New(egressItem *model.EgressItem) *Trojan {
	configPath := filepath.Join(DefaultConfigPath, fmt.Sprintf("trojan-%d.json", egressItem.Id))
	pidFile := filepath.Join(DefaultConfigPath, fmt.Sprintf("trojan-%d.pid", egressItem.Id))

	processManager := process.NewManager("trojan", "trojan-go", DefaultBinPath, pidFile)

	t := &Trojan{
		egressItem:     egressItem,
		configPath:     configPath,
		processManager: processManager,
		certManager:    nil, // 将在Configure时创建
	}

	logging.LogStartup("trojan-proxy", "1.0", map[string]any{
		"config_path": t.configPath,
		"pid_file":    pidFile,
	})

	return t
}

// SetCertConfig 设置证书配置
func (t *Trojan) SetCertConfig(certConfig *cert.Config) {
	t.certConfig = certConfig
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

	// 验证trojan配置的完整性
	if err := t.validateTrojanConfig(cfg); err != nil {
		return fmt.Errorf("trojan配置验证失败: %w", err)
	}

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

	// 处理TLS证书 - trojan必须有证书才能工作
	var certPath, keyPath string

	// 检查是否配置了DNS配置ID
	if cfg.DnsConfigId == nil {
		return fmt.Errorf("trojan代理必须配置dns_config_id以申请TLS证书")
	}

	// 检查是否有证书配置
	if t.certConfig == nil {
		return fmt.Errorf("trojan代理必须配置证书管理器以申请TLS证书")
	}

	dnsConfig, err := cert.GetGlobalDNSConfigManager().GetDNSConfig(*cfg.DnsConfigId)
	if err != nil {
		return fmt.Errorf("获取DNS配置失败: %w", err)
	}

	domain := dnsConfig.Domain

	// 为这个trojan实例创建独立的证书管理器
	if t.certManager == nil {
		var err error
		t.certManager, err = cert.NewManager(t.certConfig)
		if err != nil {
			log.WithError(err).Error("创建证书管理器失败")
			return fmt.Errorf("创建证书管理器失败: %w", err)
		}
		log.WithField("domain", domain).Info("为trojan实例创建独立的证书管理器")
	}

	// 确保证书存在且有效
	certInfo, err := t.certManager.EnsureCertificate(domain, *cfg.DnsConfigId)
	if err != nil {
		log.WithError(err).WithField("domain", domain).Error("获取TLS证书失败")
		return fmt.Errorf("获取TLS证书失败: %w", err)
	}

	// trojan必须有证书
	if certInfo == nil {
		return fmt.Errorf("无法为域名 %s 获取TLS证书,trojan代理无法启动", domain)
	}

	certPath = certInfo.CertPath
	keyPath = certInfo.KeyPath
	log.WithFields(logrus.Fields{
		"domain":     domain,
		"cert_path":  certPath,
		"key_path":   keyPath,
		"expires_at": certInfo.ExpiresAt,
	}).Info("使用TLS证书")

	// 生成trojan服务端配置
	sslConfig := map[string]any{
		"verify":          true,
		"verify_hostname": true,
		"cipher":          "ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384",
		"cipher_tls13":    "TLS_AES_128_GCM_SHA256:TLS_CHACHA20_POLY1305_SHA256:TLS_AES_256_GCM_SHA384",
		"sni":             domain,
	}

	// 验证证书路径（trojan必须有证书）
	if certPath == "" || keyPath == "" {
		return fmt.Errorf("trojan代理必须有有效的TLS证书和私钥文件")
	}

	// 验证证书文件是否存在
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return fmt.Errorf("证书文件不存在: %s", certPath)
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return fmt.Errorf("私钥文件不存在: %s", keyPath)
	}

	// 设置证书路径
	sslConfig["cert"] = certPath
	sslConfig["key"] = keyPath

	config := map[string]any{
		"run_type":   "server",                // 运行为服务端模式
		"local_addr": "0.0.0.0",               // 监听外网地址
		"local_port": *cfg.Port,               // 从通用字段获取监听端口
		"password":   []string{*cfg.Password}, // 从通用字段获取
		"log_level":  1,
		"ssl":        sslConfig,
		"tcp": map[string]any{
			"no_delay":       true,
			"keep_alive":     true,
			"reuse_port":     false,
			"fast_open":      false,
			"fast_open_qlen": 20,
		},
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

// validateTrojanConfig 验证trojan配置的完整性
func (t *Trojan) validateTrojanConfig(cfg *model.EgressItem) error {
	if cfg.Port == nil {
		return fmt.Errorf("port不能为空")
	}

	if cfg.Password == nil || *cfg.Password == "" {
		return fmt.Errorf("password不能为空")
	}

	// 解析egress配置
	var egressConfig map[string]interface{}
	if err := json.Unmarshal([]byte(cfg.EgressConfig), &egressConfig); err != nil {
		return fmt.Errorf("解析egress配置失败: %w", err)
	}

	// 检查DNS配置ID（trojan必须有证书）
	if cfg.DnsConfigId == nil {
		return fmt.Errorf("trojan代理必须配置dns_config_id以申请TLS证书")
	}

	// 检查证书配置
	if t.certConfig == nil {
		return fmt.Errorf("trojan代理必须配置证书管理器")
	}

	if t.certConfig.Email == "" {
		return fmt.Errorf("证书管理器必须配置有效的邮箱地址")
	}

	return nil
}

// Cleanup 清理配置文件和PID文件
func (t *Trojan) Cleanup() error {
	log := logging.GetProxyLogger().WithField("proxy_type", "trojan")
	log.Debug("开始清理trojan配置文件和PID文件")

	var errors []error

	// 确保进程已停止
	if t.IsRunning() {
		if err := t.Stop(); err != nil {
			log.WithError(err).Warn("停止进程失败，继续清理文件")
			errors = append(errors, fmt.Errorf("停止进程失败: %w", err))
		}
	}

	// 删除配置文件
	if _, err := os.Stat(t.configPath); err == nil {
		if err := os.Remove(t.configPath); err != nil {
			log.WithError(err).WithField("config_path", t.configPath).Error("删除配置文件失败")
			errors = append(errors, fmt.Errorf("删除配置文件失败: %w", err))
		} else {
			log.WithField("config_path", t.configPath).Info("配置文件已删除")
		}
	}

	// 删除PID文件（通过进程管理器）
	if err := t.processManager.RemovePIDFile(); err != nil {
		log.WithError(err).Error("删除PID文件失败")
		errors = append(errors, fmt.Errorf("删除PID文件失败: %w", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("清理过程中发生错误: %v", errors)
	}

	log.Info("trojan清理完成")
	return nil
}

// GetConfigPath 获取配置文件路径
func (t *Trojan) GetConfigPath() string {
	return t.configPath
}

// GetPIDPath 获取PID文件路径
func (t *Trojan) GetPIDPath() string {
	return t.processManager.GetPIDFile()
}
