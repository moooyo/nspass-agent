package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nspass/nspass-agent/pkg/logger"
	"gopkg.in/yaml.v3"
)

// Config 主配置结构
type Config struct {
	ServerID       string         `yaml:"server_id" json:"server_id"` // 服务器ID
	API            APIConfig      `yaml:"api" json:"api"`
	Proxy          ProxyConfig    `yaml:"proxy" json:"proxy"`
	IPTables       IPTablesConfig `yaml:"iptables" json:"iptables"`
	Logger         logger.Config  `yaml:"logger" json:"logger"`
	UpdateInterval int            `yaml:"update_interval" json:"update_interval"` // 秒
	LogLevel       string         `yaml:"log_level" json:"log_level"`
}

// APIConfig API配置
type APIConfig struct {
	BaseURL       string `yaml:"base_url" json:"base_url"`
	Token         string `yaml:"token" json:"token"`
	Timeout       int    `yaml:"timeout" json:"timeout"` // 秒
	RetryCount    int    `yaml:"retry_count" json:"retry_count"`
	RetryDelay    int    `yaml:"retry_delay" json:"retry_delay"`
	TLS           bool   `yaml:"tls" json:"tls"`                         // 是否启用TLS
	TLSSkipVerify bool   `yaml:"tls_skip_verify" json:"tls_skip_verify"` // 是否跳过TLS证书验证
}

// ProxyConfig 代理配置
type ProxyConfig struct {
	BinPath       string   `yaml:"bin_path" json:"bin_path"`               // 代理软件安装路径
	ConfigPath    string   `yaml:"config_path" json:"config_path"`         // 代理配置文件路径
	EnabledTypes  []string `yaml:"enabled_types" json:"enabled_types"`     // 启用的代理类型
	AutoStart     bool     `yaml:"auto_start" json:"auto_start"`           // 是否自动启动
	RestartOnFail bool     `yaml:"restart_on_fail" json:"restart_on_fail"` // 失败时是否重启

	// 进程监控配置
	Monitor MonitorConfig `yaml:"monitor" json:"monitor"` // 进程监控配置
}

// MonitorConfig 进程监控配置
type MonitorConfig struct {
	Enable          bool `yaml:"enable" json:"enable"`                     // 是否启用进程监控
	CheckInterval   int  `yaml:"check_interval" json:"check_interval"`     // 检查间隔（秒）
	RestartCooldown int  `yaml:"restart_cooldown" json:"restart_cooldown"` // 重启冷却时间（秒）
	MaxRestarts     int  `yaml:"max_restarts" json:"max_restarts"`         // 最大重启次数（每小时）
	HealthTimeout   int  `yaml:"health_timeout" json:"health_timeout"`     // 健康检查超时（秒）
}

// IPTablesConfig iptables配置
type IPTablesConfig struct {
	Enable      bool   `yaml:"enable" json:"enable"`
	ChainPrefix string `yaml:"chain_prefix" json:"chain_prefix"`
	BackupPath  string `yaml:"backup_path" json:"backup_path"`
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// 设置默认值
	setDefaults(&config)

	// 处理向后兼容性
	if config.LogLevel != "" && config.Logger.Level == "" {
		config.Logger.Level = config.LogLevel
	}

	return &config, nil
}

// setDefaults 设置默认配置值
func setDefaults(config *Config) {
	if config.UpdateInterval == 0 {
		config.UpdateInterval = 300 // 5分钟
	}

	if config.LogLevel == "" {
		config.LogLevel = "info"
	}

	if config.API.Timeout == 0 {
		config.API.Timeout = 30
	}

	if config.API.RetryCount == 0 {
		config.API.RetryCount = 3
	}

	if config.API.RetryDelay == 0 {
		config.API.RetryDelay = 5
	}

	if config.Proxy.BinPath == "" {
		config.Proxy.BinPath = "/usr/local/bin"
	}

	if config.Proxy.ConfigPath == "" {
		config.Proxy.ConfigPath = "/etc/nspass/proxy"
	}

	if len(config.Proxy.EnabledTypes) == 0 {
		config.Proxy.EnabledTypes = []string{"shadowsocks", "trojan", "snell"}
	}

	if config.Proxy.Monitor.CheckInterval == 0 {
		config.Proxy.Monitor.CheckInterval = 30 // 30秒检查一次
	}

	if config.Proxy.Monitor.RestartCooldown == 0 {
		config.Proxy.Monitor.RestartCooldown = 60 // 重启后60秒冷却
	}

	if config.Proxy.Monitor.MaxRestarts == 0 {
		config.Proxy.Monitor.MaxRestarts = 10 // 每小时最多重启10次
	}

	if config.Proxy.Monitor.HealthTimeout == 0 {
		config.Proxy.Monitor.HealthTimeout = 5 // 健康检查5秒超时
	}

	if config.IPTables.BackupPath == "" {
		config.IPTables.BackupPath = "/etc/nspass/iptables-backup"
	}

	if config.IPTables.ChainPrefix == "" {
		config.IPTables.ChainPrefix = "NSPASS_"
	}

	// 日志配置默认值
	if config.Logger.Level == "" {
		if config.LogLevel != "" {
			config.Logger.Level = config.LogLevel
		} else {
			config.Logger.Level = "info"
		}
	}
	if config.Logger.Format == "" {
		config.Logger.Format = "json"
	}
	if config.Logger.Output == "" {
		config.Logger.Output = "stdout"
	}
	if config.Logger.File == "" {
		config.Logger.File = "/var/log/nspass/agent.log"
	}
	if config.Logger.MaxSize == 0 {
		config.Logger.MaxSize = 100
	}
	if config.Logger.MaxBackups == 0 {
		config.Logger.MaxBackups = 5
	}
	if config.Logger.MaxAge == 0 {
		config.Logger.MaxAge = 30
	}
}

// Validate 验证配置的有效性
func (c *Config) Validate() error {
	if c.ServerID == "" {
		return fmt.Errorf("server_id不能为空")
	}

	if c.API.BaseURL == "" {
		return fmt.Errorf("API base_url不能为空")
	}

	if c.UpdateInterval <= 0 {
		return fmt.Errorf("update_interval必须大于0")
	}

	return nil
}

// SaveConfig 保存配置文件
func SaveConfig(config *Config, path string) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
