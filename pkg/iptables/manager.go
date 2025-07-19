package iptables

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nspass/nspass-agent/generated/model"
	"github.com/nspass/nspass-agent/pkg/api"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/sirupsen/logrus"
)

// Rule 表示一条iptables规则
type Rule struct {
	ID      string `json:"id"`
	Table   string `json:"table"`  // filter, nat, mangle, raw
	Chain   string `json:"chain"`  // INPUT, OUTPUT, FORWARD, PREROUTING, POSTROUTING等
	Rule    string `json:"rule"`   // 完整的规则内容
	Action  string `json:"action"` // add, insert, delete
	Enabled bool   `json:"enabled"`
}

// RuleSet 规则集合，用于配置对比
type RuleSet map[string]*Rule // key: 规则的唯一标识符

// ManagerInterface 定义iptables管理器接口
type ManagerInterface interface {
	UpdateRulesFromProto(configs []*model.IptablesConfig) error
	GetRulesSummary() map[string]interface{}
}

// Manager 基于iptables-save/restore的管理器
type Manager struct {
	config          config.IPTablesConfig
	mu              sync.RWMutex
	rulesFilePath   string
	backupDir       string
	templateManager *TemplateManager

	// 当前管理的规则状态
	managedRules map[string]*Rule // rule ID -> rule
	lastUpdate   time.Time
}

// NewManager 创建新的iptables管理器
func NewManager(cfg config.IPTablesConfig) ManagerInterface {
	rulesDir := "/etc/nspass/iptables"
	if cfg.BackupPath != "" {
		rulesDir = cfg.BackupPath
	}

	// 初始化模板管理器
	templateManager, err := NewTemplateManager()
	if err != nil {
		logger.LogError(err, "初始化模板管理器失败", nil)
		// 可以继续使用，但会回退到原来的字符串拼接方式
		templateManager = nil
	}

	manager := &Manager{
		config:          cfg,
		rulesFilePath:   filepath.Join(rulesDir, "rules.v4"),
		backupDir:       filepath.Join(rulesDir, "backup"),
		templateManager: templateManager,
		managedRules:    make(map[string]*Rule),
	}

	// 确保目录存在
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		logger.LogError(err, "创建规则目录失败", logrus.Fields{
			"rules_dir": rulesDir,
		})
	}
	if err := os.MkdirAll(manager.backupDir, 0755); err != nil {
		logger.LogError(err, "创建备份目录失败", logrus.Fields{
			"backup_dir": manager.backupDir,
		})
	}

	logger.LogStartup("iptables-manager", "1.0", map[string]interface{}{
		"enabled":      cfg.Enable,
		"chain_prefix": cfg.ChainPrefix,
		"rules_file":   manager.rulesFilePath,
		"backup_dir":   manager.backupDir,
	})

	return manager
}

// UpdateRulesFromProto 使用proto配置更新iptables规则
func (m *Manager) UpdateRulesFromProto(configs []*model.IptablesConfig) error {
	if !m.config.Enable {
		logger.GetIPTablesLogger().Info("iptables管理已禁用，跳过规则更新")
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	startTime := time.Now()
	log := logger.GetIPTablesLogger()

	log.WithField("config_count", len(configs)).Info("使用proto配置更新iptables规则")

	// 1. 备份当前规则
	if err := m.backupCurrentRules(); err != nil {
		logger.LogError(err, "备份当前规则失败", nil)
	}

	// 2. 转换proto配置为内部规则格式
	newRules := make(map[string]*Rule)
	enabledCount := 0
	for _, config := range configs {
		if !config.IsEnabled {
			log.WithField("config_id", config.Id).Debug("跳过已禁用的iptables配置")
			continue
		}

		// 转换proto配置为规则参数
		table, chain, ruleText := api.ConvertProtoIptablesConfigToRuleParts(config)

		rule := &Rule{
			ID:      fmt.Sprintf("%d", config.Id),
			Table:   table,
			Chain:   chain,
			Rule:    ruleText,
			Action:  "add",
			Enabled: config.IsEnabled,
		}

		newRules[rule.ID] = rule
		enabledCount++

		log.WithFields(logrus.Fields{
			"config_id": config.Id,
			"server_id": config.ServerId,
			"table":     rule.Table,
			"chain":     rule.Chain,
			"rule":      rule.Rule,
		}).Debug("转换proto iptables配置为规则")
	}

	log.WithFields(logrus.Fields{
		"total_configs":   len(configs),
		"enabled_configs": enabledCount,
	}).Info("配置转换完成")

	// 3. 获取当前完整的iptables规则
	currentRulesContent, err := m.getCurrentRulesContent()
	if err != nil {
		return fmt.Errorf("获取当前规则失败: %w", err)
	}

	// 4. 生成新的规则文件内容
	newRulesContent, err := m.generateRulesContent(currentRulesContent, newRules)
	if err != nil {
		return fmt.Errorf("生成新规则内容失败: %w", err)
	}

	// 5. 应用新规则
	if err := m.applyRules(newRulesContent); err != nil {
		// 应用失败，尝试恢复
		logger.LogError(err, "应用新规则失败，尝试恢复", nil)
		if restoreErr := m.restoreFromBackup(); restoreErr != nil {
			logger.LogError(restoreErr, "恢复规则失败", nil)
		}
		return fmt.Errorf("应用规则失败: %w", err)
	}

	// 6. 保存规则文件
	if err := m.saveRulesFile(newRulesContent); err != nil {
		logger.LogError(err, "保存规则文件失败", nil)
	}

	// 7. 更新内存状态
	oldRulesCount := len(m.managedRules)
	m.managedRules = newRules
	m.lastUpdate = time.Now()

	duration := time.Since(startTime)

	// 记录性能指标
	logger.LogPerformance("iptables_rules_update_from_proto", duration, logrus.Fields{
		"configs_processed": len(configs),
		"configs_enabled":   enabledCount,
		"old_rules":         oldRulesCount,
		"new_rules":         len(newRules),
	})

	log.WithFields(logrus.Fields{
		"managed_rules": len(m.managedRules),
		"last_update":   m.lastUpdate,
		"duration_ms":   duration.Milliseconds(),
	}).Info("iptables规则更新完成")

	return nil
}

// getCurrentRulesContent 获取当前的iptables规则内容
func (m *Manager) getCurrentRulesContent() (string, error) {
	log := logger.GetIPTablesLogger()
	log.Debug("获取当前系统iptables规则")

	cmd := exec.Command("iptables-save")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("执行iptables-save失败: %w", err)
	}

	content := string(output)
	log.WithField("rules_size", len(content)).Debug("当前规则获取完成")
	return content, nil
}

// generateRulesContent 生成新的规则文件内容
func (m *Manager) generateRulesContent(currentContent string, newRules map[string]*Rule) (string, error) {
	log := logger.GetIPTablesLogger()
	log.WithField("new_rules_count", len(newRules)).Info("开始生成新的规则文件内容")

	// 解析当前规则
	tables, err := m.parseIPTablesContent(currentContent)
	if err != nil {
		return "", fmt.Errorf("解析当前规则失败: %w", err)
	}

	// 移除旧的管理规则
	removedCount := m.removeOldManagedRules(tables)

	// 添加新的管理规则
	addedCount := m.addNewManagedRules(tables, newRules)

	// 生成新的规则内容
	newContent, err := m.generateIPTablesContent(tables)
	if err != nil {
		return "", fmt.Errorf("生成规则内容失败: %w", err)
	}

	log.WithFields(logrus.Fields{
		"new_content_size": len(newContent),
		"rules_removed":    removedCount,
		"rules_added":      addedCount,
	}).Info("新规则文件内容生成完成")

	return newContent, nil
}

// parseIPTablesContent 解析iptables-save格式的内容
func (m *Manager) parseIPTablesContent(content string) (map[string]*IPTablesTable, error) {
	tables := make(map[string]*IPTablesTable)

	scanner := bufio.NewScanner(strings.NewReader(content))
	var currentTable *IPTablesTable
	var currentTableName string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过注释和空行
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 表头
		if strings.HasPrefix(line, "*") {
			currentTableName = strings.TrimPrefix(line, "*")
			currentTable = &IPTablesTable{
				Name:   currentTableName,
				Chains: make(map[string]*IPTablesChain),
				Rules:  []string{},
			}
			tables[currentTableName] = currentTable
			continue
		}

		// 表结束
		if line == "COMMIT" {
			currentTable = nil
			currentTableName = ""
			continue
		}

		if currentTable == nil {
			continue
		}

		// 链定义
		if strings.HasPrefix(line, ":") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				chainName := strings.TrimPrefix(parts[0], ":")
				policy := parts[1]
				counters := strings.Join(parts[2:], " ")

				currentTable.Chains[chainName] = &IPTablesChain{
					Name:     chainName,
					Policy:   policy,
					Counters: counters,
				}
			}
			continue
		}

		// 规则行
		if strings.HasPrefix(line, "-A ") || strings.HasPrefix(line, "-I ") {
			currentTable.Rules = append(currentTable.Rules, line)
		}
	}

	return tables, scanner.Err()
}

// removeOldManagedRules 移除旧的管理规则
func (m *Manager) removeOldManagedRules(tables map[string]*IPTablesTable) int {
	log := logger.GetIPTablesLogger()
	removedCount := 0
	removedChains := 0

	for _, table := range tables {
		var newRules []string
		for _, rule := range table.Rules {
			// 检查是否是我们管理的规则
			if !m.isManagedRule(rule) {
				newRules = append(newRules, rule)
			} else {
				removedCount++
				log.WithFields(logrus.Fields{
					"table": table.Name,
					"rule":  rule,
				}).Debug("移除旧的管理规则")
			}
		}
		table.Rules = newRules

		// 移除我们管理的自定义链
		for chainName := range table.Chains {
			if strings.HasPrefix(chainName, m.config.ChainPrefix) {
				delete(table.Chains, chainName)
				removedChains++
				log.WithFields(logrus.Fields{
					"table": table.Name,
					"chain": chainName,
				}).Debug("移除管理的自定义链")
			}
		}
	}

	log.WithFields(logrus.Fields{
		"removed_rules":  removedCount,
		"removed_chains": removedChains,
	}).Info("旧的管理规则移除完成")

	return removedCount
}

// addNewManagedRules 添加新的管理规则
func (m *Manager) addNewManagedRules(tables map[string]*IPTablesTable, newRules map[string]*Rule) int {
	log := logger.GetIPTablesLogger()
	addedCount := 0
	addedChains := 0

	// 按表分组规则
	rulesByTable := make(map[string][]*Rule)
	for _, rule := range newRules {
		rulesByTable[rule.Table] = append(rulesByTable[rule.Table], rule)
	}

	// 为每个表添加规则
	for tableName, rules := range rulesByTable {
		table := tables[tableName]
		if table == nil {
			// 创建新表
			table = &IPTablesTable{
				Name:   tableName,
				Chains: make(map[string]*IPTablesChain),
				Rules:  []string{},
			}
			tables[tableName] = table
			log.WithField("table", tableName).Debug("创建新表")
		}

		// 添加自定义链
		customChains := make(map[string]bool)
		for _, rule := range rules {
			if strings.HasPrefix(rule.Chain, m.config.ChainPrefix) {
				customChains[rule.Chain] = true
			}
		}

		for chainName := range customChains {
			if _, exists := table.Chains[chainName]; !exists {
				table.Chains[chainName] = &IPTablesChain{
					Name:     chainName,
					Policy:   "-",
					Counters: "[0:0]",
				}
				addedChains++
				log.WithFields(logrus.Fields{
					"table": tableName,
					"chain": chainName,
				}).Debug("创建新的自定义链")
			}
		}

		// 添加规则
		for _, rule := range rules {
			var ruleStr string
			var err error

			// 使用模板生成规则字符串
			if m.templateManager != nil {
				ruleStr, err = m.templateManager.GenerateRule(rule)
				if err != nil {
					log.WithError(err).WithField("rule_id", rule.ID).Warn("使用模板生成规则失败，回退到字符串拼接")
					ruleStr = ""
				}
			}

			// 如果模板生成失败或没有模板管理器，使用原始方式
			if ruleStr == "" {
				if rule.Action == "insert" {
					ruleStr = fmt.Sprintf("-I %s %s", rule.Chain, rule.Rule)
				} else {
					ruleStr = fmt.Sprintf("-A %s %s", rule.Chain, rule.Rule)
				}
				ruleStr += fmt.Sprintf(" -m comment --comment \"NSPass:%s\"", rule.ID)
			}

			table.Rules = append(table.Rules, ruleStr)
			addedCount++

			log.WithFields(logrus.Fields{
				"rule_id": rule.ID,
				"table":   tableName,
				"chain":   rule.Chain,
				"rule":    ruleStr,
			}).Debug("添加新的管理规则")
		}
	}

	log.WithFields(logrus.Fields{
		"added_rules":  addedCount,
		"added_chains": addedChains,
	}).Info("新的管理规则添加完成")

	return addedCount
}

// generateIPTablesContent 生成iptables-save格式的内容
func (m *Manager) generateIPTablesContent(tables map[string]*IPTablesTable) (string, error) {
	// 如果有模板管理器，使用模板生成
	if m.templateManager != nil {
		return m.templateManager.GenerateIPTablesContent(tables, m.config.ChainPrefix)
	}

	// 回退到原始的字符串拼接方式
	var content strings.Builder

	// 按固定顺序输出表
	tableOrder := []string{"raw", "mangle", "nat", "filter"}

	for _, tableName := range tableOrder {
		table, exists := tables[tableName]
		if !exists {
			continue
		}

		// 表头
		content.WriteString(fmt.Sprintf("*%s\n", table.Name))

		// 链定义 - 按名称排序
		chainNames := make([]string, 0, len(table.Chains))
		for chainName := range table.Chains {
			chainNames = append(chainNames, chainName)
		}
		sort.Strings(chainNames)

		for _, chainName := range chainNames {
			chain := table.Chains[chainName]
			content.WriteString(fmt.Sprintf(":%s %s %s\n", chain.Name, chain.Policy, chain.Counters))
		}

		// 规则
		for _, rule := range table.Rules {
			content.WriteString(rule + "\n")
		}

		// 表结束
		content.WriteString("COMMIT\n")
	}

	return content.String(), nil
}

// applyRules 应用新规则
func (m *Manager) applyRules(content string) error {
	log := logger.GetIPTablesLogger()
	log.Info("开始应用新的iptables规则")

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "nspass-iptables-*.rules")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// 写入规则内容
	if _, err := tmpFile.WriteString(content); err != nil {
		return fmt.Errorf("写入临时规则文件失败: %w", err)
	}
	tmpFile.Close()

	// 应用规则
	cmd := exec.Command("iptables-restore", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables-restore失败: %w, 输出: %s", err, string(output))
	}

	log.WithField("output", string(output)).Info("iptables规则应用成功")
	return nil
}

// backupCurrentRules 备份当前规则
func (m *Manager) backupCurrentRules() error {
	timestamp := time.Now().Format("20060102_150405")
	backupFile := filepath.Join(m.backupDir, fmt.Sprintf("iptables_backup_%s.rules", timestamp))

	cmd := exec.Command("iptables-save")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("备份当前规则失败: %w", err)
	}

	if err := os.WriteFile(backupFile, output, 0644); err != nil {
		return fmt.Errorf("写入备份文件失败: %w", err)
	}

	logger.GetIPTablesLogger().WithField("backup_file", backupFile).Info("当前规则备份完成")
	return nil
}

// saveRulesFile 保存规则文件
func (m *Manager) saveRulesFile(content string) error {
	if err := os.WriteFile(m.rulesFilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("保存规则文件失败: %w", err)
	}

	logger.GetIPTablesLogger().WithField("rules_file", m.rulesFilePath).Info("规则文件保存完成")
	return nil
}

// restoreFromBackup 从备份恢复
func (m *Manager) restoreFromBackup() error {
	// 找到最新的备份文件
	files, err := os.ReadDir(m.backupDir)
	if err != nil {
		return fmt.Errorf("读取备份目录失败: %w", err)
	}

	var latestBackup string
	var latestTime time.Time

	for _, file := range files {
		if strings.HasPrefix(file.Name(), "iptables_backup_") && strings.HasSuffix(file.Name(), ".rules") {
			if info, err := file.Info(); err == nil {
				if info.ModTime().After(latestTime) {
					latestTime = info.ModTime()
					latestBackup = filepath.Join(m.backupDir, file.Name())
				}
			}
		}
	}

	if latestBackup == "" {
		return fmt.Errorf("未找到备份文件")
	}

	cmd := exec.Command("iptables-restore", latestBackup)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("恢复备份失败: %w, 输出: %s", err, string(output))
	}

	logger.GetIPTablesLogger().WithField("backup_file", latestBackup).Info("从备份恢复成功")
	return nil
}

// isManagedRule 检查是否是管理的规则
func (m *Manager) isManagedRule(rule string) bool {
	return strings.Contains(rule, "NSPass:") || strings.Contains(rule, m.config.ChainPrefix)
}

// GetRulesSummary 获取规则摘要
func (m *Manager) GetRulesSummary() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	summary := map[string]interface{}{
		"managed_rules_count": len(m.managedRules),
		"enabled":             m.config.Enable,
		"chain_prefix":        m.config.ChainPrefix,
		"rules_file":          m.rulesFilePath,
		"backup_dir":          m.backupDir,
		"last_update":         m.lastUpdate.Format(time.RFC3339),
	}

	// 按表统计规则
	tableStats := make(map[string]int)
	for _, rule := range m.managedRules {
		tableStats[rule.Table]++
	}
	summary["rules_by_table"] = tableStats

	return summary
}

// IPTablesTable 表示一个iptables表
type IPTablesTable struct {
	Name   string
	Chains map[string]*IPTablesChain
	Rules  []string
}

// IPTablesChain 表示一个iptables链
type IPTablesChain struct {
	Name     string
	Policy   string
	Counters string
}
