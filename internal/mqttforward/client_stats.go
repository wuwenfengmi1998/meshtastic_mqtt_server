package mqttforward

import "sync"

// ClientStats 在内存中维护每个 MQTT 客户端的收/发包数量。
// key 取自 mqtt.Client.ID（broker 内部唯一标识），客户端断开时由调用方调用 Delete 清除，
// 重新连接同一 client_id 会拿到一份新的零值计数，符合"断链就清空"。
type ClientStats struct {
	mu  sync.RWMutex
	all map[string]*clientCounter
}

type clientCounter struct {
	In  int64 // 客户端 → 服务器（broker 收到的报文数）
	Out int64 // 服务器 → 客户端（broker 发出的报文数）
}

// NewClientStats 返回一个空的统计器。
func NewClientStats() *ClientStats {
	return &ClientStats{all: make(map[string]*clientCounter)}
}

// IncIn 在 broker 收到客户端报文时调用。clientID 为空直接忽略。
func (s *ClientStats) IncIn(clientID string) {
	if s == nil || clientID == "" {
		return
	}
	s.mu.Lock()
	c, ok := s.all[clientID]
	if !ok {
		c = &clientCounter{}
		s.all[clientID] = c
	}
	c.In++
	s.mu.Unlock()
}

// IncOut 在 broker 向客户端发出报文时调用。
func (s *ClientStats) IncOut(clientID string) {
	if s == nil || clientID == "" {
		return
	}
	s.mu.Lock()
	c, ok := s.all[clientID]
	if !ok {
		c = &clientCounter{}
		s.all[clientID] = c
	}
	c.Out++
	s.mu.Unlock()
}

// Get 返回指定 clientID 当前的收/发包数量；不存在时返回 0,0。
func (s *ClientStats) Get(clientID string) (in, out int64) {
	if s == nil || clientID == "" {
		return 0, 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if c, ok := s.all[clientID]; ok {
		return c.In, c.Out
	}
	return 0, 0
}

// Delete 在客户端断开连接时清除其计数。重新连接同一 clientID 会从 0 重新计起。
func (s *ClientStats) Delete(clientID string) {
	if s == nil || clientID == "" {
		return
	}
	s.mu.Lock()
	delete(s.all, clientID)
	s.mu.Unlock()
}
