package utils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/sirupsen/logrus"
)

// FileUtils 文件操作工具
type FileUtils struct {
	logger *logrus.Entry
}

// NewFileUtils 创建文件操作工具实例
func NewFileUtils(component string) *FileUtils {
	return &FileUtils{
		logger: logger.GetComponentLogger(component),
	}
}

// EnsureDirectory 确保目录存在，如果不存在则创建
func (f *FileUtils) EnsureDirectory(dirPath string, perm os.FileMode) error {
	if err := os.MkdirAll(dirPath, perm); err != nil {
		logger.LogError(err, "创建目录失败", logrus.Fields{
			"directory": dirPath,
			"perm":      perm,
		})
		return fmt.Errorf("创建目录失败: %w", err)
	}

	f.logger.WithFields(logrus.Fields{
		"directory": dirPath,
		"perm":      perm,
	}).Debug("目录已创建或已存在")

	return nil
}

// EnsureConfigDirectory 确保配置目录存在
func (f *FileUtils) EnsureConfigDirectory(configPath string) error {
	configDir := filepath.Dir(configPath)
	return f.EnsureDirectory(configDir, 0755)
}

// WriteFileSecure 安全地写入文件（带权限控制）
func (f *FileUtils) WriteFileSecure(filePath string, data []byte, perm os.FileMode) error {
	// 确保父目录存在
	if err := f.EnsureConfigDirectory(filePath); err != nil {
		return err
	}

	if err := os.WriteFile(filePath, data, perm); err != nil {
		logger.LogError(err, "写入文件失败", logrus.Fields{
			"file_path": filePath,
			"perm":      perm,
		})
		return fmt.Errorf("写入文件失败: %w", err)
	}

	f.logger.WithFields(logrus.Fields{
		"file_path": filePath,
		"size":      len(data),
	}).Debug("文件写入成功")

	return nil
}

// WriteConfigFile 写入配置文件（限制权限为600）
func (f *FileUtils) WriteConfigFile(filePath string, data []byte) error {
	return f.WriteFileSecure(filePath, data, 0600)
}

// WritePidFile 写入PID文件
func (f *FileUtils) WritePidFile(pidFile string, pid int) error {
	data := []byte(fmt.Sprintf("%d", pid))
	return f.WriteFileSecure(pidFile, data, 0644)
}

// FileExists 检查文件是否存在
func (f *FileUtils) FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

// RemoveFileIfExists 如果文件存在则删除
func (f *FileUtils) RemoveFileIfExists(filePath string) error {
	if !f.FileExists(filePath) {
		return nil
	}

	if err := os.Remove(filePath); err != nil {
		logger.LogError(err, "删除文件失败", logrus.Fields{
			"file_path": filePath,
		})
		return fmt.Errorf("删除文件失败: %w", err)
	}

	f.logger.WithField("file_path", filePath).Debug("文件已删除")
	return nil
}

// CreateSymlink 创建符号链接
func (f *FileUtils) CreateSymlink(source, target string) error {
	// 如果目标已存在，先删除
	if f.FileExists(target) {
		if err := os.Remove(target); err != nil {
			f.logger.WithError(err).Warnf("删除已存在的符号链接失败: %s", target)
		}
	}

	if err := os.Symlink(source, target); err != nil {
		logger.LogError(err, "创建符号链接失败", logrus.Fields{
			"source": source,
			"target": target,
		})
		return fmt.Errorf("创建符号链接失败: %w", err)
	}

	f.logger.WithFields(logrus.Fields{
		"source": source,
		"target": target,
	}).Debug("符号链接已创建")

	return nil
}
