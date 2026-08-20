// Package ratelimit 提供按 key(IP、用户名等)计数的失败限速器。
//
// 语义:key 在 Window 时间窗内连续失败 MaxFailures 次后,封锁 BlockFor 时长;
// 成功调用 Reset 清空计数。并发安全,内部定期清理过期条目。
package ratelimit

import (
	"fmt"
	"sync"
	"time"
)

const (
	defaultMaxFailures = 5
	defaultWindow      = time.Minute
	defaultBlockFor    = 10 * time.Minute
)

// FailureLimiter 按 key 跟踪失败次数并执行临时封锁。
type FailureLimiter struct {
	mu        sync.Mutex
	max       int
	window    time.Duration
	blockFor  time.Duration
	fails     map[string]*failState
	now       func() time.Time
	stopped   chan struct{}
	stopOnce  sync.Once
	maxEntries int
}

type failState struct {
	count        int
	windowStart  time.Time
	blockedUntil time.Time
}

// Options 自定义限速参数,零值使用默认(5 次/分钟,封锁 10 分钟)。
type Options struct {
	MaxFailures int
	Window      time.Duration
	BlockFor    time.Duration
}

// New 构造限速器并启动清理协程;服务退出时应调用 Stop。
func New(opts Options) *FailureLimiter {
	if opts.MaxFailures <= 0 {
		opts.MaxFailures = defaultMaxFailures
	}
	if opts.Window <= 0 {
		opts.Window = defaultWindow
	}
	if opts.BlockFor <= 0 {
		opts.BlockFor = defaultBlockFor
	}
	l := &FailureLimiter{
		max:       opts.MaxFailures,
		window:    opts.Window,
		blockFor:  opts.BlockFor,
		fails:     make(map[string]*failState),
		now:       time.Now,
		stopped:   make(chan struct{}),
		maxEntries: 8192,
	}
	go l.cleanupLoop()
	return l
}

// Stop 结束清理协程。
func (l *FailureLimiter) Stop() error {
	l.stopOnce.Do(func() { close(l.stopped) })
	return nil
}

func (l *FailureLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-l.stopped:
			return
		case <-ticker.C:
			l.purge()
		}
	}
}

func (l *FailureLimiter) purge() {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, st := range l.fails {
		if now.After(st.blockedUntil) && now.Sub(st.windowStart) > l.window {
			delete(l.fails, key)
		}
	}
}

// Blocked 报告 key 当前是否被封锁;未封锁时返回剩余限制描述。
func (l *FailureLimiter) Blocked(key string) bool {
	if key == "" {
		return false
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	st, ok := l.fails[key]
	return ok && now.Before(st.blockedUntil)
}

// BlockedRemaining 返回封锁剩余时长;未封锁返回 0。
func (l *FailureLimiter) BlockedRemaining(key string) time.Duration {
	if key == "" {
		return 0
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	st, ok := l.fails[key]
	if !ok || now.After(st.blockedUntil) {
		return 0
	}
	return time.Until(st.blockedUntil)
}

// Fail 记录一次失败;达到阈值返回 true 表示本次触发封锁。
func (l *FailureLimiter) Fail(key string) bool {
	if key == "" {
		return false
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	st, ok := l.fails[key]
	if !ok || now.Sub(st.windowStart) > l.window {
		st = &failState{windowStart: now}
		l.fails[key] = st
		if len(l.fails) > l.maxEntries {
			l.purgeLocked(now)
		}
	}
	st.count++
	if st.count >= l.max {
		st.blockedUntil = now.Add(l.blockFor)
		st.count = 0
		st.windowStart = now
		return true
	}
	return false
}

// Reset 清除 key 的失败计数。
func (l *FailureLimiter) Reset(key string) {
	if key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, key)
}

func (l *FailureLimiter) purgeLocked(now time.Time) {
	for key, st := range l.fails {
		if now.After(st.blockedUntil) && now.Sub(st.windowStart) > l.window {
			delete(l.fails, key)
		}
	}
}

// BlockedError 构造统一的封锁提示文案。
func (l *FailureLimiter) BlockedError(key string) error {
	return fmt.Errorf("too many failed attempts, retry after %s", l.BlockedRemaining(key).Round(time.Second))
}
