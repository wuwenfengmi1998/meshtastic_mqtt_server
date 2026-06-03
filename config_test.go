package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigCreatesDefaultFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mesh_mqtt_go", configFileName)

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.MQTT.Host != "0.0.0.0" {
		t.Fatalf("host = %q, want 0.0.0.0", cfg.MQTT.Host)
	}
	if cfg.MQTT.Port != 1883 {
		t.Fatalf("port = %d, want 1883", cfg.MQTT.Port)
	}
	if cfg.MQTT.TLS.Enabled {
		t.Fatalf("tls enabled = true, want false")
	}
	if cfg.Meshtastic.PSK != "AQ==" {
		t.Fatalf("psk = %q, want AQ==", cfg.Meshtastic.PSK)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("default config was not written: %v", err)
	}
}

func TestLoadConfigFillsMissingFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mesh_mqtt_go", configFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("mqtt:\n  port: 1884\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.MQTT.Port != 1884 {
		t.Fatalf("port = %d, want 1884", cfg.MQTT.Port)
	}
	if cfg.MQTT.Host != "0.0.0.0" {
		t.Fatalf("host = %q, want 0.0.0.0", cfg.MQTT.Host)
	}
	if cfg.Meshtastic.PSK != "AQ==" {
		t.Fatalf("psk = %q, want AQ==", cfg.Meshtastic.PSK)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"host:", "tls:", "enabled:", "cert_file:", "key_file:", "meshtastic:", "psk:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("completed config missing %q in:\n%s", want, text)
		}
	}
}

func TestLoadConfigPreservesExplicitFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mesh_mqtt_go", configFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	content := "mqtt:\n  host: 127.0.0.1\n  port: 1885\n  tls:\n    enabled: false\n    cert_file: cert.pem\n    key_file: key.pem\nmeshtastic:\n  psk: AQ==\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.MQTT.TLS.Enabled {
		t.Fatalf("tls enabled = true, want explicit false")
	}
	if cfg.MQTT.TLS.CertFile != "cert.pem" || cfg.MQTT.TLS.KeyFile != "key.pem" {
		t.Fatalf("tls paths = %q/%q, want cert.pem/key.pem", cfg.MQTT.TLS.CertFile, cfg.MQTT.TLS.KeyFile)
	}
}

func TestLoadConfigMalformedYAMLDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mesh_mqtt_go", configFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	content := "mqtt:\n  port: [\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := loadConfig(path)
	if err == nil {
		t.Fatalf("loadConfig() error = nil, want parse error")
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != content {
		t.Fatalf("malformed config was overwritten: %q", string(data))
	}
}

func TestBuildTLSConfigDisabled(t *testing.T) {
	cfg, err := buildTLSConfig(tlsConfig{})
	if err != nil {
		t.Fatalf("buildTLSConfig() error = %v", err)
	}
	if cfg != nil {
		t.Fatalf("buildTLSConfig() = %#v, want nil", cfg)
	}
}

func TestBuildTLSConfigRequiresCertAndKey(t *testing.T) {
	_, err := buildTLSConfig(tlsConfig{Enabled: true})
	if err == nil || !strings.Contains(err.Error(), "cert_file") {
		t.Fatalf("missing cert error = %v, want cert_file error", err)
	}

	_, err = buildTLSConfig(tlsConfig{Enabled: true, CertFile: "cert.pem"})
	if err == nil || !strings.Contains(err.Error(), "key_file") {
		t.Fatalf("missing key error = %v, want key_file error", err)
	}
}
