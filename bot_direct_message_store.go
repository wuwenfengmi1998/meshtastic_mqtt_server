package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type botDirectMessageListOptions struct {
	listOptions
	BotID       uint64
	PeerNodeNum int64
	Direction   string
}

// InsertBotDirectMessage 把一条机器人 DM（出向或入向）写入 bot_direct_messages 表。
func (s *store) InsertBotDirectMessage(row *botDirectMessageRecord) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not configured")
	}
	if row == nil {
		return fmt.Errorf("bot direct message is required")
	}
	if row.Direction == "" {
		return fmt.Errorf("bot direct message direction is required")
	}
	return s.db.Create(row).Error
}

// UpdateBotDirectMessageStatus 更新一条出向 DM 的发送状态（pending → published/failed）。
func (s *store) UpdateBotDirectMessageStatus(id uint64, status, errText string, publishedAt *time.Time) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not configured")
	}
	if id == 0 {
		return fmt.Errorf("bot direct message id is required")
	}
	updates := map[string]any{
		"status":       status,
		"error":        strings.TrimSpace(errText),
		"published_at": publishedAt,
	}
	result := s.db.Model(&botDirectMessageRecord{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListBotDirectMessagesByConversation 按 (bot, peer) 反序拉取 DM 历史，给 /admin/bot/direct 页面。
func (s *store) ListBotDirectMessagesByConversation(opts botDirectMessageListOptions) ([]botDirectMessageRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store is not configured")
	}
	if opts.BotID == 0 {
		return nil, fmt.Errorf("bot id is required")
	}
	if opts.PeerNodeNum == 0 {
		return nil, fmt.Errorf("peer node num is required")
	}
	opts.listOptions = normalizeListOptions(opts.listOptions)
	var rows []botDirectMessageRecord
	q := s.db.Model(&botDirectMessageRecord{}).
		Where("bot_id = ? AND peer_node_num = ?", opts.BotID, opts.PeerNodeNum).
		Order("created_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	if opts.Direction != "" {
		q = q.Where("direction = ?", opts.Direction)
	}
	if opts.Since != nil {
		q = q.Where("created_at >= ?", *opts.Since)
	}
	if opts.Until != nil {
		q = q.Where("created_at <= ?", *opts.Until)
	}
	return rows, q.Find(&rows).Error
}

// CountBotDirectMessagesByConversation 返回会话总条数（前端无限滚动可用，可选）。
func (s *store) CountBotDirectMessagesByConversation(opts botDirectMessageListOptions) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store is not configured")
	}
	if opts.BotID == 0 || opts.PeerNodeNum == 0 {
		return 0, fmt.Errorf("bot id and peer node num are required")
	}
	var total int64
	q := s.db.Model(&botDirectMessageRecord{}).
		Where("bot_id = ? AND peer_node_num = ?", opts.BotID, opts.PeerNodeNum)
	if opts.Direction != "" {
		q = q.Where("direction = ?", opts.Direction)
	}
	if opts.Since != nil {
		q = q.Where("created_at >= ?", *opts.Since)
	}
	if opts.Until != nil {
		q = q.Where("created_at <= ?", *opts.Until)
	}
	return total, q.Count(&total).Error
}

// FindBotForIncomingPKIPacket 在 bot_direct_messages 写入路径上判断接收方是否为受管 bot。
// 返回的 bot 用于填充 BotID/BotNodeID/BotNodeNum；不命中时返回 ErrRecordNotFound。
func (s *store) FindBotForIncomingPKIPacket(toNodeNum int64) (*botNodeRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store is not configured")
	}
	bot, err := s.GetBotNodeByNodeNum(toNodeNum)
	if err != nil {
		return nil, err
	}
	if !bot.Enabled {
		return nil, errors.New("bot disabled")
	}
	return bot, nil
}

// botDirectConversation 是 /admin/bot/direct 侧边栏需要的会话摘要。
// LastMessageAt / LastText / LastDirection 描述会话最后一条消息，便于按时间排序与预览；
// UnreadCount 仅对 inbound 计数（即未读消息数）。
type botDirectConversation struct {
	BotID         uint64    `gorm:"column:bot_id"`
	PeerNodeID    string    `gorm:"column:peer_node_id"`
	PeerNodeNum   int64     `gorm:"column:peer_node_num"`
	LastMessageAt time.Time `gorm:"column:last_message_at"`
	LastText      string    `gorm:"column:last_text"`
	LastDirection string    `gorm:"column:last_direction"`
	UnreadCount   int64     `gorm:"column:unread_count"`
	TotalCount    int64     `gorm:"column:total_count"`
}

// ListBotDirectConversations 聚合给定 bot 下的所有 (peer) 会话，返回最后一条消息及未读数。
// 按最后一条消息时间倒序（最新会话排前面）。limit/offset 走 listOptions。
func (s *store) ListBotDirectConversations(botID uint64, opts listOptions) ([]botDirectConversation, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store is not configured")
	}
	if botID == 0 {
		return nil, fmt.Errorf("bot id is required")
	}
	opts = normalizeListOptions(opts)
	var rows []botDirectConversation
	// 先把每对会话的最后一条消息 ID 取出来，再把这条消息的元数据 join 回去；
	// 同时聚合 unread_count（inbound 且 read_at IS NULL）和 total_count。
	// 这样的两步 join 避免在 GROUP BY 后引用非聚合列（MySQL 严格模式 / SQLite 兼容）。
	subLast := s.db.Model(&botDirectMessageRecord{}).
		Select("bot_id, peer_node_id, peer_node_num, MAX(id) AS last_id, COUNT(*) AS total_count, SUM(CASE WHEN direction = ? AND read_at IS NULL THEN 1 ELSE 0 END) AS unread_count", botDirectMessageDirectionInbound).
		Where("bot_id = ?", botID).
		Group("bot_id, peer_node_id, peer_node_num")
	q := s.db.Table("(?) AS agg", subLast).
		Select("agg.bot_id AS bot_id, agg.peer_node_id AS peer_node_id, agg.peer_node_num AS peer_node_num, m.created_at AS last_message_at, m.text AS last_text, m.direction AS last_direction, agg.unread_count AS unread_count, agg.total_count AS total_count").
		Joins("JOIN bot_direct_messages m ON m.id = agg.last_id").
		Order("m.created_at DESC").
		Order("m.id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Scan(&rows).Error
}

// MarkBotDirectMessagesRead 把 (bot, peer) 下未读的 inbound 消息全部标记为已读，返回更新行数。
func (s *store) MarkBotDirectMessagesRead(botID uint64, peerNodeNum int64) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store is not configured")
	}
	if botID == 0 || peerNodeNum == 0 {
		return 0, fmt.Errorf("bot id and peer node num are required")
	}
	now := time.Now()
	result := s.db.Model(&botDirectMessageRecord{}).
		Where("bot_id = ? AND peer_node_num = ? AND direction = ? AND read_at IS NULL", botID, peerNodeNum, botDirectMessageDirectionInbound).
		Update("read_at", &now)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// CountBotDirectUnread 返回某个 bot 全部未读 inbound 消息总数（用于头部小红点）。
func (s *store) CountBotDirectUnread(botID uint64) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store is not configured")
	}
	if botID == 0 {
		return 0, fmt.Errorf("bot id is required")
	}
	var total int64
	err := s.db.Model(&botDirectMessageRecord{}).
		Where("bot_id = ? AND direction = ? AND read_at IS NULL", botID, botDirectMessageDirectionInbound).
		Count(&total).Error
	return total, err
}

// isInboundBotDirectMessage 判断 record 是否是“PKI 加密、发往受管 bot”的入向 DM。
// 仅在 type=text_message、pki_encrypted=true、packet_to_num 命中受管 bot 时返回 true。
// 任何步骤失败都返回 false，让记录回落到 text_message 表（与之前行为兼容）。
func isInboundBotDirectMessage(s *store, record map[string]any) bool {
	if s == nil || record == nil {
		return false
	}
	if pki, _ := record["pki_encrypted"].(bool); !pki {
		return false
	}
	toNum, ok := uint32FromRecord(record["packet_to_num"])
	if !ok || toNum == 0 {
		return false
	}
	bot, err := s.FindBotForIncomingPKIPacket(int64(toNum))
	if err != nil || bot == nil {
		return false
	}
	return true
}

// insertInboundBotDirectMessage 把一条入向 PKI DM 转写入 bot_direct_messages 表。
// 失败时返回错误，由 dbWriteQueue 统一打印 db_error 事件。
func insertInboundBotDirectMessage(s *store, record map[string]any, clientInfo mqttClientInfo) error {
	if s == nil {
		return fmt.Errorf("store is not configured")
	}
	if record == nil {
		return fmt.Errorf("record is required")
	}
	toNum, ok := uint32FromRecord(record["packet_to_num"])
	if !ok || toNum == 0 {
		return fmt.Errorf("missing packet_to_num")
	}
	bot, err := s.FindBotForIncomingPKIPacket(int64(toNum))
	if err != nil {
		return fmt.Errorf("lookup bot for inbound DM: %w", err)
	}
	peerNum, ok := uint32FromRecord(record["from_num"])
	if !ok || peerNum == 0 {
		return fmt.Errorf("missing from_num")
	}
	peerNodeID, _ := record["from"].(string)
	if peerNodeID == "" {
		return fmt.Errorf("missing from")
	}
	packetID, _ := uint32FromRecord(record["packet_id"])
	topic, _ := record["topic"].(string)
	gateway, _ := record["gateway_id"].(string)
	var gatewayPtr *string
	if gw := strings.TrimSpace(gateway); gw != "" {
		gatewayPtr = &gw
	}
	text, _ := record["text"].(string)
	wantAck, _ := record["want_ack"].(bool)
	payloadLen, _ := record["payload_len"].(int)
	if payloadLen == 0 {
		if v, ok := record["payload_len"].(int64); ok {
			payloadLen = int(v)
		}
	}
	contentJSON, encodeErr := json.Marshal(record)
	var contentPtr *string
	if encodeErr == nil {
		s := string(contentJSON)
		contentPtr = &s
	}
	now := time.Now()
	dm := &botDirectMessageRecord{
		BotID:        bot.ID,
		BotNodeID:    bot.NodeID,
		BotNodeNum:   bot.NodeNum,
		PeerNodeID:   peerNodeID,
		PeerNodeNum:  int64(peerNum),
		Direction:    botDirectMessageDirectionInbound,
		Topic:        topic,
		PacketID:     int64(packetID),
		Text:         text,
		PayloadLen:   int64(payloadLen),
		PKIEncrypted: true,
		WantAck:      wantAck,
		GatewayID:    gatewayPtr,
		Status:       botMessageStatusPublished,
		ReceivedAt:   &now,
		ContentJSON:  contentPtr,
	}
	if err := s.InsertBotDirectMessage(dm); err != nil {
		return fmt.Errorf("insert bot direct message from %s: %w", peerNodeID, err)
	}
	_ = clientInfo // mqtt 元数据已经记录在 content_json 里，这里保留参数以保持队列签名一致

	// 同时将消息添加到 LLM 队列
	longName := nullableString(record["long_name"])
	shortName := nullableString(record["short_name"])
	channelID := nullableString(record["channel_id"])
	_, _ = s.EnqueueLLMMessage(LLMMessageQueueInput{
		BotID:       bot.ID,
		BotNodeID:   bot.NodeID,
		BotNodeNum:  bot.NodeNum,
		FromNodeID:  peerNodeID,
		FromNodeNum: int64(peerNum),
		LongName:    longName,
		ShortName:   shortName,
		Text:        text,
		PacketID:    int64(packetID),
		ChannelID:   channelID,
		Topic:       topic,
		ContentJSON: contentPtr,
	})

	return nil
}
