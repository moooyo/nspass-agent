package proxy

// 代理路径常量定义
const (
	// DefaultBinPath 默认代理软件安装路径
	DefaultBinPath = "/usr/local/bin"

	// DefaultConfigPath 默认代理配置文件路径
	DefaultConfigPath = "/etc/nspass-agent"

	// ProxyBinPaths 各代理软件的二进制文件路径
	TrojanBinPath      = DefaultBinPath + "/trojan"
	ShadowsocksBinPath = DefaultBinPath + "/ss-local"
	SnellServerBinPath = DefaultBinPath + "/snell-server"
)
