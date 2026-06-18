package mqtpp

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
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

type Options struct {
	AllowEncryptedForwarding bool
	// PKIKeyResolver 在解密 PKI 加密包时被调用：toNodeNum 是包的接收者节点号（应为本地受管节点，
	// 例如机器人），fromNodeNum 是发送方节点号。回调需要返回接收方的 X25519 私钥（32B）和发送方
	// 的 X25519 公钥（32B）。当回调缺失或返回 ok=false 时，PKI 解密会被跳过（仍尝试 channel PSK）。
	PKIKeyResolver func(toNodeNum, fromNodeNum uint32) (privateKey, fromPublicKey []byte, ok bool)
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
	WantAck        bool
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

type positionInfo struct {
	LatitudeI                 *int32
	LongitudeI                *int32
	Altitude                  *int32
	Time                      uint32
	LocationSource            uint64
	AltitudeSource            uint64
	Timestamp                 uint32
	TimestampMillisAdjust     int32
	AltitudeHAE               *int32
	AltitudeGeoidalSeparation *int32
	PDOP                      uint32
	HDOP                      uint32
	VDOP                      uint32
	GPSAccuracy               uint32
	GroundSpeed               *uint32
	GroundTrack               *uint32
	FixQuality                uint32
	FixType                   uint32
	SatsInView                uint32
	SensorID                  uint32
	NextUpdate                uint32
	SeqNumber                 uint32
	PrecisionBits             uint32
}

type telemetryInfo struct {
	Time    uint32
	Type    string
	Metrics map[string]any
}

// MQTTPP 处理一个 MQTT 原始 payload，返回合规状态、原始数据和解码后的记录。
// 第一个返回值表示数据是否合规；第二个返回值在不合规时为 nil；第三个返回值是解码结果记录。
func MQTTPP(topic string, raw []byte, key []byte, opts Options) (bool, []byte, map[string]any) {

	env, err := parseServiceEnvelope(raw)
	if err != nil {
		//解包失败
		return false, nil, map[string]any{"topic": topic, "error": "protobuf decode failed: " + err.Error(), "payload_len": len(raw)}
	}
	record, err := describePacket(topic, env, key, opts)
	if err != nil {
		//解码失败
		return false, nil, map[string]any{"topic": topic, "error": err.Error(), "payload_len": len(raw)}
	}
	if record["type"] == "encrypted_packet" && !opts.AllowEncryptedForwarding {
		record["error"] = "cannot be decrypted"
		return false, nil, record
	}

	return true, raw, record
}

// ExpandPSK 展开 Base64 PSK，兼容 Meshtastic 默认索引 PSK 和短 key 补零规则。
func ExpandPSK(pskBase64 string) ([]byte, error) {
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

// MustJSON 将记录编码成 JSON；编码失败时返回包含错误信息的 JSON。
func MustJSON(record map[string]any) []byte {
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
		case 10:
			if v, ok := value.(uint64); ok && typ == protowire.VarintType {
				packet.WantAck = v != 0
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

// parsePosition 解析 POSITION_APP 的 Position payload。
func parsePosition(payload []byte) (*positionInfo, error) {
	position := &positionInfo{}
	err := walkFields(payload, func(num protowire.Number, typ protowire.Type, value any) error {
		switch num {
		case 1:
			if v, ok := value.(uint32); ok && typ == protowire.Fixed32Type {
				position.LatitudeI = int32Ptr(int32(v))
			}
		case 2:
			if v, ok := value.(uint32); ok && typ == protowire.Fixed32Type {
				position.LongitudeI = int32Ptr(int32(v))
			}
		case 3:
			if typ == protowire.VarintType {
				position.Altitude = int32Ptr(int32(varintValue(typ, value)))
			}
		case 4:
			if v, ok := value.(uint32); ok && typ == protowire.Fixed32Type {
				position.Time = v
			}
		case 5:
			position.LocationSource = varintValue(typ, value)
		case 6:
			position.AltitudeSource = varintValue(typ, value)
		case 7:
			if v, ok := value.(uint32); ok && typ == protowire.Fixed32Type {
				position.Timestamp = v
			}
		case 8:
			if typ == protowire.VarintType {
				position.TimestampMillisAdjust = int32(varintValue(typ, value))
			}
		case 9:
			if typ == protowire.VarintType {
				position.AltitudeHAE = int32Ptr(decodeZigZag32(varintValue(typ, value)))
			}
		case 10:
			if typ == protowire.VarintType {
				position.AltitudeGeoidalSeparation = int32Ptr(decodeZigZag32(varintValue(typ, value)))
			}
		case 11:
			position.PDOP = uint32(varintValue(typ, value))
		case 12:
			position.HDOP = uint32(varintValue(typ, value))
		case 13:
			position.VDOP = uint32(varintValue(typ, value))
		case 14:
			position.GPSAccuracy = uint32(varintValue(typ, value))
		case 15:
			position.GroundSpeed = uint32Ptr(uint32(varintValue(typ, value)))
		case 16:
			position.GroundTrack = uint32Ptr(uint32(varintValue(typ, value)))
		case 17:
			position.FixQuality = uint32(varintValue(typ, value))
		case 18:
			position.FixType = uint32(varintValue(typ, value))
		case 19:
			position.SatsInView = uint32(varintValue(typ, value))
		case 20:
			position.SensorID = uint32(varintValue(typ, value))
		case 21:
			position.NextUpdate = uint32(varintValue(typ, value))
		case 22:
			position.SeqNumber = uint32(varintValue(typ, value))
		case 23:
			position.PrecisionBits = uint32(varintValue(typ, value))
		}
		return nil
	})
	return position, err
}

// parseTelemetry 解析 TELEMETRY_APP 的 Telemetry payload 和具体 telemetry variant。
func parseTelemetry(payload []byte) (*telemetryInfo, error) {
	telemetry := &telemetryInfo{}
	err := walkFields(payload, func(num protowire.Number, typ protowire.Type, value any) error {
		switch num {
		case 1:
			if v, ok := value.(uint32); ok && typ == protowire.Fixed32Type {
				telemetry.Time = v
			}
		case 2:
			telemetry.Type = "device_metrics"
			telemetry.Metrics = parseMetricBytes(typ, value, deviceMetricFields)
		case 3:
			telemetry.Type = "environment_metrics"
			telemetry.Metrics = parseMetricBytes(typ, value, environmentMetricFields)
		case 4:
			telemetry.Type = "air_quality_metrics"
			telemetry.Metrics = parseMetricBytes(typ, value, airQualityMetricFields)
		case 5:
			telemetry.Type = "power_metrics"
			telemetry.Metrics = parseMetricBytes(typ, value, powerMetricFields)
		case 6:
			telemetry.Type = "local_stats"
			telemetry.Metrics = parseMetricBytes(typ, value, localStatsFields)
		case 7:
			telemetry.Type = "health_metrics"
			telemetry.Metrics = parseMetricBytes(typ, value, healthMetricFields)
		case 8:
			telemetry.Type = "host_metrics"
			telemetry.Metrics = parseMetricBytes(typ, value, hostMetricFields)
		case 9:
			telemetry.Type = "traffic_management_stats"
			telemetry.Metrics = parseMetricBytes(typ, value, trafficManagementFields)
		}
		return nil
	})
	return telemetry, err
}

type metricKind int

const (
	metricUint32 metricKind = iota
	metricUint64
	metricInt32
	metricFloat32
	metricString
	metricRepeatedFloat32
)

type metricField struct {
	Name string
	Kind metricKind
}

// parseMetricBytes 按字段定义表解析 telemetry variant 的指标字段。
func parseMetricBytes(typ protowire.Type, value any, fields map[protowire.Number]metricField) map[string]any {
	metrics := map[string]any{}
	payload, ok := value.([]byte)
	if !ok || typ != protowire.BytesType {
		return metrics
	}
	_ = walkFields(payload, func(num protowire.Number, typ protowire.Type, value any) error {
		field, ok := fields[num]
		if !ok {
			return nil
		}
		switch field.Kind {
		case metricUint32:
			metrics[field.Name] = uint32(varintValue(typ, value))
		case metricUint64:
			metrics[field.Name] = varintValue(typ, value)
		case metricInt32:
			metrics[field.Name] = int32(varintValue(typ, value))
		case metricFloat32:
			if v, ok := value.(uint32); ok && typ == protowire.Fixed32Type {
				metrics[field.Name] = float64(math.Float32frombits(v))
			}
		case metricString:
			metrics[field.Name] = stringBytes(typ, value)
		case metricRepeatedFloat32:
			if v, ok := value.(uint32); ok && typ == protowire.Fixed32Type {
				appendMetric(metrics, field.Name, float64(math.Float32frombits(v)))
			}
			if payload, ok := value.([]byte); ok && typ == protowire.BytesType {
				for len(payload) > 0 {
					v, n := protowire.ConsumeFixed32(payload)
					if n < 0 {
						break
					}
					appendMetric(metrics, field.Name, float64(math.Float32frombits(v)))
					payload = payload[n:]
				}
			}
		}
		return nil
	})
	return metrics
}

// appendMetric 追加 repeated telemetry 字段值。
func appendMetric(metrics map[string]any, name string, value any) {
	if existing, ok := metrics[name]; ok {
		metrics[name] = append(existing.([]any), value)
		return
	}
	metrics[name] = []any{value}
}

// int32Ptr 返回 int32 指针，用于记录 proto optional 字段是否出现。
func int32Ptr(v int32) *int32 {
	return &v
}

// uint32Ptr 返回 uint32 指针，用于记录 proto optional 字段是否出现。
func uint32Ptr(v uint32) *uint32 {
	return &v
}

// decodeZigZag32 解码 protobuf sint32 的 zig-zag 编码。
func decodeZigZag32(v uint64) int32 {
	return int32((v >> 1) ^ uint64(-int64(v&1)))
}

// optionalInt32 把 optional int32 指针转换成 JSON 可表达的值。
func optionalInt32(v *int32) any {
	if v == nil {
		return nil
	}
	return *v
}

// optionalUint32 把 optional uint32 指针转换成 JSON 可表达的值。
func optionalUint32(v *uint32) any {
	if v == nil {
		return nil
	}
	return *v
}

// optionalCoordinate 把 Meshtastic 1e-7 度坐标转换成浮点经纬度。
func optionalCoordinate(v *int32) any {
	if v == nil {
		return nil
	}
	return float64(*v) * 1e-7
}

// optionalDegrees100 把 1/100 度单位转换成度。
func optionalDegrees100(v *uint32) any {
	if v == nil {
		return nil
	}
	return float64(*v) / 100
}

// dopValue 把 1/100 精度因子转换成浮点值，未设置时返回 nil。
func dopValue(v uint32) any {
	if v == 0 {
		return nil
	}
	return float64(v) / 100
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
func describePacket(topic string, env *serviceEnvelope, key []byte, opts Options) (map[string]any, error) {
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
		"want_ack":        packet.WantAck,
		"via_mqtt":        packet.ViaMQTT,
		"pki_encrypted":   packet.PKIEncrypted,
	}

	if packet.PayloadVariant == "encrypted" {
		decryptedPacket, decryptStatus := tryDecryptPacket(packet, env.ChannelID, key, opts)
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
		decrypted, err := describePacket(topic, &decryptedEnv, key, opts)
		if err != nil {
			return nil, err
		}
		decrypted["payload_variant"] = "decoded"
		decrypted["decrypt_success"] = true
		decrypted["decrypt_status"] = decryptStatus
		// PKI 解密的包要保留 pki_encrypted 标记（tryDecryptPacket 在成功后会把它标记到 packet 上）
		decrypted["pki_encrypted"] = decryptedPacket.PKIEncrypted
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
		record, err := decodePosition(packet)
		if err != nil {
			return nil, err
		}
		return merge(decodedBase, record), nil
	case telemetryApp:
		record, err := decodeTelemetry(packet)
		if err != nil {
			return nil, err
		}
		return merge(decodedBase, record), nil
	case routingApp:
		return merge(decodedBase, map[string]any{"type": "routing"}), nil
	case tracerouteApp:
		return merge(decodedBase, map[string]any{"type": "traceroute"}), nil
	default:
		return merge(decodedBase, map[string]any{"type": "decoded_packet"}), nil
	}
}

// tryDecryptPacket 尝试解密 encrypted MeshPacket，并返回解密状态。
// 解密优先级（与固件 perhapsDecode 对齐）：
//  1. 若包是 PKI 风格（channel=0、to 非广播、PSK 无关）且调用方提供了 PKIKeyResolver，
//     则用 X25519 + AES-CCM(M=8,L=2) 解密。
//  2. 否则回落到 channel PSK + AES-CTR 路径。
func tryDecryptPacket(packet *meshPacket, channelID string, key []byte, opts Options) (*meshPacket, string) {
	// 先尝试 PKI 路径：固件发出的 PKI 包 channel=0、to 非广播、长度 > pkcOverhead。
	// channel_id 字面量在 ServiceEnvelope 上一般是 "PKI"，但有些转发路径会保留原 channel 名，
	// 因此这里以 channel 字段=0 + 注册了 resolver 为充分条件即尝试解密。
	if opts.PKIKeyResolver != nil && packet.Channel == 0 && packet.To != 0 && packet.To != NodeNumBroadcast &&
		len(packet.Encrypted) > pkcOverhead {
		if decrypted, status, ok := tryDecryptPKIPacket(packet, opts.PKIKeyResolver); ok {
			return decrypted, status
		}
	}

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

// tryDecryptPKIPacket 用接收方私钥 + 发送方公钥派生共享密钥并 AES-CCM 解密。
// 第三个返回值表示是否“尝试且解出了合法 Data 包”——返回 false 时调用方会回落到 PSK 路径。
func tryDecryptPKIPacket(packet *meshPacket, resolver func(toNodeNum, fromNodeNum uint32) ([]byte, []byte, bool)) (*meshPacket, string, bool) {
	privateKey, fromPublic, ok := resolver(packet.To, packet.From)
	if !ok {
		return nil, "", false
	}
	if len(privateKey) != 32 || len(fromPublic) != 32 {
		return nil, "", false
	}
	encryptedLen := len(packet.Encrypted) - pkcOverhead
	ciphertext := packet.Encrypted[:encryptedLen]
	auth := packet.Encrypted[encryptedLen : encryptedLen+8]
	extraNonce := binary.LittleEndian.Uint32(packet.Encrypted[encryptedLen+8:])
	sharedKey, err := pkiSharedKey(privateKey, fromPublic)
	if err != nil {
		return nil, "", false
	}
	plaintext, err := aesCCMDecrypt(sharedKey, pkiNonce(packet.ID, packet.From, extraNonce), ciphertext, auth)
	if err != nil {
		return nil, "", false
	}
	decoded, err := parseDataPacket(plaintext)
	if err != nil {
		return nil, "", false
	}
	if decoded.Portnum == unknownApp {
		return nil, "", false
	}
	decrypted := *packet
	decrypted.Encrypted = nil
	decrypted.Decoded = decoded
	decrypted.PayloadVariant = "decoded"
	decrypted.PKIEncrypted = true
	return &decrypted, "pki success", true
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

// decodePosition 将 POSITION_APP payload 解码为位置 JSON 字段。
func decodePosition(packet *meshPacket) (map[string]any, error) {
	position, err := parsePosition(packet.Decoded.Payload)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"type":                        "position",
		"from":                        nodeNumToID(packet.From),
		"from_num":                    packet.From,
		"latitude":                    optionalCoordinate(position.LatitudeI),
		"longitude":                   optionalCoordinate(position.LongitudeI),
		"altitude":                    optionalInt32(position.Altitude),
		"time":                        position.Time,
		"location_source":             enumName(locationSourceNames, position.LocationSource),
		"altitude_source":             enumName(altitudeSourceNames, position.AltitudeSource),
		"timestamp":                   position.Timestamp,
		"timestamp_millis_adjust":     position.TimestampMillisAdjust,
		"altitude_hae":                optionalInt32(position.AltitudeHAE),
		"altitude_geoidal_separation": optionalInt32(position.AltitudeGeoidalSeparation),
		"pdop":                        dopValue(position.PDOP),
		"hdop":                        dopValue(position.HDOP),
		"vdop":                        dopValue(position.VDOP),
		"gps_accuracy":                position.GPSAccuracy,
		"ground_speed":                optionalUint32(position.GroundSpeed),
		"ground_track":                optionalDegrees100(position.GroundTrack),
		"fix_quality":                 position.FixQuality,
		"fix_type":                    position.FixType,
		"sats_in_view":                position.SatsInView,
		"sensor_id":                   position.SensorID,
		"next_update":                 position.NextUpdate,
		"seq_number":                  position.SeqNumber,
		"precision_bits":              position.PrecisionBits,
	}, nil
}

// decodeTelemetry 将 TELEMETRY_APP payload 解码为遥测 JSON 字段。
func decodeTelemetry(packet *meshPacket) (map[string]any, error) {
	telemetry, err := parseTelemetry(packet.Decoded.Payload)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"type":           "telemetry",
		"from":           nodeNumToID(packet.From),
		"from_num":       packet.From,
		"time":           telemetry.Time,
		"telemetry_type": telemetry.Type,
		"metrics":        telemetry.Metrics,
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
	return cryptAESCTR(key, fromNum, packetID, ciphertext)
}

// cryptAESCTR 按 Meshtastic nonce 规则执行 AES-CTR；CTR 加密和解密是同一个 XOR 流操作。
func cryptAESCTR(key []byte, fromNum, packetID uint32, input []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aes.BlockSize)
	binary.LittleEndian.PutUint64(nonce[0:8], uint64(packetID))
	binary.LittleEndian.PutUint32(nonce[8:12], fromNum)
	output := make([]byte, len(input))
	cipher.NewCTR(block, nonce).XORKeyStream(output, input)
	return output, nil
}

// enumName 把已知枚举值转换成名称，未知值保留为数字。
func enumName(names map[uint64]string, value uint64) any {
	if name, ok := names[value]; ok {
		return name
	}
	return value
}

var locationSourceNames = map[uint64]string{
	0: "LOC_UNSET",
	1: "LOC_MANUAL",
	2: "LOC_INTERNAL",
	3: "LOC_EXTERNAL",
}

var altitudeSourceNames = map[uint64]string{
	0: "ALT_UNSET",
	1: "ALT_MANUAL",
	2: "ALT_INTERNAL",
	3: "ALT_EXTERNAL",
	4: "ALT_BAROMETRIC",
}

var deviceMetricFields = map[protowire.Number]metricField{
	1: {"battery_level", metricUint32},
	2: {"voltage", metricFloat32},
	3: {"channel_utilization", metricFloat32},
	4: {"air_util_tx", metricFloat32},
	5: {"uptime_seconds", metricUint32},
}

var environmentMetricFields = map[protowire.Number]metricField{
	1:  {"temperature", metricFloat32},
	2:  {"relative_humidity", metricFloat32},
	3:  {"barometric_pressure", metricFloat32},
	4:  {"gas_resistance", metricFloat32},
	5:  {"voltage", metricFloat32},
	6:  {"current", metricFloat32},
	7:  {"iaq", metricUint32},
	8:  {"distance", metricFloat32},
	9:  {"lux", metricFloat32},
	10: {"white_lux", metricFloat32},
	11: {"ir_lux", metricFloat32},
	12: {"uv_lux", metricFloat32},
	13: {"wind_direction", metricUint32},
	14: {"wind_speed", metricFloat32},
	15: {"weight", metricFloat32},
	16: {"wind_gust", metricFloat32},
	17: {"wind_lull", metricFloat32},
	18: {"radiation", metricFloat32},
	19: {"rainfall_1h", metricFloat32},
	20: {"rainfall_24h", metricFloat32},
	21: {"soil_moisture", metricUint32},
	22: {"soil_temperature", metricFloat32},
	23: {"one_wire_temperature", metricRepeatedFloat32},
}

var airQualityMetricFields = map[protowire.Number]metricField{
	1:  {"pm10_standard", metricUint32},
	2:  {"pm25_standard", metricUint32},
	3:  {"pm100_standard", metricUint32},
	4:  {"pm10_environmental", metricUint32},
	5:  {"pm25_environmental", metricUint32},
	6:  {"pm100_environmental", metricUint32},
	7:  {"particles_03um", metricUint32},
	8:  {"particles_05um", metricUint32},
	9:  {"particles_10um", metricUint32},
	10: {"particles_25um", metricUint32},
	11: {"particles_50um", metricUint32},
	12: {"particles_100um", metricUint32},
	13: {"co2", metricUint32},
	14: {"co2_temperature", metricFloat32},
	15: {"co2_humidity", metricFloat32},
	16: {"form_formaldehyde", metricFloat32},
	17: {"form_humidity", metricFloat32},
	18: {"form_temperature", metricFloat32},
	19: {"pm40_standard", metricUint32},
	20: {"particles_40um", metricUint32},
	21: {"pm_temperature", metricFloat32},
	22: {"pm_humidity", metricFloat32},
	23: {"pm_voc_idx", metricFloat32},
	24: {"pm_nox_idx", metricFloat32},
	25: {"particles_tps", metricFloat32},
}

var powerMetricFields = map[protowire.Number]metricField{
	1: {"ch1_voltage", metricFloat32}, 2: {"ch1_current", metricFloat32},
	3: {"ch2_voltage", metricFloat32}, 4: {"ch2_current", metricFloat32},
	5: {"ch3_voltage", metricFloat32}, 6: {"ch3_current", metricFloat32},
	7: {"ch4_voltage", metricFloat32}, 8: {"ch4_current", metricFloat32},
	9: {"ch5_voltage", metricFloat32}, 10: {"ch5_current", metricFloat32},
	11: {"ch6_voltage", metricFloat32}, 12: {"ch6_current", metricFloat32},
	13: {"ch7_voltage", metricFloat32}, 14: {"ch7_current", metricFloat32},
	15: {"ch8_voltage", metricFloat32}, 16: {"ch8_current", metricFloat32},
}

var localStatsFields = map[protowire.Number]metricField{
	1:  {"uptime_seconds", metricUint32},
	2:  {"channel_utilization", metricFloat32},
	3:  {"air_util_tx", metricFloat32},
	4:  {"num_packets_tx", metricUint32},
	5:  {"num_packets_rx", metricUint32},
	6:  {"num_packets_rx_bad", metricUint32},
	7:  {"num_online_nodes", metricUint32},
	8:  {"num_total_nodes", metricUint32},
	9:  {"num_rx_dupe", metricUint32},
	10: {"num_tx_relay", metricUint32},
	11: {"num_tx_relay_canceled", metricUint32},
	12: {"heap_total_bytes", metricUint32},
	13: {"heap_free_bytes", metricUint32},
	14: {"num_tx_dropped", metricUint32},
	15: {"noise_floor", metricInt32},
}

var healthMetricFields = map[protowire.Number]metricField{
	1: {"heart_bpm", metricUint32},
	2: {"spO2", metricUint32},
	3: {"temperature", metricFloat32},
}

var hostMetricFields = map[protowire.Number]metricField{
	1: {"uptime_seconds", metricUint32},
	2: {"freemem_bytes", metricUint64},
	3: {"diskfree1_bytes", metricUint64},
	4: {"diskfree2_bytes", metricUint64},
	5: {"diskfree3_bytes", metricUint64},
	6: {"load1", metricUint32},
	7: {"load5", metricUint32},
	8: {"load15", metricUint32},
	9: {"user_string", metricString},
}

var trafficManagementFields = map[protowire.Number]metricField{
	1: {"packets_inspected", metricUint32},
	2: {"position_dedup_drops", metricUint32},
	3: {"nodeinfo_cache_hits", metricUint32},
	4: {"rate_limit_drops", metricUint32},
	5: {"unknown_packet_drops", metricUint32},
	6: {"hop_exhausted_packets", metricUint32},
	7: {"router_hops_preserved", metricUint32},
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
