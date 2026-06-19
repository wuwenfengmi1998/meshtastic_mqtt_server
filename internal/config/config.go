package config

import (
	cryptotls "crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

const FileName = "config.yaml"

const (
	DriverSQLite = "sqlite"
	DriverMySQL  = "mysql"
)

type Config struct {
	MQTT       MQTTConfig       `yaml:"mqtt"`
	Meshtastic MeshtasticConfig `yaml:"meshtastic"`
	Database   DatabaseConfig   `yaml:"database"`
	Web        WebConfig        `yaml:"web"`
	AI         AIConfig         `yaml:"ai"`
	ConsoleLog ConsoleLogConfig `yaml:"console_log"`
	Key        []byte           `yaml:"-"`
}

type MQTTConfig struct {
	Host string    `yaml:"host"`
	Port int       `yaml:"port"`
	TLS  TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type MeshtasticConfig struct {
	PSK string `yaml:"psk"`
}

type DatabaseConfig struct {
	Driver string       `yaml:"driver"`
	SQLite SQLiteConfig `yaml:"sqlite"`
	MySQL  MySQLConfig  `yaml:"mysql"`
}

type SQLiteConfig struct {
	Path string `yaml:"path"`
}

type MySQLConfig struct {
	DSN string `yaml:"dsn"`
}

type WebConfig struct {
	Enabled         bool           `yaml:"enabled"`
	PortEnabled     bool           `yaml:"port_enabled"`
	SocketEnabled   bool           `yaml:"socket_enabled"`
	Host            string         `yaml:"host"`
	Port            int            `yaml:"port"`
	SocketPath      string         `yaml:"socket_path"`
	StaticDir       string         `yaml:"static_dir"`
	MapTileCacheDir string         `yaml:"map_tile_cache_dir"`
	Admin           WebAdminConfig `yaml:"admin"`
}

type WebAdminConfig struct {
	Username      string `yaml:"username"`
	Password      string `yaml:"password"`
	SessionSecret string `yaml:"session_secret"`
	SessionSecure bool   `yaml:"session_secure"`
}

type AIConfig struct {
	Enabled bool   `yaml:"enabled"`
	DataDir string `yaml:"data_dir"`
}

// ConsoleLogConfig 控制各模块是否在控制台打印日志。后续若新增模块，按需扩展。
type ConsoleLogConfig struct {
	Web        bool `yaml:"web"`
	MQTT       bool `yaml:"mqtt"`
	LLM        bool `yaml:"llm"`
	SQL        bool `yaml:"sql"`
	Meshtastic bool `yaml:"meshtastic"`
}

type rawConfig struct {
	MQTT       *rawMQTTConfig       `yaml:"mqtt"`
	Meshtastic *rawMeshtasticConfig `yaml:"meshtastic"`
	Database   *rawDatabaseConfig   `yaml:"database"`
	Web        *rawWebConfig        `yaml:"web"`
	AI         *rawAIConfig         `yaml:"ai"`
	ConsoleLog *rawConsoleLogConfig `yaml:"console_log"`
}

type rawConsoleLogConfig struct {
	Web        *bool `yaml:"web"`
	MQTT       *bool `yaml:"mqtt"`
	LLM        *bool `yaml:"llm"`
	SQL        *bool `yaml:"sql"`
	Meshtastic *bool `yaml:"meshtastic"`
}

type rawAIConfig struct {
	Enabled *bool   `yaml:"enabled"`
	DataDir *string `yaml:"data_dir"`
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

type rawWebConfig struct {
	Enabled         *bool              `yaml:"enabled"`
	PortEnabled     *bool              `yaml:"port_enabled"`
	SocketEnabled   *bool              `yaml:"socket_enabled"`
	Host            *string            `yaml:"host"`
	Port            *int               `yaml:"port"`
	SocketPath      *string            `yaml:"socket_path"`
	StaticDir       *string            `yaml:"static_dir"`
	MapTileCacheDir *string            `yaml:"map_tile_cache_dir"`
	Admin           *rawWebAdminConfig `yaml:"admin"`
}

type rawWebAdminConfig struct {
	Username      *string `yaml:"username"`
	Password      *string `yaml:"password"`
	SessionSecret *string `yaml:"session_secret"`
	SessionSecure *bool   `yaml:"session_secure"`
}

// Default 返回内置默认配置。
func Default() *Config {
	return &Config{
		MQTT: MQTTConfig{
			Host: "0.0.0.0",
			Port: 1883,
			TLS: TLSConfig{
				Enabled:  false,
				CertFile: "",
				KeyFile:  "",
			},
		},
		Meshtastic: MeshtasticConfig{
			PSK: "AQ==",
		},
		Database: DatabaseConfig{
			Driver: DriverSQLite,
			SQLite: SQLiteConfig{Path: defaultSQLitePath()},
			MySQL:  MySQLConfig{DSN: ""},
		},
		Web: WebConfig{
			Enabled:         true,
			PortEnabled:     true,
			SocketEnabled:   defaultWebSocketPath() != "",
			Host:            "0.0.0.0",
			Port:            8080,
			SocketPath:      defaultWebSocketPath(),
			StaticDir:       "./dist",
			MapTileCacheDir: defaultMapTileCacheDir(),
			Admin: WebAdminConfig{
				Username:      "admin",
				Password:      "admin",
				SessionSecret: "",
				SessionSecure: false,
			},
		},
		AI: AIConfig{
			Enabled: false,
			DataDir: defaultDataDir(),
		},
		ConsoleLog: ConsoleLogConfig{
			Web:        true,
			MQTT:       true,
			LLM:        true,
			SQL:        true,
			Meshtastic: true,
		},
	}
}

// DefaultDir 根据操作系统返回配置目录。
func DefaultDir() string {
	return defaultConfigDirForGOOS(runtime.GOOS)
}

func defaultConfigDirForGOOS(goos string) string {
	if useRelativeDefaultPath(goos) {
		return filepath.Join(".", "win", "etc", "mesh_mqtt_go")
	}
	return filepath.Join(string(filepath.Separator), "etc", "mesh_mqtt_go")
}

func useRelativeDefaultPath(goos string) bool {
	return goos == "windows" || goos == "darwin"
}

// DefaultPath 返回默认配置文件路径。
func DefaultPath() string {
	return filepath.Join(DefaultDir(), FileName)
}

func defaultSQLitePath() string {
	return defaultSQLitePathForGOOS(runtime.GOOS)
}

func defaultWebSocketPath() string {
	return defaultWebSocketPathForGOOS(runtime.GOOS)
}

func defaultMapTileCacheDir() string {
	return defaultMapTileCacheDirForGOOS(runtime.GOOS)
}

func defaultMapTileCacheDirForGOOS(goos string) string {
	if useRelativeDefaultPath(goos) {
		return filepath.Join(".", "win", "srv", "mesh_mqtt_go")
	}
	return filepath.Join(string(filepath.Separator), "srv", "mesh_mqtt_go")
}

func defaultWebSocketPathForGOOS(goos string) string {
	if goos == "windows" {
		return ""
	}
	if useRelativeDefaultPath(goos) {
		return filepath.Join(".", "win", "opt", "mesh_mqtt_go", "web.sock")
	}
	return filepath.Join(string(filepath.Separator), "opt", "mesh_mqtt_go", "web.sock")
}

func ClearWebSocketPathOnUnsupportedGOOS(cfg *Config, goos string) bool {
	if goos != "windows" {
		return false
	}
	changed := false
	if cfg.Web.SocketPath != "" {
		cfg.Web.SocketPath = ""
		changed = true
	}
	if cfg.Web.SocketEnabled {
		cfg.Web.SocketEnabled = false
		changed = true
	}
	return changed
}

func defaultSQLitePathForGOOS(goos string) string {
	if useRelativeDefaultPath(goos) {
		return filepath.Join(".", "win", "etc", "mesh_mqtt_go", "mesh_mqtt_go.db")
	}
	return filepath.Join(string(filepath.Separator), "srv", "mesh_mqtt_go", "mesh_mqtt_go.db")
}

func defaultDataDir() string {
	return defaultDataDirForGOOS(runtime.GOOS)
}

func defaultDataDirForGOOS(goos string) string {
	if useRelativeDefaultPath(goos) {
		return filepath.Join(".", "win", "srv", "mesh_mqtt_go")
	}
	return filepath.Join(string(filepath.Separator), "srv", "mesh_mqtt_go")
}

// Load 加载配置文件；文件不存在时生成，字段缺失时自动补全并写回。
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create config directory %s: %w", filepath.Dir(path), err)
	}

	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat config file %s: %w", path, err)
		}
		cfg := Default()
		if err := Write(path, cfg); err != nil {
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

	cfg, changed := normalize(raw)
	if ClearWebSocketPathOnUnsupportedGOOS(cfg, runtime.GOOS) {
		changed = true
	}
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	if changed {
		if err := Write(path, cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// normalize 将原始配置合并到默认配置，并标记是否补齐了缺失项。
func normalize(raw rawConfig) (*Config, bool) {
	cfg := Default()
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

	if raw.Web == nil {
		changed = true
	} else {
		if raw.Web.Enabled == nil {
			changed = true
		} else {
			cfg.Web.Enabled = *raw.Web.Enabled
		}
		if raw.Web.PortEnabled == nil {
			changed = true
		} else {
			cfg.Web.PortEnabled = *raw.Web.PortEnabled
		}
		if raw.Web.SocketEnabled == nil {
			changed = true
		} else {
			cfg.Web.SocketEnabled = *raw.Web.SocketEnabled
		}
		if raw.Web.Host == nil {
			changed = true
		} else {
			cfg.Web.Host = *raw.Web.Host
		}
		if raw.Web.Port == nil {
			changed = true
		} else {
			cfg.Web.Port = *raw.Web.Port
		}
		if raw.Web.SocketPath == nil {
			changed = true
		} else {
			cfg.Web.SocketPath = *raw.Web.SocketPath
		}
		if raw.Web.StaticDir == nil {
			changed = true
		} else {
			cfg.Web.StaticDir = *raw.Web.StaticDir
		}
		if raw.Web.MapTileCacheDir == nil {
			changed = true
		} else {
			cfg.Web.MapTileCacheDir = *raw.Web.MapTileCacheDir
		}
		if raw.Web.Admin == nil {
			changed = true
		} else {
			if raw.Web.Admin.Username == nil {
				changed = true
			} else {
				cfg.Web.Admin.Username = *raw.Web.Admin.Username
			}
			if raw.Web.Admin.Password == nil {
				changed = true
			} else {
				cfg.Web.Admin.Password = *raw.Web.Admin.Password
			}
			if raw.Web.Admin.SessionSecret == nil {
				changed = true
			} else {
				cfg.Web.Admin.SessionSecret = *raw.Web.Admin.SessionSecret
			}
			if raw.Web.Admin.SessionSecure == nil {
				changed = true
			} else {
				cfg.Web.Admin.SessionSecure = *raw.Web.Admin.SessionSecure
			}
		}
	}

	if raw.AI == nil {
		changed = true
	} else {
		if raw.AI.Enabled == nil {
			changed = true
		} else {
			cfg.AI.Enabled = *raw.AI.Enabled
		}
		if raw.AI.DataDir == nil {
			changed = true
		} else {
			cfg.AI.DataDir = *raw.AI.DataDir
		}
	}

	if raw.ConsoleLog == nil {
		changed = true
	} else {
		if raw.ConsoleLog.Web == nil {
			changed = true
		} else {
			cfg.ConsoleLog.Web = *raw.ConsoleLog.Web
		}
		if raw.ConsoleLog.MQTT == nil {
			changed = true
		} else {
			cfg.ConsoleLog.MQTT = *raw.ConsoleLog.MQTT
		}
		if raw.ConsoleLog.LLM == nil {
			changed = true
		} else {
			cfg.ConsoleLog.LLM = *raw.ConsoleLog.LLM
		}
		if raw.ConsoleLog.SQL == nil {
			changed = true
		} else {
			cfg.ConsoleLog.SQL = *raw.ConsoleLog.SQL
		}
		if raw.ConsoleLog.Meshtastic == nil {
			changed = true
		} else {
			cfg.ConsoleLog.Meshtastic = *raw.ConsoleLog.Meshtastic
		}
	}

	return cfg, changed
}

func Validate(cfg *Config) error {
	if cfg.MQTT.Port <= 0 || cfg.MQTT.Port > 65535 {
		return fmt.Errorf("invalid mqtt port %d: must be 1-65535", cfg.MQTT.Port)
	}
	switch cfg.Database.Driver {
	case DriverSQLite:
		if cfg.Database.SQLite.Path == "" {
			return fmt.Errorf("database.sqlite.path is required when database.driver is sqlite")
		}
	case DriverMySQL:
		if cfg.Database.MySQL.DSN == "" {
			return fmt.Errorf("database.mysql.dsn is required when database.driver is mysql")
		}
	default:
		return fmt.Errorf("invalid database.driver %q: must be sqlite or mysql", cfg.Database.Driver)
	}
	if cfg.Web.Enabled {
		if !cfg.Web.PortEnabled && !cfg.Web.SocketEnabled {
			return fmt.Errorf("web.port_enabled and web.socket_enabled cannot both be false when web is enabled")
		}
		if cfg.Web.PortEnabled && (cfg.Web.Port <= 0 || cfg.Web.Port > 65535) {
			return fmt.Errorf("invalid web port %d: must be 1-65535", cfg.Web.Port)
		}
		if cfg.Web.SocketEnabled && cfg.Web.SocketPath == "" {
			return fmt.Errorf("web.socket_path is required when web.socket_enabled is true")
		}
		if cfg.Web.StaticDir == "" {
			return fmt.Errorf("web.static_dir is required when web is enabled")
		}
		if cfg.Web.MapTileCacheDir == "" {
			return fmt.Errorf("web.map_tile_cache_dir is required when web is enabled")
		}
		if cfg.Web.Admin.Username == "" {
			return fmt.Errorf("web.admin.username is required when web is enabled")
		}
		if cfg.Web.Admin.Password == "" {
			return fmt.Errorf("web.admin.password is required when web is enabled")
		}
	}
	return nil
}

func Write(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config file %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config file %s: %w", path, err)
	}
	return nil
}

// BuildTLS 根据配置构造 mochi listener 使用的 TLS 设置。
func BuildTLS(cfg TLSConfig) (*cryptotls.Config, error) {
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
