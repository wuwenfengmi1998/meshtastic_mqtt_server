package mqtpp

import (
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestMQTTPPEncryptedPacketDefaultRejected(t *testing.T) {
	raw := encryptedServiceEnvelopeTestPayload()
	valid, payload, record := MQTTPP("msh/test", raw, nil, Options{})
	if valid {
		t.Fatalf("valid = true, want false")
	}
	if payload != nil {
		t.Fatalf("payload = %v, want nil", payload)
	}
	if record["type"] != "encrypted_packet" {
		t.Fatalf("type = %v, want encrypted_packet", record["type"])
	}
	if record["error"] != "cannot be decrypted" {
		t.Fatalf("error = %v, want cannot be decrypted", record["error"])
	}
}

func TestMQTTPPEncryptedPacketAllowed(t *testing.T) {
	raw := encryptedServiceEnvelopeTestPayload()
	valid, payload, record := MQTTPP("msh/test", raw, nil, Options{AllowEncryptedForwarding: true})
	if !valid {
		t.Fatalf("valid = false, want true: %+v", record)
	}
	if string(payload) != string(raw) {
		t.Fatalf("payload = %v, want raw payload", payload)
	}
	if record["type"] != "encrypted_packet" {
		t.Fatalf("type = %v, want encrypted_packet", record["type"])
	}
	if record["error"] != nil {
		t.Fatalf("error = %v, want nil", record["error"])
	}
}

func encryptedServiceEnvelopeTestPayload() []byte {
	packet := protowire.AppendTag(nil, 5, protowire.BytesType)
	packet = protowire.AppendBytes(packet, []byte{1, 2, 3, 4})
	envelope := protowire.AppendTag(nil, 1, protowire.BytesType)
	return protowire.AppendBytes(envelope, packet)
}
