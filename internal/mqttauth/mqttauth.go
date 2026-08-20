// Package mqttauth 为 mochi-mqtt broker 提供基于配置用户的 CONNECT 认证。
//
// 设计要点:
//   - Enabled=false 时全部放行,行为与原 mqttauth.AllowHook 完全一致(平滑升级);
//   - Enabled=true 时按用户名 + bcrypt 哈希校验,可选允许匿名连接;
//   - 未知用户名也执行一次 dummy bcrypt 比较,消除用户名枚举时间侧信道;
//   - 同一来源 IP 认证失败达到阈值后临时封禁,防止在线爆破;
//   - OnACLCheck 恒返回 true:mochi 在无任何 ACL provider 时默认拒绝,
//     本 hook 不做主题级权限,保持原有全 topic 可读写行为。
package mqttauth

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"time"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"
	"golang.org/x/crypto/bcrypt"
)

// User 是一个可连接 broker 的账号,PasswordHash 为 bcrypt 散列。
type User struct {
	Username     string
	PasswordHash string
}

// Config 控制 hook 行为。
type Config struct {
	Enabled        bool
	AllowAnonymous bool
	Users          []User

	// MaxFailures 为单个 IP 在 Window 时间窗内允许的连续认证失败次数,
	// 达到后封锁该 IP BlockFor 时长。零值使用默认。
	MaxFailures int
	Window      time.Duration
	BlockFor    time.Duration

	// LogEvent 用于输出结构化事件(传入 main 包的 printJSON),可为 nil。
	LogEvent func(record map[string]any)
}

const (
	defaultMaxFailures = 5
	defaultWindow      = time.Minute
	defaultBlockFor    = 5 * time.Minute
	// dummyBcryptHash 是固定散列,用于未知用户名的耗时对齐。
	dummyBcryptHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
)

// Hook 实现 mochi 认证钩子。
type Hook struct {
	mqtt.HookBase
	cfg    Config
	users  map[string]string
	mu     sync.Mutex
	fails  map[string]*failState
	now    func() time.Time
	stopped chan struct{}
	stopOnce sync.Once
}

type failState struct {
	count        int
	windowStart  time.Time
	blockedUntil time.Time
}

// NewHook 按配置构造 hook 并启动后台清理协程,返回的 hook 交给 server.AddHook。
// 服务退出时应调用 Stop 结束清理协程。
func NewHook(cfg Config) *Hook {
	if cfg.MaxFailures <= 0 {
		cfg.MaxFailures = defaultMaxFailures
	}
	if cfg.Window <= 0 {
		cfg.Window = defaultWindow
	}
	if cfg.BlockFor <= 0 {
		cfg.BlockFor = defaultBlockFor
	}
	users := make(map[string]string, len(cfg.Users))
	for _, u := range cfg.Users {
		users[u.Username] = u.PasswordHash
	}
	h := &Hook{
		cfg:     cfg,
		users:   users,
		fails:   make(map[string]*failState),
		now:     time.Now,
		stopped: make(chan struct{}),
	}
	go h.cleanupLoop()
	return h
}

// Stop 结束后台清理协程;实现 mochi Hook 接口的 Stop。
func (h *Hook) Stop() error {
	h.stopOnce.Do(func() { close(h.stopped) })
	return nil
}

func (h *Hook) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-h.stopped:
			return
		case <-ticker.C:
			h.purgeExpired()
		}
	}
}

func (h *Hook) purgeExpired() {
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	for ip, st := range h.fails {
		if now.After(st.blockedUntil) && now.Sub(st.windowStart) > h.cfg.Window {
			delete(h.fails, ip)
		}
	}
}

// ID 返回 hook 标识。
func (h *Hook) ID() string { return "mqttauth" }

// Provides 声明处理认证与 ACL 检查。
func (h *Hook) Provides(b byte) bool {
	return bytes.Contains([]byte{
		mqtt.OnConnectAuthenticate,
		mqtt.OnACLCheck,
	}, []byte{b})
}

// OnACLCheck 恒允许:本 hook 只做连接级认证,不做主题级权限。
func (h *Hook) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
	return true
}

// OnConnectAuthenticate 校验 CONNECT 报文中的用户名/密码。
func (h *Hook) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
	if !h.cfg.Enabled {
		return true
	}
	ip := remoteHost(cl)
	if reason, ok := h.checkBlocked(ip); !ok {
		h.logEvent(map[string]any{
			"event": "mqtt_auth_rejected", "reason": reason,
			"client_id": cl.ID, "username": string(pk.Connect.Username), "remote_addr": cl.Net.Remote,
		})
		return false
	}

	username := string(pk.Connect.Username)
	password := string(pk.Connect.Password)
	allowed := h.authenticate(username, password)
	if allowed {
		h.resetFailures(ip)
		return true
	}
	blockedNow := h.recordFailure(ip, username)
	event := map[string]any{
		"event": "mqtt_auth_rejected", "reason": "invalid credentials",
		"client_id": cl.ID, "username": username, "remote_addr": cl.Net.Remote,
	}
	if blockedNow {
		event["reason"] = "invalid credentials; ip now blocked"
		event["blocked_for"] = h.cfg.BlockFor.String()
	}
	h.logEvent(event)
	return false
}

// authenticate 给出凭据判定;抽离以便单元测试。
// 未知用户名也执行 dummy bcrypt,使两种失败路径耗时一致。
func (h *Hook) authenticate(username, password string) bool {
	if !h.cfg.Enabled {
		return true
	}
	if username == "" && password == "" {
		return h.cfg.AllowAnonymous
	}
	if hash, found := h.users[username]; found {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	}
	_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(password))
	return false
}

func (h *Hook) checkBlocked(ip string) (string, bool) {
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	st, ok := h.fails[ip]
	if ok && now.Before(st.blockedUntil) {
		return fmt.Sprintf("ip blocked, retry after %s", time.Until(st.blockedUntil).Round(time.Second)), false
	}
	return "", true
}

// recordFailure 记录一次失败;达到阈值时返回 true 表示本次触发封禁。
func (h *Hook) recordFailure(ip, username string) bool {
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	st, ok := h.fails[ip]
	if !ok || now.Sub(st.windowStart) > h.cfg.Window {
		st = &failState{windowStart: now}
		h.fails[ip] = st
		if len(h.fails) > 4096 {
			h.purgeLocked(now)
		}
	}
	st.count++
	if st.count >= h.cfg.MaxFailures {
		st.blockedUntil = now.Add(h.cfg.BlockFor)
		st.count = 0
		st.windowStart = now
		return true
	}
	return false
}

func (h *Hook) resetFailures(ip string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.fails, ip)
}

func (h *Hook) purgeLocked(now time.Time) {
	for ip, st := range h.fails {
		if now.After(st.blockedUntil) && now.Sub(st.windowStart) > h.cfg.Window {
			delete(h.fails, ip)
		}
	}
}

func (h *Hook) logEvent(record map[string]any) {
	if h.cfg.LogEvent != nil {
		h.cfg.LogEvent(record)
	}
}

// remoteHost 提取客户端 IP,供失败限流使用。
func remoteHost(cl *mqtt.Client) string {
	if cl == nil {
		return "unknown"
	}
	remote := cl.Net.Remote
	if remote == "" && cl.Net.Conn != nil && cl.Net.Conn.RemoteAddr() != nil {
		remote = cl.Net.Conn.RemoteAddr().String()
	}
	if remote == "" {
		return "unknown"
	}
	if host, _, err := net.SplitHostPort(remote); err == nil {
		return host
	}
	return remote
}
