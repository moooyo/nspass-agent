package websocket

import (
	"context"
	"crypto/tls"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nspass/nspass-agent/pkg/config"
	"github.com/nspass/nspass-agent/pkg/errors"
	"github.com/nspass/nspass-agent/pkg/interfaces"
)

// ConnectionState 连接状态
type ConnectionState int

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
	StateReconnecting
)

func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	default:
		return "unknown"
	}
}

// ConnectionManager WebSocket连接管理器
type ConnectionManager struct {
	config  *config.Config
	agentID string
	token   string
	logger  interfaces.Logger

	// 连接相关
	conn    *websocket.Conn
	connMu  sync.RWMutex
	state   ConnectionState
	stateMu sync.RWMutex

	// 控制相关
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 重连相关
	reconnectDelay       time.Duration
	maxReconnectDelay    time.Duration
	reconnectAttempts    int
	maxReconnectAttempts int

	// 事件通道
	messagesChan chan []byte
	errorsChan   chan error
	stateChan    chan ConnectionState

	// 回调函数
	onConnected    func()
	onDisconnected func(error)
	onMessage      func([]byte)
	onError        func(error)
}

// NewConnectionManager 创建连接管理器
func NewConnectionManager(cfg *config.Config, agentID, token string, logger interfaces.Logger) *ConnectionManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &ConnectionManager{
		config:               cfg,
		agentID:              agentID,
		token:                token,
		logger:               logger,
		ctx:                  ctx,
		cancel:               cancel,
		state:                StateDisconnected,
		reconnectDelay:       5 * time.Second,
		maxReconnectDelay:    5 * time.Minute,
		maxReconnectAttempts: cfg.WebSocket.MaxReconnectAttempts,
		messagesChan:         make(chan []byte, 100),
		errorsChan:           make(chan error, 10),
		stateChan:            make(chan ConnectionState, 10),
	}
}

// SetCallbacks 设置回调函数
func (cm *ConnectionManager) SetCallbacks(
	onConnected func(),
	onDisconnected func(error),
	onMessage func([]byte),
	onError func(error),
) {
	cm.onConnected = onConnected
	cm.onDisconnected = onDisconnected
	cm.onMessage = onMessage
	cm.onError = onError
}

// Start 启动连接管理器
func (cm *ConnectionManager) Start() error {
	cm.logger.Info("启动WebSocket连接管理器")

	cm.wg.Add(1)
	go cm.connectionLoop()

	return nil
}

// Stop 停止连接管理器
func (cm *ConnectionManager) Stop() error {
	cm.logger.Info("停止WebSocket连接管理器")

	cm.cancel()
	cm.closeConnection()
	cm.wg.Wait()

	close(cm.messagesChan)
	close(cm.errorsChan)
	close(cm.stateChan)

	return nil
}

// SendMessage 发送消息
func (cm *ConnectionManager) SendMessage(data []byte) error {
	cm.connMu.RLock()
	conn := cm.conn
	cm.connMu.RUnlock()

	if conn == nil {
		return errors.ErrWebSocketConnection.WithContext("reason", "连接未建立")
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return errors.Wrap(err, errors.ErrorTypeWebSocket, "WEBSOCKET_SEND_FAILED", "发送消息失败")
	}

	return nil
}

// IsConnected 检查是否已连接
func (cm *ConnectionManager) IsConnected() bool {
	cm.stateMu.RLock()
	defer cm.stateMu.RUnlock()
	return cm.state == StateConnected
}

// GetState 获取连接状态
func (cm *ConnectionManager) GetState() ConnectionState {
	cm.stateMu.RLock()
	defer cm.stateMu.RUnlock()
	return cm.state
}

// connectionLoop 连接循环
func (cm *ConnectionManager) connectionLoop() {
	defer cm.wg.Done()

	for {
		select {
		case <-cm.ctx.Done():
			return
		default:
			if err := cm.connect(); err != nil {
				cm.logger.WithError(err).Error("WebSocket连接失败")
				cm.handleConnectionError(err)
			} else {
				cm.handleConnection()
			}
		}
	}
}

// connect 建立连接
func (cm *ConnectionManager) connect() error {
	cm.setState(StateConnecting)
	cm.logger.Info("正在建立WebSocket连接")

	// 构建连接URL
	wsURL := cm.config.WebSocket.ServerURL

	// 配置拨号器
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 30 * time.Second

	// 如果是HTTPS，配置TLS
	if cm.config.API.TLSSkipVerify {
		dialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	// 设置请求头 - 参考原始client.go的实现
	headers := make(map[string][]string)
	headers["Server-ID"] = []string{cm.agentID}
	headers["Agent-Token"] = []string{cm.token}
	headers["User-Agent"] = []string{"nspass-agent/1.0"}

	// 建立连接
	conn, response, err := dialer.Dial(wsURL, headers)
	if err != nil {
		return errors.Wrap(err, errors.ErrorTypeWebSocket, "WEBSOCKET_DIAL_FAILED", "WebSocket拨号失败")
	}

	defer response.Body.Close()

	cm.connMu.Lock()
	cm.conn = conn
	cm.connMu.Unlock()

	cm.setState(StateConnected)
	cm.reconnectAttempts = 0
	cm.reconnectDelay = 5 * time.Second

	cm.logger.Info("WebSocket连接已建立")

	if cm.onConnected != nil {
		cm.onConnected()
	}

	return nil
}

// handleConnection 处理连接
func (cm *ConnectionManager) handleConnection() {
	cm.wg.Add(1)
	go cm.readLoop()

	// 等待连接断开或上下文取消
	select {
	case <-cm.ctx.Done():
	case err := <-cm.errorsChan:
		cm.logger.WithError(err).Error("WebSocket连接错误")
		if cm.onError != nil {
			cm.onError(err)
		}
	}

	cm.closeConnection()
}

// readLoop 读取循环
func (cm *ConnectionManager) readLoop() {
	defer cm.wg.Done()

	cm.connMu.RLock()
	conn := cm.conn
	cm.connMu.RUnlock()

	if conn == nil {
		return
	}

	for {
		select {
		case <-cm.ctx.Done():
			return
		default:
			_, data, err := conn.ReadMessage()
			if err != nil {
				cm.errorsChan <- err
				return
			}

			if cm.onMessage != nil {
				cm.onMessage(data)
			}
		}
	}
}

// closeConnection 关闭连接
func (cm *ConnectionManager) closeConnection() {
	cm.connMu.Lock()
	if cm.conn != nil {
		cm.conn.Close()
		cm.conn = nil
	}
	cm.connMu.Unlock()

	cm.setState(StateDisconnected)

	if cm.onDisconnected != nil {
		cm.onDisconnected(nil)
	}
}

// handleConnectionError 处理连接错误
func (cm *ConnectionManager) handleConnectionError(err error) {
	if cm.maxReconnectAttempts > 0 && cm.reconnectAttempts >= cm.maxReconnectAttempts {
		cm.logger.Error("达到最大重连次数，停止重连")
		return
	}

	cm.setState(StateReconnecting)
	cm.reconnectAttempts++

	cm.logger.WithField("attempt", cm.reconnectAttempts).
		WithField("delay", cm.reconnectDelay).
		Info("准备重连WebSocket")

	select {
	case <-cm.ctx.Done():
		return
	case <-time.After(cm.reconnectDelay):
		// 指数退避
		cm.reconnectDelay *= 2
		if cm.reconnectDelay > cm.maxReconnectDelay {
			cm.reconnectDelay = cm.maxReconnectDelay
		}
	}
}

// setState 设置连接状态
func (cm *ConnectionManager) setState(state ConnectionState) {
	cm.stateMu.Lock()
	oldState := cm.state
	cm.state = state
	cm.stateMu.Unlock()

	if oldState != state {
		cm.logger.WithField("from", oldState.String()).
			WithField("to", state.String()).
			Info("WebSocket连接状态变更")

		select {
		case cm.stateChan <- state:
		default:
		}
	}
}

// GetStatus 获取连接状态信息
func (cm *ConnectionManager) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"state":              cm.GetState().String(),
		"connected":          cm.IsConnected(),
		"reconnect_attempts": cm.reconnectAttempts,
		"reconnect_delay":    cm.reconnectDelay.String(),
	}
}
