package iptables

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestTemplateManager(t *testing.T) {
	tm, err := NewTemplateManager()
	if err != nil {
		t.Fatalf("创建模板管理器失败: %v", err)
	}

	t.Run("规则模板", func(t *testing.T) {
		rule := &Rule{
			ID:     "test-rule-001",
			Action: "add",
			Chain:  "INPUT",
			Rule:   "-p tcp --dport 80 -j ACCEPT",
		}

		result, err := tm.GenerateRule(rule)
		if err != nil {
			t.Fatalf("生成规则失败: %v", err)
		}

		expected := "-A INPUT -p tcp --dport 80 -j ACCEPT -m comment --comment \"NSPass:test-rule-001\""
		if result != expected {
			t.Errorf("规则生成结果不符合预期\n期望: %s\n实际: %s", expected, result)
		}
	})

	t.Run("插入规则模板", func(t *testing.T) {
		rule := &Rule{
			ID:     "test-rule-002",
			Action: "insert",
			Chain:  "OUTPUT",
			Rule:   "-p tcp --dport 443 -j ACCEPT",
		}

		result, err := tm.GenerateRule(rule)
		if err != nil {
			t.Fatalf("生成规则失败: %v", err)
		}

		expected := "-I OUTPUT -p tcp --dport 443 -j ACCEPT -m comment --comment \"NSPass:test-rule-002\""
		if result != expected {
			t.Errorf("规则生成结果不符合预期\n期望: %s\n实际: %s", expected, result)
		}
	})

	t.Run("链模板", func(t *testing.T) {
		chain := &IPTablesChain{
			Name:     "NSPASS_CUSTOM",
			Policy:   "-",
			Counters: "[0:0]",
		}

		result, err := tm.GenerateChain(chain)
		if err != nil {
			t.Fatalf("生成链定义失败: %v", err)
		}

		expected := ":NSPASS_CUSTOM - [0:0]"
		if result != expected {
			t.Errorf("链定义生成结果不符合预期\n期望: %s\n实际: %s", expected, result)
		}
	})

	t.Run("完整iptables文件模板", func(t *testing.T) {
		tables := createTestTables()

		result, err := tm.GenerateIPTablesContent(tables, "NSPASS_")
		if err != nil {
			t.Fatalf("生成iptables文件内容失败: %v", err)
		}

		// 验证关键内容
		expectedContents := []string{
			"*nat", "*filter", ":INPUT ACCEPT [100:12345]",
			":NSPASS_CUSTOM - [0:0]", "COMMIT", "NSPass:web-rule",
		}

		for _, expected := range expectedContents {
			if !strings.Contains(result, expected) {
				t.Errorf("生成的内容应该包含: %s", expected)
			}
		}

		// 验证表的顺序
		natIndex := strings.Index(result, "*nat")
		filterIndex := strings.Index(result, "*filter")
		if natIndex > filterIndex {
			t.Error("nat表应该在filter表之前")
		}
	})
}

func TestTemplatePerformance(t *testing.T) {
	tm, err := NewTemplateManager()
	if err != nil {
		t.Fatalf("创建模板管理器失败: %v", err)
	}

	// 性能测试：生成1000条规则
	rules := make([]*Rule, 1000)
	for i := 0; i < 1000; i++ {
		rules[i] = &Rule{
			ID:     fmt.Sprintf("rule-%d", i),
			Action: "add",
			Chain:  "INPUT",
			Rule:   fmt.Sprintf("-p tcp --dport %d -j ACCEPT", 8000+i),
		}
	}

	start := time.Now()
	for _, rule := range rules {
		_, err := tm.GenerateRule(rule)
		if err != nil {
			t.Fatalf("生成规则失败: %v", err)
		}
	}
	duration := time.Since(start)

	// 性能应该在合理范围内（每条规则小于1ms）
	if duration > time.Second {
		t.Errorf("性能过慢，1000条规则生成耗时超过1秒: %v", duration)
	}
}

func TestTemplateErrorHandling(t *testing.T) {
	tm, err := NewTemplateManager()
	if err != nil {
		t.Fatalf("创建模板管理器失败: %v", err)
	}

	t.Run("空规则处理", func(t *testing.T) {
		rule := &Rule{
			ID:     "",
			Action: "",
			Chain:  "",
			Rule:   "",
		}

		result, err := tm.GenerateRule(rule)
		if err != nil {
			t.Fatalf("处理空规则失败: %v", err)
		}

		if !strings.Contains(result, "-A") {
			t.Error("即使是空规则，也应该包含基本的-A格式")
		}
	})

	t.Run("特殊字符处理", func(t *testing.T) {
		rule := &Rule{
			ID:     "rule-with-special-chars-!@#$%",
			Action: "add",
			Chain:  "INPUT",
			Rule:   "-p tcp --dport 80 -m string --string \"test string\" -j ACCEPT",
		}

		result, err := tm.GenerateRule(rule)
		if err != nil {
			t.Fatalf("处理特殊字符规则失败: %v", err)
		}

		if !strings.Contains(result, "rule-with-special-chars-!@#$%") {
			t.Error("应该正确处理特殊字符")
		}
	})
}

// createTestTables 创建测试用的表结构
func createTestTables() map[string]*IPTablesTable {
	return map[string]*IPTablesTable{
		"filter": {
			Name: "filter",
			Chains: map[string]*IPTablesChain{
				"INPUT": {
					Name:     "INPUT",
					Policy:   "ACCEPT",
					Counters: "[100:12345]",
				},
				"NSPASS_CUSTOM": {
					Name:     "NSPASS_CUSTOM",
					Policy:   "-",
					Counters: "[0:0]",
				},
			},
			Rules: []string{
				"-A INPUT -p tcp --dport 22 -j ACCEPT",
				"-A INPUT -j NSPASS_CUSTOM",
				"-A NSPASS_CUSTOM -p tcp --dport 80 -j ACCEPT -m comment --comment \"NSPass:web-rule\"",
			},
		},
		"nat": {
			Name: "nat",
			Chains: map[string]*IPTablesChain{
				"PREROUTING": {
					Name:     "PREROUTING",
					Policy:   "ACCEPT",
					Counters: "[50:6789]",
				},
			},
			Rules: []string{
				"-A PREROUTING -p tcp --dport 8080 -j REDIRECT --to-port 80",
			},
		},
	}
}
