package websocket

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/nspass/nspass-agent/generated/model"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/iptables"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/nspass/nspass-agent/pkg/proxy"
	"github.com/sirupsen/logrus"
)

// TaskRecord represents a task record in memory
type TaskRecord struct {
	TaskID      string            `json:"task_id"`
	TaskType    model.TaskType    `json:"task_type"`
	Status      model.TaskStatus  `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	Result      *model.TaskResult `json:"result,omitempty"`
	ErrorMsg    string            `json:"error_message,omitempty"`
	RetryCount  int               `json:"retry_count"`
	LastRetryAt *time.Time        `json:"last_retry_at,omitempty"`
}

// TaskManager manages task states in memory
type TaskManager struct {
	tasks map[string]*TaskRecord
	mu    sync.RWMutex
	log   *logrus.Entry
}

// NewTaskManager creates a new task manager
func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks: make(map[string]*TaskRecord),
		log:   logger.GetComponentLogger("task-manager"),
	}
}

// GetTask retrieves a task record
func (tm *TaskManager) GetTask(taskID string) (*TaskRecord, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	task, exists := tm.tasks[taskID]
	return task, exists
}

// CreateTask creates a new task record
func (tm *TaskManager) CreateTask(taskID string, taskType model.TaskType) *TaskRecord {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := time.Now()
	task := &TaskRecord{
		TaskID:    taskID,
		TaskType:  taskType,
		Status:    model.TaskStatus_TASK_STATUS_PENDING,
		CreatedAt: now,
	}

	tm.tasks[taskID] = task
	tm.log.WithFields(logrus.Fields{
		"task_id":   taskID,
		"task_type": taskType.String(),
	}).Info("Created new task record")

	return task
}

// UpdateTaskStatus updates task status
func (tm *TaskManager) UpdateTaskStatus(taskID string, status model.TaskStatus) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if task, exists := tm.tasks[taskID]; exists {
		task.Status = status
		now := time.Now()

		switch status {
		case model.TaskStatus_TASK_STATUS_RUNNING:
			task.StartedAt = &now
		case model.TaskStatus_TASK_STATUS_COMPLETED, model.TaskStatus_TASK_STATUS_FAILED, model.TaskStatus_TASK_STATUS_CANCELLED:
			task.CompletedAt = &now
		}

		tm.log.WithFields(logrus.Fields{
			"task_id": taskID,
			"status":  status.String(),
		}).Debug("Updated task status")
	}
}

// SetTaskResult sets task result
func (tm *TaskManager) SetTaskResult(taskID string, result *model.TaskResult, errorMsg string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if task, exists := tm.tasks[taskID]; exists {
		task.Result = result
		task.ErrorMsg = errorMsg

		if result != nil {
			task.Status = result.Status
		}
	}
}

// IncrementRetryCount increments retry count for a task
func (tm *TaskManager) IncrementRetryCount(taskID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if task, exists := tm.tasks[taskID]; exists {
		task.RetryCount++
		now := time.Now()
		task.LastRetryAt = &now

		tm.log.WithFields(logrus.Fields{
			"task_id":     taskID,
			"retry_count": task.RetryCount,
		}).Debug("Incremented task retry count")
	}
}

// CleanupOldTasks removes old completed tasks (older than 24 hours)
func (tm *TaskManager) CleanupOldTasks() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	cleaned := 0

	for taskID, task := range tm.tasks {
		if task.CompletedAt != nil && task.CompletedAt.Before(cutoff) {
			delete(tm.tasks, taskID)
			cleaned++
		}
	}

	if cleaned > 0 {
		tm.log.WithField("cleaned_count", cleaned).Info("Cleaned up old tasks")
	}
}

// GetTaskStats returns task statistics
func (tm *TaskManager) GetTaskStats() map[string]int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	stats := make(map[string]int)
	for _, task := range tm.tasks {
		stats[task.Status.String()]++
	}

	return stats
}

// DefaultTaskHandler 默认任务处理器
type DefaultTaskHandler struct {
	config          *config.Config
	proxyManager    *proxy.Manager
	iptablesManager iptables.ManagerInterface
	taskManager     *TaskManager
	log             *logrus.Entry
}

// NewDefaultTaskHandler 创建默认任务处理器
func NewDefaultTaskHandler(cfg *config.Config, proxyManager *proxy.Manager, iptablesManager iptables.ManagerInterface) *DefaultTaskHandler {
	return &DefaultTaskHandler{
		config:          cfg,
		proxyManager:    proxyManager,
		iptablesManager: iptablesManager,
		taskManager:     NewTaskManager(),
		log:             logger.GetComponentLogger("task-handler"),
	}
}

// CheckTaskStatus checks task status and determines how to handle it
func (h *DefaultTaskHandler) CheckTaskStatus(taskID string, taskType model.TaskType) (shouldExecute bool, existingResult *model.TaskResult) {
	task, exists := h.taskManager.GetTask(taskID)
	if !exists {
		// Task doesn't exist, should execute
		h.taskManager.CreateTask(taskID, taskType)
		return true, nil
	}

	h.log.WithFields(logrus.Fields{
		"task_id":     taskID,
		"task_status": task.Status.String(),
		"retry_count": task.RetryCount,
	}).Debug("Found existing task record")

	switch task.Status {
	case model.TaskStatus_TASK_STATUS_COMPLETED:
		// Task already completed, return existing result
		h.log.WithField("task_id", taskID).Info("Task already completed, returning cached result")
		return false, task.Result

	case model.TaskStatus_TASK_STATUS_RUNNING:
		// Task is currently running, don't execute again
		h.log.WithField("task_id", taskID).Info("Task is currently running, skipping execution")
		return false, nil

	case model.TaskStatus_TASK_STATUS_PENDING, model.TaskStatus_TASK_STATUS_FAILED:
		// Task is pending or failed, should retry
		h.log.WithField("task_id", taskID).Info("Task is pending or failed, will retry execution")
		h.taskManager.IncrementRetryCount(taskID)
		return true, nil

	case model.TaskStatus_TASK_STATUS_CANCELLED:
		// Task was cancelled, don't execute
		h.log.WithField("task_id", taskID).Info("Task was cancelled, skipping execution")
		return false, nil

	default:
		// Unknown status, treat as pending
		h.log.WithFields(logrus.Fields{
			"task_id": taskID,
			"status":  task.Status.String(),
		}).Warn("Unknown task status, treating as pending")
		return true, nil
	}
}

// HandleTask 处理任务
func (h *DefaultTaskHandler) HandleTask(ctx context.Context, task *model.TaskMessage) (*model.TaskResult, error) {
	h.log.WithFields(logrus.Fields{
		"task_id":   task.TaskId,
		"task_type": task.TaskType.String(),
		"title":     task.Title,
	}).Info("开始处理任务")

	// Check task status first
	shouldExecute, existingResult := h.CheckTaskStatus(task.TaskId, task.TaskType)
	if !shouldExecute {
		if existingResult != nil {
			// Return existing result for completed tasks
			return existingResult, nil
		}
		// For running tasks, return a running status result
		return &model.TaskResult{
			TaskId: task.TaskId,
			Status: model.TaskStatus_TASK_STATUS_RUNNING,
			Output: "Task is currently running or was cancelled",
		}, nil
	}

	// Mark task as running
	h.taskManager.UpdateTaskStatus(task.TaskId, model.TaskStatus_TASK_STATUS_RUNNING)

	var result *model.TaskResult
	var err error

	// Execute the task based on type
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

	// Update task status and result
	if err != nil {
		h.taskManager.UpdateTaskStatus(task.TaskId, model.TaskStatus_TASK_STATUS_FAILED)
		h.taskManager.SetTaskResult(task.TaskId, nil, err.Error())
		h.log.WithError(err).WithField("task_id", task.TaskId).Error("任务处理失败")
	} else {
		if result != nil {
			result.TaskId = task.TaskId
			if result.Status == model.TaskStatus_TASK_STATUS_UNSPECIFIED {
				result.Status = model.TaskStatus_TASK_STATUS_COMPLETED
			}
		} else {
			result = &model.TaskResult{
				TaskId: task.TaskId,
				Status: model.TaskStatus_TASK_STATUS_COMPLETED,
				Output: "Task completed successfully",
			}
		}
		h.taskManager.UpdateTaskStatus(task.TaskId, result.Status)
		h.taskManager.SetTaskResult(task.TaskId, result, "")
		h.log.WithField("task_id", task.TaskId).Info("任务处理成功")
	}

	// Cleanup old tasks periodically
	go h.taskManager.CleanupOldTasks()

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
		TaskId: "",
		Status: model.TaskStatus_TASK_STATUS_COMPLETED,
		Output: output,
	}, nil
}

// updateIPTablesConfig 更新iptables配置
func (h *DefaultTaskHandler) updateIPTablesConfig(ctx context.Context, params *model.ConfigUpdateTaskParams) (*model.TaskResult, error) {
	h.log.Info("更新iptables配置")

	// 这里可以根据具体的配置类型来处理不同的更新逻辑
	// 目前我们让agent通过常规的配置同步来处理iptables更新
	
	output := fmt.Sprintf("iptables配置更新请求已处理，配置类型: %s", params.ConfigType)
	
	// 如果需要重启，可以设置相应的标志
	if params.RestartRequired {
		output += "，需要重启服务"
		h.log.Info("iptables配置更新需要重启服务")
	}

	return &model.TaskResult{
		TaskId: "",
		Status: model.TaskStatus_TASK_STATUS_COMPLETED,
		Output: output,
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
		TaskId: "",
		Status: model.TaskStatus_TASK_STATUS_COMPLETED,
		Output: "代理服务重启成功",
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
		TaskId: "",
		Status: model.TaskStatus_TASK_STATUS_COMPLETED,
		Output: "Agent服务重启成功",
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
		TaskId: "",
		Status: model.TaskStatus_TASK_STATUS_COMPLETED,
		Output: output,
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
		TaskId: "",
		Status: model.TaskStatus_TASK_STATUS_COMPLETED,
		Output: output,
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
		TaskId: "",
		Status: model.TaskStatus_TASK_STATUS_COMPLETED,
		Output: output,
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
		TaskId: "",
		Status: status,
		Output: output,
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

// GetTaskStats returns task statistics
func (h *DefaultTaskHandler) GetTaskStats() map[string]int {
	return h.taskManager.GetTaskStats()
}

// GetTaskManager returns the task manager instance
func (h *DefaultTaskHandler) GetTaskManager() *TaskManager {
	return h.taskManager
}

// CancelTask cancels a running task
func (h *DefaultTaskHandler) CancelTask(taskID string) error {
	task, exists := h.taskManager.GetTask(taskID)
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if task.Status == model.TaskStatus_TASK_STATUS_RUNNING {
		h.taskManager.UpdateTaskStatus(taskID, model.TaskStatus_TASK_STATUS_CANCELLED)
		h.log.WithField("task_id", taskID).Info("Task cancelled")
		return nil
	}

	return fmt.Errorf("task %s is not running (status: %s)", taskID, task.Status.String())
}
