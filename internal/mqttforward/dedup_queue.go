package mqttforward

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

const dedupTTL = 15 * time.Second

type DedupQueue struct {
	mu      sync.Mutex
	entries map[string]time.Time
	stopCh  chan struct{}
}

func NewDedupQueue() *DedupQueue {
	return &DedupQueue{
		entries: make(map[string]time.Time),
		stopCh:  make(chan struct{}),
	}
}

func (dq *DedupQueue) TryForward(topic string, payload []byte) bool {
	hash := dedupHash(topic, payload)
	now := time.Now()
	dq.mu.Lock()
	defer dq.mu.Unlock()
	if expiry, ok := dq.entries[hash]; ok && now.Before(expiry) {
		return false
	}
	dq.entries[hash] = now.Add(dedupTTL)
	return true
}

func (dq *DedupQueue) Start() {
	go func() {
		ticker := time.NewTicker(dedupTTL)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				dq.cleanup()
			case <-dq.stopCh:
				return
			}
		}
	}()
}

func (dq *DedupQueue) Stop() {
	close(dq.stopCh)
}

func (dq *DedupQueue) cleanup() {
	now := time.Now()
	dq.mu.Lock()
	defer dq.mu.Unlock()
	for hash, expiry := range dq.entries {
		if now.After(expiry) {
			delete(dq.entries, hash)
		}
	}
}

func dedupHash(topic string, payload []byte) string {
	h := sha256.New()
	h.Write([]byte(topic))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}
