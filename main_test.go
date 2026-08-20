package main

import (
	"testing"

	configpkg "meshtastic_mqtt_server/internal/config"
)

func TestIsLoopbackHost(t *testing.T) {
	for _, in := range []string{"localhost", "127.0.0.1", "::1", "LOCALHOST"} {
		if !isLoopbackHost(in) {
			t.Errorf("%q should be loopback", in)
		}
	}
	for _, in := range []string{"0.0.0.0", "", "192.168.1.5", " ", "meshmap.lmve.net"} {
		if isLoopbackHost(in) {
			t.Errorf("%q should not be loopback", in)
		}
	}
}

func TestGuardDefaultAdminPassword(t *testing.T) {	webEnabled := func(host, password string, portEnabled bool) *configpkg.Config {
		return &configpkg.Config{
			Web: configpkg.WebConfig{
				Enabled:     true,
				PortEnabled: portEnabled,
				Host:        host,
				Admin:       configpkg.WebAdminConfig{Username: "admin", Password: password},
			},
		}
	}

	cases := []struct {
		name    string
		cfg     *configpkg.Config
		reject  bool
	}{
		{"公网+默认口令", webEnabled("0.0.0.0", "admin", true), true},
		{"公网+自定义口令", webEnabled("0.0.0.0", "s3cret!", true), false},
		{"回环+默认口令", webEnabled("127.0.0.1", "admin", true), false},
		{"空口令", webEnabled("0.0.0.0", "", true), true},
		{"仅socket", webEnabled("0.0.0.0", "admin", false), false},
	}
	for _, tc := range cases {
		err := guardDefaultAdminPassword(tc.cfg)
		if tc.reject && err == nil {
			t.Errorf("%s: expected rejection", tc.name)
		}
		if !tc.reject && err != nil {
			t.Errorf("%s: unexpected rejection: %v", tc.name, err)
		}
	}
}
