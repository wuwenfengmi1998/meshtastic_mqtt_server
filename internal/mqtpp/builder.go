package mqtpp

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

const NodeNumBroadcast uint32 = 0xffffffff

type PacketBuildOptions struct {
	FromNodeNum uint32
	ToNodeNum   uint32
	PacketID    uint32
	ChannelID   string
	GatewayID   string
	PSK         []byte
	Encrypt     bool
	ViaMQTT     bool
	HopLimit    uint32
}

type TextMessageBuildOptions struct {
	PacketBuildOptions
	Text string
}

type NodeInfoBuildOptions struct {
	PacketBuildOptions
	NodeID     string
	LongName   string
	ShortName  string
	HWModel    uint32
	Role       uint32
	IsLicensed bool
	PublicKey  []byte
}

// AckBuildOptions 描述构造一个 Routing-NONE ACK（PSK 频道路径）所需字段。
// RequestID 是被 ACK 原始包的 packet_id（写入 Data.request_id, tag 6）。
type AckBuildOptions struct {
	PacketBuildOptions
	RequestID uint32
}

func BuildTextMessageServiceEnvelope(opts TextMessageBuildOptions) ([]byte, error) {
	if opts.FromNodeNum == 0 {
		return nil, fmt.Errorf("from node number is required")
	}
	if opts.PacketID == 0 {
		return nil, fmt.Errorf("packet id is required")
	}
	if opts.ChannelID == "" {
		return nil, fmt.Errorf("channel id is required")
	}
	if strings.TrimSpace(opts.GatewayID) == "" {
		opts.GatewayID = NodeNumToID(opts.FromNodeNum)
	}
	if opts.Text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if !utf8.ValidString(opts.Text) {
		return nil, fmt.Errorf("text must be valid utf-8")
	}

	data := buildDataPacket(textMessageApp, []byte(opts.Text))
	packet, err := buildMeshPacket(opts.PacketBuildOptions, data)
	if err != nil {
		return nil, err
	}
	return buildServiceEnvelope(packet, opts.ChannelID, opts.GatewayID), nil
}

func BuildNodeInfoServiceEnvelope(opts NodeInfoBuildOptions) ([]byte, error) {
	if opts.NodeID == "" {
		opts.NodeID = NodeNumToID(opts.FromNodeNum)
	}
	opts.NodeID = truncateUTF8Bytes(opts.NodeID, 16)
	opts.LongName = truncateUTF8Bytes(strings.TrimSpace(opts.LongName), 40)
	opts.ShortName = truncateUTF8Bytes(strings.TrimSpace(opts.ShortName), 5)
	if opts.LongName == "" {
		return nil, fmt.Errorf("long name is required")
	}
	if opts.ShortName == "" {
		return nil, fmt.Errorf("short name is required")
	}
	if len(opts.PublicKey) > 32 {
		opts.PublicKey = opts.PublicKey[:32]
	}
	user := buildUserPacket(opts)
	data := buildDataPacket(nodeInfoApp, user)
	packet, err := buildMeshPacket(opts.PacketBuildOptions, data)
	if err != nil {
		return nil, err
	}
	return buildServiceEnvelope(packet, opts.ChannelID, opts.GatewayID), nil
}

// BuildAckServiceEnvelope 构造一个 PSK 频道上的 Routing ACK（error_reason=NONE）。
// 与固件 MeshModule::allocAckNak/Router::sendAckNak 行为对齐：
//   - portnum = ROUTING_APP(5)
//   - Data.request_id = 原始包 ID
//   - Routing.which_variant = error_reason，error_reason = NONE(0)
//
// 用 PacketBuildOptions 中 ChannelID + PSK 加密；调用方负责把 ToNodeNum 设为原 from。
func BuildAckServiceEnvelope(opts AckBuildOptions) ([]byte, error) {
	if opts.RequestID == 0 {
		return nil, fmt.Errorf("ack request_id is required")
	}
	if opts.ChannelID == "" {
		return nil, fmt.Errorf("channel id is required")
	}
	data := buildAckDataPacket(opts.RequestID)
	packet, err := buildMeshPacket(opts.PacketBuildOptions, data)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.GatewayID) == "" {
		opts.GatewayID = NodeNumToID(opts.FromNodeNum)
	}
	return buildServiceEnvelope(packet, opts.ChannelID, opts.GatewayID), nil
}

// buildAckDataPacket 构造 Data { portnum=ROUTING_APP, payload=Routing{error_reason=NONE}, request_id=req }。
func buildAckDataPacket(requestID uint32) []byte {
	// Routing payload: oneof variant=error_reason(tag 3), value=NONE(0) → 0x18 0x00
	routing := protowire.AppendTag(nil, 3, protowire.VarintType)
	routing = protowire.AppendVarint(routing, 0)

	var out []byte
	out = protowire.AppendTag(out, 1, protowire.VarintType)
	out = protowire.AppendVarint(out, uint64(routingApp))
	out = protowire.AppendTag(out, 2, protowire.BytesType)
	out = protowire.AppendBytes(out, routing)
	out = protowire.AppendTag(out, 6, protowire.Fixed32Type)
	out = protowire.AppendFixed32(out, requestID)
	return out
}

func NodeNumToID(nodeNum uint32) string {
	return nodeNumToID(nodeNum)
}

func truncateUTF8Bytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	out := make([]byte, 0, maxBytes)
	for _, r := range value {
		part := string(r)
		if len(out)+len(part) > maxBytes {
			break
		}
		out = append(out, part...)
	}
	return string(out)
}

func ParseNodeID(nodeID string) (uint32, error) {
	value := strings.TrimSpace(nodeID)
	if value == "" {
		return 0, fmt.Errorf("node id is required")
	}
	value = strings.TrimPrefix(value, "!")
	if len(value) != 8 {
		return 0, fmt.Errorf("node id must be !xxxxxxxx")
	}
	num, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid node id: %w", err)
	}
	return uint32(num), nil
}

func buildDataPacket(portnum uint32, payload []byte) []byte {
	var out []byte
	out = protowire.AppendTag(out, 1, protowire.VarintType)
	out = protowire.AppendVarint(out, uint64(portnum))
	out = protowire.AppendTag(out, 2, protowire.BytesType)
	out = protowire.AppendBytes(out, payload)
	return out
}

func buildUserPacket(opts NodeInfoBuildOptions) []byte {
	var out []byte
	out = protowire.AppendTag(out, 1, protowire.BytesType)
	out = protowire.AppendBytes(out, []byte(opts.NodeID))
	out = protowire.AppendTag(out, 2, protowire.BytesType)
	out = protowire.AppendBytes(out, []byte(opts.LongName))
	out = protowire.AppendTag(out, 3, protowire.BytesType)
	out = protowire.AppendBytes(out, []byte(opts.ShortName))
	if opts.HWModel != 0 {
		out = protowire.AppendTag(out, 5, protowire.VarintType)
		out = protowire.AppendVarint(out, uint64(opts.HWModel))
	}
	out = protowire.AppendTag(out, 6, protowire.VarintType)
	if opts.IsLicensed {
		out = protowire.AppendVarint(out, 1)
	} else {
		out = protowire.AppendVarint(out, 0)
	}
	out = protowire.AppendTag(out, 7, protowire.VarintType)
	out = protowire.AppendVarint(out, uint64(opts.Role))
	if len(opts.PublicKey) > 0 {
		out = protowire.AppendTag(out, 8, protowire.BytesType)
		out = protowire.AppendBytes(out, opts.PublicKey)
	}
	return out
}

func buildMeshPacket(opts PacketBuildOptions, data []byte) ([]byte, error) {
	if opts.FromNodeNum == 0 {
		return nil, fmt.Errorf("from node number is required")
	}
	if opts.PacketID == 0 {
		return nil, fmt.Errorf("packet id is required")
	}
	if opts.ChannelID == "" {
		return nil, fmt.Errorf("channel id is required")
	}
	if strings.TrimSpace(opts.GatewayID) == "" {
		opts.GatewayID = NodeNumToID(opts.FromNodeNum)
	}
	var out []byte
	out = protowire.AppendTag(out, 1, protowire.Fixed32Type)
	out = protowire.AppendFixed32(out, opts.FromNodeNum)
	out = protowire.AppendTag(out, 2, protowire.Fixed32Type)
	out = protowire.AppendFixed32(out, opts.ToNodeNum)

	if opts.Encrypt {
		if len(opts.PSK) == 0 {
			return nil, fmt.Errorf("psk is required for encrypted packet")
		}
		ciphertext, err := cryptAESCTR(opts.PSK, opts.FromNodeNum, opts.PacketID, data)
		if err != nil {
			return nil, err
		}
		out = protowire.AppendTag(out, 3, protowire.VarintType)
		out = protowire.AppendVarint(out, uint64(channelHash(opts.ChannelID, opts.PSK)))
		out = protowire.AppendTag(out, 5, protowire.BytesType)
		out = protowire.AppendBytes(out, ciphertext)
	} else {
		out = protowire.AppendTag(out, 4, protowire.BytesType)
		out = protowire.AppendBytes(out, data)
	}

	out = protowire.AppendTag(out, 6, protowire.Fixed32Type)
	out = protowire.AppendFixed32(out, opts.PacketID)
	if opts.ViaMQTT {
		out = protowire.AppendTag(out, 14, protowire.VarintType)
		out = protowire.AppendVarint(out, 1)
	}
	hop := opts.HopLimit
	if hop == 0 {
		hop = 7
	}
	out = protowire.AppendTag(out, 9, protowire.VarintType)
	out = protowire.AppendVarint(out, uint64(hop))
	out = protowire.AppendTag(out, 15, protowire.VarintType)
	out = protowire.AppendVarint(out, uint64(hop))
	return out, nil
}

func buildServiceEnvelope(packet []byte, channelID string, gatewayID string) []byte {
	var out []byte
	out = protowire.AppendTag(out, 1, protowire.BytesType)
	out = protowire.AppendBytes(out, packet)
	out = protowire.AppendTag(out, 2, protowire.BytesType)
	out = protowire.AppendBytes(out, []byte(channelID))
	out = protowire.AppendTag(out, 3, protowire.BytesType)
	out = protowire.AppendBytes(out, []byte(gatewayID))
	return out
}
