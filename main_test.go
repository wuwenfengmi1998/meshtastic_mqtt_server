package main

import (
	"testing"

	mqtt "github.com/mochi-mqtt/server/v2"

	blockingpkg "meshtastic_mqtt_server/internal/blocking"
	storepkg "meshtastic_mqtt_server/internal/store"
	"meshtastic_mqtt_server/internal/store/testutil"
)

func TestMQTTClientInfoFromClientNil(t *testing.T) {
	info := mqttClientInfoFromClient(nil)
	if info != (storepkg.MQTTClientInfo{}) {
		t.Fatalf("info = %#v, want zero value", info)
	}
}

func TestMQTTClientInfoFromClientIPv4(t *testing.T) {
	info := mqttClientInfoFromClient(&mqtt.Client{
		ID:         "client-1",
		Properties: mqtt.ClientProperties{Username: []byte("user-1")},
		Net:        mqtt.ClientConnection{Listener: "tcp", Remote: "127.0.0.1:1234"},
	})

	if info.ClientID != "client-1" || info.Username != "user-1" || info.Listener != "tcp" {
		t.Fatalf("client fields = %#v", info)
	}
	if info.RemoteAddr != "127.0.0.1:1234" || info.RemoteHost != "127.0.0.1" || info.RemotePort != "1234" {
		t.Fatalf("remote fields = %#v", info)
	}
}

func TestMQTTClientInfoFromClientIPv6(t *testing.T) {
	info := mqttClientInfoFromClient(&mqtt.Client{Net: mqtt.ClientConnection{Remote: "[::1]:1234"}})
	if info.RemoteHost != "::1" || info.RemotePort != "1234" {
		t.Fatalf("remote fields = %#v, want host ::1 and port 1234", info)
	}
}

func TestMQTTClientInfoFromClientUnsplitRemote(t *testing.T) {
	info := mqttClientInfoFromClient(&mqtt.Client{Net: mqtt.ClientConnection{Remote: "localhost"}})
	if info.RemoteHost != "localhost" || info.RemotePort != "" {
		t.Fatalf("remote fields = %#v, want host localhost and empty port", info)
	}
}

// blockingViolationForRecord 的测试用真实 *Store + blocking.Cache 走完整路径，
// 不依赖 cache 的未导出字段。

func TestBlockingViolationForRecordNode(t *testing.T) {
	st := testutil.OpenStore(t)
	defer st.Close()
	nodeNum := int64(305419896)
	if _, err := st.CreateNodeBlocking("!12345678", &nodeNum, "blocked", true); err != nil {
		t.Fatalf("CreateNodeBlocking() error = %v", err)
	}
	cache, err := blockingpkg.New(st)
	if err != nil {
		t.Fatalf("blocking.New() error = %v", err)
	}
	record := map[string]any{"type": "position", "from": "!12345678", "from_num": uint32(305419896)}
	violation := blockingViolationForRecord(cache, record)
	if violation == nil || violation["blocking_type"] != "node" {
		t.Fatalf("blockingViolationForRecord() = %#v, want node violation", violation)
	}
}

func TestBlockingViolationForRecordForbiddenWordFields(t *testing.T) {
	st := testutil.OpenStore(t)
	defer st.Close()
	if _, err := st.CreateForbiddenWordBlocking("spam", "contains", false, "blocked", true); err != nil {
		t.Fatalf("CreateForbiddenWordBlocking() error = %v", err)
	}
	cache, err := blockingpkg.New(st)
	if err != nil {
		t.Fatalf("blocking.New() error = %v", err)
	}

	for _, tc := range []struct {
		name   string
		record map[string]any
		field  string
	}{
		{name: "text", record: map[string]any{"type": "text_message", "from": "!1", "text": "has SPAM"}, field: "text"},
		{name: "nodeinfo", record: map[string]any{"type": "nodeinfo", "from": "!1", "long_name": "has SPAM"}, field: "long_name"},
		{name: "map_report", record: map[string]any{"type": "map_report", "from": "!1", "long_name": "has SPAM"}, field: "long_name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			violation := blockingViolationForRecord(cache, tc.record)
			if violation == nil || violation["blocking_type"] != "forbidden_word" || violation["blocking_field"] != tc.field || violation["matched_word"] != "spam" {
				t.Fatalf("blockingViolationForRecord() = %#v, want forbidden word on %s", violation, tc.field)
			}
		})
	}
}

func TestBlockingViolationForRecordAllowed(t *testing.T) {
	st := testutil.OpenStore(t)
	defer st.Close()
	if _, err := st.CreateForbiddenWordBlocking("spam", "contains", false, "blocked", true); err != nil {
		t.Fatalf("CreateForbiddenWordBlocking() error = %v", err)
	}
	cache, err := blockingpkg.New(st)
	if err != nil {
		t.Fatalf("blocking.New() error = %v", err)
	}
	record := map[string]any{"type": "text_message", "from": "!1", "text": "hello"}
	if violation := blockingViolationForRecord(cache, record); violation != nil {
		t.Fatalf("blockingViolationForRecord() = %#v, want nil", violation)
	}
}
