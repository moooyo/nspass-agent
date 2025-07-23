package api

import (
	"fmt"
	"strings"
	"time"

	model "github.com/moooyo/nspass-proto/generated/model"
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

// ProxyConfig 代理配置
type ProxyConfig struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"` // shadowsocks, trojan, snell
	Name    string                 `json:"name"`
	Config  map[string]interface{} `json:"config"`
	Enabled bool                   `json:"enabled"`
}

// ConvertRouteFromProto 从proto转换路由配置
func ConvertRouteFromProto(route *model.Route) RouteConfig {
	config := RouteConfig{
		ID:         fmt.Sprintf("%d", route.Id),
		RouteID:    fmt.Sprintf("%d", route.Id),
		RouteName:  route.RouteName,
		EntryPoint: route.EntryPoint,
		Port:       route.Port,
		ServerID:   route.ServerId,
		Metadata:   route.Metadata,
	}

	// 处理可选字段
	if route.Description != nil {
		config.Description = *route.Description
	}

	// 转换协议
	switch route.Protocol {
	case model.Protocol_PROTOCOL_SHADOWSOCKS:
		config.Protocol = "shadowsocks"
	case model.Protocol_PROTOCOL_SNELL:
		config.Protocol = "snell"
	default:
		config.Protocol = "unknown"
	}

	// 转换协议参数
	if route.ProtocolParams != nil {
		config.ProtocolParams = make(map[string]interface{})
		// 根据proto结构解析参数
	}

	// 转换类型
	switch route.Type {
	case model.RouteType_ROUTE_TYPE_CUSTOM:
		config.Type = "custom"
	case model.RouteType_ROUTE_TYPE_SYSTEM:
		config.Type = "system"
	default:
		config.Type = "unknown"
	}

	// 转换状态
	switch route.Status {
	case model.RouteStatus_ROUTE_STATUS_ACTIVE:
		config.Status = "active"
	case model.RouteStatus_ROUTE_STATUS_INACTIVE:
		config.Status = "inactive"
	case model.RouteStatus_ROUTE_STATUS_ERROR:
		config.Status = "error"
	default:
		config.Status = "unknown"
	}

	// 转换时间
	if route.CreatedAt != 0 {
		config.CreatedAt = time.Unix(route.CreatedAt, 0)
	}
	if route.UpdatedAt != 0 {
		config.UpdatedAt = time.Unix(route.UpdatedAt, 0)
	}

	return config
}

// ConvertEgressFromProto 从proto转换出口配置
func ConvertEgressFromProto(egress *model.EgressItem) EgressConfig {
	config := EgressConfig{
		ID:           fmt.Sprintf("%d", egress.Id),
		EgressID:     egress.EgressId,
		ServerID:     egress.ServerId,
		EgressConfig: egress.EgressConfig,
	}

	// 转换出口模式
	switch egress.EgressMode {
	case model.EgressMode_EGRESS_MODE_DIRECT:
		config.EgressMode = "direct"
	case model.EgressMode_EGRESS_MODE_IPTABLES:
		config.EgressMode = "iptables"
	case model.EgressMode_EGRESS_MODE_SS2022:
		config.EgressMode = "ss2022"
	default:
		config.EgressMode = "unknown"
	}

	// 设置可选字段
	if egress.TargetAddress != nil {
		config.TargetAddress = *egress.TargetAddress
	}

	if egress.ForwardType != nil {
		switch *egress.ForwardType {
		case model.ForwardType_FORWARD_TYPE_TCP:
			config.ForwardType = "tcp"
		case model.ForwardType_FORWARD_TYPE_UDP:
			config.ForwardType = "udp"
		case model.ForwardType_FORWARD_TYPE_ALL:
			config.ForwardType = "all"
		default:
			config.ForwardType = "unknown"
		}
	}

	if egress.DestAddress != nil {
		config.DestAddress = *egress.DestAddress
	}

	if egress.DestPort != nil {
		config.DestPort = *egress.DestPort
	}

	if egress.Password != nil {
		config.Password = *egress.Password
	}

	if egress.SupportUdp != nil {
		config.SupportUDP = *egress.SupportUdp
	}

	return config
}

// ConvertForwardRuleFromProto 从proto转换转发规则
func ConvertForwardRuleFromProto(rule *model.ForwardRule) ForwardRuleConfig {
	config := ForwardRuleConfig{
		ID:             rule.Id,
		UserID:         rule.UserId,
		Name:           rule.Name,
		ServerID:       rule.ServerId,
		SourcePort:     rule.SourcePort,
		TargetPort:     rule.TargetPort,
		SupportUDP:     rule.SupportUdp,
		Status:         rule.Status,
		TrafficUp:      rule.TrafficUp,
		TrafficDown:    rule.TrafficDown,
		LastActiveTime: rule.LastActiveTime,
		CreatedAt:      rule.CreatedAt,
		UpdatedAt:      rule.UpdatedAt,
	}

	// 转换出口模式
	if rule.EgressMode != 0 {
		switch rule.EgressMode {
		case 1:
			config.EgressMode = "direct"
		case 2:
			config.EgressMode = "proxy"
		default:
			config.EgressMode = "unknown"
		}
	}

	// 转换转发类型
	if rule.ForwardType != 0 {
		switch rule.ForwardType {
		case 1:
			config.ForwardType = "tcp"
		case 2:
			config.ForwardType = "udp"
		case 3:
			config.ForwardType = "http"
		default:
			config.ForwardType = "unknown"
		}
	}

	// 设置可选字段
	if rule.TargetAddress != nil {
		config.TargetAddress = *rule.TargetAddress
	}

	if rule.Password != nil {
		config.Password = *rule.Password
	}

	return config
}

// ConvertProtoIptablesConfigToRuleParts 将proto IptablesConfig转换为iptables规则参数
func ConvertProtoIptablesConfigToRuleParts(config *model.IptablesConfig) (table, chain, rule string) {
	table = config.TableName
	chain = config.ChainName
	
	var parts []string

	// 添加协议
	if config.Protocol != "" && strings.ToUpper(config.Protocol) != "ALL" {
		parts = append(parts, fmt.Sprintf("-p %s", strings.ToLower(config.Protocol)))
	}

	// 添加源IP
	if config.SourceIp != nil && *config.SourceIp != "" {
		parts = append(parts, fmt.Sprintf("-s %s", *config.SourceIp))
	}

	// 添加目标IP
	if config.DestIp != nil && *config.DestIp != "" {
		parts = append(parts, fmt.Sprintf("-d %s", *config.DestIp))
	}

	// 添加源端口
	if config.SourcePort != nil && *config.SourcePort != "" {
		parts = append(parts, fmt.Sprintf("--sport %s", *config.SourcePort))
	}

	// 添加目标端口
	if config.DestPort != nil && *config.DestPort != "" {
		parts = append(parts, fmt.Sprintf("--dport %s", *config.DestPort))
	}

	// 添加网络接口
	if config.Interface != nil && *config.Interface != "" {
		parts = append(parts, fmt.Sprintf("-i %s", *config.Interface))
	}

	// 添加动作
	if config.RuleAction != "" {
		parts = append(parts, fmt.Sprintf("-j %s", strings.ToUpper(config.RuleAction)))
	}

	// 添加注释
	if config.RuleComment != nil && *config.RuleComment != "" {
		parts = append(parts, fmt.Sprintf(`-m comment --comment "%s"`, *config.RuleComment))
	}

	rule = strings.Join(parts, " ")
	return table, chain, rule
}
