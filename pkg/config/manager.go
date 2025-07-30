package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/nspass/nspass-agent/pkg/logging"
	"gopkg.in/yaml.v3"
)

// ConfigFormat 配置文件格式
type ConfigFormat string

const (
	FormatYAML ConfigFormat = "yaml"
)

// ConfigValidator 配置验证器接口
type ConfigValidator interface {
	Validate(config interface{}) error
}

// ConfigWatcher 配置监听器接口
type ConfigWatcher interface {
	OnConfigChanged(oldConfig, newConfig interface{}) error
}

// Manager 配置管理器
type Manager struct {
	mu        sync.RWMutex
	configs   map[string]interface{}
	watchers  map[string][]ConfigWatcher
	validator ConfigValidator
	logger    *logging.StandardLogger
}

// NewManager 创建配置管理器
func NewManager() *Manager {
	return &Manager{
		configs:  make(map[string]interface{}),
		watchers: make(map[string][]ConfigWatcher),
		logger:   logging.NewStandardLogger("config-manager"),
	}
}

// SetValidator 设置配置验证器
func (m *Manager) SetValidator(validator ConfigValidator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.validator = validator
}

// LoadConfig 加载配置文件
func (m *Manager) LoadConfig(key, filePath string, target interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	opLogger := m.logger.WithOperation("load_config")

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		opLogger.Error("配置文件不存在", logging.StandardFields{
			ConfigPath: filePath,
			Error:      err,
		})
		return fmt.Errorf("配置文件不存在: %s", filePath)
	}

	// 读取文件内容
	data, err := os.ReadFile(filePath)
	if err != nil {
		opLogger.Error("读取配置文件失败", logging.StandardFields{
			ConfigPath: filePath,
			Error:      err,
		})
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 根据文件扩展名确定格式
	format := m.detectFormat(filePath)

	// 解析配置
	if err := m.parseConfig(data, format, target); err != nil {
		opLogger.Error("解析配置文件失败", logging.StandardFields{
			ConfigPath: filePath,
			Error:      err,
			Custom: map[string]interface{}{
				"format": string(format),
			},
		})
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 验证配置
	if m.validator != nil {
		if err := m.validator.Validate(target); err != nil {
			opLogger.Error("配置验证失败", logging.StandardFields{
				ConfigPath: filePath,
				Error:      err,
			})
			return fmt.Errorf("配置验证失败: %w", err)
		}
	}

	// 保存配置
	oldConfig := m.configs[key]
	m.configs[key] = target

	// 文件监听功能已移除，如需要可以后续添加

	// 通知监听器
	m.notifyWatchers(key, oldConfig, target)

	opLogger.Success("配置加载成功", logging.StandardFields{
		ConfigPath: filePath,
		Custom: map[string]interface{}{
			"format": string(format),
			"size":   len(data),
		},
	})

	return nil
}

// SaveConfig 保存配置文件
func (m *Manager) SaveConfig(key, filePath string, config interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	opLogger := m.logger.WithOperation("save_config")

	// 验证配置
	if m.validator != nil {
		if err := m.validator.Validate(config); err != nil {
			opLogger.Error("配置验证失败", logging.StandardFields{
				ConfigPath: filePath,
				Error:      err,
			})
			return fmt.Errorf("配置验证失败: %w", err)
		}
	}

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		opLogger.Error("创建配置目录失败", logging.StandardFields{
			ConfigPath: filePath,
			Error:      err,
		})
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 根据文件扩展名确定格式
	format := m.detectFormat(filePath)

	// 序列化配置
	data, err := m.serializeConfig(config, format)
	if err != nil {
		opLogger.Error("序列化配置失败", logging.StandardFields{
			ConfigPath: filePath,
			Error:      err,
			Custom: map[string]interface{}{
				"format": string(format),
			},
		})
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		opLogger.Error("写入配置文件失败", logging.StandardFields{
			ConfigPath: filePath,
			Error:      err,
		})
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	// 更新内存中的配置
	oldConfig := m.configs[key]
	m.configs[key] = config

	// 通知监听器
	m.notifyWatchers(key, oldConfig, config)

	opLogger.Success("配置保存成功", logging.StandardFields{
		ConfigPath: filePath,
		Custom: map[string]interface{}{
			"format": string(format),
			"size":   len(data),
		},
	})

	return nil
}

// GetConfig 获取配置
func (m *Manager) GetConfig(key string) (interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	config, exists := m.configs[key]
	return config, exists
}

// AddWatcher 添加配置监听器
func (m *Manager) AddWatcher(key string, watcher ConfigWatcher) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.watchers[key] = append(m.watchers[key], watcher)
}

// RemoveWatcher 移除配置监听器
func (m *Manager) RemoveWatcher(key string, watcher ConfigWatcher) {
	m.mu.Lock()
	defer m.mu.Unlock()

	watchers := m.watchers[key]
	for i, w := range watchers {
		if w == watcher {
			m.watchers[key] = append(watchers[:i], watchers[i+1:]...)
			break
		}
	}
}

// StartWatching 开始监听配置文件变化（功能已移除）
func (m *Manager) StartWatching() {
	// 文件监听功能已移除，如需要可以后续添加
}

// Close 关闭配置管理器
func (m *Manager) Close() error {
	// 无需关闭资源
	return nil
}

// detectFormat 检测配置文件格式
func (m *Manager) detectFormat(filePath string) ConfigFormat {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".yaml", ".yml":
		return FormatYAML
	default:
		return FormatYAML // 默认使用YAML
	}
}

// parseConfig 解析配置
func (m *Manager) parseConfig(data []byte, format ConfigFormat, target interface{}) error {
	switch format {
	case FormatYAML:
		return yaml.Unmarshal(data, target)
	default:
		return fmt.Errorf("不支持的配置格式: %s", format)
	}
}

// serializeConfig 序列化配置
func (m *Manager) serializeConfig(config interface{}, format ConfigFormat) ([]byte, error) {
	switch format {
	case FormatYAML:
		return yaml.Marshal(config)
	default:
		return nil, fmt.Errorf("不支持的配置格式: %s", format)
	}
}

// handleFileChange 处理文件变化
func (m *Manager) handleFileChange(filePath string) {
	m.logger.Info("检测到配置文件变化", logging.StandardFields{
		ConfigPath: filePath,
	})

	// 这里可以实现配置热重载逻辑
	// 暂时只记录日志
}

// notifyWatchers 通知监听器
func (m *Manager) notifyWatchers(key string, oldConfig, newConfig interface{}) {
	watchers := m.watchers[key]
	for _, watcher := range watchers {
		go func(w ConfigWatcher) {
			if err := w.OnConfigChanged(oldConfig, newConfig); err != nil {
				m.logger.Error("配置监听器处理失败", logging.StandardFields{
					Error: err,
					Custom: map[string]interface{}{
						"config_key": key,
					},
				})
			}
		}(watcher)
	}
}
