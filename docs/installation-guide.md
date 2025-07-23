# NSPass Agent 安装指南

## 🚀 推荐安装方式

由于管道安装可能在某些环境下出现问题（网络延迟、shell配置等），我们推荐直接下载安装脚本后执行：

### 方式一：直接下载安装脚本（推荐）

```bash
# 一行命令完成安装
curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh -o install.sh && chmod +x install.sh && sudo DEBUG_MODE=1 ./install.sh -sid 1 -token kuZp5DDPFtoRNE532eYAo23Jf1AledS8 -endpoint https://agent.nspass.xforward.de

# 或者分步执行
# 步骤1: 下载安装脚本
curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh -o install.sh

# 步骤2: 设置执行权限
chmod +x install.sh

# 步骤3: 执行安装（启用调试模式）
sudo DEBUG_MODE=1 ./install.sh -sid 1 -token kuZp5DDPFtoRNE532eYAo23Jf1AledS8 -endpoint https://agent.nspass.xforward.de

# 或者使用预设环境
sudo DEBUG_MODE=1 ./install.sh -sid 1 -token kuZp5DDPFtoRNE532eYAo23Jf1AledS8 -env production
```

### 方式二：管道安装（如果环境支持）

```bash
# 传统管道方式（可能在某些环境下失败）
curl -sSL https://raw.githubusercontent.com/moooyo/nspass-agent/main/scripts/install.sh | sudo DEBUG_MODE=1 bash -s -- -sid 1 -token kuZp5DDPFtoRNE532eYAo23Jf1AledS8 -endpoint https://agent.nspass.xforward.de
```

## 🔍 管道安装问题说明

管道安装 `curl ... | bash` 可能在以下情况下失败：

1. **网络中断**: 如果下载过程中网络中断，脚本可能不完整
2. **Shell配置**: 某些shell配置可能影响管道执行
3. **权限问题**: 管道中的错误处理可能不够完善
4. **环境变量**: 在管道中传递环境变量可能有限制

## ✅ 优势对比

| 安装方式 | 可靠性 | 调试能力 | 易用性 |
|---------|--------|----------|--------|
| 直接下载脚本 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 管道安装 | ⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐⭐ |

## 🛠️ 故障排除

如果安装仍然失败，请按以下步骤排查：

### 1. 检查网络连接
```bash
curl -I https://raw.githubusercontent.com
curl -I https://api.github.com
```

### 2. 检查系统要求
```bash
# 检查操作系统
cat /etc/os-release

# 检查架构
uname -m

# 检查systemd
systemctl --version
```

### 3. 手动检查下载文件
```bash
# 检查GitHub releases
curl -s https://api.github.com/repos/moooyo/nspass-agent/releases/latest | grep "tag_name"

# 检查可用的文件
curl -s https://api.github.com/repos/moooyo/nspass-agent/releases/latest | grep "browser_download_url"
```

### 4. 查看详细日志
使用 `DEBUG_MODE=1` 参数时，会自动启用调试模式，提供详细的安装日志。

## 📞 获得帮助

如果以上方法都无法解决问题，请：

1. 保存安装过程的完整输出
2. 在 [GitHub Issues](https://github.com/moooyo/nspass-agent/issues) 中创建新问题
3. 提供系统信息和错误日志

---

**推荐使用直接下载方式，它提供了最佳的安装体验和错误处理能力。**
