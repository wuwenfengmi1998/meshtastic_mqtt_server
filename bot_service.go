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
	PublishNodeInfoByID(ctx context.Context, id uint64) (*botNodeRecord, error)
}

type botService struct {
	store  *store
	server *mqtt.Server
	key    []byte
}

func newBotService(store *store, server *mqtt.Server, key []byte) *botService {
	return &botService{store: store, server: server, key: key}
}

func (s *botService) StartNodeInfoBroadcaster(ctx context.Context) {
	if s == nil || s.store == nil || s.server == nil {
		return
	}
	go s.runNodeInfoBroadcaster(ctx)
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
	psk := strings.TrimSpace(bot.PSK)
	if psk == "" {
		psk = botDefaultPSK
	}
	key, err := mqtpp.ExpandPSK(psk)
	if err != nil {
		return nil, err
	}
	fromNodeNum := uint32(bot.NodeNum)
	raw, err := mqtpp.BuildTextMessageServiceEnvelope(mqtpp.TextMessageBuildOptions{
		PacketBuildOptions: mqtpp.PacketBuildOptions{
			FromNodeNum: fromNodeNum,
			ToNodeNum:   uint32(toNodeNum),
			PacketID:    packetID,
			ChannelID:   channelID,
			GatewayID:   bot.NodeID,
			PSK:         key,
			Encrypt:     true,
			ViaMQTT:     true,
		},
		Text: text,
	})
	if err != nil {
		return nil, err
	}
	topic := botMQTTTopic(bot.TopicPrefix, channelID, bot.NodeID)
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

func (s *botService) runNodeInfoBroadcaster(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	s.broadcastDueNodeInfo(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.broadcastDueNodeInfo(ctx)
		}
	}
}

func (s *botService) broadcastDueNodeInfo(ctx context.Context) {
	rows, err := s.store.ListBotNodes(listOptions{Limit: 500})
	if err != nil {
		printJSON(map[string]any{"event": "bot_nodeinfo_broadcast_failed", "error": err.Error()})
		return
	}
	now := time.Now()
	for _, bot := range rows {
		if ctx.Err() != nil {
			return
		}
		if !bot.Enabled || !bot.NodeInfoBroadcastEnabled {
			continue
		}
		interval := time.Duration(bot.NodeInfoBroadcastIntervalSeconds) * time.Second
		if interval <= 0 {
			interval = time.Duration(botDefaultNodeInfoBroadcastSeconds) * time.Second
		}
		if bot.LastNodeInfoBroadcastAt != nil && now.Sub(*bot.LastNodeInfoBroadcastAt) < interval {
			continue
		}
		if err := s.PublishNodeInfo(ctx, bot); err != nil {
			printJSON(map[string]any{"event": "bot_nodeinfo_broadcast_failed", "bot_id": bot.ID, "node_id": bot.NodeID, "error": err.Error()})
		}
	}
}

func (s *botService) PublishNodeInfoByID(ctx context.Context, id uint64) (*botNodeRecord, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("bot service is not configured")
	}
	bot, err := s.store.GetBotNode(id)
	if err != nil {
		return nil, err
	}
	if !bot.Enabled {
		return nil, fmt.Errorf("bot node is disabled")
	}
	if err := s.PublishNodeInfo(ctx, *bot); err != nil {
		return nil, err
	}
	updated, err := s.store.GetBotNode(id)
	if err != nil {
		return bot, nil
	}
	return updated, nil
}

func (s *botService) PublishNodeInfo(_ context.Context, bot botNodeRecord) error {
	if s == nil || s.server == nil {
		return fmt.Errorf("mqtt server is not configured")
	}
	if strings.TrimSpace(bot.PublicKey) == "" && s.store != nil {
		updated, err := s.store.RegenerateBotNodeKeys(bot.ID)
		if err != nil {
			return err
		}
		bot = *updated
	}
	psk := strings.TrimSpace(bot.PSK)
	if psk == "" {
		psk = botDefaultPSK
	}
	key, err := mqtpp.ExpandPSK(psk)
	if err != nil {
		return err
	}
	packetID, err := randomPacketID()
	if err != nil {
		return err
	}
	publicKey, err := decodeBotPublicKey(bot)
	if err != nil {
		return err
	}
	raw, err := mqtpp.BuildNodeInfoServiceEnvelope(mqtpp.NodeInfoBuildOptions{
		PacketBuildOptions: mqtpp.PacketBuildOptions{
			FromNodeNum: uint32(bot.NodeNum),
			ToNodeNum:   mqtpp.NodeNumBroadcast,
			PacketID:    packetID,
			ChannelID:   bot.DefaultChannelID,
			GatewayID:   bot.NodeID,
			PSK:         key,
			Encrypt:     true,
			ViaMQTT:     true,
		},
		NodeID:     bot.NodeID,
		LongName:   bot.LongName,
		ShortName:  bot.ShortName,
		HWModel:    255,
		Role:       0,
		IsLicensed: false,
		PublicKey:  publicKey,
	})
	if err != nil {
		return err
	}
	topic := botMQTTTopic(bot.TopicPrefix, bot.DefaultChannelID, bot.NodeID)
	if err := s.server.Publish(topic, raw, false, 0); err != nil {
		return err
	}
	return s.store.UpdateBotNodeInfoBroadcastAt(bot.ID, time.Now())
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

func botMQTTTopic(topicPrefix, channelID, nodeID string) string {
	prefix := strings.Trim(strings.TrimSpace(topicPrefix), "/")
	if prefix == "" {
		prefix = botDefaultTopicPrefix
	}
	if strings.HasSuffix(prefix, "/2/e") {
		return prefix + "/" + channelID + "/" + nodeID
	}
	return prefix + "/2/e/" + channelID + "/" + nodeID
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
