package proxy

// 通用常量定义，避免在各个代理实现中重复定义

const (
	// 默认路径配置
	DefaultConfigPath = "/etc/nspass-agent"
	DefaultBinPath    = "/usr/local/bin/proxy"
	DefaultLogPath    = "/var/log/nspass"

	// 代理类型常量
	ProxyTypeShadowsocks = "shadowsocks"
	ProxyTypeTrojan      = "trojan"
	ProxyTypeSnell       = "snell"

	// 二进制文件名
	BinaryShadowsocks = "go-shadowsocks2"
	BinaryTrojan      = "trojan-go"
	BinarySnell       = "snell-server"

	// 配置文件扩展名
	ConfigExtJSON = ".json"
	ConfigExtConf = ".conf"

	// 进程状态
	StatusNotInstalled = "not_installed"
	StatusStopped      = "stopped"
	StatusRunning      = "running"
	StatusError        = "error"

	// 默认超时设置
	DefaultStartTimeout = 30 // 秒
	DefaultStopTimeout  = 10 // 秒
	DefaultRestartDelay = 1  // 秒

	// 文件权限
	ConfigFileMode = 0644
	PIDFileMode    = 0644
	DirMode        = 0755

	// 兼容性常量（保持向后兼容）
	TrojanBinPath      = DefaultBinPath + "/trojan"
	ShadowsocksBinPath = DefaultBinPath + "/ss-local"
	SnellServerBinPath = DefaultBinPath + "/snell-server"
)

// ProxyTypeConfig 代理类型配置
type ProxyTypeConfig struct {
	Type       string
	BinaryName string
	ConfigExt  string
}

// GetProxyTypeConfig 获取代理类型配置
func GetProxyTypeConfig(proxyType string) ProxyTypeConfig {
	switch proxyType {
	case ProxyTypeShadowsocks:
		return ProxyTypeConfig{
			Type:       ProxyTypeShadowsocks,
			BinaryName: BinaryShadowsocks,
			ConfigExt:  ConfigExtJSON,
		}
	case ProxyTypeTrojan:
		return ProxyTypeConfig{
			Type:       ProxyTypeTrojan,
			BinaryName: BinaryTrojan,
			ConfigExt:  ConfigExtJSON,
		}
	case ProxyTypeSnell:
		return ProxyTypeConfig{
			Type:       ProxyTypeSnell,
			BinaryName: BinarySnell,
			ConfigExt:  ConfigExtConf,
		}
	default:
		return ProxyTypeConfig{}
	}
}

// IsValidProxyType 检查是否为有效的代理类型
func IsValidProxyType(proxyType string) bool {
	switch proxyType {
	case ProxyTypeShadowsocks, ProxyTypeTrojan, ProxyTypeSnell:
		return true
	default:
		return false
	}
}

// GetSupportedProxyTypes 获取支持的代理类型列表
func GetSupportedProxyTypes() []string {
	return []string{
		ProxyTypeShadowsocks,
		ProxyTypeTrojan,
		ProxyTypeSnell,
	}
}
