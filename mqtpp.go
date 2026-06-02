package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	unknownApp     = 0
	textMessageApp = 1
	positionApp    = 3
	nodeInfoApp    = 4
	routingApp     = 5
	telemetryApp   = 67
	tracerouteApp  = 70
	mapReportApp   = 73
)

var defaultMeshtasticPSK = []byte{
	0xD4, 0xF1, 0xBB, 0x3A,
	0x20, 0x29, 0x07, 0x59,
	0xF0, 0xBC, 0xFF, 0xAB,
	0xCF, 0x4E, 0x69, 0x01,
}

type serviceEnvelope struct {
	Packet    *meshPacket
	ChannelID string
	GatewayID string
}

type meshPacket struct {
	From           uint32
	To             uint32
	Channel        uint32
	Decoded        *dataPacket
	Encrypted      []byte
	ID             uint32
	ViaMQTT        bool
	PKIEncrypted   bool
	PayloadVariant string
}

type dataPacket struct {
	Portnum uint32
	Payload []byte
}

type userInfo struct {
	ID         string
	LongName   string
	ShortName  string
	HWModel    uint64
	IsLicensed bool
	Role       uint64
	PublicKey  []byte
}

type mapReport struct {
	LongName               string
	ShortName              string
	Role                   uint64
	HWModel                uint64
	FirmwareVersion        string
	Region                 uint64
	ModemPreset            uint64
	LatitudeI              int32
	LongitudeI             int32
	Altitude               int32
	PositionPrecision      uint32
	NumOnlineLocalNodes    uint32
	HasOptedReportLocation bool
}

// MQTTPP 处理一个 MQTT 原始 payload，返回合规状态、原始数据和解码后的 JSON。
// 第一个返回值表示数据是否合规；第二个返回值在不合规时为 nil；第三个返回值是解码结果 JSON。
func MQTTPP(topic string, raw []byte, key []byte) (bool, []byte, []byte) {
	if !isCompliantMQTTPacket(raw) {
		return false, nil, nil
	}

	env, err := parseServiceEnvelope(raw)
	if err != nil {
		return true, raw, mustJSON(map[string]any{"topic": topic, "error": "protobuf decode failed: " + err.Error(), "payload_len": len(raw)})
	}
	record, err := describePacket(topic, env, key)
	if err != nil {
		return true, raw, mustJSON(map[string]any{"topic": topic, "error": err.Error(), "payload_len": len(raw)})
	}
	return true, raw, mustJSON(record)
}

// expandPSK 展开 Base64 PSK，兼容 Meshtastic 默认索引 PSK 和短 key 补零规则。
func expandPSK(pskBase64 string) ([]byte, error) {
	psk, err := base64.StdEncoding.DecodeString(pskBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid psk: %w", err)
	}
	if len(psk) == 1 {
		pskIndex := psk[0]
		if pskIndex == 0 {
			return []byte{}, nil
		}
		key := append([]byte(nil), defaultMeshtasticPSK...)
		key[len(key)-1] = byte((int(key[len(key)-1]) + int(pskIndex) - 1) & 0xff)
		return key, nil
	}
	if len(psk) > 0 && len(psk) < 16 {
		return append(psk, make([]byte, 16-len(psk))...), nil
	}
	if len(psk) > 16 && len(psk) < 32 {
		return append(psk, make([]byte, 32-len(psk))...), nil
	}
	if len(psk) != 0 && len(psk) != 16 && len(psk) != 24 && len(psk) != 32 {
		return nil, fmt.Errorf("invalid psk length %d after expansion: AES keys must be 16, 24, or 32 bytes", len(psk))
	}
	return psk, nil
}

// isCompliantMQTTPacket 判断 MQTT 原始数据是否合规；当前预留判断位置，暂时始终返回 true。
func isCompliantMQTTPacket(_ []byte) bool {
	// TODO: Add packet compliance checks here.
	return true
}

// mustJSON 将记录编码成 JSON；编码失败时返回包含错误信息的 JSON。
func mustJSON(record map[string]any) []byte {
	text, err := json.Marshal(record)
	if err != nil {
		text, _ = json.Marshal(map[string]any{"error": err.Error()})
	}
	return text
}

// parseServiceEnvelope 从 protobuf wire 数据中解析 MQTT ServiceEnvelope。
func parseServiceEnvelope(payload []byte) (*serviceEnvelope, error) {
	env := &serviceEnvelope{}
	err := walkFields(payload, func(num protowire.Number, typ protowire.Type, value any) error {
		switch num {
		case 1:
			b, ok := value.([]byte)
			if !ok || typ != protowire.BytesType {
				return nil
			}
			packet, err := parseMeshPacket(b)
			if err != nil {
				return err
			}
			env.Packet = packet
		case 2:
			if b, ok := value.([]byte); ok && typ == protowire.BytesType {
				env.ChannelID = string(b)
			}
		case 3:
			if b, ok := value.([]byte); ok && typ == protowire.BytesType {
				env.GatewayID = string(b)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if env.Packet == nil {
		env.Packet = &meshPacket{}
	}
	return env, nil
}

// parseMeshPacket 从 protobuf wire 数据中解析 MeshPacket 的关键字段。
func parseMeshPacket(payload []byte) (*meshPacket, error) {
	packet := &meshPacket{}
	err := walkFields(payload, func(num protowire.Number, typ protowire.Type, value any) error {
		switch num {
		case 1:
			if v, ok := value.(uint32); ok && typ == protowire.Fixed32Type {
				packet.From = v
			}
		case 2:
			if v, ok := value.(uint32); ok && typ == protowire.Fixed32Type {
				packet.To = v
			}
		case 3:
			if v, ok := value.(uint64); ok && typ == protowire.VarintType {
				packet.Channel = uint32(v)
			}
		case 4:
			b, ok := value.([]byte)
			if !ok || typ != protowire.BytesType {
				return nil
			}
			decoded, err := parseDataPacket(b)
			if err != nil {
				return err
			}
			packet.Decoded = decoded
			packet.Encrypted = nil
			packet.PayloadVariant = "decoded"
		case 5:
			if b, ok := value.([]byte); ok && typ == protowire.BytesType {
				packet.Encrypted = append([]byte(nil), b...)
				packet.Decoded = nil
				packet.PayloadVariant = "encrypted"
			}
		case 6:
			if v, ok := value.(uint32); ok && typ == protowire.Fixed32Type {
				packet.ID = v
			}
		case 14:
			if v, ok := value.(uint64); ok && typ == protowire.VarintType {
				packet.ViaMQTT = v != 0
			}
		case 17:
			if v, ok := value.(uint64); ok && typ == protowire.VarintType {
				packet.PKIEncrypted = v != 0
			}
		}
		return nil
	})
	return packet, err
}

// parseDataPacket 解析 MeshPacket decoded 字段中的 Data 子包。
func parseDataPacket(payload []byte) (*dataPacket, error) {
	data := &dataPacket{}
	err := walkFields(payload, func(num protowire.Number, typ protowire.Type, value any) error {
		switch num {
		case 1:
			if v, ok := value.(uint64); ok && typ == protowire.VarintType {
				data.Portnum = uint32(v)
			}
		case 2:
			if b, ok := value.([]byte); ok && typ == protowire.BytesType {
				data.Payload = append([]byte(nil), b...)
			}
		}
		return nil
	})
	return data, err
}

// parseUser 解析 NODEINFO_APP 的 User payload。
func parseUser(payload []byte) (*userInfo, error) {
	user := &userInfo{}
	err := walkFields(payload, func(num protowire.Number, typ protowire.Type, value any) error {
		switch num {
		case 1:
			user.ID = stringBytes(typ, value)
		case 2:
			user.LongName = stringBytes(typ, value)
		case 3:
			user.ShortName = stringBytes(typ, value)
		case 5:
			user.HWModel = varintValue(typ, value)
		case 6:
			user.IsLicensed = varintValue(typ, value) != 0
		case 7:
			user.Role = varintValue(typ, value)
		case 8:
			if b, ok := value.([]byte); ok && typ == protowire.BytesType {
				user.PublicKey = append([]byte(nil), b...)
			}
		}
		return nil
	})
	return user, err
}

// parseMapReport 解析 MAP_REPORT_APP 的地图报告 payload。
func parseMapReport(payload []byte) (*mapReport, error) {
	report := &mapReport{}
	err := walkFields(payload, func(num protowire.Number, typ protowire.Type, value any) error {
		switch num {
		case 1:
			report.LongName = stringBytes(typ, value)
		case 2:
			report.ShortName = stringBytes(typ, value)
		case 3:
			report.Role = varintValue(typ, value)
		case 4:
			report.HWModel = varintValue(typ, value)
		case 5:
			report.FirmwareVersion = stringBytes(typ, value)
		case 6:
			report.Region = varintValue(typ, value)
		case 7:
			report.ModemPreset = varintValue(typ, value)
		case 9:
			if v, ok := value.(uint32); ok && typ == protowire.Fixed32Type {
				report.LatitudeI = int32(v)
			}
		case 10:
			if v, ok := value.(uint32); ok && typ == protowire.Fixed32Type {
				report.LongitudeI = int32(v)
			}
		case 11:
			report.Altitude = int32(varintValue(typ, value))
		case 12:
			report.PositionPrecision = uint32(varintValue(typ, value))
		case 13:
			report.NumOnlineLocalNodes = uint32(varintValue(typ, value))
		case 14:
			report.HasOptedReportLocation = varintValue(typ, value) != 0
		}
		return nil
	})
	return report, err
}

// walkFields 遍历 protobuf wire 字段，并把字段号、类型和值交给回调处理。
func walkFields(payload []byte, handle func(protowire.Number, protowire.Type, any) error) error {
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return protowire.ParseError(n)
		}
		payload = payload[n:]

		var value any
		switch typ {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(payload)
			if n < 0 {
				return protowire.ParseError(n)
			}
			value = v
			payload = payload[n:]
		case protowire.Fixed32Type:
			v, n := protowire.ConsumeFixed32(payload)
			if n < 0 {
				return protowire.ParseError(n)
			}
			value = v
			payload = payload[n:]
		case protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(payload)
			if n < 0 {
				return protowire.ParseError(n)
			}
			value = v
			payload = payload[n:]
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(payload)
			if n < 0 {
				return protowire.ParseError(n)
			}
			value = v
			payload = payload[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, payload)
			if n < 0 {
				return protowire.ParseError(n)
			}
			payload = payload[n:]
		}

		if err := handle(num, typ, value); err != nil {
			return err
		}
	}
	return nil
}

// stringBytes 在字段类型为 bytes 时把字段值转换为字符串。
func stringBytes(typ protowire.Type, value any) string {
	if b, ok := value.([]byte); ok && typ == protowire.BytesType {
		return string(b)
	}
	return ""
}

// varintValue 在字段类型为 varint 时提取无符号整数值。
func varintValue(typ protowire.Type, value any) uint64 {
	if v, ok := value.(uint64); ok && typ == protowire.VarintType {
		return v
	}
	return 0
}

// describePacket 根据 ServiceEnvelope 和 PSK 生成统一的 JSON 记录字段。
func describePacket(topic string, env *serviceEnvelope, key []byte) (map[string]any, error) {
	packet := env.Packet
	if packet == nil {
		packet = &meshPacket{}
	}

	base := map[string]any{
		"topic":           topic,
		"channel_id":      env.ChannelID,
		"gateway_id":      env.GatewayID,
		"packet_from":     nodeNumToID(packet.From),
		"packet_from_num": packet.From,
		"packet_to":       nodeNumToID(packet.To),
		"packet_to_num":   packet.To,
		"packet_id":       packet.ID,
		"payload_variant": packet.PayloadVariant,
		"via_mqtt":        packet.ViaMQTT,
		"pki_encrypted":   packet.PKIEncrypted,
	}

	if packet.PayloadVariant == "encrypted" {
		decryptedPacket, decryptStatus := tryDecryptPacket(packet, env.ChannelID, key)
		if decryptedPacket == nil {
			return merge(base, map[string]any{
				"type":            "encrypted_packet",
				"encrypted_len":   len(packet.Encrypted),
				"decrypt_success": false,
				"decrypt_status":  decryptStatus,
			}), nil
		}

		decryptedEnv := *env
		decryptedEnv.Packet = decryptedPacket
		decrypted, err := describePacket(topic, &decryptedEnv, key)
		if err != nil {
			return nil, err
		}
		decrypted["payload_variant"] = "decoded"
		decrypted["decrypt_success"] = true
		decrypted["decrypt_status"] = decryptStatus
		return decrypted, nil
	}

	if packet.PayloadVariant != "decoded" || packet.Decoded == nil {
		return merge(base, map[string]any{"type": "empty_packet"}), nil
	}

	decodedBase := merge(base, map[string]any{
		"portnum":     enumName(portNumNames, uint64(packet.Decoded.Portnum)),
		"payload_len": len(packet.Decoded.Payload),
	})

	switch packet.Decoded.Portnum {
	case nodeInfoApp:
		record, err := decodeUser(packet)
		if err != nil {
			return nil, err
		}
		return merge(decodedBase, record), nil
	case mapReportApp:
		record, err := decodeMapReport(packet)
		if err != nil {
			return nil, err
		}
		return merge(decodedBase, record), nil
	case textMessageApp:
		return merge(decodedBase, decodeTextMessage(packet)), nil
	case positionApp:
		return merge(decodedBase, map[string]any{"type": "position"}), nil
	case telemetryApp:
		return merge(decodedBase, map[string]any{"type": "telemetry"}), nil
	case routingApp:
		return merge(decodedBase, map[string]any{"type": "routing"}), nil
	case tracerouteApp:
		return merge(decodedBase, map[string]any{"type": "traceroute"}), nil
	default:
		return merge(decodedBase, map[string]any{"type": "decoded_packet"}), nil
	}
}

// tryDecryptPacket 尝试用 channel PSK 解密 encrypted MeshPacket，并返回解密状态。
func tryDecryptPacket(packet *meshPacket, channelID string, key []byte) (*meshPacket, string) {
	if len(key) == 0 {
		return nil, "psk disables encryption"
	}
	if packet.Channel != uint32(channelHash(channelID, key)) {
		return nil, "channel hash mismatch"
	}

	plaintext, err := decryptAESCTR(key, packet.From, packet.ID, packet.Encrypted)
	if err != nil {
		return nil, err.Error()
	}
	decoded, err := parseDataPacket(plaintext)
	if err != nil {
		return nil, fmt.Sprintf("decrypted bytes are not Data protobuf: %v", err)
	}
	if decoded.Portnum == unknownApp {
		return nil, "decrypted protobuf has UNKNOWN_APP portnum"
	}

	decrypted := *packet
	decrypted.Encrypted = nil
	decrypted.Decoded = decoded
	decrypted.PayloadVariant = "decoded"
	return &decrypted, "success"
}

// decodeUser 将 NODEINFO_APP payload 解码为节点信息 JSON 字段。
func decodeUser(packet *meshPacket) (map[string]any, error) {
	user, err := parseUser(packet.Decoded.Payload)
	if err != nil {
		return nil, err
	}

	var publicKey any
	if len(user.PublicKey) > 0 {
		publicKey = hex.EncodeToString(user.PublicKey)
	}

	return map[string]any{
		"type":        "nodeinfo",
		"from":        nodeNumToID(packet.From),
		"from_num":    packet.From,
		"user_id":     user.ID,
		"long_name":   user.LongName,
		"short_name":  user.ShortName,
		"hw_model":    enumName(hardwareModelNames, user.HWModel),
		"role":        enumName(roleNames, user.Role),
		"is_licensed": user.IsLicensed,
		"public_key":  publicKey,
	}, nil
}

// decodeMapReport 将 MAP_REPORT_APP payload 解码为地图报告 JSON 字段。
func decodeMapReport(packet *meshPacket) (map[string]any, error) {
	report, err := parseMapReport(packet.Decoded.Payload)
	if err != nil {
		return nil, err
	}

	var latitude, longitude any
	if report.LatitudeI != 0 {
		latitude = float64(report.LatitudeI) * 1e-7
	}
	if report.LongitudeI != 0 {
		longitude = float64(report.LongitudeI) * 1e-7
	}

	return map[string]any{
		"type":                      "map_report",
		"from":                      nodeNumToID(packet.From),
		"from_num":                  packet.From,
		"long_name":                 report.LongName,
		"short_name":                report.ShortName,
		"role":                      enumName(roleNames, report.Role),
		"hw_model":                  enumName(hardwareModelNames, report.HWModel),
		"firmware_version":          report.FirmwareVersion,
		"region":                    enumName(regionCodeNames, report.Region),
		"modem_preset":              enumName(modemPresetNames, report.ModemPreset),
		"latitude":                  latitude,
		"longitude":                 longitude,
		"altitude":                  report.Altitude,
		"position_precision":        report.PositionPrecision,
		"num_online_local_nodes":    report.NumOnlineLocalNodes,
		"has_opted_report_location": report.HasOptedReportLocation,
	}, nil
}

// decodeTextMessage 将 TEXT_MESSAGE_APP payload 解码为聊天文本 JSON 字段。
func decodeTextMessage(packet *meshPacket) map[string]any {
	text := string(packet.Decoded.Payload)
	record := map[string]any{
		"type":     "text_message",
		"from":     nodeNumToID(packet.From),
		"from_num": packet.From,
		"text":     text,
	}
	if !utf8.Valid(packet.Decoded.Payload) {
		record["text"] = nil
		record["payload_hex"] = hex.EncodeToString(packet.Decoded.Payload)
	}
	return record
}

// merge 合并两个 JSON 字段 map，extra 中的同名字段会覆盖 base。
func merge(base map[string]any, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// nodeNumToID 将 Meshtastic 数字节点号格式化为 !xxxxxxxx 字符串。
func nodeNumToID(nodeNum uint32) string {
	return fmt.Sprintf("!%08x", nodeNum)
}

// xorHash 计算 Meshtastic channel hash 使用的逐字节异或值。
func xorHash(data []byte) byte {
	var result byte
	for _, b := range data {
		result ^= b
	}
	return result
}

// channelHash 根据 channel 名称和 PSK 计算 Meshtastic 加密包中的 channel hash。
func channelHash(channelName string, key []byte) byte {
	return xorHash([]byte(channelName)) ^ xorHash(key)
}

// decryptAESCTR 按 Meshtastic nonce 规则使用 AES-CTR 解密 payload。
func decryptAESCTR(key []byte, fromNum, packetID uint32, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aes.BlockSize)
	binary.LittleEndian.PutUint64(nonce[0:8], uint64(packetID))
	binary.LittleEndian.PutUint32(nonce[8:12], fromNum)
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCTR(block, nonce).XORKeyStream(plaintext, ciphertext)
	return plaintext, nil
}

// enumName 把已知枚举值转换成名称，未知值保留为数字。
func enumName(names map[uint64]string, value uint64) any {
	if name, ok := names[value]; ok {
		return name
	}
	return value
}

var portNumNames = map[uint64]string{
	0: "UNKNOWN_APP", 1: "TEXT_MESSAGE_APP", 2: "REMOTE_HARDWARE_APP", 3: "POSITION_APP", 4: "NODEINFO_APP",
	5: "ROUTING_APP", 6: "ADMIN_APP", 7: "TEXT_MESSAGE_COMPRESSED_APP", 8: "WAYPOINT_APP", 9: "AUDIO_APP",
	10: "DETECTION_SENSOR_APP", 11: "ALERT_APP", 12: "KEY_VERIFICATION_APP", 13: "REMOTE_SHELL_APP", 32: "REPLY_APP",
	33: "IP_TUNNEL_APP", 34: "PAXCOUNTER_APP", 35: "STORE_FORWARD_PLUSPLUS_APP", 36: "NODE_STATUS_APP", 64: "SERIAL_APP",
	65: "STORE_FORWARD_APP", 66: "RANGE_TEST_APP", 67: "TELEMETRY_APP", 68: "ZPS_APP", 69: "SIMULATOR_APP",
	70: "TRACEROUTE_APP", 71: "NEIGHBORINFO_APP", 72: "ATAK_PLUGIN", 73: "MAP_REPORT_APP", 74: "POWERSTRESS_APP",
	75: "LORAWAN_BRIDGE", 76: "RETICULUM_TUNNEL_APP", 77: "CAYENNE_APP", 78: "ATAK_PLUGIN_V2", 112: "GROUPALARM_APP",
	256: "PRIVATE_APP", 257: "ATAK_FORWARDER", 511: "MAX",
}

var roleNames = map[uint64]string{
	0: "CLIENT", 1: "CLIENT_MUTE", 2: "ROUTER", 3: "ROUTER_CLIENT", 4: "REPEATER", 5: "TRACKER", 6: "SENSOR",
	7: "TAK", 8: "CLIENT_HIDDEN", 9: "LOST_AND_FOUND", 10: "TAK_TRACKER", 11: "ROUTER_LATE", 12: "CLIENT_BASE",
}

var regionCodeNames = map[uint64]string{
	0: "UNSET", 1: "US", 2: "EU_433", 3: "EU_868", 4: "CN", 5: "JP", 6: "ANZ", 7: "KR", 8: "TW", 9: "RU",
	10: "IN", 11: "NZ_865", 12: "TH", 13: "LORA_24", 14: "UA_433", 15: "UA_868", 16: "MY_433", 17: "MY_919",
	18: "SG_923", 19: "PH_433", 20: "PH_868", 21: "PH_915", 22: "ANZ_433", 23: "KZ_433", 24: "KZ_863",
	25: "NP_865", 26: "BR_902", 27: "ITU1_2M", 28: "ITU2_2M", 29: "EU_866", 30: "EU_874", 31: "EU_917",
	32: "EU_N_868", 33: "ITU3_2M",
}

var modemPresetNames = map[uint64]string{
	0: "LONG_FAST", 1: "LONG_SLOW", 2: "VERY_LONG_SLOW", 3: "MEDIUM_SLOW", 4: "MEDIUM_FAST", 5: "SHORT_SLOW", 6: "SHORT_FAST",
	7: "LONG_MODERATE", 8: "SHORT_TURBO", 9: "LONG_TURBO", 10: "LITE_FAST", 11: "LITE_SLOW", 12: "NARROW_FAST", 13: "NARROW_SLOW",
}

var hardwareModelNames = map[uint64]string{
	0: "UNSET", 1: "TLORA_V2", 2: "TLORA_V1", 3: "TLORA_V2_1_1P6", 4: "TBEAM", 5: "HELTEC_V2_0",
	6: "TBEAM_V0P7", 7: "T_ECHO", 8: "TLORA_V1_1P3", 9: "RAK4631", 10: "HELTEC_V2_1", 11: "HELTEC_V1",
	12: "LILYGO_TBEAM_S3_CORE", 13: "RAK11200", 14: "NANO_G1", 15: "TLORA_V2_1_1P8", 16: "TLORA_T3_S3",
	17: "NANO_G1_EXPLORER", 18: "NANO_G2_ULTRA", 19: "LORA_TYPE", 20: "WIPHONE", 21: "WIO_WM1110", 22: "RAK2560",
	23: "HELTEC_HRU_3601", 24: "HELTEC_WIRELESS_BRIDGE", 25: "STATION_G1", 26: "RAK11310", 27: "SENSELORA_RP2040",
	28: "SENSELORA_S3", 29: "CANARYONE", 30: "RP2040_LORA", 31: "STATION_G2", 32: "LORA_RELAY_V1", 33: "T_ECHO_PLUS",
	34: "PPR", 35: "GENIEBLOCKS", 36: "NRF52_UNKNOWN", 37: "PORTDUINO", 38: "ANDROID_SIM", 39: "DIY_V1",
	40: "NRF52840_PCA10059", 41: "DR_DEV", 42: "M5STACK", 43: "HELTEC_V3", 44: "HELTEC_WSL_V3", 45: "BETAFPV_2400_TX",
	46: "BETAFPV_900_NANO_TX", 47: "RPI_PICO", 48: "HELTEC_WIRELESS_TRACKER", 49: "HELTEC_WIRELESS_PAPER", 50: "T_DECK",
	51: "T_WATCH_S3", 52: "PICOMPUTER_S3", 53: "HELTEC_HT62", 54: "EBYTE_ESP32_S3", 55: "ESP32_S3_PICO", 56: "CHATTER_2",
	57: "HELTEC_WIRELESS_PAPER_V1_0", 58: "HELTEC_WIRELESS_TRACKER_V1_0", 59: "UNPHONE", 60: "TD_LORAC", 61: "CDEBYTE_EORA_S3",
	62: "TWC_MESH_V4", 63: "NRF52_PROMICRO_DIY", 64: "RADIOMASTER_900_BANDIT_NANO", 65: "HELTEC_CAPSULE_SENSOR_V3",
	66: "HELTEC_VISION_MASTER_T190", 67: "HELTEC_VISION_MASTER_E213", 68: "HELTEC_VISION_MASTER_E290", 69: "HELTEC_MESH_NODE_T114",
	70: "SENSECAP_INDICATOR", 71: "TRACKER_T1000_E", 72: "RAK3172", 73: "WIO_E5", 74: "RADIOMASTER_900_BANDIT",
	75: "ME25LS01_4Y10TD", 76: "RP2040_FEATHER_RFM95", 77: "M5STACK_COREBASIC", 78: "M5STACK_CORE2", 79: "RPI_PICO2",
	80: "M5STACK_CORES3", 81: "SEEED_XIAO_S3", 82: "MS24SF1", 83: "TLORA_C6", 84: "WISMESH_TAP", 85: "ROUTASTIC",
	86: "MESH_TAB", 87: "MESHLINK", 88: "XIAO_NRF52_KIT", 89: "THINKNODE_M1", 90: "THINKNODE_M2", 91: "T_ETH_ELITE",
	92: "HELTEC_SENSOR_HUB", 93: "MUZI_BASE", 94: "HELTEC_MESH_POCKET", 95: "SEEED_SOLAR_NODE", 96: "NOMADSTAR_METEOR_PRO",
	97: "CROWPANEL", 98: "LINK_32", 99: "SEEED_WIO_TRACKER_L1", 100: "SEEED_WIO_TRACKER_L1_EINK", 101: "MUZI_R1_NEO",
	102: "T_DECK_PRO", 103: "T_LORA_PAGER", 104: "M5STACK_RESERVED", 105: "WISMESH_TAG", 106: "RAK3312", 107: "THINKNODE_M5",
	108: "HELTEC_MESH_SOLAR", 109: "T_ECHO_LITE", 110: "HELTEC_V4", 111: "M5STACK_C6L", 112: "M5STACK_CARDPUTER_ADV",
	113: "HELTEC_WIRELESS_TRACKER_V2", 114: "T_WATCH_ULTRA", 115: "THINKNODE_M3", 116: "WISMESH_TAP_V2", 117: "RAK3401",
	118: "RAK6421", 119: "THINKNODE_M4", 120: "THINKNODE_M6", 121: "MESHSTICK_1262", 122: "TBEAM_1_WATT", 123: "T5_S3_EPAPER_PRO",
	124: "TBEAM_BPF", 125: "MINI_EPAPER_S3", 126: "TDISPLAY_S3_PRO", 127: "HELTEC_MESH_NODE_T096", 128: "TRACKER_T1000_E_PRO",
	129: "THINKNODE_M7", 130: "THINKNODE_M8", 131: "THINKNODE_M9", 132: "HELTEC_V4_R8", 133: "HELTEC_MESH_NODE_T1",
	134: "STATION_G3", 135: "T_IMPULSE_PLUS", 136: "T_ECHO_CARD", 255: "PRIVATE_HW",
}
