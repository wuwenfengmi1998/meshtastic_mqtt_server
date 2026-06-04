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
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("database driver = %q, want sqlite", cfg.Database.Driver)
	}
	if cfg.Database.SQLite.Path == "" {
		t.Fatalf("sqlite path is empty")
	}
	if !cfg.Web.Enabled {
		t.Fatalf("web enabled = false, want true")
	}
	if cfg.Web.Port != 8080 {
		t.Fatalf("web port = %d, want 8080", cfg.Web.Port)
	}
	if cfg.Web.SocketPath != defaultWebSocketPath() {
		t.Fatalf("web socket path = %q, want %q", cfg.Web.SocketPath, defaultWebSocketPath())
	}
	if cfg.Web.StaticDir != "./dist" {
		t.Fatalf("web static dir = %q, want ./dist", cfg.Web.StaticDir)
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
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("database driver = %q, want sqlite", cfg.Database.Driver)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"host:", "tls:", "enabled:", "cert_file:", "key_file:", "meshtastic:", "psk:", "database:", "driver:", "sqlite:", "mysql:", "dsn:", "web:", "port:", "socket_path:", "static_dir:"} {
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
	content := "mqtt:\n  host: 127.0.0.1\n  port: 1885\n  tls:\n    enabled: false\n    cert_file: cert.pem\n    key_file: key.pem\nmeshtastic:\n  psk: AQ==\ndatabase:\n  driver: sqlite\n  sqlite:\n    path: test.db\n  mysql:\n    dsn: \"\"\n"
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

func TestLoadConfigPreservesExplicitWebFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mesh_mqtt_go", configFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	content := "web:\n  enabled: false\n  host: 127.0.0.1\n  port: 8081\n  static_dir: ./public\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.Web.Enabled {
		t.Fatalf("web enabled = true, want explicit false")
	}
	if cfg.Web.Host != "127.0.0.1" || cfg.Web.Port != 8081 || cfg.Web.StaticDir != "./public" {
		t.Fatalf("web config = %#v", cfg.Web)
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

func TestDefaultSQLitePathForGOOS(t *testing.T) {
	windowsPath := defaultSQLitePathForGOOS("windows")
	if !strings.Contains(windowsPath, filepath.Join("win", "etc", "mesh_mqtt_go", "mesh_mqtt_go.db")) {
		t.Fatalf("windows sqlite path = %q", windowsPath)
	}

	linuxPath := defaultSQLitePathForGOOS("linux")
	want := filepath.Join(string(filepath.Separator), "srv", "mesh_mqtt_go", "mesh_mqtt_go.db")
	if linuxPath != want {
		t.Fatalf("linux sqlite path = %q, want %q", linuxPath, want)
	}
}

func TestValidateConfigDatabase(t *testing.T) {
	cfg := defaultConfig()
	cfg.Database.Driver = "postgres"
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "database.driver") {
		t.Fatalf("invalid driver error = %v, want database.driver error", err)
	}

	cfg = defaultConfig()
	cfg.Database.SQLite.Path = ""
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "database.sqlite.path") {
		t.Fatalf("missing sqlite path error = %v, want database.sqlite.path error", err)
	}

	cfg = defaultConfig()
	cfg.Database.Driver = "mysql"
	cfg.Database.MySQL.DSN = ""
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "database.mysql.dsn") {
		t.Fatalf("missing mysql dsn error = %v, want database.mysql.dsn error", err)
	}
}

func TestValidateConfigWeb(t *testing.T) {
	cfg := defaultConfig()
	cfg.Web.SocketPath = ""
	cfg.Web.Port = 0
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "web port") {
		t.Fatalf("invalid web port error = %v, want web port error", err)
	}

	cfg = defaultConfig()
	cfg.Web.Port = 0
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("web socket with invalid port error = %v, want nil", err)
	}

	cfg = defaultConfig()
	cfg.Web.SocketPath = ""
	cfg.Web.Port = 0
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "web port") {
		t.Fatalf("invalid web port without socket error = %v, want web port error", err)
	}

	cfg = defaultConfig()
	cfg.Web.StaticDir = ""
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "web.static_dir") {
		t.Fatalf("missing web static dir error = %v, want web.static_dir error", err)
	}

	cfg = defaultConfig()
	cfg.Web.Enabled = false
	cfg.Web.Port = 0
	cfg.Web.StaticDir = ""
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("disabled web validate error = %v, want nil", err)
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
