package mqtpp

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

const NodeNumBroadcast uint32 = 0xffffffff

type TextMessageBuildOptions struct {
	FromNodeNum uint32
	ToNodeNum   uint32
	PacketID    uint32
	ChannelID   string
	GatewayID   string
	Text        string
	PSK         []byte
	Encrypt     bool
	ViaMQTT     bool
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
	packet, err := buildMeshPacket(opts, data)
	if err != nil {
		return nil, err
	}
	return buildServiceEnvelope(packet, opts.ChannelID, opts.GatewayID), nil
}

func NodeNumToID(nodeNum uint32) string {
	return nodeNumToID(nodeNum)
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

func buildMeshPacket(opts TextMessageBuildOptions, data []byte) ([]byte, error) {
	var out []byte
	out = protowire.AppendTag(out, 1, protowire.Fixed32Type)
	out = protowire.AppendFixed32(out, opts.FromNodeNum)
	out = protowire.AppendTag(out, 2, protowire.Fixed32Type)
	out = protowire.AppendFixed32(out, opts.ToNodeNum)

	if opts.Encrypt {
		if len(opts.PSK) == 0 {
			return nil, fmt.Errorf("psk is required for encrypted text message")
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
