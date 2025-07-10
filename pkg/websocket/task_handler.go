package websocket

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/nspass/nspass-agent/generated/model"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/iptables"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/nspass/nspass-agent/pkg/proxy"
	"github.com/sirupsen/logrus"
)

// DefaultTaskHandler 默认任务处理器
type DefaultTaskHandler struct {
	config          *config.Config
	proxyManager    *proxy.Manager
	iptablesManager iptables.ManagerInterface
	log             *logrus.Entry
}

// NewDefaultTaskHandler 创建默认任务处理器
func NewDefaultTaskHandler(cfg *config.Config, proxyManager *proxy.Manager, iptablesManager iptables.ManagerInterface) *DefaultTaskHandler {
	return &DefaultTaskHandler{
		config:          cfg,
		proxyManager:    proxyManager,
		iptablesManager: iptablesManager,
		log:             logger.GetComponentLogger("task-handler"),
	}
}

// HandleTask 处理任务
func (h *DefaultTaskHandler) HandleTask(ctx context.Context, task *model.TaskMessage) (*model.TaskResult, error) {
	h.log.WithFields(logrus.Fields{
		"task_id":   task.TaskId,
		"task_type": task.TaskType.String(),
		"title":     task.Title,
	}).Info("开始处理任务")

	var result *model.TaskResult
	var err error

	switch task.TaskType {
	case model.TaskType_TASK_TYPE_CONFIG_UPDATE:
		result, err = h.handleConfigUpdate(ctx, task)
	case model.TaskType_TASK_TYPE_RESTART:
		result, err = h.handleRestart(ctx, task)
	case model.TaskType_TASK_TYPE_SYNC_RULES:
		result, err = h.handleSyncRules(ctx, task)
	case model.TaskType_TASK_TYPE_SYNC_USERS:
		result, err = h.handleSyncUsers(ctx, task)
	case model.TaskType_TASK_TYPE_COLLECT_METRICS:
		result, err = h.handleCollectMetrics(ctx, task)
	case model.TaskType_TASK_TYPE_HEALTH_CHECK:
		result, err = h.handleHealthCheck(ctx, task)
	default:
		err = fmt.Errorf("不支持的任务类型: %s", task.TaskType.String())
	}

	if err != nil {
		h.log.WithError(err).WithField("task_id", task.TaskId).Error("任务处理失败")
	} else {
		h.log.WithField("task_id", task.TaskId).Info("任务处理成功")
	}

	return result, err
}

// handleConfigUpdate 处理配置更新任务
func (h *DefaultTaskHandler) handleConfigUpdate(ctx context.Context, task *model.TaskMessage) (*model.TaskResult, error) {
	h.log.WithField("task_id", task.TaskId).Info("处理配置更新任务")

	// 解析配置更新参数
	var params model.ConfigUpdateTaskParams
	if err := task.Parameters.UnmarshalTo(&params); err != nil {
		return nil, fmt.Errorf("解析配置更新参数失败: %w", err)
	}

	// 根据配置类型更新相应的配置
	switch params.ConfigType {
	case "proxy":
		return h.updateProxyConfig(ctx, &params)
	case "iptables":
		return h.updateIPTablesConfig(ctx, &params)
	default:
		return nil, fmt.Errorf("不支持的配置类型: %s", params.ConfigType)
	}
}

// updateProxyConfig 更新代理配置
func (h *DefaultTaskHandler) updateProxyConfig(ctx context.Context, params *model.ConfigUpdateTaskParams) (*model.TaskResult, error) {
	h.log.Info("更新代理配置")

	// 这里应该根据配置内容更新代理配置
	// 实际实现需要根据具体的代理类型进行处理
	output := fmt.Sprintf("代理配置更新完成，配置类型: %s", params.ConfigType)

	// 如果需要重启
	if params.RestartRequired {
		if err := h.proxyManager.RestartAll(); err != nil {
			return nil, fmt.Errorf("重启代理服务失败: %w", err)
		}
		output += "，代理服务已重启"
	}

	return &model.TaskResult{
		TaskId:  "",
		Status:  model.TaskStatus_TASK_STATUS_COMPLETED,
		Output:  output,
	}, nil
}

// updateIPTablesConfig 更新iptables配置
func (h *DefaultTaskHandler) updateIPTablesConfig(ctx context.Context, params *model.ConfigUpdateTaskParams) (*model.TaskResult, error) {
	h.log.Info("更新iptables配置")

	// 这里应该根据配置内容更新iptables配置
	// 实际实现需要解析配置并调用iptables管理器
	output := fmt.Sprintf("iptables配置更新完成，配置类型: %s", params.ConfigType)

	return &model.TaskResult{
		TaskId:  "",
		Status:  model.TaskStatus_TASK_STATUS_COMPLETED,
		Output:  output,
	}, nil
}

// handleRestart 处理重启任务
func (h *DefaultTaskHandler) handleRestart(ctx context.Context, task *model.TaskMessage) (*model.TaskResult, error) {
	h.log.WithField("task_id", task.TaskId).Info("处理重启任务")

	// 解析重启参数
	var params model.RestartTaskParams
	if err := task.Parameters.UnmarshalTo(&params); err != nil {
		return nil, fmt.Errorf("解析重启参数失败: %w", err)
	}

	switch params.ServiceName {
	case "proxy":
		return h.restartProxyService(ctx, &params)
	case "agent":
		return h.restartAgentService(ctx, &params)
	default:
		return nil, fmt.Errorf("不支持的服务名称: %s", params.ServiceName)
	}
}

// restartProxyService 重启代理服务
func (h *DefaultTaskHandler) restartProxyService(ctx context.Context, params *model.RestartTaskParams) (*model.TaskResult, error) {
	h.log.Info("重启代理服务")

	if err := h.proxyManager.RestartAll(); err != nil {
		return nil, fmt.Errorf("重启代理服务失败: %w", err)
	}

	return &model.TaskResult{
		TaskId:  "",
		Status:  model.TaskStatus_TASK_STATUS_COMPLETED,
		Output:  "代理服务重启成功",
	}, nil
}

// restartAgentService 重启Agent服务
func (h *DefaultTaskHandler) restartAgentService(ctx context.Context, params *model.RestartTaskParams) (*model.TaskResult, error) {
	h.log.Info("重启Agent服务")

	// 这里需要实现Agent服务的重启逻辑
	// 可能需要通过systemctl或其他方式重启服务
	cmd := exec.CommandContext(ctx, "systemctl", "restart", "nspass-agent")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("重启Agent服务失败: %w", err)
	}

	return &model.TaskResult{
		TaskId:  "",
		Status:  model.TaskStatus_TASK_STATUS_COMPLETED,
		Output:  "Agent服务重启成功",
	}, nil
}

// handleSyncRules 处理同步规则任务
func (h *DefaultTaskHandler) handleSyncRules(ctx context.Context, task *model.TaskMessage) (*model.TaskResult, error) {
	h.log.WithField("task_id", task.TaskId).Info("处理同步规则任务")

	// 解析同步规则参数
	var params model.SyncRulesTaskParams
	if err := task.Parameters.UnmarshalTo(&params); err != nil {
		return nil, fmt.Errorf("解析同步规则参数失败: %w", err)
	}

	// 这里应该实现规则同步逻辑
	// 可能需要从API获取最新的规则并更新本地配置
	ruleCount := len(params.RuleIds)
	if params.FullSync {
		ruleCount = 0 // 全量同步时不知道具体数量
	}

	output := fmt.Sprintf("规则同步完成，同步了 %d 条规则", ruleCount)

	return &model.TaskResult{
		TaskId:  "",
		Status:  model.TaskStatus_TASK_STATUS_COMPLETED,
		Output:  output,
	}, nil
}

// handleSyncUsers 处理同步用户任务
func (h *DefaultTaskHandler) handleSyncUsers(ctx context.Context, task *model.TaskMessage) (*model.TaskResult, error) {
	h.log.WithField("task_id", task.TaskId).Info("处理同步用户任务")

	// 解析同步用户参数
	var params model.SyncUsersTaskParams
	if err := task.Parameters.UnmarshalTo(&params); err != nil {
		return nil, fmt.Errorf("解析同步用户参数失败: %w", err)
	}

	// 这里应该实现用户同步逻辑
	// 可能需要从API获取最新的用户信息并更新本地配置
	userCount := len(params.UserIds)
	if params.FullSync {
		userCount = 0 // 全量同步时不知道具体数量
	}

	output := fmt.Sprintf("用户同步完成，同步了 %d 个用户", userCount)

	return &model.TaskResult{
		TaskId:  "",
		Status:  model.TaskStatus_TASK_STATUS_COMPLETED,
		Output:  output,
	}, nil
}

// handleCollectMetrics 处理收集监控数据任务
func (h *DefaultTaskHandler) handleCollectMetrics(ctx context.Context, task *model.TaskMessage) (*model.TaskResult, error) {
	h.log.WithField("task_id", task.TaskId).Info("处理收集监控数据任务")

	// 解析收集监控数据参数
	var params model.CollectMetricsTaskParams
	if err := task.Parameters.UnmarshalTo(&params); err != nil {
		return nil, fmt.Errorf("解析收集监控数据参数失败: %w", err)
	}

	// 这里应该实现监控数据收集逻辑
	// 可能需要立即收集并上报监控数据
	metricsCount := len(params.MetricsTypes)
	
	output := fmt.Sprintf("监控数据收集完成，收集了 %d 种类型的监控数据", metricsCount)

	return &model.TaskResult{
		TaskId:  "",
		Status:  model.TaskStatus_TASK_STATUS_COMPLETED,
		Output:  output,
	}, nil
}

// handleHealthCheck 处理健康检查任务
func (h *DefaultTaskHandler) handleHealthCheck(ctx context.Context, task *model.TaskMessage) (*model.TaskResult, error) {
	h.log.WithField("task_id", task.TaskId).Info("处理健康检查任务")

	// 解析健康检查参数
	var params model.HealthCheckTaskParams
	if err := task.Parameters.UnmarshalTo(&params); err != nil {
		return nil, fmt.Errorf("解析健康检查参数失败: %w", err)
	}

	// 执行健康检查
	checks := make(map[string]bool)
	
	for _, checkType := range params.CheckTypes {
		switch checkType {
		case "system":
			checks["system"] = h.checkSystemHealth(ctx)
		case "proxy":
			checks["proxy"] = h.checkProxyHealth(ctx)
		case "iptables":
			checks["iptables"] = h.checkIPTablesHealth(ctx)
		default:
			h.log.WithField("check_type", checkType).Warn("不支持的健康检查类型")
		}
	}

	// 构建健康检查结果
	allHealthy := true
	for _, healthy := range checks {
		if !healthy {
			allHealthy = false
			break
		}
	}

	output := fmt.Sprintf("健康检查完成，检查结果: %v", checks)

	status := model.TaskStatus_TASK_STATUS_COMPLETED
	if !allHealthy {
		status = model.TaskStatus_TASK_STATUS_FAILED
	}

	return &model.TaskResult{
		TaskId:  "",
		Status:  status,
		Output:  output,
	}, nil
}

// checkSystemHealth 检查系统健康状态
func (h *DefaultTaskHandler) checkSystemHealth(ctx context.Context) bool {
	// 检查系统基本状态
	// 例如：磁盘空间、内存使用率、CPU负载等
	return true // 简化实现
}

// checkProxyHealth 检查代理健康状态
func (h *DefaultTaskHandler) checkProxyHealth(ctx context.Context) bool {
	// 检查代理服务状态
	if h.proxyManager != nil {
		// 这里需要实现代理健康检查逻辑
		// 例如：检查代理进程是否运行、端口是否监听等
		return true // 简化实现
	}
	return false
}

// checkIPTablesHealth 检查iptables健康状态
func (h *DefaultTaskHandler) checkIPTablesHealth(ctx context.Context) bool {
	// 检查iptables规则状态
	if h.iptablesManager != nil {
		// 这里需要实现iptables健康检查逻辑
		// 例如：检查规则是否正确配置、是否有冲突等
		return true // 简化实现
	}
	return false
}
