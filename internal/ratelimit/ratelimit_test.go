package ratelimit

import (
	"testing"
	"time"
)

func newTestLimiter(t *testing.T, opts Options) *FailureLimiter {
	t.Helper()
	if opts.MaxFailures == 0 {
		opts.MaxFailures = 3
	}
	l := New(opts)
	t.Cleanup(func() { _ = l.Stop() })
	return l
}

func TestFailBlocksAndExpires(t *testing.T) {
	l := newTestLimiter(t, Options{})
	now := time.Unix(1700000000, 0)
	l.now = func() time.Time { return now }

	for i := 0; i < 5; i++ {
		l.Fail("1.2.3.4")
	}
	if !l.Blocked("1.2.3.4") {
		t.Fatal("key should be blocked after 5 failures")
	}
	now = now.Add(11 * time.Minute)
	if l.Blocked("1.2.3.4") {
		t.Fatal("key should be unblocked after blockFor")
	}
}

func TestFailWindowResets(t *testing.T) {
	l := newTestLimiter(t, Options{})
	now := time.Unix(1700000000, 0)
	l.now = func() time.Time { return now }

	for i := 0; i < 4; i++ {
		l.Fail("ip")
	}
	now = now.Add(2 * time.Minute) // 超过 window,计数应清零
	l.Fail("ip")
	l.Fail("ip")
	if l.Blocked("ip") {
		t.Fatal("counts should reset after window passes")
	}
}

func TestSuccessResets(t *testing.T) {
	l := newTestLimiter(t, Options{})
	l.Fail("ip")
	l.Reset("ip")
	if l.Blocked("ip") {
		t.Fatal("reset must clear failures")
	}
}

func TestDifferentKeysIndependent(t *testing.T) {
	l := newTestLimiter(t, Options{})
	for i := 0; i < 10; i++ {
		l.Fail("a")
	}
	if !l.Blocked("a") {
		t.Fatal("a should be blocked")
	}
	if l.Blocked("b") {
		t.Fatal("b must not be affected by a")
	}
}

func TestEmptyKeyIgnored(t *testing.T) {
	l := newTestLimiter(t, Options{})
	for i := 0; i < 100; i++ {
		l.Fail("")
	}
	if l.Blocked("") {
		t.Fatal("empty key must not be tracked")
	}
}
