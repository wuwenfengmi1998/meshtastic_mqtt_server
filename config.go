package main

import (
	cryptotls "crypto/tls"

	cfgpkg "meshtastic_mqtt_server/internal/config"
)

// 桥接到 internal/config — 让根目录其余文件无须修改即可继续使用旧的非导出名字。

const (
	configFileName       = cfgpkg.FileName
	databaseDriverSQLite = cfgpkg.DriverSQLite
	databaseDriverMySQL  = cfgpkg.DriverMySQL
)

// 旧的小写类型名通过别名继续可用。
type (
	config           = cfgpkg.Config
	mqttConfig       = cfgpkg.MQTTConfig
	tlsConfig        = cfgpkg.TLSConfig
	meshtasticConfig = cfgpkg.MeshtasticConfig
	databaseConfig   = cfgpkg.DatabaseConfig
	sqliteConfig     = cfgpkg.SQLiteConfig
	mysqlConfig      = cfgpkg.MySQLConfig
	webConfig        = cfgpkg.WebConfig
	webAdminConfig   = cfgpkg.WebAdminConfig
	aiConfig         = cfgpkg.AIConfig
)

func defaultConfig() *config                      { return cfgpkg.Default() }
func defaultConfigDir() string                    { return cfgpkg.DefaultDir() }
func defaultConfigPath() string                   { return cfgpkg.DefaultPath() }
func loadConfig(path string) (*config, error)     { return cfgpkg.Load(path) }
func writeConfig(path string, cfg *config) error  { return cfgpkg.Write(path, cfg) }
func validateConfig(cfg *config) error            { return cfgpkg.Validate(cfg) }
func clearWebSocketPathOnUnsupportedGOOS(cfg *config, goos string) bool {
	return cfgpkg.ClearWebSocketPathOnUnsupportedGOOS(cfg, goos)
}

func buildTLSConfig(cfg tlsConfig) (*cryptotls.Config, error) {
	return cfgpkg.BuildTLS(cfg)
}
