package runtimesettings

import (
	"fmt"
	"sync"

	storepkg "meshtastic_mqtt_server/internal/store"
)

// Cache 把 runtime_settings 表中常用的开关缓存到内存中，避免每次拦截热路径
// 都查 DB。AdminRoute 修改后通过 Reload 重新加载。
type Cache struct {
	mu       sync.RWMutex
	settings storepkg.RuntimeSettingsSnapshot
}

// New 从 store 中加载初始快照并返回缓存。
func New(s *storepkg.Store) (*Cache, error) {
	cache := &Cache{}
	if err := cache.Reload(s); err != nil {
		return nil, err
	}
	return cache, nil
}

// Reload 重新读取数据库快照覆盖当前值。
func (c *Cache) Reload(s *storepkg.Store) error {
	if s == nil {
		return fmt.Errorf("store is required")
	}
	settings, err := s.GetRuntimeSettings()
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.settings = settings
	c.mu.Unlock()
	return nil
}

// Snapshot 返回当前快照的副本（结构体拷贝，调用方可以安全持有）。
func (c *Cache) Snapshot() storepkg.RuntimeSettingsSnapshot {
	if c == nil {
		return storepkg.RuntimeSettingsSnapshot{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.settings
}

// AllowEncryptedForwarding 是 mqtt 转发热路径上常被检查的标志位的快捷读法。
func (c *Cache) AllowEncryptedForwarding() bool {
	return c.Snapshot().AllowEncryptedForwarding
}
