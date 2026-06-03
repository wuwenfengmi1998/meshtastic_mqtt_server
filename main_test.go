package main

import (
	"testing"

	mqtt "github.com/mochi-mqtt/server/v2"
)

func TestMQTTClientInfoFromClientNil(t *testing.T) {
	info := mqttClientInfoFromClient(nil)
	if info != (mqttClientInfo{}) {
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
