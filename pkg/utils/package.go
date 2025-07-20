package utils

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/sirupsen/logrus"
)

// PackageManager 包管理器工具
type PackageManager struct {
	logger    *logrus.Entry
	manager   string
	available bool
}

// NewPackageManager 创建包管理器实例
func NewPackageManager(component string) *PackageManager {
	pm := &PackageManager{
		logger: logger.GetComponentLogger(component + "-package"),
	}

	// 检测可用的包管理器
	pm.detectPackageManager()
	return pm
}

// detectPackageManager 检测系统的包管理器
func (pm *PackageManager) detectPackageManager() {
	managers := []struct {
		command string
		name    string
	}{
		{"apt-get", "apt"},
		{"yum", "yum"},
		{"dnf", "dnf"},
		{"pacman", "pacman"},
		{"zypper", "zypper"},
		{"apk", "apk"},
	}

	for _, mgr := range managers {
		if _, err := exec.LookPath(mgr.command); err == nil {
			pm.manager = mgr.name
			pm.available = true
			pm.logger.WithField("package_manager", mgr.name).Debug("检测到包管理器")
			return
		}
	}

	pm.logger.Warn("未检测到支持的包管理器")
	pm.available = false
}

// IsAvailable 检查包管理器是否可用
func (pm *PackageManager) IsAvailable() bool {
	return pm.available
}

// GetManagerName 获取包管理器名称
func (pm *PackageManager) GetManagerName() string {
	return pm.manager
}

// UpdatePackageList 更新包列表
func (pm *PackageManager) UpdatePackageList() error {
	if !pm.available {
		return fmt.Errorf("包管理器不可用")
	}

	var cmd *exec.Cmd
	switch pm.manager {
	case "apt":
		cmd = exec.Command("apt-get", "update")
	case "yum", "dnf":
		// yum/dnf 通常不需要手动更新包列表
		return nil
	case "pacman":
		cmd = exec.Command("pacman", "-Sy")
	case "zypper":
		cmd = exec.Command("zypper", "refresh")
	case "apk":
		cmd = exec.Command("apk", "update")
	default:
		return fmt.Errorf("不支持的包管理器: %s", pm.manager)
	}

	pm.logger.Debug("更新包列表")
	if err := cmd.Run(); err != nil {
		logger.LogError(err, "更新包列表失败", logrus.Fields{
			"pkg_manager": pm.manager,
		})
		return fmt.Errorf("更新包列表失败: %w", err)
	}

	return nil
}

// InstallPackage 安装软件包
func (pm *PackageManager) InstallPackage(packageName string) error {
	if !pm.available {
		return fmt.Errorf("包管理器不可用")
	}

	startTime := time.Now()
	log := pm.logger.WithField("package", packageName)

	// 先尝试更新包列表
	if err := pm.UpdatePackageList(); err != nil {
		log.WithError(err).Warn("更新包列表失败，继续安装")
	}

	var cmd *exec.Cmd
	switch pm.manager {
	case "apt":
		cmd = exec.Command("apt-get", "install", "-y", packageName)
	case "yum":
		cmd = exec.Command("yum", "install", "-y", packageName)
	case "dnf":
		cmd = exec.Command("dnf", "install", "-y", packageName)
	case "pacman":
		cmd = exec.Command("pacman", "-S", "--noconfirm", packageName)
	case "zypper":
		cmd = exec.Command("zypper", "install", "-y", packageName)
	case "apk":
		cmd = exec.Command("apk", "add", packageName)
	default:
		return fmt.Errorf("不支持的包管理器: %s", pm.manager)
	}

	log.Info("开始安装软件包")
	if err := cmd.Run(); err != nil {
		logger.LogError(err, "安装软件包失败", logrus.Fields{
			"pkg_manager": pm.manager,
			"package":     packageName,
		})
		return fmt.Errorf("安装%s失败: %w", packageName, err)
	}

	duration := time.Since(startTime)
	logger.LogPerformance("package_install", duration, logrus.Fields{
		"pkg_manager": pm.manager,
		"package":     packageName,
	})

	log.WithField("duration_ms", duration.Milliseconds()).Info("软件包安装完成")
	return nil
}

// RemovePackage 卸载软件包
func (pm *PackageManager) RemovePackage(packageName string) error {
	if !pm.available {
		return fmt.Errorf("包管理器不可用")
	}

	log := pm.logger.WithField("package", packageName)

	var cmd *exec.Cmd
	switch pm.manager {
	case "apt":
		cmd = exec.Command("apt-get", "remove", "-y", packageName)
	case "yum":
		cmd = exec.Command("yum", "remove", "-y", packageName)
	case "dnf":
		cmd = exec.Command("dnf", "remove", "-y", packageName)
	case "pacman":
		cmd = exec.Command("pacman", "-R", "--noconfirm", packageName)
	case "zypper":
		cmd = exec.Command("zypper", "remove", "-y", packageName)
	case "apk":
		cmd = exec.Command("apk", "del", packageName)
	default:
		return fmt.Errorf("不支持的包管理器: %s", pm.manager)
	}

	log.Info("开始卸载软件包")
	if err := cmd.Run(); err != nil {
		logger.LogError(err, "卸载软件包失败", logrus.Fields{
			"pkg_manager": pm.manager,
			"package":     packageName,
		})
		return fmt.Errorf("卸载%s失败: %w", packageName, err)
	}

	log.Info("软件包卸载完成")
	return nil
}

// IsPackageInstalled 检查软件包是否已安装
func (pm *PackageManager) IsPackageInstalled(packageName string) bool {
	if !pm.available {
		return false
	}

	var cmd *exec.Cmd
	switch pm.manager {
	case "apt":
		cmd = exec.Command("dpkg", "-l", packageName)
	case "yum", "dnf":
		cmd = exec.Command("rpm", "-q", packageName)
	case "pacman":
		cmd = exec.Command("pacman", "-Q", packageName)
	case "zypper":
		cmd = exec.Command("rpm", "-q", packageName)
	case "apk":
		cmd = exec.Command("apk", "info", "-e", packageName)
	default:
		return false
	}

	return cmd.Run() == nil
}
