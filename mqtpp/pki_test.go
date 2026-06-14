package mqtpp

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/binary"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestBuildPKITextMessageRoundTrip(t *testing.T) {
	curve := ecdh.X25519()
	senderPriv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate sender key: %v", err)
	}
	recipientPriv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate recipient key: %v", err)
	}

	const text = "hello over PKI 你好"
	const fromNum uint32 = 0x12345678
	const toNum uint32 = 0xa1b2c3d4
	const packetID uint32 = 0xdeadbeef

	raw, err := BuildPKITextMessageServiceEnvelope(PKITextMessageBuildOptions{
		FromNodeNum:   fromNum,
		ToNodeNum:     toNum,
		PacketID:      packetID,
		GatewayID:     NodeNumToID(fromNum),
		ViaMQTT:       true,
		SenderPrivate: senderPriv.Bytes(),
		RecipientPub:  recipientPriv.PublicKey().Bytes(),
		SenderPublic:  senderPriv.PublicKey().Bytes(),
		Text:          text,
	})
	if err != nil {
		t.Fatalf("BuildPKITextMessageServiceEnvelope: %v", err)
	}

	env, err := parseServiceEnvelope(raw)
	if err != nil {
		t.Fatalf("parseServiceEnvelope: %v", err)
	}
	if env.ChannelID != PKIChannelID {
		t.Fatalf("channel_id = %q want %q", env.ChannelID, PKIChannelID)
	}
	if env.GatewayID != NodeNumToID(fromNum) {
		t.Fatalf("gateway_id = %q", env.GatewayID)
	}
	pkt := env.Packet
	if pkt.From != fromNum || pkt.To != toNum || pkt.ID != packetID {
		t.Fatalf("packet header mismatch: %+v", pkt)
	}
	if !pkt.PKIEncrypted {
		t.Fatalf("pki_encrypted = false")
	}
	if !pkt.ViaMQTT {
		t.Fatalf("via_mqtt = false")
	}
	if pkt.Channel != 0 {
		t.Fatalf("channel = %d want 0", pkt.Channel)
	}
	if pkt.PayloadVariant != "encrypted" || len(pkt.Encrypted) <= pkcOverhead {
		t.Fatalf("encrypted payload missing: %+v", pkt)
	}

	// 收件人用对端私钥 + 发件人公钥推导共享密钥并解密
	sharedKey, err := pkiSharedKey(recipientPriv.Bytes(), senderPriv.PublicKey().Bytes())
	if err != nil {
		t.Fatalf("pkiSharedKey: %v", err)
	}
	encryptedLen := len(pkt.Encrypted) - pkcOverhead
	ciphertext := pkt.Encrypted[:encryptedLen]
	auth := pkt.Encrypted[encryptedLen : encryptedLen+8]
	extraNonce := binary.LittleEndian.Uint32(pkt.Encrypted[encryptedLen+8:])
	plaintext, err := aesCCMDecrypt(sharedKey, pkiNonce(packetID, fromNum, extraNonce), ciphertext, auth)
	if err != nil {
		t.Fatalf("aesCCMDecrypt: %v", err)
	}
	data, err := parseDataPacket(plaintext)
	if err != nil {
		t.Fatalf("parseDataPacket: %v", err)
	}
	if data.Portnum != textMessageApp {
		t.Fatalf("portnum = %d", data.Portnum)
	}
	if string(data.Payload) != text {
		t.Fatalf("text = %q want %q", string(data.Payload), text)
	}

	// 同样用 MQTTPP 解析路径：PKI 包对外应被识别为 encrypted_packet（无法解密），
	// 但用错的 PSK 不应误报“channel hash mismatch” 之外的奇怪错误。
	dummyPSK, _ := ExpandPSK("AQ==")
	_, _, record := MQTTPP("msh/2/e/PKI/!12345678", raw, dummyPSK, Options{AllowEncryptedForwarding: true})
	if record["channel_id"] != PKIChannelID {
		t.Fatalf("MQTTPP record channel_id = %v", record["channel_id"])
	}
	if record["pki_encrypted"] != true {
		t.Fatalf("pki_encrypted record = %v", record["pki_encrypted"])
	}
}

func TestPKINonceLayoutMatchesFirmware(t *testing.T) {
	// 复刻 firmware initNonce(fromNode, packetId, extraNonce) 期望的字节布局：
	// nonce[0..8) = packetId(uint64 LE)
	// nonce[4..8) 被 extraNonce(uint32 LE) 覆盖（当 extraNonce != 0）
	// nonce[8..12) = fromNode(uint32 LE)
	// nonce[12] = 0
	got := pkiNonce(0xaabbccdd, 0x11223344, 0x55667788)
	want := []byte{
		0xdd, 0xcc, 0xbb, 0xaa, // packetId low 4 bytes，未被 extraNonce 覆盖前
		0x88, 0x77, 0x66, 0x55, // extraNonce 覆盖 nonce[4..8)
		0x44, 0x33, 0x22, 0x11, // fromNode
		0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("pkiNonce = % x\nwant      % x", got, want)
	}
}

func TestBuildPKITextMessageRejectsBroadcast(t *testing.T) {
	curve := ecdh.X25519()
	priv, _ := curve.GenerateKey(rand.Reader)
	pub, _ := curve.GenerateKey(rand.Reader)
	if _, err := BuildPKITextMessageServiceEnvelope(PKITextMessageBuildOptions{
		FromNodeNum:   0x1,
		ToNodeNum:     NodeNumBroadcast,
		PacketID:      0x2,
		SenderPrivate: priv.Bytes(),
		RecipientPub:  pub.PublicKey().Bytes(),
		Text:          "hi",
	}); err == nil {
		t.Fatalf("expected error for broadcast destination")
	}
}

// 确认 MeshPacket 中确实带上 pki_encrypted (tag 17) 与 public_key (tag 16)
func TestBuildPKIMeshPacketTags(t *testing.T) {
	encrypted := []byte{0x01, 0x02, 0x03}
	pub := make([]byte, 32)
	for i := range pub {
		pub[i] = byte(i)
	}
	raw := buildPKIMeshPacket(0x11, 0x22, 0x33, true, encrypted, pub)
	tags := map[protowire.Number]bool{}
	if err := walkFields(raw, func(num protowire.Number, _ protowire.Type, _ any) error {
		tags[num] = true
		return nil
	}); err != nil {
		t.Fatalf("walkFields: %v", err)
	}
	for _, want := range []protowire.Number{1, 2, 5, 6, 14, 16, 17} {
		if !tags[want] {
			t.Fatalf("missing tag %d", want)
		}
	}
}

// 端到端：发送方构造 PKI 包，接收方通过 PKIKeyResolver 解密并还原文本消息记录。
func TestMQTTPPDecryptsPKIWithResolver(t *testing.T) {
	curve := ecdh.X25519()
	senderPriv, _ := curve.GenerateKey(rand.Reader)
	recipientPriv, _ := curve.GenerateKey(rand.Reader)

	const text = "hello PKI inbound"
	const fromNum uint32 = 0xaaaa1111
	const toNum uint32 = 0xbbbb2222
	const packetID uint32 = 0x77777777

	raw, err := BuildPKITextMessageServiceEnvelope(PKITextMessageBuildOptions{
		FromNodeNum:   fromNum,
		ToNodeNum:     toNum,
		PacketID:      packetID,
		GatewayID:     NodeNumToID(fromNum),
		ViaMQTT:       true,
		SenderPrivate: senderPriv.Bytes(),
		RecipientPub:  recipientPriv.PublicKey().Bytes(),
		SenderPublic:  senderPriv.PublicKey().Bytes(),
		Text:          text,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	resolver := func(to, from uint32) ([]byte, []byte, bool) {
		if to != toNum || from != fromNum {
			return nil, nil, false
		}
		return recipientPriv.Bytes(), senderPriv.PublicKey().Bytes(), true
	}
	dummyPSK, _ := ExpandPSK("AQ==")
	valid, _, record := MQTTPP("msh/2/e/PKI/!aaaa1111", raw, dummyPSK, Options{PKIKeyResolver: resolver})
	if !valid {
		t.Fatalf("MQTTPP not valid: %#v", record)
	}
	if record["type"] != "text_message" {
		t.Fatalf("type = %v, want text_message", record["type"])
	}
	if record["text"] != text {
		t.Fatalf("text = %v", record["text"])
	}
	if record["pki_encrypted"] != true {
		t.Fatalf("pki_encrypted = %v", record["pki_encrypted"])
	}
}
