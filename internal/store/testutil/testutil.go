// Package testutil 提供给其它包测试使用的 store 临时实例工厂。
//
// 重构前 db_test.go 中的 openTestStore helper 被 8+ 个测试文件复用；
// 现在抽到这里，让 store 包外的测试也可以零样板地拿到一个临时 SQLite store。
package testutil

import (
	"path/filepath"
	"testing"

	"meshtastic_mqtt_server/internal/config"
	"meshtastic_mqtt_server/internal/store"
)

// OpenStore 返回一个写在 t.TempDir() 中的临时 SQLite store。
// 测试结束时调用方需要 defer st.Close()。
func OpenStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.OpenStore(config.DatabaseConfig{
		Driver: config.DriverSQLite,
		SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "mesh_mqtt_go.db")},
	}, false)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	return st
}
