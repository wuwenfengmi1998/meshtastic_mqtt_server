package main

import (
	"path/filepath"
	"testing"

	storepkg "meshtastic_mqtt_server/internal/store"
)

// openTestStore 在根目录的测试中沿用旧函数名，但底层调用 internal/store 的实现。
func openTestStore(t *testing.T) *store {
	t.Helper()
	st, err := storepkg.OpenStore(databaseConfig{
		Driver: databaseDriverSQLite,
		SQLite: sqliteConfig{Path: filepath.Join(t.TempDir(), "mesh_mqtt_go.db")},
	})
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	return st
}
