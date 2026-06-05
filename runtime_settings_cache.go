package main

import (
	"fmt"
	"sync"
)

type runtimeSettingsCache struct {
	mu       sync.RWMutex
	settings runtimeSettingsSnapshot
}

func newRuntimeSettingsCache(store *store) (*runtimeSettingsCache, error) {
	cache := &runtimeSettingsCache{}
	if err := cache.Reload(store); err != nil {
		return nil, err
	}
	return cache, nil
}

func (c *runtimeSettingsCache) Reload(store *store) error {
	if store == nil {
		return fmt.Errorf("store is required")
	}
	settings, err := store.GetRuntimeSettings()
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.settings = settings
	c.mu.Unlock()
	return nil
}

func (c *runtimeSettingsCache) Snapshot() runtimeSettingsSnapshot {
	if c == nil {
		return runtimeSettingsSnapshot{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.settings
}

func (c *runtimeSettingsCache) AllowEncryptedForwarding() bool {
	return c.Snapshot().AllowEncryptedForwarding
}
