package api

import (
	"fmt"
	"strings"
	"time"

	model "github.com/nspass/nspass-agent/generated/model"
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

// IPTablesConfig iptables配置
type IPTablesConfig struct {
	ID            string            `json:"id"`
	ConfigName    string            `json:"config_name"`
	TableType     string            `json:"table_type"`
	ChainType     string            `json:"chain_type"`
	Protocol      string            `json:"protocol"`
	SourceIP      string            `json:"source_ip"`
	SourcePort    string            `json:"source_port"`
	DestIP        string            `json:"dest_ip"`
	DestPort      string            `json:"dest_port"`
	Action        string            `json:"action"`
	JumpTarget    string            `json:"jump_target"`
	Rule          string            `json:"rule"`
	Priority      int32             `json:"priority"`
	Description   string            `json:"description"`
	IsEnabled     bool              `json:"is_enabled"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Metadata      map[string]string `json:"metadata"`
}

// IPTablesConfigsResponse 获取 iptables 配置的响应
type IPTablesConfigsResponse struct {
	Configs    []IPTablesConfig `json:"configs"`
	TotalCount int32            `json:"total_count"`
}

// ConvertRouteFromProto 从proto转换路由配置
func ConvertRouteFromProto(route *model.Route) RouteConfig {
	config := RouteConfig{
		ID:         route.Id,
		RouteID:    route.RouteId,
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
	if route.Protocol != 0 {
		switch route.Protocol {
		case 1:
			config.Protocol = "shadowsocks"
		case 2:
			config.Protocol = "snell"
		default:
			config.Protocol = "unknown"
		}
	}

	// 转换协议参数
	if route.ProtocolParams != nil {
		config.ProtocolParams = make(map[string]interface{})
		// 这里需要根据具体的proto结构来解析参数
		// 暂时留空，后续根据实际需要填充
	}

	// 转换类型
	if route.Type != 0 {
		switch route.Type {
		case 1:
			config.Type = "custom"
		case 2:
			config.Type = "system"
		default:
			config.Type = "unknown"
		}
	}

	// 转换状态
	if route.Status != 0 {
		switch route.Status {
		case 1:
			config.Status = "active"
		case 2:
			config.Status = "inactive"
		case 3:
			config.Status = "error"
		default:
			config.Status = "unknown"
		}
	}

	// 转换时间
	if route.CreatedAt != nil {
		config.CreatedAt = route.CreatedAt.AsTime()
	}
	if route.UpdatedAt != nil {
		config.UpdatedAt = route.UpdatedAt.AsTime()
	}

	return config
}

// ConvertEgressFromProto 从proto转换出口配置
func ConvertEgressFromProto(egress *model.EgressItem) EgressConfig {
	config := EgressConfig{
		ID:           egress.Id,
		EgressID:     egress.EgressId,
		ServerID:     egress.ServerId,
		EgressConfig: egress.EgressConfig,
	}

	// 转换出口模式
	if egress.EgressMode != 0 {
		switch egress.EgressMode {
		case 1:
			config.EgressMode = "direct"
		case 2:
			config.EgressMode = "iptables"
		case 3:
			config.EgressMode = "ss2022"
		default:
			config.EgressMode = "unknown"
		}
	}

	// 设置可选字段
	if egress.TargetAddress != nil {
		config.TargetAddress = *egress.TargetAddress
	}

	if egress.ForwardType != nil {
		switch *egress.ForwardType {
		case 1:
			config.ForwardType = "tcp"
		case 2:
			config.ForwardType = "udp"
		case 3:
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

// ConvertIPTablesConfigToRule 将IPTablesConfig转换为IPTableRule
func ConvertIPTablesConfigToRule(config IPTablesConfig) IPTableRule {
	rule := IPTableRule{
		ID:      config.ID,
		Enabled: config.IsEnabled,
		Action:  "add", // 默认动作
	}

	// 映射表类型
	switch config.TableType {
	case "FILTER":
		rule.Table = "filter"
	case "NAT":
		rule.Table = "nat"
	case "MANGLE":
		rule.Table = "mangle"
	case "RAW":
		rule.Table = "raw"
	default:
		rule.Table = "filter" // 默认
	}

	// 映射链类型
	switch config.ChainType {
	case "INPUT":
		rule.Chain = "INPUT"
	case "OUTPUT":
		rule.Chain = "OUTPUT"
	case "FORWARD":
		rule.Chain = "FORWARD"
	case "PREROUTING":
		rule.Chain = "PREROUTING"
	case "POSTROUTING":
		rule.Chain = "POSTROUTING"
	default:
		rule.Chain = "INPUT" // 默认
	}

	// 如果有预制的规则，直接使用
	if config.Rule != "" {
		rule.Rule = config.Rule
	} else {
		// 否则根据配置参数构建规则
		rule.Rule = buildIPTableRule(config)
	}

	return rule
}

// buildIPTableRule 根据配置构建iptables规则
func buildIPTableRule(config IPTablesConfig) string {
	var parts []string

	// 添加协议
	if config.Protocol != "" && config.Protocol != "ALL" {
		parts = append(parts, fmt.Sprintf("-p %s", strings.ToLower(config.Protocol)))
	}

	// 添加源地址
	if config.SourceIP != "" {
		parts = append(parts, fmt.Sprintf("-s %s", config.SourceIP))
	}

	// 添加源端口
	if config.SourcePort != "" {
		parts = append(parts, fmt.Sprintf("--sport %s", config.SourcePort))
	}

	// 添加目标地址
	if config.DestIP != "" {
		parts = append(parts, fmt.Sprintf("-d %s", config.DestIP))
	}

	// 添加目标端口
	if config.DestPort != "" {
		parts = append(parts, fmt.Sprintf("--dport %s", config.DestPort))
	}

	// 添加动作
	if config.JumpTarget != "" {
		switch config.Action {
		case "ACCEPT":
			parts = append(parts, "-j ACCEPT")
		case "DROP":
			parts = append(parts, "-j DROP")
		case "REJECT":
			parts = append(parts, "-j REJECT")
		case "DNAT":
			parts = append(parts, fmt.Sprintf("-j DNAT --to-destination %s", config.JumpTarget))
		case "SNAT":
			parts = append(parts, fmt.Sprintf("-j SNAT --to-source %s", config.JumpTarget))
		case "MASQUERADE":
			parts = append(parts, "-j MASQUERADE")
		default:
			parts = append(parts, fmt.Sprintf("-j %s", config.Action))
		}
	}

	return strings.Join(parts, " ")
}
