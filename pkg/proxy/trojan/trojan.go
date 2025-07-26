package trojan

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/moooyo/nspass-proto/generated/model"
	"github.com/nspass/nspass-agent/pkg/logger"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultConfigPath 默认代理配置文件路径
	DefaultConfigPath = "/etc/nspass-agent"
	// DefaultBinPath 默认代理软件安装路径
	DefaultBinPath = "/usr/local/bin/proxy"
)

// Trojan trojan代理实现
type Trojan struct {
	egressItem *model.EgressItem // 出口配置
	configPath string
	pidFile    string
}

// New 创建新的Trojan实例
func New(egressItem *model.EgressItem) *Trojan {
	t := &Trojan{
		egressItem: egressItem,
		configPath: filepath.Join(DefaultConfigPath, fmt.Sprintf("trojan-%s.json", egressItem.EgressId)),
		pidFile:    filepath.Join(DefaultConfigPath, fmt.Sprintf("trojan-%s.pid", egressItem.EgressId)),
	}

	logger.LogStartup("trojan-proxy", "1.0", map[string]interface{}{
		"config_path": t.configPath,
		"pid_file":    t.pidFile,
	})

	return t
}

// Type 返回代理类型
func (t *Trojan) Type() string {
	return "trojan"
}

// Configure 配置trojan
func (t *Trojan) Configure(cfg *model.EgressItem) error {
	startTime := time.Now()
	log := logger.GetProxyLogger().WithField("proxy_type", "trojan")

	log.WithField("config_path", t.configPath).Debug("开始配置trojan")

	// 确保配置目录存在
	configDir := filepath.Dir(t.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		logger.LogError(err, "创建配置目录失败", logrus.Fields{
			"config_dir": configDir,
		})
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 先停止现有服务
	if t.IsRunning() {
		log.Debug("停止现有trojan服务以更新配置")
		if err := t.Stop(); err != nil {
			logger.LogError(err, "停止trojan服务失败", nil)
		}
	}

	// 从EgressItem中解析配置
	egressConfig := make(map[string]interface{})
	if cfg.EgressConfig != "" {
		if err := json.Unmarshal([]byte(cfg.EgressConfig), &egressConfig); err != nil {
			log.WithError(err).Error("解析出口配置失败")
			return fmt.Errorf("解析出口配置失败: %w", err)
		}
	}

	// 生成trojan配置
	config := map[string]interface{}{
		"run_type":    "client",
		"local_addr":  "127.0.0.1",
		"local_port":  1080,
		"remote_addr": egressConfig["server"],
		"remote_port": egressConfig["port"],
		"password":    []string{egressConfig["password"].(string)},
		"log_level":   1,
		"ssl": map[string]interface{}{
			"verify":          true,
			"verify_hostname": true,
			"cert":            "",
			"cipher":          "ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384",
			"cipher_tls13":    "TLS_AES_128_GCM_SHA256:TLS_CHACHA20_POLY1305_SHA256:TLS_AES_256_GCM_SHA384",
			"sni":             egressConfig["sni"],
		},
		"tcp": map[string]interface{}{
			"no_delay":       true,
			"keep_alive":     true,
			"reuse_port":     false,
			"fast_open":      false,
			"fast_open_qlen": 20,
		},
	}

	// 如果有自定义本地端口
	if localPort, ok := egressConfig["local_port"]; ok {
		config["local_port"] = localPort
	}

	if localAddr, ok := egressConfig["local_addr"]; ok {
		config["local_addr"] = localAddr
	}

	// 写入配置文件
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		logger.LogError(err, "序列化配置失败", logrus.Fields{
			"config": cfg,
		})
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(t.configPath, data, 0600); err != nil {
		logger.LogError(err, "写入配置文件失败", logrus.Fields{
			"config_path": t.configPath,
		})
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	duration := time.Since(startTime)
	logger.LogPerformance("trojan_configure", duration, logrus.Fields{
		"config_size": len(data),
	})

	log.WithFields(logrus.Fields{
		"config_path": t.configPath,
		"duration_ms": duration.Milliseconds(),
	}).Info("trojan配置已更新")

	return nil
}

// Start 启动trojan
func (t *Trojan) Start() error {
	startTime := time.Now()
	log := logger.GetProxyLogger().WithField("proxy_type", "trojan")

	if t.IsRunning() {
		log.Debug("trojan已在运行")
		return nil
	}

	if !t.IsInstalled() {
		logger.LogError(fmt.Errorf("trojan未安装"), "无法启动未安装的trojan", nil)
		return fmt.Errorf("trojan未安装")
	}

	log.Debug("启动trojan服务")

	// 启动trojan
	trojanBinaryPath := filepath.Join(DefaultBinPath, "trojan-go")
	cmd := exec.Command(trojanBinaryPath, "-c", t.configPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logger.LogError(err, "启动trojan失败", logrus.Fields{
			"config_path": t.configPath,
		})
		return fmt.Errorf("启动trojan失败: %w", err)
	}

	// 写入PID文件
	pid := cmd.Process.Pid
	if err := os.WriteFile(t.pidFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		logger.LogError(err, "写入PID文件失败", logrus.Fields{
			"pid":      pid,
			"pid_file": t.pidFile,
		})
	}

	duration := time.Since(startTime)
	logger.LogPerformance("trojan_start", duration, logrus.Fields{
		"pid": pid,
	})

	// 记录状态变更
	logger.LogStateChange("trojan", "stopped", "running", "正常启动")

	log.WithFields(logrus.Fields{
		"pid":         pid,
		"duration_ms": duration.Milliseconds(),
	}).Info("trojan服务已启动")

	return nil
}

// Stop 停止trojan
func (t *Trojan) Stop() error {
	startTime := time.Now()
	log := logger.GetProxyLogger().WithField("proxy_type", "trojan")

	log.Debug("停止trojan服务")

	// 读取PID文件
	pidData, err := os.ReadFile(t.pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("PID文件不存在，trojan可能已停止")
			return nil
		}
		logger.LogError(err, "读取PID文件失败", logrus.Fields{
			"pid_file": t.pidFile,
		})
		return fmt.Errorf("读取PID文件失败: %w", err)
	}

	pid, err := strconv.Atoi(string(pidData))
	if err != nil {
		logger.LogError(err, "解析PID失败", logrus.Fields{
			"pid_data": string(pidData),
		})
		return fmt.Errorf("解析PID失败: %w", err)
	}

	// 发送TERM信号
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		logger.LogError(err, "停止进程失败", logrus.Fields{
			"pid": pid,
		})
		return fmt.Errorf("停止进程失败: %w", err)
	}

	// 删除PID文件
	os.Remove(t.pidFile)

	duration := time.Since(startTime)
	logger.LogPerformance("trojan_stop", duration, logrus.Fields{
		"pid": pid,
	})

	// 记录状态变更
	logger.LogStateChange("trojan", "running", "stopped", "正常停止")

	log.WithFields(logrus.Fields{
		"pid":         pid,
		"duration_ms": duration.Milliseconds(),
	}).Info("trojan服务已停止")

	return nil
}

// Restart 重启trojan
func (t *Trojan) Restart() error {
	if err := t.Stop(); err != nil {
		logrus.Warnf("停止trojan失败: %v", err)
	}

	return t.Start()
}

// Status 获取trojan状态
func (t *Trojan) Status() (string, error) {
	log := logger.GetProxyLogger().WithField("proxy_type", "trojan")

	if !t.IsInstalled() {
		log.Debug("trojan未安装")
		return "not_installed", nil
	}

	if t.IsRunning() {
		log.Debug("trojan正在运行")
		return "running", nil
	}

	log.Debug("trojan已停止")
	return "stopped", nil
}

// IsInstalled 检查是否已安装
func (t *Trojan) IsInstalled() bool {
	binaryPath := filepath.Join(DefaultBinPath, "trojan-go")
	_, err := os.Stat(binaryPath)
	installed := err == nil

	logger.GetProxyLogger().WithFields(logrus.Fields{
		"proxy_type":  "trojan",
		"binary_path": binaryPath,
		"installed":   installed,
	}).Debug("检查安装状态")

	return installed
}

// IsRunning 检查是否正在运行
func (t *Trojan) IsRunning() bool {
	log := logger.GetProxyLogger().WithField("proxy_type", "trojan")

	// 检查PID文件
	pidData, err := os.ReadFile(t.pidFile)
	if err != nil {
		log.WithField("pid_file", t.pidFile).Debug("PID文件不存在或读取失败")
		return false
	}

	pid, err := strconv.Atoi(string(pidData))
	if err != nil {
		log.WithField("pid_data", string(pidData)).Debug("解析PID失败")
		return false
	}

	// 检查进程是否存在
	if err := syscall.Kill(pid, 0); err != nil {
		log.WithField("pid", pid).Debug("进程不存在")
		return false
	}

	log.WithField("pid", pid).Debug("trojan进程运行中")
	return true
}
