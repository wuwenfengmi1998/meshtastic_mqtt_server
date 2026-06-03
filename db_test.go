package main

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenStoreCreatesTables(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	for _, table := range []string{"nodeinfo_map", "text_message"} {
		var name string
		if err := st.db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name); err != nil {
			t.Fatalf("%s table missing: %v", table, err)
		}
		if name != table {
			t.Fatalf("table name = %q, want %s", name, table)
		}
	}

	var oldCount int
	if err := st.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'nodeinfo'").Scan(&oldCount); err != nil {
		t.Fatal(err)
	}
	if oldCount != 0 {
		t.Fatalf("old nodeinfo table count = %d, want 0", oldCount)
	}
}

func TestUpsertNodeInfoMapInsertsAndUpdatesSameNode(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	first := nodeInfoRecord("first name")
	if err := st.UpsertNodeInfoMap(first); err != nil {
		t.Fatalf("first UpsertNodeInfoMap() error = %v", err)
	}

	second := nodeInfoRecord("second name")
	second["short_name"] = "snd"
	if err := st.UpsertNodeInfoMap(second); err != nil {
		t.Fatalf("second UpsertNodeInfoMap() error = %v", err)
	}

	var count int
	if err := st.db.QueryRow("SELECT COUNT(*) FROM nodeinfo_map WHERE node_id = ?", "!12345678").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("node row count = %d, want 1", count)
	}

	var latestType, longName, content string
	if err := st.db.QueryRow("SELECT latest_type, long_name, content_json FROM nodeinfo_map WHERE node_id = ?", "!12345678").Scan(&latestType, &longName, &content); err != nil {
		t.Fatal(err)
	}
	if latestType != "nodeinfo" {
		t.Fatalf("latest_type = %q, want nodeinfo", latestType)
	}
	if longName != "second name" {
		t.Fatalf("long_name = %q, want second name", longName)
	}
	if !strings.Contains(content, "second name") {
		t.Fatalf("content_json = %q, want updated content", content)
	}
}

func TestUpsertNodeInfoMapMergesNodeInfoThenMapReport(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.UpsertNodeInfoMap(nodeInfoRecord("node name")); err != nil {
		t.Fatalf("nodeinfo UpsertNodeInfoMap() error = %v", err)
	}
	if err := st.UpsertNodeInfoMap(mapReportRecord("map name")); err != nil {
		t.Fatalf("map_report UpsertNodeInfoMap() error = %v", err)
	}

	var count int
	if err := st.db.QueryRow("SELECT COUNT(*) FROM nodeinfo_map WHERE node_id = ?", "!12345678").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("node row count = %d, want 1", count)
	}

	var latestType, userID, publicKey, longName, firmware, content string
	var latitude float64
	var opted sql.NullBool
	if err := st.db.QueryRow("SELECT latest_type, user_id, public_key, long_name, firmware_version, latitude, has_opted_report_location, content_json FROM nodeinfo_map WHERE node_id = ?", "!12345678").Scan(&latestType, &userID, &publicKey, &longName, &firmware, &latitude, &opted, &content); err != nil {
		t.Fatal(err)
	}
	if latestType != "map_report" {
		t.Fatalf("latest_type = %q, want map_report", latestType)
	}
	if userID != "!12345678" || publicKey != "abcd" {
		t.Fatalf("nodeinfo fields not preserved: user_id=%q public_key=%q", userID, publicKey)
	}
	if longName != "map name" {
		t.Fatalf("long_name = %q, want map name", longName)
	}
	if firmware != "1.2.3" {
		t.Fatalf("firmware = %q, want 1.2.3", firmware)
	}
	if latitude != 42.5 {
		t.Fatalf("latitude = %v, want 42.5", latitude)
	}
	if !opted.Valid || opted.Bool {
		t.Fatalf("has_opted_report_location = %+v, want valid false", opted)
	}
	if !strings.Contains(content, "map_report") {
		t.Fatalf("content_json = %q, want latest map_report content", content)
	}
}

func TestUpsertNodeInfoMapMergesMapReportThenNodeInfo(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.UpsertNodeInfoMap(mapReportRecord("map name")); err != nil {
		t.Fatalf("map_report UpsertNodeInfoMap() error = %v", err)
	}
	if err := st.UpsertNodeInfoMap(nodeInfoRecord("node name")); err != nil {
		t.Fatalf("nodeinfo UpsertNodeInfoMap() error = %v", err)
	}

	var latestType, userID, longName, firmware string
	var latitude float64
	if err := st.db.QueryRow("SELECT latest_type, user_id, long_name, firmware_version, latitude FROM nodeinfo_map WHERE node_id = ?", "!12345678").Scan(&latestType, &userID, &longName, &firmware, &latitude); err != nil {
		t.Fatal(err)
	}
	if latestType != "nodeinfo" {
		t.Fatalf("latest_type = %q, want nodeinfo", latestType)
	}
	if userID != "!12345678" {
		t.Fatalf("user_id = %q, want !12345678", userID)
	}
	if longName != "node name" {
		t.Fatalf("long_name = %q, want node name", longName)
	}
	if firmware != "1.2.3" || latitude != 42.5 {
		t.Fatalf("map fields not preserved: firmware=%q latitude=%v", firmware, latitude)
	}
}

func TestUpsertNodeInfoMapRequiresNodeFields(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.UpsertNodeInfoMap(map[string]any{"type": "nodeinfo", "from_num": 1}); err == nil || !strings.Contains(err.Error(), "from") {
		t.Fatalf("missing from error = %v, want from error", err)
	}
	if err := st.UpsertNodeInfoMap(map[string]any{"type": "nodeinfo", "from": "!00000001"}); err == nil || !strings.Contains(err.Error(), "from_num") {
		t.Fatalf("missing from_num error = %v, want from_num error", err)
	}
}

func TestNodeInfoMapFromRecordRejectsWrongType(t *testing.T) {
	_, err := nodeInfoMapFromRecord(map[string]any{"type": "text_message"})
	if err == nil {
		t.Fatalf("nodeInfoMapFromRecord() error = nil, want error")
	}
}

func TestNodeInfoMapNullablePublicKey(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	record := map[string]any{"type": "nodeinfo", "from": "!00000001", "from_num": 1, "public_key": nil}
	if err := st.UpsertNodeInfoMap(record); err != nil {
		t.Fatalf("UpsertNodeInfoMap() error = %v", err)
	}

	var publicKey sql.NullString
	if err := st.db.QueryRow("SELECT public_key FROM nodeinfo_map WHERE node_id = ?", "!00000001").Scan(&publicKey); err != nil {
		t.Fatal(err)
	}
	if publicKey.Valid {
		t.Fatalf("public_key valid = true, want null")
	}
}

func TestInsertTextMessageAppendsRows(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	clientInfo := mqttClientInfo{ClientID: "client-1", Username: "user-1", Listener: "tcp", RemoteAddr: "127.0.0.1:54321", RemoteHost: "127.0.0.1", RemotePort: "54321"}
	if err := st.InsertTextMessage(textMessageTestRecord("hello"), clientInfo); err != nil {
		t.Fatalf("first InsertTextMessage() error = %v", err)
	}
	if err := st.InsertTextMessage(textMessageTestRecord("hello again"), clientInfo); err != nil {
		t.Fatalf("second InsertTextMessage() error = %v", err)
	}

	var count int
	if err := st.db.QueryRow("SELECT COUNT(*) FROM text_message WHERE from_id = ?", "!12345678").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("text_message count = %d, want 2", count)
	}

	rows, err := st.db.Query("SELECT id FROM text_message ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] <= 0 || ids[1] <= ids[0] {
		t.Fatalf("ids = %v, want increasing positive ids", ids)
	}
}

func TestInsertTextMessageStoresClientInfo(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	clientInfo := mqttClientInfo{ClientID: "client-1", Username: "user-1", Listener: "tcp", RemoteAddr: "127.0.0.1:54321", RemoteHost: "127.0.0.1", RemotePort: "54321"}
	if err := st.InsertTextMessage(textMessageTestRecord("hello"), clientInfo); err != nil {
		t.Fatalf("InsertTextMessage() error = %v", err)
	}

	var clientID, username, listener, remoteAddr, remoteHost, remotePort string
	if err := st.db.QueryRow("SELECT mqtt_client_id, mqtt_username, mqtt_listener, mqtt_remote_addr, mqtt_remote_host, mqtt_remote_port FROM text_message LIMIT 1").Scan(&clientID, &username, &listener, &remoteAddr, &remoteHost, &remotePort); err != nil {
		t.Fatal(err)
	}
	if clientID != "client-1" || username != "user-1" || listener != "tcp" || remoteAddr != "127.0.0.1:54321" || remoteHost != "127.0.0.1" || remotePort != "54321" {
		t.Fatalf("client info = %q %q %q %q %q %q", clientID, username, listener, remoteAddr, remoteHost, remotePort)
	}
}

func TestInsertTextMessageStoresPayloadHex(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	record := textMessageTestRecord(nil)
	record["payload_hex"] = "fffefd"
	if err := st.InsertTextMessage(record, mqttClientInfo{}); err != nil {
		t.Fatalf("InsertTextMessage() error = %v", err)
	}

	var text sql.NullString
	var payloadHex string
	if err := st.db.QueryRow("SELECT text, payload_hex FROM text_message LIMIT 1").Scan(&text, &payloadHex); err != nil {
		t.Fatal(err)
	}
	if text.Valid {
		t.Fatalf("text valid = true, want null")
	}
	if payloadHex != "fffefd" {
		t.Fatalf("payload_hex = %q, want fffefd", payloadHex)
	}
}

func TestInsertTextMessageRequiresFields(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.InsertTextMessage(map[string]any{"type": "nodeinfo"}, mqttClientInfo{}); err == nil || !strings.Contains(err.Error(), "text_message") {
		t.Fatalf("wrong type error = %v, want text_message error", err)
	}
	if err := st.InsertTextMessage(map[string]any{"type": "text_message", "from_num": 1, "topic": "msh/test"}, mqttClientInfo{}); err == nil || !strings.Contains(err.Error(), "from") {
		t.Fatalf("missing from error = %v, want from error", err)
	}
	if err := st.InsertTextMessage(map[string]any{"type": "text_message", "from": "!00000001", "topic": "msh/test"}, mqttClientInfo{}); err == nil || !strings.Contains(err.Error(), "from_num") {
		t.Fatalf("missing from_num error = %v, want from_num error", err)
	}
	if err := st.InsertTextMessage(map[string]any{"type": "text_message", "from": "!00000001", "from_num": 1}, mqttClientInfo{}); err == nil || !strings.Contains(err.Error(), "topic") {
		t.Fatalf("missing topic error = %v, want topic error", err)
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

func nodeInfoRecord(longName string) map[string]any {
	return map[string]any{
		"type":        "nodeinfo",
		"from":        "!12345678",
		"from_num":    uint32(0x12345678),
		"user_id":     "!12345678",
		"long_name":   longName,
		"short_name":  "nod",
		"hw_model":    "TEST_HW",
		"role":        "CLIENT",
		"is_licensed": true,
		"public_key":  "abcd",
	}
}

func mapReportRecord(longName string) map[string]any {
	return map[string]any{
		"type":                      "map_report",
		"from":                      "!12345678",
		"from_num":                  uint32(0x12345678),
		"long_name":                 longName,
		"short_name":                "map",
		"role":                      "CLIENT_MUTE",
		"hw_model":                  "TEST_HW_2",
		"firmware_version":          "1.2.3",
		"region":                    "US",
		"modem_preset":              "LONG_FAST",
		"latitude":                  42.5,
		"longitude":                 -83.1,
		"altitude":                  int32(200),
		"position_precision":        uint32(12),
		"num_online_local_nodes":    uint32(3),
		"has_opted_report_location": false,
	}
}

func textMessageTestRecord(text any) map[string]any {
	return map[string]any{
		"type":            "text_message",
		"topic":           "msh/US/test",
		"channel_id":      "LongFast",
		"gateway_id":      "!gateway",
		"from":            "!12345678",
		"from_num":        uint32(0x12345678),
		"text":            text,
		"packet_id":       uint32(42),
		"packet_to":       "!ffffffff",
		"packet_to_num":   uint32(0xffffffff),
		"portnum":         "TEXT_MESSAGE_APP",
		"payload_len":     5,
		"payload_variant": "decoded",
		"via_mqtt":        true,
		"pki_encrypted":   false,
	}
}
