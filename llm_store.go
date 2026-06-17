package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// LLMMessageQueueInput 是添加 LLM 队列消息的输入
type LLMMessageQueueInput struct {
	BotID       uint64 // 0 表示频道消息
	BotNodeID   string // 频道消息可为空
	BotNodeNum  int64  // 频道消息可为 0
	FromNodeID  string
	FromNodeNum int64
	LongName    *string
	ShortName   *string
	Text        string
	PacketID    int64
	ChannelID   *string
	Topic       string
	ContentJSON *string
}

// EnqueueLLMMessage 将消息添加到 LLM 队列
func (s *store) EnqueueLLMMessage(input LLMMessageQueueInput) (*llmMessageQueueRecord, error) {
	var err error

	if input.BotID == 0 {
		return nil, nil // bot_id 为 0 的消息不再入队
	}

	// 检查机器人级别的 LLM 队列设置
	bot, err := s.GetBotNode(input.BotID)
	if err != nil {
		return nil, nil // 机器人不存在，静默返回
	}
	if !bot.LLMQueueEnabled {
		return nil, nil // 机器人的 LLM 队列未启用，静默返回
	}

	if input.FromNodeID == "" {
		return nil, fmt.Errorf("from_node_id is required")
	}
	if input.Text == "" {
		return nil, fmt.Errorf("text is required")
	}

	// 检查是否存在重复消息：相同 bot_id + packet_id 且未删除
	var existing llmMessageQueueRecord
	err = s.db.Where("bot_id = ? AND packet_id = ? AND deleted_at IS NULL", input.BotID, input.PacketID).
		Take(&existing).Error
	if err == nil {
		// 重复消息，直接返回已存在的
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("check duplicate llm message: %w", err)
	}

	now := time.Now()
	record := &llmMessageQueueRecord{
		BotID:       input.BotID,
		BotNodeID:   input.BotNodeID,
		BotNodeNum:  input.BotNodeNum,
		FromNodeID:  input.FromNodeID,
		FromNodeNum: input.FromNodeNum,
		LongName:    input.LongName,
		ShortName:   input.ShortName,
		Text:        input.Text,
		PacketID:    input.PacketID,
		ChannelID:   input.ChannelID,
		Topic:       input.Topic,
		Status:      llmMessageStatusPending,
		ReceivedAt:  now,
		ContentJSON: input.ContentJSON,
	}

	if err := s.db.Create(record).Error; err != nil {
		return nil, fmt.Errorf("enqueue llm message: %w", err)
	}
	return record, nil
}

// ListLLMMessages 列出 LLM 队列消息
func (s *store) ListLLMMessages(opts listOptions, botID uint64, includeDeleted bool) ([]llmMessageQueueRecord, int64, error) {
	var rows []llmMessageQueueRecord
	query := s.db.Model(&llmMessageQueueRecord{})

	if botID > 0 {
		query = query.Where("bot_id = ?", botID)
	}
	if !includeDeleted {
		query = query.Where("deleted_at IS NULL")
	}

	// 先获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count llm messages: %w", err)
	}

	// 排序和分页
	query = query.Order("created_at DESC")
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}

	if err := query.Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list llm messages: %w", err)
	}
	return rows, total, nil
}

// GetLLMMessage 获取单条 LLM 消息
func (s *store) GetLLMMessage(id uint64) (*llmMessageQueueRecord, error) {
	var record llmMessageQueueRecord
	if err := s.db.Where("id = ?", id).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get llm message %d: %w", id, err)
	}
	return &record, nil
}

// UpdateLLMMessageStatus 更新 LLM 消息状态
func (s *store) UpdateLLMMessageStatus(id uint64, status string, errorMsg string) error {
	updates := map[string]any{
		"status": status,
		"error":  errorMsg,
	}
	if status == llmMessageStatusProcessed {
		now := time.Now()
		updates["processed_at"] = &now
	}
	if err := s.db.Model(&llmMessageQueueRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update llm message status %d: %w", id, err)
	}
	return nil
}

// SoftDeleteLLMMessage 软删除 LLM 消息
func (s *store) SoftDeleteLLMMessage(id uint64) error {
	now := time.Now()
	if err := s.db.Model(&llmMessageQueueRecord{}).Where("id = ?", id).Update("deleted_at", &now).Error; err != nil {
		return fmt.Errorf("soft delete llm message %d: %w", id, err)
	}
	return nil
}

// SoftDeleteLLMMessagesByBot 软删除指定机器人的所有消息
func (s *store) SoftDeleteLLMMessagesByBot(botID uint64) error {
	now := time.Now()
	if err := s.db.Model(&llmMessageQueueRecord{}).Where("bot_id = ? AND deleted_at IS NULL", botID).Update("deleted_at", &now).Error; err != nil {
		return fmt.Errorf("soft delete llm messages for bot %d: %w", botID, err)
	}
	return nil
}

// CleanupDeletedLLMMessages 清理已软删除超过指定时间的消息
func (s *store) CleanupDeletedLLMMessages(before time.Time) (int64, error) {
	result := s.db.Where("deleted_at IS NOT NULL AND deleted_at < ?", before).Delete(&llmMessageQueueRecord{})
	if result.Error != nil {
		return 0, fmt.Errorf("cleanup deleted llm messages: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// llmMessageDTO 将数据库记录转换为 API 响应格式
func llmMessageDTO(row llmMessageQueueRecord) map[string]any {
	return map[string]any{
		"id":            row.ID,
		"bot_id":        row.BotID,
		"bot_node_id":   row.BotNodeID,
		"bot_node_num":  row.BotNodeNum,
		"from_node_id":  row.FromNodeID,
		"from_node_num": row.FromNodeNum,
		"long_name":     row.LongName,
		"short_name":    row.ShortName,
		"text":          row.Text,
		"packet_id":     row.PacketID,
		"channel_id":    row.ChannelID,
		"topic":         row.Topic,
		"status":        row.Status,
		"error":         row.Error,
		"received_at":   row.ReceivedAt,
		"processed_at":  row.ProcessedAt,
		"deleted_at":    row.DeletedAt,
		"created_at":    row.CreatedAt,
	}
}

// enqueueChannelMessageToLLM 将频道消息添加到 LLM 队列
// 为每个启用了「包含频道消息」的机器人都创建一条独立的队列记录
func enqueueChannelMessageToLLM(s *store, record map[string]any) error {
	if s == nil {
		return nil
	}

	text, _ := record["text"].(string)
	if text == "" {
		return nil
	}

	fromNodeID, _ := record["from"].(string)
	if fromNodeID == "" {
		return nil
	}

	fromNodeNum, err := int64FromAny(record["from_num"])
	if err != nil {
		fromNodeNum = 0
	}

	var packetID int64
	if p, ok := record["packet_id"].(float64); ok {
		packetID = int64(p)
	}

	topic, _ := record["topic"].(string)

	var longName, shortName *string
	if ln, ok := record["long_name"].(string); ok && ln != "" {
		longName = &ln
	}
	if sn, ok := record["short_name"].(string); ok && sn != "" {
		shortName = &sn
	}

	var channelID *string
	if cid, ok := record["channel_id"].(string); ok && cid != "" {
		channelID = &cid
	}

	contentJSON, err := json.Marshal(record)
	var contentPtr *string
	if err == nil {
		s := string(contentJSON)
		contentPtr = &s
	}

	// 查询所有启用了 LLM 队列且包含频道消息的机器人
	var bots []botNodeRecord
	err = s.db.Where("llm_queue_enabled = ? AND llm_include_channel_messages = ?", true, true).Find(&bots).Error
	if err != nil {
		return fmt.Errorf("query bots for channel message enqueue: %w", err)
	}

	// 为每个符合条件的机器人创建一条队列记录
	for _, bot := range bots {
		_, _ = s.EnqueueLLMMessage(LLMMessageQueueInput{
			BotID:       bot.ID,
			BotNodeID:   bot.NodeID,
			BotNodeNum:  bot.NodeNum,
			FromNodeID:  fromNodeID,
			FromNodeNum: fromNodeNum,
			LongName:    longName,
			ShortName:   shortName,
			Text:        text,
			PacketID:    packetID,
			ChannelID:   channelID,
			Topic:       topic,
			ContentJSON: contentPtr,
		})
	}

	return nil
}
