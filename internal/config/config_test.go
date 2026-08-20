package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func loadRaw(t *testing.T, data string) rawConfig {
	t.Helper()
	var raw rawConfig
	if err := yaml.Unmarshal([]byte(data), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	return raw
}

func TestNormalizePlaintextPasswordBecomesHash(t *testing.T) {
	raw := loadRaw(t, `
mqtt:
  auth:
    enabled: true
    users:
      - username: mesh
        password: secret
`)
	cfg, changed := normalize(raw)
	if !changed {
		t.Fatal("plaintext password must mark config changed")
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("validate: %v", err)
	}
	u := cfg.MQTT.Auth.Users[0]
	if u.Password != "" {
		t.Error("plaintext must be cleared after hashing")
	}
	if !strings.HasPrefix(u.PasswordHash, "$2") {
		t.Errorf("expected bcrypt hash, got %q", u.PasswordHash)
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(out), "password: secret") {
		t.Error("serialized config must not contain the plaintext password")
	}
	if !strings.Contains(string(out), "password_hash") {
		t.Error("serialized config must contain password_hash")
	}
}

func TestValidateAuthErrors(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"启用但无用户", "mqtt:\n  auth:\n    enabled: true\n", "至少一个用户"},
		{"坏哈希", "mqtt:\n  auth:\n    enabled: true\n    users:\n      - username: a\n        password_hash: not-bcrypt\n", "bcrypt"},
		{"缺哈希", "mqtt:\n  auth:\n    enabled: true\n    users:\n      - username: a\n", "password_hash"},
		{"重复用户", "mqtt:\n  auth:\n    enabled: true\n    users:\n      - username: a\n        password_hash: " + fakeHash + "\n      - username: a\n        password_hash: " + fakeHash + "\n", "duplicate"},
	}
	for _, tc := range cases {
		cfg, _ := normalize(loadRaw(t, tc.yaml))
		err := Validate(cfg)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want error containing %q", tc.name, err, tc.want)
		}
	}
}

func TestValidateAuthDisabledOk(t *testing.T) {
	cfg, _ := normalize(loadRaw(t, "mqtt:\n"))
	if err := Validate(cfg); err != nil {
		t.Fatalf("disabled auth must pass: %v", err)
	}
	if cfg.MQTT.Auth.Enabled {
		t.Error("auth must default to disabled")
	}
}

const fakeHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
