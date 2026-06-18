package runtimesettings

import (
	"testing"

	storepkg "meshtastic_mqtt_server/internal/store"
	"meshtastic_mqtt_server/internal/store/testutil"
)

func openTestStore(t *testing.T) *storepkg.Store {
	return testutil.OpenStore(t)
}

func TestRuntimeSettingsCacheReload(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	cache, err := New(st)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if cache.AllowEncryptedForwarding() {
		t.Fatalf("AllowEncryptedForwarding() = true, want false")
	}

	if _, err := st.SetBoolRuntimeSetting(storepkg.RuntimeSettingAllowEncryptedForwarding, true, "test setting"); err != nil {
		t.Fatalf("SetBoolRuntimeSetting(true) error = %v", err)
	}
	if err := cache.Reload(st); err != nil {
		t.Fatalf("Reload() after true error = %v", err)
	}
	if !cache.AllowEncryptedForwarding() {
		t.Fatalf("AllowEncryptedForwarding() = false, want true")
	}

	if _, err := st.SetBoolRuntimeSetting(storepkg.RuntimeSettingAllowEncryptedForwarding, false, "test setting"); err != nil {
		t.Fatalf("SetBoolRuntimeSetting(false) error = %v", err)
	}
	if err := cache.Reload(st); err != nil {
		t.Fatalf("Reload() after false error = %v", err)
	}
	if cache.AllowEncryptedForwarding() {
		t.Fatalf("AllowEncryptedForwarding() = true, want false")
	}
}
