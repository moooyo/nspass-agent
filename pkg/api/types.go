package api

import (
	"time"
)

// ServerConfigData 服务器配置数据
type ServerConfigData struct {
	ServerID     string              `json:"server_id"`
	ServerName   string              `json:"server_name"`
	Routes       []RouteConfig       `json:"routes"`
	Egress       []EgressConfig      `json:"egress"`
	ForwardRules []ForwardRuleConfig `json:"forward_rules"`
	Metadata     map[string]string   `json:"metadata"`
	LastUpdated  time.Time           `json:"last_updated"`
}

// RouteConfig 路由配置
type RouteConfig struct {
	ID             string                 `json:"id"`
	RouteID        string                 `json:"route_id"`
	RouteName      string                 `json:"route_name"`
	EntryPoint     string                 `json:"entry_point"`
	Port           int32                  `json:"port"`
	Protocol       string                 `json:"protocol"`
	ProtocolParams map[string]interface{} `json:"protocol_params"`
	Type           string                 `json:"type"`
	Status         string                 `json:"status"`
	ServerID       string                 `json:"server_id"`
	Description    string                 `json:"description"`
	Metadata       map[string]string      `json:"metadata"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// EgressConfig 出口配置
type EgressConfig struct {
	ID            string `json:"id"`
	EgressID      string `json:"egress_id"`
	ServerID      string `json:"server_id"`
	EgressMode    string `json:"egress_mode"`
	EgressConfig  string `json:"egress_config"`
	TargetAddress string `json:"target_address,omitempty"`
	ForwardType   string `json:"forward_type,omitempty"`
	DestAddress   string `json:"dest_address,omitempty"`
	DestPort      string `json:"dest_port,omitempty"`
	Password      string `json:"password,omitempty"`
	SupportUDP    bool   `json:"support_udp,omitempty"`
}

// ForwardRuleConfig 转发规则配置
type ForwardRuleConfig struct {
	ID             uint32 `json:"id"`
	UserID         uint32 `json:"user_id"`
	Name           string `json:"name"`
	ServerID       uint32 `json:"server_id"`
	EgressMode     string `json:"egress_mode"`
	ForwardType    string `json:"forward_type"`
	SourcePort     int32  `json:"source_port"`
	TargetAddress  string `json:"target_address,omitempty"`
	TargetPort     int32  `json:"target_port"`
	Password       string `json:"password,omitempty"`
	SupportUDP     bool   `json:"support_udp"`
	Status         string `json:"status"`
	TrafficUp      int64  `json:"traffic_up"`
	TrafficDown    int64  `json:"traffic_down"`
	LastActiveTime int64  `json:"last_active_time"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

// AgentStatusReport Agent状态上报
type AgentStatusReport struct {
	ServerID    string        `json:"server_id"`
	IPv4Address string        `json:"ipv4_address,omitempty"`
	IPv6Address string        `json:"ipv6_address,omitempty"`
	Activity    AgentActivity `json:"activity"`
	ReportTime  time.Time     `json:"report_time"`
}

// AgentActivity Agent活动信息
type AgentActivity struct {
	ActiveConnections  int32                `json:"active_connections"`
	TotalBytesSent     int64                `json:"total_bytes_sent"`
	TotalBytesReceived int64                `json:"total_bytes_received"`
	ProxyServices      []ProxyServiceStatus `json:"proxy_services"`
	LastActivity       time.Time            `json:"last_activity"`
	CPUUsage           float32              `json:"cpu_usage"`
	MemoryUsage        float32              `json:"memory_usage"`
	DiskUsage          float32              `json:"disk_usage"`
}

// ProxyServiceStatus 代理服务状态
type ProxyServiceStatus struct {
	ServiceName     string    `json:"service_name"`
	ServiceStatus   string    `json:"service_status"`
	Port            int32     `json:"port"`
	ConnectionCount int32     `json:"connection_count"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	LastCheck       time.Time `json:"last_check"`
}

// ServerConfigUpdateInfo 服务器配置更新信息
type ServerConfigUpdateInfo struct {
	HasUpdate     bool      `json:"has_update"`
	ConfigVersion string    `json:"config_version"`
	UpdateTime    time.Time `json:"update_time"`
	UpdateMessage string    `json:"update_message"`
}
