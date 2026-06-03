package main

import (
	cryptotls "crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

const configFileName = "config.yaml"

type config struct {
	MQTT       mqttConfig       `yaml:"mqtt"`
	Meshtastic meshtasticConfig `yaml:"meshtastic"`
	Database   databaseConfig   `yaml:"database"`
	key        []byte
}

type mqttConfig struct {
	Host string    `yaml:"host"`
	Port int       `yaml:"port"`
	TLS  tlsConfig `yaml:"tls"`
}

type tlsConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type meshtasticConfig struct {
	PSK string `yaml:"psk"`
}

type databaseConfig struct {
	Driver string       `yaml:"driver"`
	SQLite sqliteConfig `yaml:"sqlite"`
	MySQL  mysqlConfig  `yaml:"mysql"`
}

type sqliteConfig struct {
	Path string `yaml:"path"`
}

type mysqlConfig struct {
	DSN string `yaml:"dsn"`
}

type rawConfig struct {
	MQTT       *rawMQTTConfig       `yaml:"mqtt"`
	Meshtastic *rawMeshtasticConfig `yaml:"meshtastic"`
	Database   *rawDatabaseConfig   `yaml:"database"`
}

type rawMQTTConfig struct {
	Host *string       `yaml:"host"`
	Port *int          `yaml:"port"`
	TLS  *rawTLSConfig `yaml:"tls"`
}

type rawTLSConfig struct {
	Enabled  *bool   `yaml:"enabled"`
	CertFile *string `yaml:"cert_file"`
	KeyFile  *string `yaml:"key_file"`
}

type rawMeshtasticConfig struct {
	PSK *string `yaml:"psk"`
}

type rawDatabaseConfig struct {
	Driver *string          `yaml:"driver"`
	SQLite *rawSQLiteConfig `yaml:"sqlite"`
	MySQL  *rawMySQLConfig  `yaml:"mysql"`
}

type rawSQLiteConfig struct {
	Path *string `yaml:"path"`
}

type rawMySQLConfig struct {
	DSN *string `yaml:"dsn"`
}

// defaultConfig 返回内置默认配置。
func defaultConfig() *config {
	return &config{
		MQTT: mqttConfig{
			Host: "0.0.0.0",
			Port: 1883,
			TLS: tlsConfig{
				Enabled:  false,
				CertFile: "",
				KeyFile:  "",
			},
		},
		Meshtastic: meshtasticConfig{
			PSK: "AQ==",
		},
		Database: databaseConfig{
			Driver: "sqlite",
			SQLite: sqliteConfig{Path: defaultSQLitePath()},
			MySQL:  mysqlConfig{DSN: ""},
		},
	}
}

// defaultConfigDir 根据操作系统返回配置目录。
func defaultConfigDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(".", "win", "etc", "mesh_mqtt_go")
	}
	return filepath.Join(string(filepath.Separator), "etc", "mesh_mqtt_go")
}

// defaultConfigPath 返回默认配置文件路径。
func defaultConfigPath() string {
	return filepath.Join(defaultConfigDir(), configFileName)
}

func defaultSQLitePath() string {
	return defaultSQLitePathForGOOS(runtime.GOOS)
}

func defaultSQLitePathForGOOS(goos string) string {
	if goos == "windows" {
		return filepath.Join(".", "win", "etc", "mesh_mqtt_go", "mesh_mqtt_go.db")
	}
	return filepath.Join(string(filepath.Separator), "srv", "mesh_mqtt_go", "mesh_mqtt_go.db")
}

// loadConfig 加载配置文件；文件不存在时生成，字段缺失时自动补全并写回。
func loadConfig(path string) (*config, error) {
	if path == "" {
		path = defaultConfigPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create config directory %s: %w", filepath.Dir(path), err)
	}

	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat config file %s: %w", path, err)
		}
		cfg := defaultConfig()
		if err := writeConfig(path, cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}

	cfg, changed := normalizeConfig(raw)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	if changed {
		if err := writeConfig(path, cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// normalizeConfig 将原始配置合并到默认配置，并标记是否补齐了缺失项。
func normalizeConfig(raw rawConfig) (*config, bool) {
	cfg := defaultConfig()
	changed := false

	if raw.MQTT == nil {
		changed = true
	} else {
		if raw.MQTT.Host == nil {
			changed = true
		} else {
			cfg.MQTT.Host = *raw.MQTT.Host
		}
		if raw.MQTT.Port == nil {
			changed = true
		} else {
			cfg.MQTT.Port = *raw.MQTT.Port
		}
		if raw.MQTT.TLS == nil {
			changed = true
		} else {
			if raw.MQTT.TLS.Enabled == nil {
				changed = true
			} else {
				cfg.MQTT.TLS.Enabled = *raw.MQTT.TLS.Enabled
			}
			if raw.MQTT.TLS.CertFile == nil {
				changed = true
			} else {
				cfg.MQTT.TLS.CertFile = *raw.MQTT.TLS.CertFile
			}
			if raw.MQTT.TLS.KeyFile == nil {
				changed = true
			} else {
				cfg.MQTT.TLS.KeyFile = *raw.MQTT.TLS.KeyFile
			}
		}
	}

	if raw.Meshtastic == nil {
		changed = true
	} else if raw.Meshtastic.PSK == nil {
		changed = true
	} else {
		cfg.Meshtastic.PSK = *raw.Meshtastic.PSK
	}

	if raw.Database == nil {
		changed = true
	} else {
		if raw.Database.Driver == nil {
			changed = true
		} else {
			cfg.Database.Driver = *raw.Database.Driver
		}
		if raw.Database.SQLite == nil {
			changed = true
		} else if raw.Database.SQLite.Path == nil {
			changed = true
		} else {
			cfg.Database.SQLite.Path = *raw.Database.SQLite.Path
		}
		if raw.Database.MySQL == nil {
			changed = true
		} else if raw.Database.MySQL.DSN == nil {
			changed = true
		} else {
			cfg.Database.MySQL.DSN = *raw.Database.MySQL.DSN
		}
	}

	return cfg, changed
}

func validateConfig(cfg *config) error {
	if cfg.MQTT.Port <= 0 || cfg.MQTT.Port > 65535 {
		return fmt.Errorf("invalid mqtt port %d: must be 1-65535", cfg.MQTT.Port)
	}
	switch cfg.Database.Driver {
	case "sqlite":
		if cfg.Database.SQLite.Path == "" {
			return fmt.Errorf("database.sqlite.path is required when database.driver is sqlite")
		}
	case "mysql":
		if cfg.Database.MySQL.DSN == "" {
			return fmt.Errorf("database.mysql.dsn is required when database.driver is mysql")
		}
	default:
		return fmt.Errorf("invalid database.driver %q: must be sqlite or mysql", cfg.Database.Driver)
	}
	return nil
}

func writeConfig(path string, cfg *config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config file %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config file %s: %w", path, err)
	}
	return nil
}

// buildTLSConfig 根据配置构造 mochi listener 使用的 TLS 设置。
func buildTLSConfig(cfg tlsConfig) (*cryptotls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.CertFile == "" {
		return nil, fmt.Errorf("mqtt tls cert_file is required when tls is enabled")
	}
	if cfg.KeyFile == "" {
		return nil, fmt.Errorf("mqtt tls key_file is required when tls is enabled")
	}

	cert, err := cryptotls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load mqtt tls certificate: %w", err)
	}
	return &cryptotls.Config{
		MinVersion:   cryptotls.VersionTLS12,
		Certificates: []cryptotls.Certificate{cert},
	}, nil
}
