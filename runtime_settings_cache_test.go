package main

import "testing"

func TestRuntimeSettingsCacheReload(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	cache, err := newRuntimeSettingsCache(st)
	if err != nil {
		t.Fatalf("newRuntimeSettingsCache() error = %v", err)
	}
	if cache.AllowEncryptedForwarding() {
		t.Fatalf("AllowEncryptedForwarding() = true, want false")
	}

	if _, err := st.SetBoolRuntimeSetting(runtimeSettingAllowEncryptedForwarding, true, "test setting"); err != nil {
		t.Fatalf("SetBoolRuntimeSetting(true) error = %v", err)
	}
	if err := cache.Reload(st); err != nil {
		t.Fatalf("Reload() after true error = %v", err)
	}
	if !cache.AllowEncryptedForwarding() {
		t.Fatalf("AllowEncryptedForwarding() = false, want true")
	}

	if _, err := st.SetBoolRuntimeSetting(runtimeSettingAllowEncryptedForwarding, false, "test setting"); err != nil {
		t.Fatalf("SetBoolRuntimeSetting(false) error = %v", err)
	}
	if err := cache.Reload(st); err != nil {
		t.Fatalf("Reload() after false error = %v", err)
	}
	if cache.AllowEncryptedForwarding() {
		t.Fatalf("AllowEncryptedForwarding() = true, want false")
	}
}
