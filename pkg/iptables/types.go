package iptables

// RuleComparison 规则比较结果
type RuleComparison struct {
	ToAdd     []*Rule `json:"to_add"`
	ToDelete  []*Rule `json:"to_delete"`
	Unchanged []*Rule `json:"unchanged"`
}

// ManagerStats 管理器统计信息
type ManagerStats struct {
	DesiredRulesCount int            `json:"desired_rules_count"`
	CurrentRulesCount int            `json:"current_rules_count"`
	ManagedChains     int            `json:"managed_chains"`
	Enabled           bool           `json:"enabled"`
	ChainPrefix       string         `json:"chain_prefix"`
	RulesByTable      map[string]int `json:"rules_by_table"`
	LastUpdate        string         `json:"last_update"`
}

// RuleOperation 规则操作类型
type RuleOperation string

const (
	OperationAdd    RuleOperation = "add"
	OperationDelete RuleOperation = "delete"
	OperationInsert RuleOperation = "insert"
)

// TableType iptables表类型
type TableType string

const (
	TableFilter TableType = "filter"
	TableNAT    TableType = "nat"
	TableMangle TableType = "mangle"
	TableRaw    TableType = "raw"
)

// ChainType iptables链类型
type ChainType string

const (
	ChainInput       ChainType = "INPUT"
	ChainOutput      ChainType = "OUTPUT"
	ChainForward     ChainType = "FORWARD"
	ChainPrerouting  ChainType = "PREROUTING"
	ChainPostrouting ChainType = "POSTROUTING"
)
