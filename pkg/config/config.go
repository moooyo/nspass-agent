package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nspass/nspass-agent/pkg/logging"
	"gopkg.in/yaml.v3"
)

// DefaultValues 包含所有默认配置值
type DefaultValues struct {
	UpdateInterval             int
	LogLevel                   string
	APITimeout                 int
	APIRetryCount              int
	APIRetryDelay              int
	ProxyBinPath               string
	ProxyConfigPath            string
	ProxyEnabledTypes          []string
	MonitorCheckInterval       int
	MonitorRestartCooldown     int
	MonitorMaxRestarts         int
	MonitorHealthTimeout       int
	IPTablesBackupPath         string
	IPTablesChainPrefix        string
	LoggerMaxSize              int
	LoggerMaxBackups           int
	LoggerMaxAge               int
	WebSocketHeartbeatInterval string
	WebSocketMetricsInterval   string
	WebSocketReconnectInterval string
	WebSocketAgentID           string
	UpgradeAgentScriptURL      string
	UpgradeProxyScriptURL      string
	UpgradeTimeout             int
	CertStorePath              string
	CertExpiryThreshold        int
	CertCheckInterval          int
}

// GetDefaultValues 返回默认配置值
func GetDefaultValues() DefaultValues {
	return DefaultValues{
		UpdateInterval:             300, // 5分钟
		LogLevel:                   "info",
		APITimeout:                 30,
		APIRetryCount:              3,
		APIRetryDelay:              5,
		ProxyBinPath:               "/usr/local/bin",
		ProxyConfigPath:            "/etc/nspass/proxy",
		ProxyEnabledTypes:          []string{"shadowsocks", "trojan", "snell"},
		MonitorCheckInterval:       30,
		MonitorRestartCooldown:     60,
		MonitorMaxRestarts:         10,
		MonitorHealthTimeout:       5,
		IPTablesBackupPath:         "/etc/nspass/iptables-backup",
		IPTablesChainPrefix:        "NSPASS_",
		LoggerMaxSize:              100,
		LoggerMaxBackups:           5,
		LoggerMaxAge:               30,
		WebSocketHeartbeatInterval: "30s",
		WebSocketMetricsInterval:   "60s",
		WebSocketReconnectInterval: "5s",
		WebSocketAgentID:           "agent-001",
		UpgradeAgentScriptURL:      "https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/agent_upgrade.sh",
		UpgradeProxyScriptURL:      "https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/proxy_upgrade.sh",
		UpgradeTimeout:             600, // 10分钟
		CertStorePath:              "/etc/nspass-agent/certs",
		CertExpiryThreshold:        7,  // 7天
		CertCheckInterval:          24, // 24小时
	}
}

// Config 主配置结构
type Config struct {
	ServerID       string          `yaml:"server_id" json:"server_id"` // 服务器ID
	API            APIConfig       `yaml:"api" json:"api"`
	Proxy          ProxyConfig     `yaml:"proxy" json:"proxy"`
	IPTables       IPTablesConfig  `yaml:"iptables" json:"iptables"`
	Logger         logging.Config  `yaml:"logger" json:"logger"`
	WebSocket      WebSocketConfig `yaml:"websocket" json:"websocket"`             // WebSocket配置
	Upgrade        UpgradeConfig   `yaml:"upgrade" json:"upgrade"`                 // 升级配置
	Certificate    CertConfig      `yaml:"certificate" json:"certificate"`         // 证书管理配置
	UpdateInterval int             `yaml:"update_interval" json:"update_interval"` // 秒
	LogLevel       string          `yaml:"log_level" json:"log_level"`
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

// WebSocketConfig WebSocket配置
type WebSocketConfig struct {
	HeartbeatInterval    string `yaml:"heartbeat_interval" json:"heartbeat_interval"`         // 心跳间隔
	MetricsInterval      string `yaml:"metrics_interval" json:"metrics_interval"`             // 监控数据上报间隔
	ReconnectInterval    string `yaml:"reconnect_interval" json:"reconnect_interval"`         // 重连间隔
	MaxReconnectAttempts int    `yaml:"max_reconnect_attempts" json:"max_reconnect_attempts"` // 最大重连次数（0表示无限）
}

// UpgradeConfig 升级配置
type UpgradeConfig struct {
	Enabled        bool   `yaml:"enabled" json:"enabled"`                   // 是否启用升级功能
	AgentScriptURL string `yaml:"agent_script_url" json:"agent_script_url"` // Agent升级脚本URL
	ProxyScriptURL string `yaml:"proxy_script_url" json:"proxy_script_url"` // Proxy升级脚本URL
	Timeout        int    `yaml:"timeout" json:"timeout"`                   // 升级超时时间（秒）
	VerifyChecksum bool   `yaml:"verify_checksum" json:"verify_checksum"`   // 是否验证校验和
	BackupEnabled  bool   `yaml:"backup_enabled" json:"backup_enabled"`     // 是否启用备份
}

// CertConfig 证书管理配置
type CertConfig struct {
	Enabled         bool   `yaml:"enabled" json:"enabled"`                   // 是否启用证书管理
	Email           string `yaml:"email" json:"email"`                       // ACME账户邮箱
	StorePath       string `yaml:"store_path" json:"store_path"`             // 证书存储路径
	UseStaging      bool   `yaml:"use_staging" json:"use_staging"`           // 是否使用Let's Encrypt测试环境
	ExpiryThreshold int    `yaml:"expiry_threshold" json:"expiry_threshold"` // 证书过期阈值（天）
	CheckInterval   int    `yaml:"check_interval" json:"check_interval"`     // 过期检查间隔（小时）
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
	defaults := GetDefaultValues()

	// 基础配置
	if config.UpdateInterval == 0 {
		config.UpdateInterval = defaults.UpdateInterval
	}
	if config.LogLevel == "" {
		config.LogLevel = defaults.LogLevel
	}

	// API配置
	setAPIDefaults(&config.API, defaults)

	// 代理配置
	setProxyDefaults(&config.Proxy, defaults)

	// IPTables配置
	setIPTablesDefaults(&config.IPTables, defaults)

	// 日志配置
	setLoggerDefaults(&config.Logger, config.LogLevel, defaults)

	// WebSocket配置
	setWebSocketDefaults(&config.WebSocket, defaults)

	// 升级配置
	setUpgradeDefaults(&config.Upgrade, defaults)

	// 证书配置
	setCertDefaults(&config.Certificate, defaults)
}

// setAPIDefaults 设置API配置默认值
func setAPIDefaults(api *APIConfig, defaults DefaultValues) {
	if api.Timeout == 0 {
		api.Timeout = defaults.APITimeout
	}
	if api.RetryCount == 0 {
		api.RetryCount = defaults.APIRetryCount
	}
	if api.RetryDelay == 0 {
		api.RetryDelay = defaults.APIRetryDelay
	}
}

// setProxyDefaults 设置代理配置默认值
func setProxyDefaults(proxy *ProxyConfig, defaults DefaultValues) {
	if proxy.BinPath == "" {
		proxy.BinPath = defaults.ProxyBinPath
	}
	if proxy.ConfigPath == "" {
		proxy.ConfigPath = defaults.ProxyConfigPath
	}
	if len(proxy.EnabledTypes) == 0 {
		proxy.EnabledTypes = defaults.ProxyEnabledTypes
	}

	// 监控配置
	if proxy.Monitor.CheckInterval == 0 {
		proxy.Monitor.CheckInterval = defaults.MonitorCheckInterval
	}
	if proxy.Monitor.RestartCooldown == 0 {
		proxy.Monitor.RestartCooldown = defaults.MonitorRestartCooldown
	}
	if proxy.Monitor.MaxRestarts == 0 {
		proxy.Monitor.MaxRestarts = defaults.MonitorMaxRestarts
	}
	if proxy.Monitor.HealthTimeout == 0 {
		proxy.Monitor.HealthTimeout = defaults.MonitorHealthTimeout
	}
}

// setIPTablesDefaults 设置IPTables配置默认值
func setIPTablesDefaults(iptables *IPTablesConfig, defaults DefaultValues) {
	if iptables.BackupPath == "" {
		iptables.BackupPath = defaults.IPTablesBackupPath
	}
	if iptables.ChainPrefix == "" {
		iptables.ChainPrefix = defaults.IPTablesChainPrefix
	}
}

// setLoggerDefaults 设置日志配置默认值
func setLoggerDefaults(logger *logging.Config, logLevel string, defaults DefaultValues) {
	if logger.Level == "" {
		if logLevel != "" {
			logger.Level = logLevel
		} else {
			logger.Level = defaults.LogLevel
		}
	}
	if logger.Format == "" {
		logger.Format = "json"
	}
	if logger.File == "" {
		logger.File = "/var/log/nspass/agent.log"
	}
	if logger.MaxSize == 0 {
		logger.MaxSize = defaults.LoggerMaxSize
	}
	if logger.MaxBackups == 0 {
		logger.MaxBackups = defaults.LoggerMaxBackups
	}
	if logger.MaxAge == 0 {
		logger.MaxAge = defaults.LoggerMaxAge
	}
}

// setWebSocketDefaults 设置WebSocket配置默认值
func setWebSocketDefaults(ws *WebSocketConfig, defaults DefaultValues) {
	if ws.HeartbeatInterval == "" {
		ws.HeartbeatInterval = defaults.WebSocketHeartbeatInterval
	}
	if ws.MetricsInterval == "" {
		ws.MetricsInterval = defaults.WebSocketMetricsInterval
	}
	if ws.ReconnectInterval == "" {
		ws.ReconnectInterval = defaults.WebSocketReconnectInterval
	}
}

// setUpgradeDefaults 设置升级配置默认值
func setUpgradeDefaults(upgrade *UpgradeConfig, defaults DefaultValues) {
	if upgrade.AgentScriptURL == "" {
		upgrade.AgentScriptURL = defaults.UpgradeAgentScriptURL
	}
	if upgrade.ProxyScriptURL == "" {
		upgrade.ProxyScriptURL = defaults.UpgradeProxyScriptURL
	}
	if upgrade.Timeout == 0 {
		upgrade.Timeout = defaults.UpgradeTimeout
	}
	// 默认启用升级功能和备份
	upgrade.Enabled = true
	upgrade.BackupEnabled = true
}

// ValidationError 配置验证错误
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("配置验证失败 [%s]: %s", e.Field, e.Message)
}

// Validate 验证配置的有效性
func (c *Config) Validate() error {
	var errors []ValidationError

	// 验证基础配置
	if c.ServerID == "" {
		errors = append(errors, ValidationError{"server_id", "不能为空"})
	}

	if c.UpdateInterval <= 0 {
		errors = append(errors, ValidationError{"update_interval", "必须大于0"})
	}

	// 验证API配置
	if err := c.validateAPI(); err != nil {
		errors = append(errors, err...)
	}

	// 验证WebSocket配置
	if err := c.validateWebSocket(); err != nil {
		errors = append(errors, err...)
	}

	// 验证代理配置
	if err := c.validateProxy(); err != nil {
		errors = append(errors, err...)
	}

	if len(errors) > 0 {
		return fmt.Errorf("配置验证失败: %v", errors)
	}

	return nil
}

// validateAPI 验证API配置
func (c *Config) validateAPI() []ValidationError {
	var errors []ValidationError

	if c.API.BaseURL == "" {
		errors = append(errors, ValidationError{"api.base_url", "不能为空"})
	}

	if c.API.Timeout <= 0 {
		errors = append(errors, ValidationError{"api.timeout", "必须大于0"})
	}

	if c.API.RetryCount < 0 {
		errors = append(errors, ValidationError{"api.retry_count", "不能小于0"})
	}

	return errors
}

// validateWebSocket 验证WebSocket配置
func (c *Config) validateWebSocket() []ValidationError {
	var errors []ValidationError

	// 验证时间间隔格式
	if c.WebSocket.HeartbeatInterval != "" {
		if _, err := time.ParseDuration(c.WebSocket.HeartbeatInterval); err != nil {
			errors = append(errors, ValidationError{"websocket.heartbeat_interval", "时间格式无效"})
		}
	}

	return errors
}

// validateProxy 验证代理配置
func (c *Config) validateProxy() []ValidationError {
	var errors []ValidationError

	if c.Proxy.Monitor.CheckInterval <= 0 {
		errors = append(errors, ValidationError{"proxy.monitor.check_interval", "必须大于0"})
	}

	if c.Proxy.Monitor.MaxRestarts < 0 {
		errors = append(errors, ValidationError{"proxy.monitor.max_restarts", "不能小于0"})
	}

	return errors
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

// setCertDefaults 设置证书配置默认值
func setCertDefaults(cert *CertConfig, defaults DefaultValues) {
	if cert.Email == "" {
		cert.Email = ""
	}
	if cert.StorePath == "" {
		cert.StorePath = defaults.CertStorePath
	}
	if cert.ExpiryThreshold == 0 {
		cert.ExpiryThreshold = defaults.CertExpiryThreshold
	}
	if cert.CheckInterval == 0 {
		cert.CheckInterval = defaults.CertCheckInterval
	}
}
