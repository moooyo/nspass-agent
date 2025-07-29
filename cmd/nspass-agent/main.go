package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nspass/nspass-agent/pkg/agent"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/logging"

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
		Long:    "NSPass代理服务管理Agent,负责管理各种代理软件和网络配置",
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
	basicConfig := logging.DefaultConfig()
	basicConfig.Level = logLevel
	basicConfig.Output = "stdout"
	if err := logging.Initialize(basicConfig); err != nil {
		logrus.Fatal("初始化基础日志失败: ", err)
	}

	systemLogger := logging.GetSystemLogger()
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
		logging.LogError(err, "加载配置文件失败", logrus.Fields{
			"config_path": configPath,
		})
		os.Exit(1)
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		logging.LogError(err, "配置验证失败", logrus.Fields{
			"config_path": configPath,
		})
		os.Exit(1)
	}

	// 根据配置重新初始化日志系统
	if err := logging.Initialize(cfg.Logger); err != nil {
		systemLogger.WithError(err).Warn("根据配置重新初始化日志失败，继续使用基础配置")
	} else {
		systemLogger.WithField("config", cfg.Logger).Info("日志系统已根据配置重新初始化")
	}

	// 记录启动信息
	logging.LogStartup("nspass-agent", Version, map[string]interface{}{
		"server_id":       cfg.ServerID,
		"config_path":     configPath,
		"api_url":         cfg.API.BaseURL,
		"update_interval": cfg.UpdateInterval,
		"iptables":        cfg.IPTables.Enable,
		"proxies":         cfg.Proxy.EnabledTypes,
	})

	// 创建增强Agent服务
	systemLogger.Info("初始化增强Agent服务...")
	agentService, err := agent.NewEnhancedService(cfg, cfg.ServerID)
	if err != nil {
		logging.LogError(err, "创建增强Agent服务失败", logrus.Fields{
			"server_id": cfg.ServerID,
		})
		os.Exit(1)
	}

	// 启动增强Agent服务
	systemLogger.Info("启动增强Agent服务...")
	if err := agentService.Start(); err != nil {
		logging.LogError(err, "启动增强Agent服务失败", logrus.Fields{
			"server_id": cfg.ServerID,
		})
		os.Exit(1)
	}

	systemLogger.WithFields(logrus.Fields{
		"server_id":        cfg.ServerID,
		"startup_duration": time.Since(startTime).Milliseconds(),
	}).Info("增强NSPass Agent 启动完成")

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	receivedSignal := <-sigChan

	shutdownStart := time.Now()
	systemLogger.WithField("signal", receivedSignal).Info("增强NSPass Agent 正在关闭...")

	// 停止增强Agent服务
	if err := agentService.Stop(); err != nil {
		logging.LogError(err, "停止增强Agent服务失败", nil)
	}

	shutdownDuration := time.Since(shutdownStart)
	totalDuration := time.Since(startTime)

	logging.LogShutdown("nspass-agent", shutdownDuration)
	systemLogger.WithFields(logrus.Fields{
		"shutdown_duration": shutdownDuration.Milliseconds(),
		"total_duration":    totalDuration.Milliseconds(),
	}).Info("增强NSPass Agent 已安全关闭")
}
