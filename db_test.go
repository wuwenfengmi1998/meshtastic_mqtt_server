package main

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestOpenStoreCreatesTables(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	for _, table := range []string{"users", "login_log", "discard_details", "node_blocking", "ip_blocking", "forbidden_word_blocking", "nodeinfo", "map_report", "text_message", "position", "telemetry", "routing", "traceroute"} {
		var name string
		if err := rawTestDB(t, st).QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name); err != nil {
			t.Fatalf("%s table missing: %v", table, err)
		}
		if name != table {
			t.Fatalf("table name = %q, want %s", name, table)
		}
	}

	var oldCount int
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'nodeinfo_map'").Scan(&oldCount); err != nil {
		t.Fatal(err)
	}
	if oldCount != 0 {
		t.Fatalf("nodeinfo_map table count = %d, want 0", oldCount)
	}
}

func TestUpsertNodeInfoInsertsAndUpdatesSameNode(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	first := nodeInfoTestRecord("first name")
	if err := st.UpsertNodeInfo(first); err != nil {
		t.Fatalf("first UpsertNodeInfo() error = %v", err)
	}

	second := nodeInfoTestRecord("second name")
	second["short_name"] = "snd"
	if err := st.UpsertNodeInfo(second); err != nil {
		t.Fatalf("second UpsertNodeInfo() error = %v", err)
	}

	var count int
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM nodeinfo WHERE node_id = ?", "!12345678").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("nodeinfo row count = %d, want 1", count)
	}

	var longName, shortName, content string
	if err := rawTestDB(t, st).QueryRow("SELECT long_name, short_name, content_json FROM nodeinfo WHERE node_id = ?", "!12345678").Scan(&longName, &shortName, &content); err != nil {
		t.Fatal(err)
	}
	if longName != "second name" || shortName != "snd" {
		t.Fatalf("nodeinfo names = %q/%q, want second name/snd", longName, shortName)
	}
	if !strings.Contains(content, "second name") {
		t.Fatalf("content_json = %q, want updated content", content)
	}
}

func TestUpsertMapReportInsertsAndUpdatesSameNode(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	first := mapReportTestRecord("first map")
	if err := st.UpsertMapReport(first); err != nil {
		t.Fatalf("first UpsertMapReport() error = %v", err)
	}

	second := mapReportTestRecord("second map")
	second["latitude"] = 43.5
	if err := st.UpsertMapReport(second); err != nil {
		t.Fatalf("second UpsertMapReport() error = %v", err)
	}

	var count int
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM map_report WHERE node_id = ?", "!12345678").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("map_report row count = %d, want 1", count)
	}

	var longName string
	var latitude float64
	var opted sql.NullBool
	if err := rawTestDB(t, st).QueryRow("SELECT long_name, latitude, has_opted_report_location FROM map_report WHERE node_id = ?", "!12345678").Scan(&longName, &latitude, &opted); err != nil {
		t.Fatal(err)
	}
	if longName != "second map" || latitude != 43.5 {
		t.Fatalf("map_report row = %q/%v, want second map/43.5", longName, latitude)
	}
	if !opted.Valid || opted.Bool {
		t.Fatalf("has_opted_report_location = %+v, want valid false", opted)
	}
}

func TestListMapReportsFiltersByBounds(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	inside := mapReportTestRecord("inside")
	inside["from"] = "!00000001"
	inside["from_num"] = uint32(1)
	inside["latitude"] = 10.5
	inside["longitude"] = 20.5
	outside := mapReportTestRecord("outside")
	outside["from"] = "!00000002"
	outside["from_num"] = uint32(2)
	outside["latitude"] = 50.0
	outside["longitude"] = 20.5
	missingCoords := mapReportTestRecord("missing coords")
	missingCoords["from"] = "!00000003"
	missingCoords["from_num"] = uint32(3)
	delete(missingCoords, "latitude")
	delete(missingCoords, "longitude")

	for _, record := range []map[string]any{inside, outside, missingCoords} {
		if err := st.UpsertMapReport(record); err != nil {
			t.Fatalf("UpsertMapReport() error = %v", err)
		}
	}

	minLat, maxLat := 10.0, 11.0
	minLng, maxLng := 20.0, 21.0
	opts := listOptions{Limit: 100, MinLat: &minLat, MaxLat: &maxLat, MinLng: &minLng, MaxLng: &maxLng}
	rows, err := st.ListMapReports(opts)
	if err != nil {
		t.Fatalf("ListMapReports() error = %v", err)
	}
	if len(rows) != 1 || rows[0].NodeID != "!00000001" {
		t.Fatalf("ListMapReports() = %+v, want only !00000001", rows)
	}
	total, err := st.CountMapReports(opts)
	if err != nil || total != 1 {
		t.Fatalf("CountMapReports() = %d, %v, want 1, nil", total, err)
	}
}

func TestListMapReportsFiltersAcrossAntimeridian(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	west := mapReportTestRecord("west")
	west["from"] = "!00000001"
	west["from_num"] = uint32(1)
	west["latitude"] = 0.0
	west["longitude"] = 175.0
	east := mapReportTestRecord("east")
	east["from"] = "!00000002"
	east["from_num"] = uint32(2)
	east["latitude"] = 0.0
	east["longitude"] = -175.0
	outside := mapReportTestRecord("outside")
	outside["from"] = "!00000003"
	outside["from_num"] = uint32(3)
	outside["latitude"] = 0.0
	outside["longitude"] = 0.0

	for _, record := range []map[string]any{west, east, outside} {
		if err := st.UpsertMapReport(record); err != nil {
			t.Fatalf("UpsertMapReport() error = %v", err)
		}
	}

	minLat, maxLat := -10.0, 10.0
	minLng, maxLng := 170.0, -170.0
	rows, err := st.ListMapReports(listOptions{Limit: 100, MinLat: &minLat, MaxLat: &maxLat, MinLng: &minLng, MaxLng: &maxLng})
	if err != nil {
		t.Fatalf("ListMapReports() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ListMapReports() length = %d, want 2: %+v", len(rows), rows)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.NodeID] = true
	}
	if !seen["!00000001"] || !seen["!00000002"] || seen["!00000003"] {
		t.Fatalf("seen nodes = %+v, want west/east only", seen)
	}
}

func TestListMapReportViewportReturnsPointsBelowThreshold(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	for index := 0; index < 3; index++ {
		record := mapReportTestRecord("point")
		record["from"] = "!0000000" + string(rune('1'+index))
		record["from_num"] = uint32(index + 1)
		record["latitude"] = float64(index)
		record["longitude"] = float64(index)
		if err := st.UpsertMapReport(record); err != nil {
			t.Fatalf("UpsertMapReport() error = %v", err)
		}
	}

	minLat, maxLat := -1.0, 5.0
	minLng, maxLng := -1.0, 5.0
	result, err := st.ListMapReportViewport(mapReportViewportOptions{
		ListOptions:      listOptions{MinLat: &minLat, MaxLat: &maxLat, MinLng: &minLng, MaxLng: &maxLng},
		Zoom:             8,
		Limit:            1000,
		ClusterThreshold: 10,
		TargetCells:      64,
	})
	if err != nil {
		t.Fatalf("ListMapReportViewport() error = %v", err)
	}
	if result.Mode != "points" || result.Total != 3 || len(result.Points) != 3 || len(result.Clusters) != 0 {
		t.Fatalf("viewport result = %+v, want 3 points", result)
	}
}

func TestListMapReportViewportReturnsClustersAboveThreshold(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	for index := 0; index < 4; index++ {
		record := mapReportTestRecord("cluster")
		record["from"] = "!0000000" + string(rune('1'+index))
		record["from_num"] = uint32(index + 1)
		record["latitude"] = 10.0 + float64(index)*0.01
		record["longitude"] = 20.0 + float64(index)*0.01
		if err := st.UpsertMapReport(record); err != nil {
			t.Fatalf("UpsertMapReport() error = %v", err)
		}
	}

	minLat, maxLat := 9.0, 11.0
	minLng, maxLng := 19.0, 21.0
	result, err := st.ListMapReportViewport(mapReportViewportOptions{
		ListOptions:      listOptions{MinLat: &minLat, MaxLat: &maxLat, MinLng: &minLng, MaxLng: &maxLng},
		Zoom:             4,
		Limit:            1000,
		ClusterThreshold: 2,
		TargetCells:      1,
	})
	if err != nil {
		t.Fatalf("ListMapReportViewport() error = %v", err)
	}
	if result.Mode != "clusters" || result.Total != 4 || len(result.Clusters) != 1 || result.Clusters[0].Count != 4 {
		t.Fatalf("viewport result = %+v, want one cluster with count 4", result)
	}
	if result.Clusters[0].Latitude < 10 || result.Clusters[0].Latitude > 10.1 || result.Clusters[0].Longitude < 20 || result.Clusters[0].Longitude > 20.1 {
		t.Fatalf("cluster center = %v/%v, want center near inserted points", result.Clusters[0].Latitude, result.Clusters[0].Longitude)
	}
}

func TestNodeInfoAndMapReportAreStoredSeparately(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.UpsertNodeInfo(nodeInfoTestRecord("node name")); err != nil {
		t.Fatalf("UpsertNodeInfo() error = %v", err)
	}
	if err := st.UpsertMapReport(mapReportTestRecord("map name")); err != nil {
		t.Fatalf("UpsertMapReport() error = %v", err)
	}

	var nodeLongName, userID, publicKey string
	if err := rawTestDB(t, st).QueryRow("SELECT long_name, user_id, public_key FROM nodeinfo WHERE node_id = ?", "!12345678").Scan(&nodeLongName, &userID, &publicKey); err != nil {
		t.Fatal(err)
	}
	if nodeLongName != "map name" || userID != "!12345678" || publicKey != "abcd" {
		t.Fatalf("nodeinfo row = %q/%q/%q, want synced map name plus node-only fields", nodeLongName, userID, publicKey)
	}

	var mapLongName, firmware string
	var latitude float64
	if err := rawTestDB(t, st).QueryRow("SELECT long_name, firmware_version, latitude FROM map_report WHERE node_id = ?", "!12345678").Scan(&mapLongName, &firmware, &latitude); err != nil {
		t.Fatal(err)
	}
	if mapLongName != "map name" || firmware != "1.2.3" || latitude != 42.5 {
		t.Fatalf("map_report row = %q/%q/%v, want map fields", mapLongName, firmware, latitude)
	}
}

func TestUpsertNodeInfoUpdatesExistingMapReportFields(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.UpsertMapReport(mapReportTestRecord("map name")); err != nil {
		t.Fatalf("UpsertMapReport() error = %v", err)
	}
	node := nodeInfoTestRecord("node name")
	node["short_name"] = "nod"
	node["hw_model"] = "NODE_HW"
	node["role"] = "CLIENT"
	if err := st.UpsertNodeInfo(node); err != nil {
		t.Fatalf("UpsertNodeInfo() error = %v", err)
	}

	var longName, shortName, hwModel, role, firmware string
	var latitude float64
	if err := rawTestDB(t, st).QueryRow("SELECT long_name, short_name, hw_model, role, firmware_version, latitude FROM map_report WHERE node_id = ?", "!12345678").Scan(&longName, &shortName, &hwModel, &role, &firmware, &latitude); err != nil {
		t.Fatal(err)
	}
	if longName != "node name" || shortName != "nod" || hwModel != "NODE_HW" || role != "CLIENT" || firmware != "1.2.3" || latitude != 42.5 {
		t.Fatalf("map_report row = %q/%q/%q/%q firmware %q lat %v, want node fields plus existing map fields", longName, shortName, hwModel, role, firmware, latitude)
	}
}

func TestUpsertNodeInfoDoesNotCreateMapReport(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.UpsertNodeInfo(nodeInfoTestRecord("node name")); err != nil {
		t.Fatalf("UpsertNodeInfo() error = %v", err)
	}

	var count int
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM map_report WHERE node_id = ?", "!12345678").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("map_report count = %d, want 0", count)
	}
}

func TestUpsertMapReportUpdatesExistingNodeInfoFields(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.UpsertNodeInfo(nodeInfoTestRecord("node name")); err != nil {
		t.Fatalf("UpsertNodeInfo() error = %v", err)
	}
	report := mapReportTestRecord("map name")
	report["short_name"] = "map"
	report["hw_model"] = "MAP_HW"
	report["role"] = "CLIENT_MUTE"
	if err := st.UpsertMapReport(report); err != nil {
		t.Fatalf("UpsertMapReport() error = %v", err)
	}

	var longName, shortName, hwModel, role, userID, publicKey string
	if err := rawTestDB(t, st).QueryRow("SELECT long_name, short_name, hw_model, role, user_id, public_key FROM nodeinfo WHERE node_id = ?", "!12345678").Scan(&longName, &shortName, &hwModel, &role, &userID, &publicKey); err != nil {
		t.Fatal(err)
	}
	if longName != "map name" || shortName != "map" || hwModel != "MAP_HW" || role != "CLIENT_MUTE" || userID != "!12345678" || publicKey != "abcd" {
		t.Fatalf("nodeinfo row = %q/%q/%q/%q user %q key %q, want map fields plus existing node-only fields", longName, shortName, hwModel, role, userID, publicKey)
	}
}

func TestUpsertMapReportDoesNotCreateNodeInfo(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.UpsertMapReport(mapReportTestRecord("map name")); err != nil {
		t.Fatalf("UpsertMapReport() error = %v", err)
	}

	var count int
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM nodeinfo WHERE node_id = ?", "!12345678").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("nodeinfo count = %d, want 0", count)
	}
}

func TestDeleteNodeDeletesNodeInfoAndMapReport(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.UpsertNodeInfo(nodeInfoTestRecord("node name")); err != nil {
		t.Fatalf("UpsertNodeInfo() error = %v", err)
	}
	if err := st.UpsertMapReport(mapReportTestRecord("map name")); err != nil {
		t.Fatalf("UpsertMapReport() error = %v", err)
	}
	if err := st.DeleteNode("!12345678"); err != nil {
		t.Fatalf("DeleteNode() error = %v", err)
	}

	var nodeCount, reportCount int
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM nodeinfo WHERE node_id = ?", "!12345678").Scan(&nodeCount); err != nil {
		t.Fatal(err)
	}
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM map_report WHERE node_id = ?", "!12345678").Scan(&reportCount); err != nil {
		t.Fatal(err)
	}
	if nodeCount != 0 || reportCount != 0 {
		t.Fatalf("nodeinfo/map_report counts = %d/%d, want 0/0", nodeCount, reportCount)
	}
	if err := st.DeleteNode("!12345678"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("DeleteNode(missing) error = %v, want record not found", err)
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

func TestUpsertMapReportRequiresNodeFields(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.UpsertMapReport(map[string]any{"type": "map_report", "from_num": 1}); err == nil || !strings.Contains(err.Error(), "from") {
		t.Fatalf("missing from error = %v, want from error", err)
	}
	if err := st.UpsertMapReport(map[string]any{"type": "map_report", "from": "!00000001"}); err == nil || !strings.Contains(err.Error(), "from_num") {
		t.Fatalf("missing from_num error = %v, want from_num error", err)
	}
}

func TestNodeInfoFromRecordRejectsWrongType(t *testing.T) {
	_, err := nodeInfoFromRecord(map[string]any{"type": "map_report"})
	if err == nil {
		t.Fatalf("nodeInfoFromRecord() error = nil, want error")
	}
}

func TestMapReportFromRecordRejectsWrongType(t *testing.T) {
	_, err := mapReportFromRecord(map[string]any{"type": "nodeinfo"})
	if err == nil {
		t.Fatalf("mapReportFromRecord() error = nil, want error")
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
	if err := rawTestDB(t, st).QueryRow("SELECT public_key FROM nodeinfo WHERE node_id = ?", "!00000001").Scan(&publicKey); err != nil {
		t.Fatal(err)
	}
	if publicKey.Valid {
		t.Fatalf("public_key valid = true, want null")
	}
}

func TestEnsureDefaultAdminCreatesAdminUser(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.EnsureDefaultAdmin("admin", "admin"); err != nil {
		t.Fatalf("EnsureDefaultAdmin() error = %v", err)
	}

	user, err := st.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}
	if user.Role != adminRole {
		t.Fatalf("role = %q, want admin", user.Role)
	}
	if user.PasswordHash == "admin" || user.PasswordHash == "" {
		t.Fatalf("password hash = %q, want bcrypt hash", user.PasswordHash)
	}
	if !verifyPassword(user.PasswordHash, "admin") {
		t.Fatalf("admin password did not verify")
	}
}

func TestEnsureDefaultAdminDoesNotOverwriteExistingUser(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.EnsureDefaultAdmin("admin", "first"); err != nil {
		t.Fatalf("first EnsureDefaultAdmin() error = %v", err)
	}
	if err := st.EnsureDefaultAdmin("admin", "second"); err != nil {
		t.Fatalf("second EnsureDefaultAdmin() error = %v", err)
	}
	user, err := st.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}
	if !verifyPassword(user.PasswordHash, "first") || verifyPassword(user.PasswordHash, "second") {
		t.Fatalf("admin password was overwritten")
	}
}

func TestCreateAdminUserCreatesHashedAdmin(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	user, err := st.CreateAdminUser("new-admin", "secret")
	if err != nil {
		t.Fatalf("CreateAdminUser() error = %v", err)
	}
	if user.Username != "new-admin" || user.Role != adminRole {
		t.Fatalf("user = %#v, want new-admin admin", user)
	}
	if user.PasswordHash == "secret" || !verifyPassword(user.PasswordHash, "secret") {
		t.Fatalf("password hash did not verify")
	}
}

func TestCreateAdminUserRejectsDuplicateUsername(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if _, err := st.CreateAdminUser("new-admin", "secret"); err != nil {
		t.Fatalf("first CreateAdminUser() error = %v", err)
	}
	if _, err := st.CreateAdminUser("new-admin", "secret"); !errors.Is(err, errUserAlreadyExists) {
		t.Fatalf("duplicate CreateAdminUser() error = %v, want errUserAlreadyExists", err)
	}
}

func TestUpdateUserPasswordChangesHash(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	user, err := st.CreateAdminUser("new-admin", "old-secret")
	if err != nil {
		t.Fatalf("CreateAdminUser() error = %v", err)
	}
	oldHash := user.PasswordHash
	updated, err := st.UpdateUserPassword(user.ID, "new-secret")
	if err != nil {
		t.Fatalf("UpdateUserPassword() error = %v", err)
	}
	if updated.PasswordHash == oldHash {
		t.Fatalf("password hash did not change")
	}
	if verifyPassword(updated.PasswordHash, "old-secret") || !verifyPassword(updated.PasswordHash, "new-secret") {
		t.Fatalf("updated password verification mismatch")
	}
}

func TestUpdateUserPasswordMissingUser(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if _, err := st.UpdateUserPassword(999, "new-secret"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("UpdateUserPassword() error = %v, want record not found", err)
	}
}

func TestInsertAndListLoginLogs(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	userID := uint64(1)
	if err := st.InsertLoginLog(loginLogRecord{Username: "admin", UserID: &userID, Success: true, Reason: "success", RemoteAddr: "127.0.0.1:1234", RemoteHost: "127.0.0.1", UserAgent: "test-agent"}); err != nil {
		t.Fatalf("InsertLoginLog(success) error = %v", err)
	}
	if err := st.InsertLoginLog(loginLogRecord{Username: "admin", Success: false, Reason: "invalid username or password", RemoteAddr: "127.0.0.1:1235", RemoteHost: "127.0.0.1", UserAgent: "test-agent"}); err != nil {
		t.Fatalf("InsertLoginLog(failure) error = %v", err)
	}

	logs, err := st.ListLoginLogs(listOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListLoginLogs() error = %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("login logs len = %d, want 2", len(logs))
	}
	if logs[0].ID <= logs[1].ID {
		t.Fatalf("login logs not newest first: ids %d, %d", logs[0].ID, logs[1].ID)
	}
	if logs[0].Success || logs[0].Reason != "invalid username or password" {
		t.Fatalf("latest log = %#v, want failure", logs[0])
	}
	if logs[1].UserID == nil || *logs[1].UserID != userID || !logs[1].Success {
		t.Fatalf("success log = %#v, want user id and success", logs[1])
	}
}

func TestInsertDiscardDetailsStoresRawBase64AndClientInfo(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	raw := []byte{0xff, 0x00, 0x01}
	clientInfo := mqttClientInfo{ClientID: "client-1", Username: "user-1", Listener: "tcp", RemoteAddr: "127.0.0.1:54321", RemoteHost: "127.0.0.1", RemotePort: "54321"}
	record := map[string]any{"topic": "msh/US/test", "error": "protobuf decode failed", "payload_len": len(raw)}
	if err := st.InsertDiscardDetails(record, raw, clientInfo); err != nil {
		t.Fatalf("InsertDiscardDetails() error = %v", err)
	}

	var topic, errorText, rawBase64, clientID, username, listener, remoteAddr, remoteHost, remotePort, contentJSON string
	var payloadLen int64
	if err := rawTestDB(t, st).QueryRow("SELECT topic, error, payload_len, raw_base64, mqtt_client_id, mqtt_username, mqtt_listener, mqtt_remote_addr, mqtt_remote_host, mqtt_remote_port, content_json FROM discard_details LIMIT 1").Scan(&topic, &errorText, &payloadLen, &rawBase64, &clientID, &username, &listener, &remoteAddr, &remoteHost, &remotePort, &contentJSON); err != nil {
		t.Fatal(err)
	}
	if topic != "msh/US/test" || errorText != "protobuf decode failed" || payloadLen != int64(len(raw)) || rawBase64 != base64.StdEncoding.EncodeToString(raw) {
		t.Fatalf("discard details row = topic %q error %q len %d raw %q", topic, errorText, payloadLen, rawBase64)
	}
	if clientID != "client-1" || username != "user-1" || listener != "tcp" || remoteAddr != "127.0.0.1:54321" || remoteHost != "127.0.0.1" || remotePort != "54321" {
		t.Fatalf("client info = %q %q %q %q %q %q", clientID, username, listener, remoteAddr, remoteHost, remotePort)
	}
	if !strings.Contains(contentJSON, "protobuf decode failed") {
		t.Fatalf("content_json = %q, want error", contentJSON)
	}
}

func TestListDiscardDetailsOrdersNewestFirst(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.InsertDiscardDetails(map[string]any{"topic": "first", "error": "first"}, []byte{1}, mqttClientInfo{}); err != nil {
		t.Fatalf("first InsertDiscardDetails() error = %v", err)
	}
	if err := st.InsertDiscardDetails(map[string]any{"topic": "second", "error": "second"}, []byte{2}, mqttClientInfo{}); err != nil {
		t.Fatalf("second InsertDiscardDetails() error = %v", err)
	}
	rows, err := st.ListDiscardDetails(listOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListDiscardDetails() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("discard details len = %d, want 2", len(rows))
	}
	if rows[0].ID <= rows[1].ID || rows[0].Topic != "second" {
		t.Fatalf("discard details order = %#v", rows)
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

func TestDeleteTextMessageDeletesRows(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.InsertTextMessage(textMessageTestRecord("hello"), mqttClientInfo{}); err != nil {
		t.Fatalf("InsertTextMessage() error = %v", err)
	}
	var id uint64
	if err := rawTestDB(t, st).QueryRow("SELECT id FROM text_message LIMIT 1").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteTextMessage(id); err != nil {
		t.Fatalf("DeleteTextMessage() error = %v", err)
	}
	var count int
	if err := rawTestDB(t, st).QueryRow("SELECT COUNT(*) FROM text_message WHERE id = ?", id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("text_message count = %d, want 0", count)
	}
	if err := st.DeleteTextMessage(id); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("DeleteTextMessage(missing) error = %v, want record not found", err)
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

func TestInsertPositionCreatesMapReportWhenMissing(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.InsertPosition(positionTestRecord(), mqttClientInfo{}); err != nil {
		t.Fatalf("InsertPosition() error = %v", err)
	}

	var nodeID string
	var nodeNum int64
	var latitude, longitude float64
	var altitude, precision int64
	if err := rawTestDB(t, st).QueryRow("SELECT node_id, node_num, latitude, longitude, altitude, position_precision FROM map_report WHERE node_id = ?", "!12345678").Scan(&nodeID, &nodeNum, &latitude, &longitude, &altitude, &precision); err != nil {
		t.Fatal(err)
	}
	if nodeID != "!12345678" || nodeNum != 0x12345678 || latitude != 42.5 || longitude != -83.1 || altitude != 200 || precision != 16 {
		t.Fatalf("map_report from position = %q/%d lat %v lon %v alt %v precision %v", nodeID, nodeNum, latitude, longitude, altitude, precision)
	}
}

func TestInsertPositionUpdatesExistingMapReportCoordinates(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if err := st.UpsertMapReport(mapReportTestRecord("map name")); err != nil {
		t.Fatalf("UpsertMapReport() error = %v", err)
	}
	position := positionTestRecord()
	position["latitude"] = 30.25
	position["longitude"] = 120.75
	position["altitude"] = int32(88)
	position["precision_bits"] = uint32(10)
	if err := st.InsertPosition(position, mqttClientInfo{}); err != nil {
		t.Fatalf("InsertPosition() error = %v", err)
	}

	var longName string
	var latitude, longitude float64
	var altitude, precision int64
	if err := rawTestDB(t, st).QueryRow("SELECT long_name, latitude, longitude, altitude, position_precision FROM map_report WHERE node_id = ?", "!12345678").Scan(&longName, &latitude, &longitude, &altitude, &precision); err != nil {
		t.Fatal(err)
	}
	if longName != "map name" || latitude != 30.25 || longitude != 120.75 || altitude != 88 || precision != 10 {
		t.Fatalf("map_report after position = %q lat %v lon %v alt %v precision %v", longName, latitude, longitude, altitude, precision)
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

func nodeInfoTestRecord(longName string) map[string]any {
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

func mapReportTestRecord(longName string) map[string]any {
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
