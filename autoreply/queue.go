package autoreply

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

const (
	statusPending    = "pending"
	statusProcessing = "processing"
	statusProcessed  = "processed"
	statusFailed     = "error"
)

// DBMessageQueue implements MessageQueue using GORM
type DBMessageQueue struct {
	db *gorm.DB
}

// NewDBMessageQueue creates a new database-backed message queue
func NewDBMessageQueue(db *gorm.DB) *DBMessageQueue {
	return &DBMessageQueue{db: db}
}

// llmMessageQueueRecord is the database record for LLM messages
type llmMessageQueueRecord struct {
	ID          uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	BotID       uint64     `gorm:"column:bot_id;not null;index"`
	BotNodeID   string     `gorm:"column:bot_node_id;not null"`
	BotNodeNum  int64      `gorm:"column:bot_node_num;not null"`
	FromNodeID  string     `gorm:"column:from_node_id;not null"`
	FromNodeNum int64      `gorm:"column:from_node_num;not null"`
	LongName    *string    `gorm:"column:long_name"`
	ShortName   *string    `gorm:"column:short_name"`
	Text        string     `gorm:"column:text;type:text;not null"`
	PacketID    int64      `gorm:"column:packet_id;not null"`
	ChannelID   *string    `gorm:"column:channel_id"`
	Topic       string     `gorm:"column:topic;not null"`
	MessageType string     `gorm:"column:message_type;not null;default:'direct'"` // "channel" 或 "direct"
	Status      string     `gorm:"column:status;not null;index"`
	Error       string     `gorm:"column:error;type:text"`
	Reply       string     `gorm:"column:reply;type:text"`
	ReceivedAt  time.Time  `gorm:"column:received_at;not null"`
	ProcessedAt *time.Time `gorm:"column:processed_at;index"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime;index"`
}

func (llmMessageQueueRecord) TableName() string {
	return "llm_message_queue"
}

// GetPendingMessages returns pending messages from the queue
func (q *DBMessageQueue) GetPendingMessages(botID uint64, limit int) ([]QueuedMessage, error) {
	var records []llmMessageQueueRecord
	query := q.db.Where("status = ?", statusPending).Order("created_at ASC")
	if botID > 0 {
		query = query.Where("bot_id = ?", botID)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to query pending messages: %w", err)
	}

	messages := make([]QueuedMessage, 0, len(records))
	for _, r := range records {
		messages = append(messages, QueuedMessage{
			ID:          r.ID,
			BotID:       r.BotID,
			BotNodeID:   r.BotNodeID,
			BotNodeNum:  r.BotNodeNum,
			FromNodeID:  r.FromNodeID,
			FromNodeNum: r.FromNodeNum,
			LongName:    r.LongName,
			ShortName:   r.ShortName,
			Text:        r.Text,
			PacketID:    r.PacketID,
			ChannelID:   r.ChannelID,
			Topic:       r.Topic,
			MessageType: r.MessageType,
			ReceivedAt:  r.ReceivedAt,
		})
	}
	return messages, nil
}

// MarkAsProcessing marks a message as being processed
func (q *DBMessageQueue) MarkAsProcessing(id uint64) error {
	return q.db.Model(&llmMessageQueueRecord{}).Where("id = ?", id).Update("status", statusProcessing).Error
}

// MarkAsProcessed marks a message as successfully processed
func (q *DBMessageQueue) MarkAsProcessed(id uint64, reply string) error {
	now := time.Now()
	return q.db.Model(&llmMessageQueueRecord{}).Where("id = ?", id).Updates(map[string]any{
		"status":      statusProcessed,
		"reply":       reply,
		"processed_at": &now,
	}).Error
}

// MarkAsFailed marks a message as failed
func (q *DBMessageQueue) MarkAsFailed(id uint64, error string) error {
	return q.db.Model(&llmMessageQueueRecord{}).Where("id = ?", id).Updates(map[string]any{
		"status": statusFailed,
		"error":  error,
	}).Error
}
