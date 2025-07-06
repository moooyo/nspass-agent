package main

import (
	"fmt"
	"log"
	"time"

	"github.com/nspass/nspass-agent/pkg/api"
	"github.com/nspass/nspass-agent/pkg/config"
)

// 这是一个演示如何使用新的 iptables 配置获取功能的示例
func main() {
	// 创建API配置
	apiConfig := config.APIConfig{
		BaseURL:    "https://api.example.com", // 替换为实际的API地址
		Token:      "your-api-token",          // 替换为实际的API Token
		Timeout:    30,
		RetryCount: 3,
		RetryDelay: 5,
	}

	// 创建API客户端
	client := api.NewClient(apiConfig, "example-server-id")

	// 获取服务器的 iptables 配置
	fmt.Println("正在获取服务器 iptables 配置...")

	iptablesConfigs, err := client.GetServerIPTablesConfigs("example-server-id")
	if err != nil {
		log.Printf("获取 iptables 配置失败: %v", err)
		return
	}

	fmt.Printf("成功获取 %d 个 iptables 配置\n", len(iptablesConfigs.Configs))

	// 显示配置详情
	for i, config := range iptablesConfigs.Configs {
		fmt.Printf("\n配置 %d:\n", i+1)
		fmt.Printf("  ID: %s\n", config.ID)
		fmt.Printf("  名称: %s\n", config.ConfigName)
		fmt.Printf("  表类型: %s\n", config.TableType)
		fmt.Printf("  链类型: %s\n", config.ChainType)
		fmt.Printf("  协议: %s\n", config.Protocol)
		fmt.Printf("  源IP: %s\n", config.SourceIP)
		fmt.Printf("  源端口: %s\n", config.SourcePort)
		fmt.Printf("  目标IP: %s\n", config.DestIP)
		fmt.Printf("  目标端口: %s\n", config.DestPort)
		fmt.Printf("  动作: %s\n", config.Action)
		fmt.Printf("  跳转目标: %s\n", config.JumpTarget)
		fmt.Printf("  规则: %s\n", config.Rule)
		fmt.Printf("  是否启用: %t\n", config.IsEnabled)
		fmt.Printf("  创建时间: %s\n", config.CreatedAt.Format(time.RFC3339))
		fmt.Printf("  更新时间: %s\n", config.UpdatedAt.Format(time.RFC3339))

		// 转换为 IPTableRule
		rule := api.ConvertIPTablesConfigToRule(config)
		fmt.Printf("  转换后的规则:\n")
		fmt.Printf("    表: %s\n", rule.Table)
		fmt.Printf("    链: %s\n", rule.Chain)
		fmt.Printf("    规则内容: %s\n", rule.Rule)
		fmt.Printf("    动作: %s\n", rule.Action)
		fmt.Printf("    是否启用: %t\n", rule.Enabled)
	}

	fmt.Printf("\n总配置数: %d\n", iptablesConfigs.TotalCount)
}
