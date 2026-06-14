package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"meshtastic_mqtt_server/mqtpp"

	mqtt "github.com/mochi-mqtt/server/v2"
	"gorm.io/gorm"
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

// MaybeAutoAck 在收到一个发往本地 bot 的 want_ack 包时，回送一个 Routing-NONE ACK。
//
// 与固件 ReliableRouter::sniffReceived 行为对齐：
//   - PKI 加密包（pki_encrypted=true）用 X25519+AES-CCM 加密 ACK，channel_id="PKI"
//   - 其它情况用原 channel + bot PSK 加密
//
// 解析失败、目标不是受管 bot、或缺少必要的密钥时，安静返回不报错——这条路径只是“尽力”。
func (s *botService) MaybeAutoAck(record map[string]any) {
	if s == nil || s.store == nil || s.server == nil || record == nil {
		return
	}
	wantAck, _ := record["want_ack"].(bool)
	if !wantAck {
		return
	}
	toNum, ok := uint32FromRecord(record["packet_to_num"])
	if !ok || toNum == 0 || toNum == mqtpp.NodeNumBroadcast {
		return
	}
	fromNum, ok := uint32FromRecord(record["packet_from_num"])
	if !ok || fromNum == 0 {
		return
	}
	requestID, ok := uint32FromRecord(record["packet_id"])
	if !ok || requestID == 0 {
		return
	}
	bot, err := s.store.GetBotNodeByNodeNum(int64(toNum))
	if err != nil || bot == nil || !bot.Enabled {
		return
	}
	pkiEncrypted, _ := record["pki_encrypted"].(bool)
	channelID, _ := record["channel_id"].(string)

	ackPacketID, err := randomPacketID()
	if err != nil {
		return
	}
	topic := botMQTTTopic(bot.TopicPrefix, fallbackChannelID(channelID, pkiEncrypted, bot.DefaultChannelID), bot.NodeID)

	var raw []byte
	if pkiEncrypted {
		raw, err = s.buildPKIAck(bot, fromNum, ackPacketID, requestID)
		if err == nil {
			topic = botMQTTTopic(bot.TopicPrefix, mqtpp.PKIChannelID, bot.NodeID)
		}
	} else {
		raw, err = s.buildPSKAck(bot, fromNum, ackPacketID, requestID, channelID)
	}
	if err != nil || raw == nil {
		printJSON(map[string]any{"event": "bot_auto_ack_skipped", "bot_node_id": bot.NodeID, "to": fromNum, "request_id": requestID, "error": errString(err)})
		return
	}
	if err := s.server.Publish(topic, raw, false, 0); err != nil {
		printJSON(map[string]any{"event": "bot_auto_ack_publish_failed", "bot_node_id": bot.NodeID, "topic": topic, "error": err.Error()})
	}
}

func (s *botService) buildPKIAck(bot *botNodeRecord, toNum, ackPacketID, requestID uint32) ([]byte, error) {
	privateKeyB64 := strings.TrimSpace(bot.PrivateKey)
	if privateKeyB64 == "" {
		return nil, fmt.Errorf("bot has no private key")
	}
	privateKey, err := base64.StdEncoding.DecodeString(privateKeyB64)
	if err != nil {
		return nil, err
	}
	senderPublic, err := decodeBotPublicKey(*bot)
	if err != nil {
		return nil, err
	}
	recipientPublic, ok := lookupNodeInfoPublicKey(s.store, toNum)
	if !ok {
		return nil, fmt.Errorf("recipient %s has no public key on file", mqtpp.NodeNumToID(toNum))
	}
	return mqtpp.BuildPKIAckServiceEnvelope(mqtpp.PKIAckBuildOptions{
		FromNodeNum:   uint32(bot.NodeNum),
		ToNodeNum:     toNum,
		PacketID:      ackPacketID,
		RequestID:     requestID,
		GatewayID:     bot.NodeID,
		ViaMQTT:       true,
		SenderPrivate: privateKey,
		RecipientPub:  recipientPublic,
		SenderPublic:  senderPublic,
	})
}

func (s *botService) buildPSKAck(bot *botNodeRecord, toNum, ackPacketID, requestID uint32, channelID string) ([]byte, error) {
	channel := fallbackChannelID(channelID, false, bot.DefaultChannelID)
	if channel == "" || channel == mqtpp.PKIChannelID {
		return nil, fmt.Errorf("no channel id available for psk ack")
	}
	psk := strings.TrimSpace(bot.PSK)
	if psk == "" {
		psk = botDefaultPSK
	}
	key, err := mqtpp.ExpandPSK(psk)
	if err != nil {
		return nil, err
	}
	return mqtpp.BuildAckServiceEnvelope(mqtpp.AckBuildOptions{
		PacketBuildOptions: mqtpp.PacketBuildOptions{
			FromNodeNum: uint32(bot.NodeNum),
			ToNodeNum:   toNum,
			PacketID:    ackPacketID,
			ChannelID:   channel,
			GatewayID:   bot.NodeID,
			PSK:         key,
			Encrypt:     true,
			ViaMQTT:     true,
		},
		RequestID: requestID,
	})
}

func fallbackChannelID(channelID string, pkiEncrypted bool, defaultChannelID string) string {
	channelID = strings.TrimSpace(channelID)
	if pkiEncrypted {
		return mqtpp.PKIChannelID
	}
	if channelID != "" && channelID != mqtpp.PKIChannelID {
		return channelID
	}
	return defaultChannelID
}

func uint32FromRecord(value any) (uint32, bool) {
	switch v := value.(type) {
	case uint32:
		return v, true
	case int:
		if v >= 0 {
			return uint32(v), true
		}
	case int64:
		if v >= 0 {
			return uint32(v), true
		}
	case uint64:
		return uint32(v), true
	case float64:
		if v >= 0 {
			return uint32(v), true
		}
	}
	return 0, false
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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
	toNodeNum, toNodeID, err := botMessageTarget(messageType, req)
	if err != nil {
		return nil, err
	}
	packetID, err := randomPacketID()
	if err != nil {
		return nil, err
	}
	fromNodeNum := uint32(bot.NodeNum)

	// direct 私聊走 PKI；channel 群聊保留旧的 AES-CTR + PSK 路径
	if messageType == botMessageTypeDirect {
		return s.sendPKIDirect(bot, fromNodeNum, uint32(toNodeNum), toNodeID, packetID, text, req.CreatedBy)
	}

	channelID := strings.TrimSpace(req.ChannelID)
	if channelID == "" {
		channelID = bot.DefaultChannelID
	}
	if channelID == "" {
		return nil, fmt.Errorf("channel id is required")
	}
	psk := strings.TrimSpace(bot.PSK)
	if psk == "" {
		psk = botDefaultPSK
	}
	key, err := mqtpp.ExpandPSK(psk)
	if err != nil {
		return nil, err
	}
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
		ToNodeNum:   int64PtrOrNil(toNodeNum, false),
		Topic:       topic,
		PacketID:    int64(packetID),
		Text:        text,
		PayloadLen:  int64(len(raw)),
		Encrypted:   true,
		Status:      botMessageStatusPending,
		CreatedBy:   strings.TrimSpace(req.CreatedBy),
	}
	return s.persistAndPublish(row, topic, raw)
}

// sendPKIDirect 按固件 PKI 流程发送私聊：
//   - 从 nodeinfo 中查目标节点的 X25519 公钥
//   - 用 bot 自身私钥与对端公钥派生共享密钥，AES-CCM(M=8,L=2) 加密
//   - ServiceEnvelope.channel_id = "PKI"，topic 也用 "PKI"
func (s *botService) sendPKIDirect(bot *botNodeRecord, fromNodeNum, toNodeNum uint32, toNodeID *string, packetID uint32, text, createdBy string) (*botMessageRecord, error) {
	if toNodeID == nil {
		return nil, fmt.Errorf("target node id is required for pki direct message")
	}
	privateKeyB64 := strings.TrimSpace(bot.PrivateKey)
	if privateKeyB64 == "" {
		return nil, fmt.Errorf("bot has no private key, regenerate keys first")
	}
	privateKey, err := base64.StdEncoding.DecodeString(privateKeyB64)
	if err != nil {
		return nil, fmt.Errorf("invalid bot private key: %w", err)
	}
	senderPublic, err := decodeBotPublicKey(*bot)
	if err != nil {
		return nil, err
	}
	recipientPublic, err := s.lookupRecipientPublicKey(*toNodeID)
	if err != nil {
		return nil, err
	}

	raw, err := mqtpp.BuildPKITextMessageServiceEnvelope(mqtpp.PKITextMessageBuildOptions{
		FromNodeNum:   fromNodeNum,
		ToNodeNum:     toNodeNum,
		PacketID:      packetID,
		GatewayID:     bot.NodeID,
		ViaMQTT:       true,
		SenderPrivate: privateKey,
		RecipientPub:  recipientPublic,
		SenderPublic:  senderPublic,
		Text:          text,
	})
	if err != nil {
		return nil, err
	}
	topic := botMQTTTopic(bot.TopicPrefix, mqtpp.PKIChannelID, bot.NodeID)
	row := &botMessageRecord{
		BotID:       bot.ID,
		BotNodeID:   bot.NodeID,
		BotNodeNum:  bot.NodeNum,
		MessageType: botMessageTypeDirect,
		ChannelID:   mqtpp.PKIChannelID,
		ToNodeID:    toNodeID,
		ToNodeNum:   int64PtrOrNil(int64(toNodeNum), true),
		Topic:       topic,
		PacketID:    int64(packetID),
		Text:        text,
		PayloadLen:  int64(len(raw)),
		Encrypted:   true,
		Status:      botMessageStatusPending,
		CreatedBy:   strings.TrimSpace(createdBy),
	}
	result, err := s.persistAndPublish(row, topic, raw)
	// 不论发送结果如何，都把 DM 镜像写入 bot_direct_messages 以驱动 /admin/bot/direct 渲染。
	// 这里把发送结果（status/error/published_at）同步过去——成功时 status=published，
	// 失败时 status=failed，前端就能看到本地视图与发送日志一致。
	s.recordOutboundDirectMessage(bot, row, *toNodeID, toNodeNum, text, len(raw), err)
	return result, err
}

// recordOutboundDirectMessage 把出向 PKI DM 写入 bot_direct_messages。失败仅打日志。
func (s *botService) recordOutboundDirectMessage(bot *botNodeRecord, msg *botMessageRecord, peerNodeID string, peerNodeNum uint32, text string, payloadLen int, sendErr error) {
	if s == nil || s.store == nil || msg == nil || bot == nil {
		return
	}
	status := msg.Status
	if status == "" {
		if sendErr != nil {
			status = botMessageStatusFailed
		} else {
			status = botMessageStatusPublished
		}
	}
	errText := msg.Error
	if errText == "" && sendErr != nil {
		errText = sendErr.Error()
	}
	createdBy := strings.TrimSpace(msg.CreatedBy)
	var createdByPtr *string
	if createdBy != "" {
		createdByPtr = &createdBy
	}
	gateway := strings.TrimSpace(bot.NodeID)
	var gatewayPtr *string
	if gateway != "" {
		gatewayPtr = &gateway
	}
	var botMessageID *uint64
	if msg.ID != 0 {
		id := msg.ID
		botMessageID = &id
	}
	dm := &botDirectMessageRecord{
		BotID:        bot.ID,
		BotNodeID:    bot.NodeID,
		BotNodeNum:   bot.NodeNum,
		PeerNodeID:   peerNodeID,
		PeerNodeNum:  int64(peerNodeNum),
		Direction:    botDirectMessageDirectionOutbound,
		Topic:        msg.Topic,
		PacketID:     msg.PacketID,
		Text:         text,
		PayloadLen:   int64(payloadLen),
		PKIEncrypted: true,
		WantAck:      false, // 我们当前发送的 DM 默认不显式请求 ack
		GatewayID:    gatewayPtr,
		Status:       status,
		Error:        strings.TrimSpace(errText),
		BotMessageID: botMessageID,
		CreatedBy:    createdByPtr,
		PublishedAt:  msg.PublishedAt,
	}
	if err := s.store.InsertBotDirectMessage(dm); err != nil {
		printJSON(map[string]any{
			"event":           "bot_direct_message_outbound_persist_failed",
			"bot_node_id":     bot.NodeID,
			"peer_node_id":    peerNodeID,
			"bot_message_id":  msg.ID,
			"error":           err.Error(),
		})
	}
}

// lookupRecipientPublicKey 从 nodeinfo 表中按 node_id 查询目标节点的 X25519 公钥（hex 编码）。
func (s *botService) lookupRecipientPublicKey(nodeID string) ([]byte, error) {
	node, err := s.store.GetNodeInfo(nodeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("recipient node %s not found in nodeinfo, cannot send PKI message", nodeID)
		}
		return nil, err
	}
	if node.PublicKey == nil || strings.TrimSpace(*node.PublicKey) == "" {
		return nil, fmt.Errorf("recipient node %s has no public key on file", nodeID)
	}
	keyHex := strings.TrimSpace(*node.PublicKey)
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		// 兼容历史上可能存储为 base64 的情况
		if alt, altErr := base64.StdEncoding.DecodeString(keyHex); altErr == nil {
			keyBytes = alt
		} else {
			return nil, fmt.Errorf("invalid recipient public key for %s: %w", nodeID, err)
		}
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("recipient public key for %s has unexpected length %d", nodeID, len(keyBytes))
	}
	return keyBytes, nil
}

// persistAndPublish 把消息记录入库后发布到 MQTT，统一处理失败状态写回。
func (s *botService) persistAndPublish(row *botMessageRecord, topic string, raw []byte) (*botMessageRecord, error) {
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
	if s.store != nil {
		valid, _, record := mqtpp.MQTTPP(topic, raw, key, mqtpp.Options{AllowEncryptedForwarding: true})
		if valid && record["type"] == "nodeinfo" {
			if err := s.store.UpsertNodeInfo(record); err != nil {
				return err
			}
		}
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
