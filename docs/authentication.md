# NSPass Agent 鉴权机制

## 概述

NSPass Agent 使用自定义的HTTP Header鉴权方式与后端API通信，而不是传统的Bearer Token方式。

## 鉴权方式

每个API请求都会在HTTP Header中包含以下字段：

- `X-Server-ID`: 服务器唯一标识符
- `X-Token`: API访问令牌
- `Content-Type`: application/json

## 配置示例

在配置文件中需要设置以下字段：

```yaml
# 服务器ID（用于API鉴权）
server_id: "your-server-id-here"

# API配置
api:
  base_url: "https://api.nspass.com"
  token: "your-api-token-here"
  timeout: 30
  retry_count: 3
```

## 实现细节

### 1. 配置加载

Agent启动时会从配置文件中读取 `server_id` 和 `api.token` 字段。

### 2. API客户端初始化

```go
// 创建API客户端时传入serverID
apiClient := api.NewClient(cfg.API, cfg.ServerID)
```

### 3. HTTP请求鉴权

每个HTTP请求都会自动添加鉴权Headers：

```go
func (c *Client) setAuthHeaders(req *http.Request) {
    req.Header.Set("X-Server-ID", c.serverID)
    req.Header.Set("X-Token", c.config.Token)
    req.Header.Set("Content-Type", "application/json")
}
```

## 安全注意事项

1. **配置文件权限**: 确保配置文件权限设置为 600，只允许运行用户访问
2. **Token安全性**: API Token应该定期轮换
3. **网络传输**: 确保使用HTTPS进行API通信
4. **日志安全**: 避免在日志中记录完整的Token信息

## 错误处理

如果鉴权失败，API会返回相应的HTTP状态码：

- `401 Unauthorized`: Token无效或缺失
- `403 Forbidden`: Server ID无效或没有权限
- `400 Bad Request`: 请求格式错误

## 测试验证

可以使用以下命令验证配置是否正确：

```bash
# 检查配置文件语法
nspass-agent --config /path/to/config.yaml --dry-run

# 启用详细日志模式查看鉴权过程
nspass-agent --config /path/to/config.yaml --log-level debug
```

## 迁移指南

从Bearer Token迁移到Header鉴权：

1. 在配置文件中添加 `server_id` 字段
2. 重启Agent服务
3. 验证API调用正常工作
4. 可选：清理旧的鉴权相关代码（如果有自定义修改） 