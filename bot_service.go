package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"meshtastic_mqtt_server/mqtpp"

	mqtt "github.com/mochi-mqtt/server/v2"
)

const botMaxTextBytes = 200

type botSendTextRequest struct {
	BotID       uint64
	MessageType string
	ChannelID   string
	ToNodeID    string
	ToNodeNum   *int64
	Text        string
	CreatedBy   string
}

type botTextSender interface {
	SendText(ctx context.Context, req botSendTextRequest) (*botMessageRecord, error)
}

type botService struct {
	store  *store
	server *mqtt.Server
	key    []byte
}

func newBotService(store *store, server *mqtt.Server, key []byte) *botService {
	return &botService{store: store, server: server, key: key}
}

func (s *botService) SendText(_ context.Context, req botSendTextRequest) (*botMessageRecord, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("bot service is not configured")
	}
	bot, err := s.store.GetBotNode(req.BotID)
	if err != nil {
		return nil, err
	}
	if !bot.Enabled {
		return nil, fmt.Errorf("bot node is disabled")
	}
	messageType, err := normalizeBotMessageType(req.MessageType)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if !utf8.ValidString(text) {
		return nil, fmt.Errorf("text must be valid utf-8")
	}
	if len([]byte(text)) > botMaxTextBytes {
		return nil, fmt.Errorf("text is too long, max %d bytes", botMaxTextBytes)
	}
	channelID := strings.TrimSpace(req.ChannelID)
	if channelID == "" {
		channelID = bot.DefaultChannelID
	}
	if channelID == "" {
		return nil, fmt.Errorf("channel id is required")
	}
	toNodeNum, toNodeID, err := botMessageTarget(messageType, req)
	if err != nil {
		return nil, err
	}
	packetID, err := randomPacketID()
	if err != nil {
		return nil, err
	}
	fromNodeNum := uint32(bot.NodeNum)
	raw, err := mqtpp.BuildTextMessageServiceEnvelope(mqtpp.TextMessageBuildOptions{
		FromNodeNum: fromNodeNum,
		ToNodeNum:   uint32(toNodeNum),
		PacketID:    packetID,
		ChannelID:   channelID,
		GatewayID:   bot.NodeID,
		Text:        text,
		PSK:         s.key,
		Encrypt:     true,
		ViaMQTT:     true,
	})
	if err != nil {
		return nil, err
	}
	topic := strings.Trim(bot.TopicPrefix, "/") + "/" + channelID + "/" + bot.NodeID
	row := &botMessageRecord{
		BotID:       bot.ID,
		BotNodeID:   bot.NodeID,
		BotNodeNum:  bot.NodeNum,
		MessageType: messageType,
		ChannelID:   channelID,
		ToNodeID:    toNodeID,
		ToNodeNum:   int64PtrOrNil(toNodeNum, messageType == botMessageTypeDirect),
		Topic:       topic,
		PacketID:    int64(packetID),
		Text:        text,
		PayloadLen:  int64(len(raw)),
		Encrypted:   true,
		Status:      botMessageStatusPending,
		CreatedBy:   strings.TrimSpace(req.CreatedBy),
	}
	if err := s.store.InsertBotMessage(row); err != nil {
		return nil, err
	}
	if s.server == nil {
		_ = s.store.UpdateBotMessageStatus(row.ID, botMessageStatusFailed, "mqtt server is not configured", nil)
		row.Status = botMessageStatusFailed
		row.Error = "mqtt server is not configured"
		return row, fmt.Errorf("mqtt server is not configured")
	}
	if err := s.server.Publish(topic, raw, false, 0); err != nil {
		_ = s.store.UpdateBotMessageStatus(row.ID, botMessageStatusFailed, err.Error(), nil)
		row.Status = botMessageStatusFailed
		row.Error = err.Error()
		return row, err
	}
	now := time.Now()
	if err := s.store.UpdateBotMessageStatus(row.ID, botMessageStatusPublished, "", &now); err != nil {
		return nil, err
	}
	row.Status = botMessageStatusPublished
	row.Error = ""
	row.PublishedAt = &now
	return row, nil
}

func normalizeBotMessageType(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "", botMessageTypeChannel:
		return botMessageTypeChannel, nil
	case botMessageTypeDirect:
		return botMessageTypeDirect, nil
	default:
		return "", fmt.Errorf("message type must be channel or direct")
	}
}

func botMessageTarget(messageType string, req botSendTextRequest) (int64, *string, error) {
	if messageType == botMessageTypeChannel {
		return int64(mqtpp.NodeNumBroadcast), nil, nil
	}
	if req.ToNodeNum != nil && *req.ToNodeNum > 0 {
		if err := validateBotNodeNum(*req.ToNodeNum); err != nil {
			return 0, nil, err
		}
		nodeID := mqtpp.NodeNumToID(uint32(*req.ToNodeNum))
		return *req.ToNodeNum, &nodeID, nil
	}
	toNodeID := strings.TrimSpace(req.ToNodeID)
	if toNodeID == "" {
		return 0, nil, fmt.Errorf("target node is required for direct message")
	}
	nodeNum, err := mqtpp.ParseNodeID(toNodeID)
	if err != nil {
		return 0, nil, err
	}
	if err := validateBotNodeNum(int64(nodeNum)); err != nil {
		return 0, nil, err
	}
	normalized := mqtpp.NodeNumToID(nodeNum)
	return int64(nodeNum), &normalized, nil
}

func randomPacketID() (uint32, error) {
	for i := 0; i < 8; i++ {
		var buf [4]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return 0, err
		}
		id := binary.LittleEndian.Uint32(buf[:])
		if id != 0 {
			return id, nil
		}
	}
	return 0, fmt.Errorf("generate packet id failed")
}

func int64PtrOrNil(value int64, ok bool) *int64 {
	if !ok {
		return nil
	}
	return &value
}
