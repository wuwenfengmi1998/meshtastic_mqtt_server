package mqttauth

import (
	"testing"
	"time"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"
	"golang.org/x/crypto/bcrypt"
)

func hash(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return string(h)
}

func newTestHook(t *testing.T, cfg Config) *Hook {
	t.Helper()
	if cfg.MaxFailures == 0 {
		cfg.MaxFailures = 3
	}
	if cfg.Window == 0 {
		cfg.Window = time.Minute
	}
	if cfg.BlockFor == 0 {
		cfg.BlockFor = 5 * time.Minute
	}
	h := NewHook(cfg)
	t.Cleanup(func() { _ = h.Stop() })
	return h
}

func TestAuthenticate(t *testing.T) {
	h := newTestHook(t, Config{
		Enabled:        true,
		AllowAnonymous: false,
		Users:          []User{{Username: "mesh", PasswordHash: hash(t, "secret")}},
	})

	cases := []struct {
		name     string
		user     string
		pass     string
		allowed  bool
	}{
		{"正确凭据", "mesh", "secret", true},
		{"错误密码", "mesh", "wrong", false},
		{"未知用户", "nobody", "secret", false},
		{"匿名未启用", "", "", false},
		{"仅用户名", "mesh", "", false},
		{"仅密码", "", "secret", false},
	}
	for _, tc := range cases {
		if got := h.authenticate(tc.user, tc.pass); got != tc.allowed {
			t.Errorf("%s: authenticate(%q,%q)=%v want %v", tc.name, tc.user, tc.pass, got, tc.allowed)
		}
	}
}

func TestAuthenticateDisabledAllowsAll(t *testing.T) {
	h := newTestHook(t, Config{Enabled: false})
	for _, tc := range [][2]string{{"", ""}, {"any", "thing"}} {
		if !h.authenticate(tc[0], tc[1]) {
			t.Errorf("disabled hook must allow %v", tc)
		}
	}
}

func TestAuthenticateAnonymousAllowed(t *testing.T) {
	h := newTestHook(t, Config{Enabled: true, AllowAnonymous: true})
	if !h.authenticate("", "") {
		t.Error("anonymous should be allowed")
	}
	if h.authenticate("mesh", "x") {
		t.Error("unknown user must still be rejected")
	}
}

func TestOnConnectAuthenticateDisabled(t *testing.T) {
	h := newTestHook(t, Config{Enabled: false})
	if !h.OnConnectAuthenticate(&mqtt.Client{}, packets.Packet{}) {
		t.Error("disabled hook must return true")
	}
}

func TestFailureLimiterBlocks(t *testing.T) {
	h := newTestHook(t, Config{Enabled: true, Users: []User{{Username: "mesh", PasswordHash: hash(t, "secret")}}})
	now := time.Unix(1700000000, 0)
	h.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if h.OnConnectAuthenticate(&mqtt.Client{}, connectPacket("mesh", "wrong")) {
			t.Fatalf("attempt %d should fail", i)
		}
	}
	// 第 3 次失败触发封禁;之后即使凭据正确也应被拒。
	now = now.Add(time.Second)
	if h.OnConnectAuthenticate(&mqtt.Client{}, connectPacket("mesh", "secret")) {
		t.Fatal("blocked ip must be rejected even with valid credentials")
	}
	// 封禁到期后恢复。
	now = now.Add(6 * time.Minute)
	if !h.OnConnectAuthenticate(&mqtt.Client{}, connectPacket("mesh", "secret")) {
		t.Fatal("credentials should work after block expires")
	}
}

func TestSuccessResetsFailures(t *testing.T) {
	h := newTestHook(t, Config{Enabled: true, Users: []User{{Username: "mesh", PasswordHash: hash(t, "secret")}}})
	now := time.Unix(1700000000, 0)
	h.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		h.OnConnectAuthenticate(&mqtt.Client{}, connectPacket("mesh", "wrong"))
	}
	if !h.OnConnectAuthenticate(&mqtt.Client{}, connectPacket("mesh", "secret")) {
		t.Fatal("valid login should succeed")
	}
	// 成功后计数清零:再失败 2 次应有计数条目但未被封禁(阈值为 3)。
	now = now.Add(2 * time.Second)
	h.OnConnectAuthenticate(&mqtt.Client{}, connectPacket("mesh", "wrong"))
	h.OnConnectAuthenticate(&mqtt.Client{}, connectPacket("mesh", "wrong"))
	h.mu.Lock()
	st := h.fails["unknown"]
	h.mu.Unlock()
	if st != nil && !st.blockedUntil.IsZero() {
		t.Fatal("failures should have been reset by successful login")
	}
}

func TestRemoteHost(t *testing.T) {
	cl := &mqtt.Client{}
	if got := remoteHost(cl); got == "" {
		t.Error("remoteHost should never return empty string")
	}
}

func connectPacket(username, password string) packets.Packet {
	pk := packets.Packet{}
	pk.Connect.Username = []byte(username)
	pk.Connect.Password = []byte(password)
	return pk
}
