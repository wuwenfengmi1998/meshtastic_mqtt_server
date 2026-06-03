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

	for _, table := range []string{"nodeinfo_map", "text_message", "position", "telemetry", "routing", "traceroute"} {
		var name string
		if err := rawTestDB(t, st).QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name); err != nil {
			t.Fatalf("%s table missing: %v", table, err)
		}
		if name != table {
			t.Fatalf("table name = %q, want %s", name, table)
		}
	}

	var oldCount int
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'nodeinfo'").Scan(&oldCount); err != nil {
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
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM nodeinfo_map WHERE node_id = ?", "!12345678").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("node row count = %d, want 1", count)
	}

	var latestType, longName, content string
	if err := rawTestDB(t, st).QueryRow("SELECT latest_type, long_name, content_json FROM nodeinfo_map WHERE node_id = ?", "!12345678").Scan(&latestType, &longName, &content); err != nil {
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
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM nodeinfo_map WHERE node_id = ?", "!12345678").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("node row count = %d, want 1", count)
	}

	var latestType, userID, publicKey, longName, firmware, content string
	var latitude float64
	var opted sql.NullBool
	if err := rawTestDB(t, st).QueryRow("SELECT latest_type, user_id, public_key, long_name, firmware_version, latitude, has_opted_report_location, content_json FROM nodeinfo_map WHERE node_id = ?", "!12345678").Scan(&latestType, &userID, &publicKey, &longName, &firmware, &latitude, &opted, &content); err != nil {
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
	if err := rawTestDB(t, st).QueryRow("SELECT latest_type, user_id, long_name, firmware_version, latitude FROM nodeinfo_map WHERE node_id = ?", "!12345678").Scan(&latestType, &userID, &longName, &firmware, &latitude); err != nil {
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
	if err := rawTestDB(t, st).QueryRow("SELECT public_key FROM nodeinfo_map WHERE node_id = ?", "!00000001").Scan(&publicKey); err != nil {
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
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM text_message WHERE from_id = ?", "!12345678").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("text_message count = %d, want 2", count)
	}

	rows, err := rawTestDB(t, st).Query("SELECT id FROM text_message ORDER BY id")
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
	if err := rawTestDB(t, st).QueryRow("SELECT mqtt_client_id, mqtt_username, mqtt_listener, mqtt_remote_addr, mqtt_remote_host, mqtt_remote_port FROM text_message LIMIT 1").Scan(&clientID, &username, &listener, &remoteAddr, &remoteHost, &remotePort); err != nil {
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
	if err := rawTestDB(t, st).QueryRow("SELECT text, payload_hex FROM text_message LIMIT 1").Scan(&text, &payloadHex); err != nil {
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

func TestInsertPositionAppendsRows(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	clientInfo := mqttClientInfo{ClientID: "client-1", RemoteAddr: "127.0.0.1:54321", RemoteHost: "127.0.0.1", RemotePort: "54321"}
	if err := st.InsertPosition(positionTestRecord(), clientInfo); err != nil {
		t.Fatalf("first InsertPosition() error = %v", err)
	}
	if err := st.InsertPosition(positionTestRecord(), clientInfo); err != nil {
		t.Fatalf("second InsertPosition() error = %v", err)
	}

	var count int
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM position WHERE from_id = ?", "!12345678").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("position count = %d, want 2", count)
	}

	var latitude, longitude float64
	var altitude int64
	var locationSource, remoteHost string
	if err := rawTestDB(t, st).QueryRow("SELECT latitude, longitude, altitude, location_source, mqtt_remote_host FROM position ORDER BY id LIMIT 1").Scan(&latitude, &longitude, &altitude, &locationSource, &remoteHost); err != nil {
		t.Fatal(err)
	}
	if latitude != 42.5 || longitude != -83.1 || altitude != 200 || locationSource != "LOC_INTERNAL" || remoteHost != "127.0.0.1" {
		t.Fatalf("position row = lat %v lon %v alt %v source %q remote %q", latitude, longitude, altitude, locationSource, remoteHost)
	}
}

func TestInsertTelemetryAppendsRowsAndStoresMetricsJSON(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.InsertTelemetry(telemetryTestRecord(), mqttClientInfo{}); err != nil {
		t.Fatalf("InsertTelemetry() error = %v", err)
	}

	var telemetryType, metricsJSON, contentJSON string
	if err := rawTestDB(t, st).QueryRow("SELECT telemetry_type, metrics_json, content_json FROM telemetry LIMIT 1").Scan(&telemetryType, &metricsJSON, &contentJSON); err != nil {
		t.Fatal(err)
	}
	if telemetryType != "device_metrics" {
		t.Fatalf("telemetry_type = %q, want device_metrics", telemetryType)
	}
	if !strings.Contains(metricsJSON, "battery_level") || !strings.Contains(metricsJSON, "voltage") {
		t.Fatalf("metrics_json = %q, want battery_level and voltage", metricsJSON)
	}
	if !strings.Contains(contentJSON, "telemetry") {
		t.Fatalf("content_json = %q, want telemetry", contentJSON)
	}
}

func TestInsertRoutingAndTracerouteAppendRows(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.InsertRouting(routingTestRecord(), mqttClientInfo{}); err != nil {
		t.Fatalf("first InsertRouting() error = %v", err)
	}
	if err := st.InsertRouting(routingTestRecord(), mqttClientInfo{}); err != nil {
		t.Fatalf("second InsertRouting() error = %v", err)
	}
	if err := st.InsertTraceroute(tracerouteTestRecord(), mqttClientInfo{}); err != nil {
		t.Fatalf("first InsertTraceroute() error = %v", err)
	}
	if err := st.InsertTraceroute(tracerouteTestRecord(), mqttClientInfo{}); err != nil {
		t.Fatalf("second InsertTraceroute() error = %v", err)
	}

	for _, table := range []string{"routing", "traceroute"} {
		var count int
		if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM "+table+" WHERE from_id = ?", "!12345678").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Fatalf("%s count = %d, want 2", table, count)
		}

		var packetID int64
		var contentJSON string
		if err := rawTestDB(t, st).QueryRow("SELECT packet_id, content_json FROM "+table+" ORDER BY id LIMIT 1").Scan(&packetID, &contentJSON); err != nil {
			t.Fatal(err)
		}
		if packetID != 42 || !strings.Contains(contentJSON, table) {
			t.Fatalf("%s row packet_id=%d content_json=%q", table, packetID, contentJSON)
		}
	}
}

func TestInsertPacketTablesRequireFields(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	tests := []struct {
		name   string
		insert func(map[string]any) error
		record map[string]any
	}{
		{name: "position", insert: func(r map[string]any) error { return st.InsertPosition(r, mqttClientInfo{}) }, record: positionTestRecord()},
		{name: "telemetry", insert: func(r map[string]any) error { return st.InsertTelemetry(r, mqttClientInfo{}) }, record: telemetryTestRecord()},
		{name: "routing", insert: func(r map[string]any) error { return st.InsertRouting(r, mqttClientInfo{}) }, record: routingTestRecord()},
		{name: "traceroute", insert: func(r map[string]any) error { return st.InsertTraceroute(r, mqttClientInfo{}) }, record: tracerouteTestRecord()},
	}

	for _, tt := range tests {
		wrongType := cloneRecord(tt.record)
		wrongType["type"] = "text_message"
		if err := tt.insert(wrongType); err == nil || !strings.Contains(err.Error(), tt.name) {
			t.Fatalf("%s wrong type error = %v, want %s", tt.name, err, tt.name)
		}

		missingFrom := cloneRecord(tt.record)
		delete(missingFrom, "from")
		if err := tt.insert(missingFrom); err == nil || !strings.Contains(err.Error(), "from") {
			t.Fatalf("%s missing from error = %v, want from error", tt.name, err)
		}

		missingFromNum := cloneRecord(tt.record)
		delete(missingFromNum, "from_num")
		if err := tt.insert(missingFromNum); err == nil || !strings.Contains(err.Error(), "from_num") {
			t.Fatalf("%s missing from_num error = %v, want from_num error", tt.name, err)
		}

		missingTopic := cloneRecord(tt.record)
		delete(missingTopic, "topic")
		if err := tt.insert(missingTopic); err == nil || !strings.Contains(err.Error(), "topic") {
			t.Fatalf("%s missing topic error = %v, want topic error", tt.name, err)
		}
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

func rawTestDB(t *testing.T, st *store) *sql.DB {
	t.Helper()
	db, err := st.db.DB()
	if err != nil {
		t.Fatalf("st.db.DB() error = %v", err)
	}
	return db
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
	record := commonPacketTestRecord("text_message", "TEXT_MESSAGE_APP")
	record["text"] = text
	return record
}

func positionTestRecord() map[string]any {
	record := commonPacketTestRecord("position", "POSITION_APP")
	record["latitude"] = 42.5
	record["longitude"] = -83.1
	record["altitude"] = int32(200)
	record["time"] = uint32(123456)
	record["location_source"] = "LOC_INTERNAL"
	record["altitude_source"] = "ALT_INTERNAL"
	record["timestamp"] = uint32(123456)
	record["timestamp_millis_adjust"] = uint32(10)
	record["altitude_hae"] = int32(210)
	record["altitude_geoidal_separation"] = int32(20)
	record["pdop"] = 1.1
	record["hdop"] = 1.2
	record["vdop"] = 1.3
	record["gps_accuracy"] = uint32(1000)
	record["ground_speed"] = uint32(2)
	record["ground_track"] = 180.5
	record["fix_quality"] = uint32(1)
	record["fix_type"] = uint32(3)
	record["sats_in_view"] = uint32(8)
	record["sensor_id"] = uint32(1)
	record["next_update"] = uint32(60)
	record["seq_number"] = uint32(7)
	record["precision_bits"] = uint32(16)
	return record
}

func telemetryTestRecord() map[string]any {
	record := commonPacketTestRecord("telemetry", "TELEMETRY_APP")
	record["time"] = uint32(123456)
	record["telemetry_type"] = "device_metrics"
	record["metrics"] = map[string]any{"battery_level": 85, "voltage": 4.1}
	return record
}

func routingTestRecord() map[string]any {
	return commonPacketTestRecord("routing", "ROUTING_APP")
}

func tracerouteTestRecord() map[string]any {
	return commonPacketTestRecord("traceroute", "TRACEROUTE_APP")
}

func commonPacketTestRecord(recordType, portnum string) map[string]any {
	return map[string]any{
		"type":            recordType,
		"topic":           "msh/US/test",
		"channel_id":      "LongFast",
		"gateway_id":      "!gateway",
		"from":            "!12345678",
		"from_num":        uint32(0x12345678),
		"packet_id":       uint32(42),
		"packet_to":       "!ffffffff",
		"packet_to_num":   uint32(0xffffffff),
		"portnum":         portnum,
		"payload_len":     5,
		"payload_variant": "decoded",
		"via_mqtt":        true,
		"pki_encrypted":   false,
	}
}

func cloneRecord(record map[string]any) map[string]any {
	clone := make(map[string]any, len(record))
	for key, value := range record {
		clone[key] = value
	}
	return clone
}
