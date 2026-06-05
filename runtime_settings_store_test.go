package main

import "testing"

func TestRuntimeSettingsDefaultAndUpdates(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	settings, err := st.GetRuntimeSettings()
	if err != nil {
		t.Fatalf("GetRuntimeSettings() error = %v", err)
	}
	if settings.AllowEncryptedForwarding {
		t.Fatalf("AllowEncryptedForwarding = true, want false")
	}

	if _, err := st.SetBoolRuntimeSetting(runtimeSettingAllowEncryptedForwarding, true, "test setting"); err != nil {
		t.Fatalf("SetBoolRuntimeSetting(true) error = %v", err)
	}
	settings, err = st.GetRuntimeSettings()
	if err != nil {
		t.Fatalf("GetRuntimeSettings() after true error = %v", err)
	}
	if !settings.AllowEncryptedForwarding {
		t.Fatalf("AllowEncryptedForwarding = false, want true")
	}

	if _, err := st.SetBoolRuntimeSetting(runtimeSettingAllowEncryptedForwarding, false, "test setting"); err != nil {
		t.Fatalf("SetBoolRuntimeSetting(false) error = %v", err)
	}
	settings, err = st.GetRuntimeSettings()
	if err != nil {
		t.Fatalf("GetRuntimeSettings() after false error = %v", err)
	}
	if settings.AllowEncryptedForwarding {
		t.Fatalf("AllowEncryptedForwarding = true, want false")
	}
}
