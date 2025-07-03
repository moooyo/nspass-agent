package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nspass/nspass-agent/pkg/api"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/iptables"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/nspass/nspass-agent/pkg/proxy"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	configPath = "/etc/nspass/config.yaml"
	logLevel   = "info"

	// 构建时注入的版本信息
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "nspass-agent",
		Short:   "NSPass代理服务管理Agent",
		Long:    "NSPass代理服务管理Agent，负责管理各种代理软件和网络配置",
		Version: getVersionInfo(),
		Run:     runAgent,
	}

	rootCmd.Flags().StringVarP(&configPath, "config", "c", configPath, "配置文件路径")
	rootCmd.Flags().StringVarP(&logLevel, "log-level", "l", logLevel, "日志级别 (debug, info, warn, error)")

	if err := rootCmd.Execute(); err != nil {
		// 在logger初始化之前，使用基础输出
		logrus.Fatal(err)
	}
}

func getVersionInfo() string {
	return "版本: " + Version + "\n提交: " + Commit + "\n构建时间: " + BuildTime
}

func runAgent(cmd *cobra.Command, args []string) {
	startTime := time.Now()

	// 先使用基础日志配置，稍后会被配置文件覆盖
	basicConfig := logger.DefaultConfig()
	basicConfig.Level = logLevel
	basicConfig.Output = "stdout"
	if err := logger.Initialize(basicConfig); err != nil {
		logrus.Fatal("初始化基础日志失败: ", err)
	}

	systemLogger := logger.GetSystemLogger()
	systemLogger.WithFields(logrus.Fields{
		"version":    Version,
		"commit":     Commit,
		"build_time": BuildTime,
		"config":     configPath,
		"log_level":  logLevel,
	}).Info("NSPass Agent 启动中...")

	// 加载配置
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.LogError(err, "加载配置文件失败", logrus.Fields{
			"config_path": configPath,
		})
		os.Exit(1)
	}

	// 根据配置重新初始化日志系统
	if err := logger.Initialize(cfg.Logger); err != nil {
		systemLogger.WithError(err).Warn("根据配置重新初始化日志失败，继续使用基础配置")
	} else {
		systemLogger.WithField("config", cfg.Logger).Info("日志系统已根据配置重新初始化")
	}

	// 记录启动信息
	logger.LogStartup("nspass-agent", Version, map[string]interface{}{
		"config_path": configPath,
		"api_url":     cfg.API.BaseURL,
		"iptables":    cfg.IPTables.Enable,
		"proxies":     cfg.Proxy.EnabledTypes,
	})

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 初始化各个模块
	systemLogger.Info("初始化各个模块...")

	apiClient := api.NewClient(cfg.API)
	logger.GetAPILogger().WithFields(logrus.Fields{
		"base_url":    cfg.API.BaseURL,
		"timeout":     cfg.API.Timeout,
		"retry_count": cfg.API.RetryCount,
		"retry_delay": cfg.API.RetryDelay,
	}).Info("API客户端初始化完成")

	proxyManager := proxy.NewManager(cfg.Proxy)
	logger.GetProxyLogger().WithFields(logrus.Fields{
		"enabled_types": cfg.Proxy.EnabledTypes,
		"auto_start":    cfg.Proxy.AutoStart,
		"bin_path":      cfg.Proxy.BinPath,
		"config_path":   cfg.Proxy.ConfigPath,
	}).Info("代理管理器初始化完成")

	iptablesManager := iptables.NewManager(cfg.IPTables)
	logger.GetIPTablesLogger().WithFields(logrus.Fields{
		"enabled":      cfg.IPTables.Enable,
		"chain_prefix": cfg.IPTables.ChainPrefix,
		"backup_path":  cfg.IPTables.BackupPath,
	}).Info("iptables管理器初始化完成")

	// 启动主服务循环
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.UpdateInterval) * time.Second)
		defer ticker.Stop()

		systemLogger.WithField("interval", cfg.UpdateInterval).Info("启动配置更新循环")

		// 立即执行一次配置更新
		if err := updateConfiguration(apiClient, proxyManager, iptablesManager); err != nil {
			logger.LogError(err, "初始配置更新失败", nil)
		}

		for {
			select {
			case <-ctx.Done():
				systemLogger.Info("收到停止信号，退出配置更新循环")
				return
			case <-ticker.C:
				systemLogger.Debug("开始定时配置更新")
				if err := updateConfiguration(apiClient, proxyManager, iptablesManager); err != nil {
					logger.LogError(err, "定时配置更新失败", nil)
				}
			}
		}
	}()

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	receivedSignal := <-sigChan

	shutdownStart := time.Now()
	systemLogger.WithField("signal", receivedSignal).Info("NSPass Agent 正在关闭...")
	cancel()

	// 等待清理完成
	time.Sleep(2 * time.Second)

	shutdownDuration := time.Since(shutdownStart)
	totalDuration := time.Since(startTime)

	logger.LogShutdown("nspass-agent", shutdownDuration)
	systemLogger.WithFields(logrus.Fields{
		"shutdown_duration": shutdownDuration.Milliseconds(),
		"total_duration":    totalDuration.Milliseconds(),
	}).Info("NSPass Agent 已安全关闭")
}

func updateConfiguration(apiClient *api.Client, proxyManager *proxy.Manager, iptablesManager iptables.ManagerInterface) error {
	startTime := time.Now()
	systemLogger := logger.GetSystemLogger()

	systemLogger.Debug("开始从API获取配置")

	// 从API获取配置
	configuration, err := apiClient.GetConfiguration()
	if err != nil {
		logger.LogError(err, "从API获取配置失败", logrus.Fields{
			"api_url": apiClient.GetBaseURL(),
		})
		return err
	}

	systemLogger.WithFields(logrus.Fields{
		"proxies_count":        len(configuration.Proxies),
		"iptables_rules_count": len(configuration.IPTablesRules),
		"updated_at":           configuration.UpdatedAt,
	}).Info("成功获取API配置")

	// 更新代理配置
	if len(configuration.Proxies) > 0 {
		logger.GetProxyLogger().WithField("count", len(configuration.Proxies)).Info("开始更新代理配置")
		if err := proxyManager.UpdateProxies(configuration.Proxies); err != nil {
			logger.LogError(err, "更新代理配置失败", logrus.Fields{
				"proxies_count": len(configuration.Proxies),
			})
		} else {
			logger.GetProxyLogger().Info("代理配置更新成功")
		}
	} else {
		logger.GetProxyLogger().Debug("没有代理配置需要更新")
	}

	// 更新iptables规则
	if len(configuration.IPTablesRules) > 0 {
		iptablesLogger := logger.GetIPTablesLogger()
		iptablesLogger.WithField("count", len(configuration.IPTablesRules)).Info("开始更新iptables规则")

		updateStart := time.Now()
		if err := iptablesManager.UpdateRules(configuration.IPTablesRules); err != nil {
			logger.LogError(err, "更新iptables规则失败", logrus.Fields{
				"rules_count": len(configuration.IPTablesRules),
			})
		} else {
			updateDuration := time.Since(updateStart)
			iptablesLogger.Info("iptables规则更新成功")

			// 记录性能指标
			logger.LogPerformance("iptables_update", updateDuration, logrus.Fields{
				"rules_count": len(configuration.IPTablesRules),
			})

			// 输出规则摘要
			summary := iptablesManager.GetRulesSummary()
			iptablesLogger.WithFields(logrus.Fields{
				"summary": summary,
			}).Info("iptables规则摘要")
		}
	} else {
		logger.GetIPTablesLogger().Debug("没有iptables规则需要更新")
	}

	duration := time.Since(startTime)

	// 记录配置更新性能
	logger.LogPerformance("configuration_update", duration, logrus.Fields{
		"proxies_count": len(configuration.Proxies),
		"rules_count":   len(configuration.IPTablesRules),
	})

	systemLogger.WithField("duration", duration).Info("配置更新完成")
	return nil
}
