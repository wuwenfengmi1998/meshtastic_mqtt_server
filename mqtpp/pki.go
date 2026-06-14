package mqtpp

import (
	"crypto/aes"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

// PKIChannelID 是固件在 ServiceEnvelope/MQTT topic 中标识 PKI 加密包时使用的字面量
// 见 firmware/src/mqtt/MQTT.cpp 中 `channelId = isPKIEncrypted ? "PKI" : channels.getGlobalId(chIndex);`
const PKIChannelID = "PKI"

// pkcOverhead 与固件 MESHTASTIC_PKC_OVERHEAD 一致：8 字节 AES-CCM 认证标签 + 4 字节 extraNonce
const pkcOverhead = 12

// PKITextMessageBuildOptions 描述构造一条 PKI 加密 DM 所需的全部上下文
type PKITextMessageBuildOptions struct {
	FromNodeNum    uint32
	ToNodeNum      uint32
	PacketID       uint32
	GatewayID      string
	ViaMQTT        bool
	SenderPrivate  []byte // X25519 32 字节私钥
	RecipientPub   []byte // X25519 32 字节公钥
	SenderPublic   []byte // 可选；附在 MeshPacket.public_key (tag 16)
	Text           string
}

// BuildPKITextMessageServiceEnvelope 构造一条遵循固件实现的 PKI 私聊文本消息：
//   - data 包: portnum=TEXT_MESSAGE_APP, payload=text
//   - 共享密钥: SHA256(X25519(senderPriv, recipientPub))
//   - AES-CCM(M=8,L=2,AAD=0); nonce = packetId(8B LE) | fromNode(4B LE) | extraNonce(4B LE，覆盖 fromNode 后续 4 字节)
//   - encrypted bytes 末尾追加 8 字节 auth + 4 字节 extraNonce(LE)
//   - MeshPacket.channel = 0, pki_encrypted(tag17)=1
//   - ServiceEnvelope.channel_id 固定 "PKI"
func BuildPKITextMessageServiceEnvelope(opts PKITextMessageBuildOptions) ([]byte, error) {
	if opts.FromNodeNum == 0 {
		return nil, fmt.Errorf("from node number is required")
	}
	if opts.ToNodeNum == 0 || opts.ToNodeNum == NodeNumBroadcast {
		return nil, fmt.Errorf("pki direct message requires a non-broadcast destination")
	}
	if opts.PacketID == 0 {
		return nil, fmt.Errorf("packet id is required")
	}
	if opts.Text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if !utf8.ValidString(opts.Text) {
		return nil, fmt.Errorf("text must be valid utf-8")
	}
	if len(opts.SenderPrivate) != 32 {
		return nil, fmt.Errorf("sender private key must be 32 bytes")
	}
	if len(opts.RecipientPub) != 32 {
		return nil, fmt.Errorf("recipient public key must be 32 bytes")
	}
	if strings.TrimSpace(opts.GatewayID) == "" {
		opts.GatewayID = NodeNumToID(opts.FromNodeNum)
	}

	plaintext := buildDataPacket(textMessageApp, []byte(opts.Text))

	sharedKey, err := pkiSharedKey(opts.SenderPrivate, opts.RecipientPub)
	if err != nil {
		return nil, err
	}

	var extraNonceBuf [4]byte
	if _, err := rand.Read(extraNonceBuf[:]); err != nil {
		return nil, err
	}
	extraNonce := binary.LittleEndian.Uint32(extraNonceBuf[:])

	ciphertext, auth, err := aesCCMEncrypt(sharedKey, pkiNonce(opts.PacketID, opts.FromNodeNum, extraNonce), plaintext)
	if err != nil {
		return nil, err
	}

	encrypted := make([]byte, 0, len(ciphertext)+pkcOverhead)
	encrypted = append(encrypted, ciphertext...)
	encrypted = append(encrypted, auth...)
	encrypted = append(encrypted, extraNonceBuf[:]...)

	packet := buildPKIMeshPacket(opts.FromNodeNum, opts.ToNodeNum, opts.PacketID, opts.ViaMQTT, encrypted, opts.SenderPublic)
	return buildServiceEnvelope(packet, PKIChannelID, opts.GatewayID), nil
}

// pkiSharedKey 用 X25519 计算共享密钥，再做一次 SHA-256（与固件一致）。
func pkiSharedKey(privateKey, publicKey []byte) ([]byte, error) {
	curve := ecdh.X25519()
	priv, err := curve.NewPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid sender private key: %w", err)
	}
	pub, err := curve.NewPublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient public key: %w", err)
	}
	shared, err := priv.ECDH(pub)
	if err != nil {
		return nil, fmt.Errorf("x25519 ecdh failed: %w", err)
	}
	digest := sha256.Sum256(shared)
	return digest[:], nil
}

// pkiNonce 完整复刻固件 CryptoEngine::initNonce(fromNode, packetId, extraNonce) 的字节布局。
// 固件实现（mesh/CryptoEngine.cpp）：
//
//	memcpy(nonce + 0, &packetId,  8);  // packetId 是 uint64，写入 nonce[0..8)
//	memcpy(nonce + 8, &fromNode,  4);  // fromNode 写入 nonce[8..12)
//	if (extraNonce)
//	    memcpy(nonce + 4, &extraNonce, 4); // extraNonce 覆盖 nonce[4..8)
//
// 因此 13 字节 nonce 布局为：packetId_lo(4B LE) | extraNonce_or_packetId_hi(4B LE) | fromNode(4B LE) | 0x00
func pkiNonce(packetID, fromNode, extraNonce uint32) []byte {
	nonce := make([]byte, 16)
	binary.LittleEndian.PutUint64(nonce[0:8], uint64(packetID)) // packetId 是 uint64，高 32 位为 0
	binary.LittleEndian.PutUint32(nonce[8:12], fromNode)
	if extraNonce != 0 {
		binary.LittleEndian.PutUint32(nonce[4:8], extraNonce)
	}
	// CCM L=2 → nonce 占 15-L=13 字节
	return nonce[:13]
}

// aesCCMEncrypt 使用与固件相同的参数（AES-CCM, M=8 即 8 字节 tag, L=2, 无 AAD）。
func aesCCMEncrypt(key, nonce, plaintext []byte) (ciphertext []byte, auth []byte, err error) {
	if len(nonce) != 13 {
		return nil, nil, fmt.Errorf("ccm nonce must be 13 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	const tagLen = 8
	if len(plaintext) > 0xffff {
		return nil, nil, fmt.Errorf("plaintext too large for L=2 ccm")
	}

	// CBC-MAC 鉴权
	var x [aes.BlockSize]byte
	var b [aes.BlockSize]byte
	b[0] = byte((tagLen-2)/2) << 3 // M', AAD=0 时 Adata=0
	b[0] |= byte(2 - 1)            // L'=L-1
	copy(b[1:], nonce[:13])
	binary.BigEndian.PutUint16(b[14:], uint16(len(plaintext)))
	block.Encrypt(x[:], b[:])

	// 鉴权明文
	for offset := 0; offset < len(plaintext); offset += aes.BlockSize {
		end := offset + aes.BlockSize
		if end > len(plaintext) {
			end = len(plaintext)
		}
		var blk [aes.BlockSize]byte
		copy(blk[:], plaintext[offset:end])
		for i := range x {
			x[i] ^= blk[i]
		}
		block.Encrypt(x[:], x[:])
	}

	// CTR 流：A_i = L' | nonce | counter_be16
	var a [aes.BlockSize]byte
	a[0] = byte(2 - 1)
	copy(a[1:], nonce[:13])
	encryptCounter := func(i uint16) [aes.BlockSize]byte {
		var ai [aes.BlockSize]byte
		copy(ai[:], a[:])
		binary.BigEndian.PutUint16(ai[14:], i)
		var s [aes.BlockSize]byte
		block.Encrypt(s[:], ai[:])
		return s
	}

	ciphertext = make([]byte, len(plaintext))
	for i, offset := 1, 0; offset < len(plaintext); i, offset = i+1, offset+aes.BlockSize {
		s := encryptCounter(uint16(i))
		end := offset + aes.BlockSize
		if end > len(plaintext) {
			end = len(plaintext)
		}
		for j := offset; j < end; j++ {
			ciphertext[j] = plaintext[j] ^ s[j-offset]
		}
	}

	// auth = T XOR S_0
	s0 := encryptCounter(0)
	auth = make([]byte, tagLen)
	for i := 0; i < tagLen; i++ {
		auth[i] = x[i] ^ s0[i]
	}
	return ciphertext, auth, nil
}

// aesCCMDecrypt 与 encrypt 对称，验证标签后返回明文。仅用于测试与可能的回程解密。
func aesCCMDecrypt(key, nonce, ciphertext, auth []byte) ([]byte, error) {
	if len(nonce) != 13 {
		return nil, fmt.Errorf("ccm nonce must be 13 bytes")
	}
	if len(auth) != 8 {
		return nil, fmt.Errorf("ccm auth tag must be 8 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// 先 CTR 解密
	var a [aes.BlockSize]byte
	a[0] = byte(2 - 1)
	copy(a[1:], nonce[:13])
	encryptCounter := func(i uint16) [aes.BlockSize]byte {
		var ai [aes.BlockSize]byte
		copy(ai[:], a[:])
		binary.BigEndian.PutUint16(ai[14:], i)
		var s [aes.BlockSize]byte
		block.Encrypt(s[:], ai[:])
		return s
	}
	plain := make([]byte, len(ciphertext))
	for i, offset := 1, 0; offset < len(ciphertext); i, offset = i+1, offset+aes.BlockSize {
		s := encryptCounter(uint16(i))
		end := offset + aes.BlockSize
		if end > len(ciphertext) {
			end = len(ciphertext)
		}
		for j := offset; j < end; j++ {
			plain[j] = ciphertext[j] ^ s[j-offset]
		}
	}

	// 再 CBC-MAC 校验
	var x [aes.BlockSize]byte
	var b [aes.BlockSize]byte
	b[0] = byte((8-2)/2) << 3
	b[0] |= byte(2 - 1)
	copy(b[1:], nonce[:13])
	binary.BigEndian.PutUint16(b[14:], uint16(len(plain)))
	block.Encrypt(x[:], b[:])
	for offset := 0; offset < len(plain); offset += aes.BlockSize {
		end := offset + aes.BlockSize
		if end > len(plain) {
			end = len(plain)
		}
		var blk [aes.BlockSize]byte
		copy(blk[:], plain[offset:end])
		for i := range x {
			x[i] ^= blk[i]
		}
		block.Encrypt(x[:], x[:])
	}
	s0 := encryptCounter(0)
	expected := make([]byte, 8)
	for i := 0; i < 8; i++ {
		expected[i] = x[i] ^ s0[i]
	}
	if subtle.ConstantTimeCompare(expected, auth) != 1 {
		return nil, fmt.Errorf("aes-ccm auth mismatch")
	}
	return plain, nil
}

// buildPKIMeshPacket 构造一个 PKI 加密的 MeshPacket：
//   - tag 1/2: from/to (fixed32)
//   - tag 3 channel = 0 （省略，默认即为 0）
//   - tag 5 encrypted (含 ciphertext|auth|extraNonce)
//   - tag 6 packet_id
//   - tag 14 via_mqtt
//   - tag 16 public_key（可选，附带发送者公钥）
//   - tag 17 pki_encrypted = 1
func buildPKIMeshPacket(from, to, packetID uint32, viaMQTT bool, encrypted []byte, senderPublic []byte) []byte {
	var out []byte
	out = protowire.AppendTag(out, 1, protowire.Fixed32Type)
	out = protowire.AppendFixed32(out, from)
	out = protowire.AppendTag(out, 2, protowire.Fixed32Type)
	out = protowire.AppendFixed32(out, to)
	out = protowire.AppendTag(out, 5, protowire.BytesType)
	out = protowire.AppendBytes(out, encrypted)
	out = protowire.AppendTag(out, 6, protowire.Fixed32Type)
	out = protowire.AppendFixed32(out, packetID)
	if viaMQTT {
		out = protowire.AppendTag(out, 14, protowire.VarintType)
		out = protowire.AppendVarint(out, 1)
	}
	if len(senderPublic) == 32 {
		out = protowire.AppendTag(out, 16, protowire.BytesType)
		out = protowire.AppendBytes(out, senderPublic)
	}
	out = protowire.AppendTag(out, 17, protowire.VarintType)
	out = protowire.AppendVarint(out, 1)
	return out
}
