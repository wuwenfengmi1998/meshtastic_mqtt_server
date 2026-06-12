package mqtpp

import "testing"

func TestBuildTextMessageServiceEnvelopeRoundTrip(t *testing.T) {
	key, err := ExpandPSK("AQ==")
	if err != nil {
		t.Fatalf("ExpandPSK() error = %v", err)
	}

	raw, err := BuildTextMessageServiceEnvelope(TextMessageBuildOptions{
		PacketBuildOptions: PacketBuildOptions{
			FromNodeNum: 0x12345678,
			ToNodeNum:   NodeNumBroadcast,
			PacketID:    0x87654321,
			ChannelID:   "LongFast",
			GatewayID:   "!12345678",
			PSK:         key,
			Encrypt:     true,
			ViaMQTT:     true,
		},
		Text: "hello from bot",
	})
	if err != nil {
		t.Fatalf("BuildTextMessageServiceEnvelope() error = %v", err)
	}

	valid, _, record := MQTTPP("msh/2/e/LongFast/!12345678", raw, key, Options{})
	if !valid {
		t.Fatalf("MQTTPP() valid = false, record = %#v", record)
	}
	if record["type"] != "text_message" {
		t.Fatalf("record type = %v", record["type"])
	}
	if record["text"] != "hello from bot" {
		t.Fatalf("text = %v", record["text"])
	}
	if record["from_num"] != uint32(0x12345678) {
		t.Fatalf("from_num = %v", record["from_num"])
	}
	if record["packet_to_num"] != uint32(NodeNumBroadcast) {
		t.Fatalf("packet_to_num = %v", record["packet_to_num"])
	}
	if record["decrypt_success"] != true {
		t.Fatalf("decrypt_success = %v", record["decrypt_success"])
	}
}

func TestBuildTextMessageServiceEnvelopeDirectRoundTrip(t *testing.T) {
	key, err := ExpandPSK("AQ==")
	if err != nil {
		t.Fatalf("ExpandPSK() error = %v", err)
	}

	raw, err := BuildTextMessageServiceEnvelope(TextMessageBuildOptions{
		PacketBuildOptions: PacketBuildOptions{
			FromNodeNum: 0x12345678,
			ToNodeNum:   0x10203040,
			PacketID:    0x11111111,
			ChannelID:   "LongFast",
			GatewayID:   "!12345678",
			PSK:         key,
			Encrypt:     true,
			ViaMQTT:     true,
		},
		Text: "direct hello",
	})
	if err != nil {
		t.Fatalf("BuildTextMessageServiceEnvelope() error = %v", err)
	}

	valid, _, record := MQTTPP("msh/2/e/LongFast/!12345678", raw, key, Options{})
	if !valid {
		t.Fatalf("MQTTPP() valid = false, record = %#v", record)
	}
	if record["text"] != "direct hello" {
		t.Fatalf("text = %v", record["text"])
	}
	if record["packet_to"] != "!10203040" {
		t.Fatalf("packet_to = %v", record["packet_to"])
	}
	if record["packet_to_num"] != uint32(0x10203040) {
		t.Fatalf("packet_to_num = %v", record["packet_to_num"])
	}
}

func TestBuildNodeInfoServiceEnvelopeRoundTrip(t *testing.T) {
	key, err := ExpandPSK("AQ==")
	if err != nil {
		t.Fatalf("ExpandPSK() error = %v", err)
	}

	raw, err := BuildNodeInfoServiceEnvelope(NodeInfoBuildOptions{
		PacketBuildOptions: PacketBuildOptions{
			FromNodeNum: 0x12345678,
			ToNodeNum:   NodeNumBroadcast,
			PacketID:    0x22222222,
			ChannelID:   "LongFast",
			GatewayID:   "!12345678",
			PSK:         key,
			Encrypt:     true,
			ViaMQTT:     true,
		},
		NodeID:     "!12345678",
		LongName:   "MQTT Bot",
		ShortName:  "BT",
		HWModel:    255,
		Role:       0,
		IsLicensed: false,
		PublicKey:  []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("BuildNodeInfoServiceEnvelope() error = %v", err)
	}

	valid, _, record := MQTTPP("msh/2/e/LongFast/!12345678", raw, key, Options{})
	if !valid {
		t.Fatalf("MQTTPP() valid = false, record = %#v", record)
	}
	if record["type"] != "nodeinfo" {
		t.Fatalf("record type = %v", record["type"])
	}
	if record["long_name"] != "MQTT Bot" {
		t.Fatalf("long_name = %v", record["long_name"])
	}
	if record["short_name"] != "BT" {
		t.Fatalf("short_name = %v", record["short_name"])
	}
	if record["hw_model"] != "PRIVATE_HW" {
		t.Fatalf("hw_model = %v", record["hw_model"])
	}
	if record["role"] != "CLIENT" {
		t.Fatalf("role = %v", record["role"])
	}
	if record["is_licensed"] != false {
		t.Fatalf("is_licensed = %v", record["is_licensed"])
	}
	if record["public_key"] != "010203" {
		t.Fatalf("public_key = %v", record["public_key"])
	}
}

func TestParseNodeID(t *testing.T) {
	num, err := ParseNodeID("!1234abcd")
	if err != nil {
		t.Fatalf("ParseNodeID() error = %v", err)
	}
	if num != 0x1234abcd {
		t.Fatalf("num = %#x", num)
	}
	if NodeNumToID(num) != "!1234abcd" {
		t.Fatalf("NodeNumToID() = %s", NodeNumToID(num))
	}
}
