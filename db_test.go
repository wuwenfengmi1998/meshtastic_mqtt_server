package main

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenStoreCreatesNodeInfoTable(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	var name string
	if err := st.db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'nodeinfo'").Scan(&name); err != nil {
		t.Fatalf("nodeinfo table missing: %v", err)
	}
	if name != "nodeinfo" {
		t.Fatalf("table name = %q, want nodeinfo", name)
	}
}

func TestUpsertNodeInfoInsertsAndUpdatesSameNode(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	first := map[string]any{
		"type":        "nodeinfo",
		"from":        "!12345678",
		"from_num":    uint32(0x12345678),
		"user_id":     "!12345678",
		"long_name":   "first name",
		"short_name":  "fst",
		"hw_model":    "TEST_HW",
		"role":        "CLIENT",
		"is_licensed": true,
		"public_key":  "abcd",
	}
	if err := st.UpsertNodeInfo(first); err != nil {
		t.Fatalf("first UpsertNodeInfo() error = %v", err)
	}

	second := map[string]any{
		"type":        "nodeinfo",
		"from":        "!12345678",
		"from_num":    uint32(0x12345678),
		"user_id":     "!12345678",
		"long_name":   "second name",
		"short_name":  "snd",
		"hw_model":    "TEST_HW_2",
		"role":        "CLIENT_MUTE",
		"is_licensed": false,
		"public_key":  nil,
	}
	if err := st.UpsertNodeInfo(second); err != nil {
		t.Fatalf("second UpsertNodeInfo() error = %v", err)
	}

	var count int
	if err := st.db.QueryRow("SELECT COUNT(*) FROM nodeinfo WHERE node_id = ?", "!12345678").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("node row count = %d, want 1", count)
	}

	var longName, content string
	if err := st.db.QueryRow("SELECT long_name, content_json FROM nodeinfo WHERE node_id = ?", "!12345678").Scan(&longName, &content); err != nil {
		t.Fatal(err)
	}
	if longName != "second name" {
		t.Fatalf("long_name = %q, want second name", longName)
	}
	if !strings.Contains(content, "second name") {
		t.Fatalf("content_json = %q, want updated content", content)
	}
}

func TestUpsertNodeInfoRequiresNodeFields(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.UpsertNodeInfo(map[string]any{"type": "nodeinfo", "from_num": 1}); err == nil || !strings.Contains(err.Error(), "from") {
		t.Fatalf("missing from error = %v, want from error", err)
	}
	if err := st.UpsertNodeInfo(map[string]any{"type": "nodeinfo", "from": "!00000001"}); err == nil || !strings.Contains(err.Error(), "from_num") {
		t.Fatalf("missing from_num error = %v, want from_num error", err)
	}
}

func openTestStore(t *testing.T) *store {
	t.Helper()
	st, err := openStore(databaseConfig{
		Driver: databaseDriverSQLite,
		SQLite: sqliteConfig{Path: filepath.Join(t.TempDir(), "mesh_mqtt_go.db")},
	})
	if err != nil {
		t.Fatalf("openStore() error = %v", err)
	}
	return st
}

func TestNodeInfoFromRecordRejectsWrongType(t *testing.T) {
	_, err := nodeInfoFromRecord(map[string]any{"type": "text_message"})
	if err == nil {
		t.Fatalf("nodeInfoFromRecord() error = nil, want error")
	}
}

func TestNodeInfoNullablePublicKey(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	record := map[string]any{"type": "nodeinfo", "from": "!00000001", "from_num": 1, "public_key": nil}
	if err := st.UpsertNodeInfo(record); err != nil {
		t.Fatalf("UpsertNodeInfo() error = %v", err)
	}

	var publicKey sql.NullString
	if err := st.db.QueryRow("SELECT public_key FROM nodeinfo WHERE node_id = ?", "!00000001").Scan(&publicKey); err != nil {
		t.Fatal(err)
	}
	if publicKey.Valid {
		t.Fatalf("public_key valid = true, want null")
	}
}
