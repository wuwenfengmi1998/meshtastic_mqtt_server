package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ============================================
// LLM Provider (llm_providers) - 多 AI API 配置
// ============================================

// ListLLMProviders 列出所有 LLM Provider
func (s *Store) ListLLMProviders(includeInactive bool) ([]LLMProviderRecord, error) {
	var rows []LLMProviderRecord
	query := s.db.Model(&LLMProviderRecord{})
	if !includeInactive {
		query = query.Where("active = ?", true)
	}
	if err := query.Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list llm providers: %w", err)
	}
	return rows, nil
}

// GetLLMProvider 获取单个 LLM Provider
func (s *Store) GetLLMProvider(name string) (*LLMProviderRecord, error) {
	var record LLMProviderRecord
	if err := s.db.Where("name = ?", name).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get llm provider %s: %w", name, err)
	}
	return &record, nil
}

// CreateLLMProvider 创建 LLM Provider
func (s *Store) CreateLLMProvider(record *LLMProviderRecord) error {
	if err := s.db.Create(record).Error; err != nil {
		return fmt.Errorf("create llm provider %s: %w", record.Name, err)
	}
	return nil
}

// UpdateLLMProvider 更新 LLM Provider
func (s *Store) UpdateLLMProvider(name string, updates map[string]any) error {
	if err := s.db.Model(&LLMProviderRecord{}).Where("name = ?", name).Updates(updates).Error; err != nil {
		return fmt.Errorf("update llm provider %s: %w", name, err)
	}
	return nil
}

// DeleteLLMProvider 删除 LLM Provider
func (s *Store) DeleteLLMProvider(name string) error {
	if err := s.db.Where("name = ?", name).Delete(&LLMProviderRecord{}).Error; err != nil {
		return fmt.Errorf("delete llm provider %s: %w", name, err)
	}
	return nil
}

// EnsureDefaultLLMProvider 确保存在默认 LLM Provider 配置
// 只有当数据库中完全没有任何 provider 配置时，才创建默认配置
func (s *Store) EnsureDefaultLLMProvider() error {
	// 先检查是否已经有任何 provider 配置
	providers, err := s.ListLLMProviders(true)
	if err != nil {
		return fmt.Errorf("list llm providers: %w", err)
	}
	if len(providers) > 0 {
		return nil // 已有配置，不创建默认
	}
	// 创建默认配置
	defaultConfig := &LLMProviderRecord{
		Name:                "default",
		Active:              true,
		APIKey:              "",
		BaseURL:             "https://ark.cn-beijing.volces.com/api/v3",
		Model:               "",
		Timeout:             120,
		ContextWindowTokens: 262144,
	}
	return s.CreateLLMProvider(defaultConfig)
}

// ============================================
// LLM Tool Router (llm_tool_router) - 工具路由配置
// ============================================

// GetLLMToolRouter 获取当前激活的 Tool Router 配置
func (s *Store) GetLLMToolRouter() (*LLMToolRouterRecord, error) {
	var record LLMToolRouterRecord
	// 默认取第一条记录（ID 最小的），因为通常只需要一个配置
	if err := s.db.Order("id ASC").First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get llm tool router: %w", err)
	}
	return &record, nil
}

// CreateLLMToolRouter 创建 Tool Router 配置
func (s *Store) CreateLLMToolRouter(record *LLMToolRouterRecord) error {
	if err := s.db.Create(record).Error; err != nil {
		return fmt.Errorf("create llm tool router: %w", err)
	}
	return nil
}

// UpdateLLMToolRouter 更新 Tool Router 配置
func (s *Store) UpdateLLMToolRouter(id uint64, updates map[string]any) error {
	if err := s.db.Model(&LLMToolRouterRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update llm tool router %d: %w", id, err)
	}
	return nil
}

// EnsureDefaultLLMToolRouter 确保存在默认 Tool Router 配置
func (s *Store) EnsureDefaultLLMToolRouter() error {
	_, err := s.GetLLMToolRouter()
	if err == nil {
		return nil // 已存在
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// 创建默认配置
	defaultConfig := &LLMToolRouterRecord{
		Enabled:      true,
		OpenAIName:   "",
		Timeout:      30,
		MaxTokens:    512,
		SystemPrompt: "你可以按需直接调用可用工具来回答用户问题。\n每个工具的 description 描述了它的适用场景和调用条件。\n工具结果优先于模型内置知识；工具失败时必须如实说明，不要编造结果。\n只调用确实必要的工具。",
	}
	return s.CreateLLMToolRouter(defaultConfig)
}

// ============================================
// LLM Topic Config (llm_topic_config) - 话题选择配置
// ============================================

// GetLLMTopicConfig 获取当前激活的话题选择配置
func (s *Store) GetLLMTopicConfig() (*LLMTopicConfigRecord, error) {
	var record LLMTopicConfigRecord
	// 默认取第一条记录（ID 最小的），因为通常只需要一个配置
	if err := s.db.Order("id ASC").First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get llm topic config: %w", err)
	}
	return &record, nil
}

// CreateLLMTopicConfig 创建话题选择配置
func (s *Store) CreateLLMTopicConfig(record *LLMTopicConfigRecord) error {
	if err := s.db.Create(record).Error; err != nil {
		return fmt.Errorf("create llm topic config: %w", err)
	}
	return nil
}

// UpdateLLMTopicConfig 更新话题选择配置
func (s *Store) UpdateLLMTopicConfig(id uint64, updates map[string]any) error {
	if err := s.db.Model(&LLMTopicConfigRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update llm topic config %d: %w", id, err)
	}
	return nil
}

// EnsureDefaultLLMTopicConfig 确保存在默认话题选择配置
func (s *Store) EnsureDefaultLLMTopicConfig() error {
	_, err := s.GetLLMTopicConfig()
	if err == nil {
		return nil // 已存在
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// 创建默认配置（默认未启用）
	defaultConfig := &LLMTopicConfigRecord{
		Enabled:      false,
		OpenAIName:   "",
		Timeout:      30,
		MaxTokens:    512,
		SystemPrompt: "你是一个话题过滤器，判断用户最新消息是否属于应当回复的话题范围。\n如果应当回复，请输出 REPLY；如果不应当回复，请输出 IGNORE。\n只输出 REPLY 或 IGNORE，不要输出任何其他内容。",
	}
	return s.CreateLLMTopicConfig(defaultConfig)
}

// ============================================
// LLM Primary Config (llm_primary_config) - 主 AI 回复配置
// ============================================

// GetLLMPrimaryConfig 获取当前激活的主 AI 回复配置
func (s *Store) GetLLMPrimaryConfig() (*LLMPrimaryConfigRecord, error) {
	var record LLMPrimaryConfigRecord
	// 默认取第一条记录（ID 最小的），因为通常只需要一个配置
	if err := s.db.Order("id ASC").First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get llm primary config: %w", err)
	}
	return &record, nil
}

// GetLLMPrimaryConfigSystemPrompt 获取主 AI 回复配置中的系统提示词
// 如果没有配置或出错，返回空字符串（autoreply service 会处理这种情况）
func (s *Store) GetLLMPrimaryConfigSystemPrompt() (string, error) {
	record, err := s.GetLLMPrimaryConfig()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 没有配置时返回空字符串，使用默认行为
			return "", nil
		}
		return "", err
	}
	return record.SystemPrompt, nil
}

// GetLLMPrimaryConfigEnableTool 获取是否启用工具调用
func (s *Store) GetLLMPrimaryConfigEnableTool() (bool, error) {
	record, err := s.GetLLMPrimaryConfig()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return record.EnableTool, nil
}

// CreateLLMPrimaryConfig 创建主 AI 回复配置
func (s *Store) CreateLLMPrimaryConfig(record *LLMPrimaryConfigRecord) error {
	if err := s.db.Create(record).Error; err != nil {
		return fmt.Errorf("create llm primary config: %w", err)
	}
	return nil
}

// UpdateLLMPrimaryConfig 更新主 AI 回复配置
func (s *Store) UpdateLLMPrimaryConfig(id uint64, updates map[string]any) error {
	if err := s.db.Model(&LLMPrimaryConfigRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update llm primary config %d: %w", id, err)
	}
	return nil
}

// EnsureDefaultLLMPrimaryConfig 确保存在默认主 AI 回复配置
func (s *Store) EnsureDefaultLLMPrimaryConfig() error {
	_, err := s.GetLLMPrimaryConfig()
	if err == nil {
		return nil // 已存在
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// 创建默认配置
	defaultConfig := &LLMPrimaryConfigRecord{
		Enabled:      false,
		ProviderName: "",
		Timeout:      120,
		MaxTokens:    1024,
		SystemPrompt: "你是一个 Meshtastic 网络助手。请简洁回答用户问题。\n回答要简短清晰，适合在低带宽无线电环境传输。每次回复限制在200 bytes以内。",
		EnableTool:   false,
	}
	return s.CreateLLMPrimaryConfig(defaultConfig)
}

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
	MessageType string // "channel" 或 "direct"
	ContentJSON *string
}

// EnqueueLLMMessage 将消息添加到 LLM 队列
func (s *Store) EnqueueLLMMessage(input LLMMessageQueueInput) (*LLMMessageQueueRecord, error) {
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

	// 忽略机器人自己发送的消息，避免自循环
	if input.FromNodeID == bot.NodeID {
		return nil, nil
	}

	if input.FromNodeID == "" {
		return nil, fmt.Errorf("from_node_id is required")
	}
	if input.Text == "" {
		return nil, fmt.Errorf("text is required")
	}

	// 检查是否存在重复消息
	// packet_id > 0: 用 bot_id + packet_id 去重（频道消息）
	// packet_id = 0: 用 bot_id + from_node_id + text 去重（私聊消息，可能没有 packet_id）
	// 只排除 pending/processing 状态的消息，允许 error 状态的消息重新入队
	var existing LLMMessageQueueRecord
	if input.PacketID > 0 {
		// 频道消息：用 bot_id + packet_id 去重
		err = s.db.Where("bot_id = ? AND packet_id = ? AND deleted_at IS NULL AND status IN (?, ?)",
			input.BotID, input.PacketID, LLMMessageStatusPending, LLMMessageStatusProcessing).
			Take(&existing).Error
	} else {
		// 私聊消息：用 bot_id + from_node_id + text 去重（避免同一人连续发相同内容被拒绝）
		err = s.db.Where("bot_id = ? AND from_node_id = ? AND text = ? AND deleted_at IS NULL AND status IN (?, ?)",
			input.BotID, input.FromNodeID, input.Text, LLMMessageStatusPending, LLMMessageStatusProcessing).
			Take(&existing).Error
	}
	if err == nil {
		// 存在正在处理或待处理的相同消息，直接返回
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("check duplicate llm message: %w", err)
	}

	now := time.Now()
	messageType := input.MessageType
	if messageType == "" {
		messageType = "direct"
	}
	record := &LLMMessageQueueRecord{
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
		MessageType: messageType,
		Status:      LLMMessageStatusPending,
		ReceivedAt:  now,
		ContentJSON: input.ContentJSON,
	}

	if err := s.db.Create(record).Error; err != nil {
		return nil, fmt.Errorf("enqueue llm message: %w", err)
	}
	return record, nil
}

// ListLLMMessages 列出 LLM 队列消息
func (s *Store) ListLLMMessages(opts ListOptions, botID uint64, includeDeleted bool) ([]LLMMessageQueueRecord, int64, error) {
	var rows []LLMMessageQueueRecord
	query := s.db.Model(&LLMMessageQueueRecord{})

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
func (s *Store) GetLLMMessage(id uint64) (*LLMMessageQueueRecord, error) {
	var record LLMMessageQueueRecord
	if err := s.db.Where("id = ?", id).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get llm message %d: %w", id, err)
	}
	return &record, nil
}

// UpdateLLMMessageStatus 更新 LLM 消息状态
func (s *Store) UpdateLLMMessageStatus(id uint64, status string, errorMsg string) error {
	updates := map[string]any{
		"status": status,
		"error":  errorMsg,
	}
	if status == LLMMessageStatusProcessed {
		now := time.Now()
		updates["processed_at"] = &now
	}
	if err := s.db.Model(&LLMMessageQueueRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update llm message status %d: %w", id, err)
	}
	return nil
}

// SoftDeleteLLMMessage 软删除 LLM 消息
func (s *Store) SoftDeleteLLMMessage(id uint64) error {
	now := time.Now()
	if err := s.db.Model(&LLMMessageQueueRecord{}).Where("id = ?", id).Update("deleted_at", &now).Error; err != nil {
		return fmt.Errorf("soft delete llm message %d: %w", id, err)
	}
	return nil
}

// SoftDeleteLLMMessagesByBot 软删除指定机器人的所有消息
func (s *Store) SoftDeleteLLMMessagesByBot(botID uint64) error {
	now := time.Now()
	if err := s.db.Model(&LLMMessageQueueRecord{}).Where("bot_id = ? AND deleted_at IS NULL", botID).Update("deleted_at", &now).Error; err != nil {
		return fmt.Errorf("soft delete llm messages for bot %d: %w", botID, err)
	}
	return nil
}

// CleanupDeletedLLMMessages 清理已软删除超过指定时间的消息
func (s *Store) CleanupDeletedLLMMessages(before time.Time) (int64, error) {
	result := s.db.Where("deleted_at IS NOT NULL AND deleted_at < ?", before).Delete(&LLMMessageQueueRecord{})
	if result.Error != nil {
		return 0, fmt.Errorf("cleanup deleted llm messages: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// LLMMessageDTO 将数据库记录转换为 API 响应格式
func LLMMessageDTO(row LLMMessageQueueRecord) map[string]any {
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
func enqueueChannelMessageToLLM(s *Store, record map[string]any) error {
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

	// record 来自 describePacket 直接构造的 map，packet_id 是 uint32，
	// 并未经过 JSON 往返（不会变成 float64），必须用类型安全的转换兜底各种整型。
	packetID, _ := int64FromAny(record["packet_id"])

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
	// SQLite 中 numeric 布尔值用 1/0 存储，必须用整数查询
	var bots []BotNodeRecord
	err = s.db.Where("llm_queue_enabled = ? AND llm_include_channel_messages = ?", 1, 1).Find(&bots).Error
	if err != nil {
		return fmt.Errorf("query bots for channel message enqueue: %w", err)
	}

	// 为每个符合条件的机器人创建一条队列记录（忽略机器人自己发送的消息）
	for _, bot := range bots {
		if fromNodeID == bot.NodeID {
			continue
		}
		_, err = s.EnqueueLLMMessage(LLMMessageQueueInput{
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
			MessageType: "channel",
			ContentJSON: contentPtr,
		})
		if err != nil {
			printJSON(map[string]any{
				"event":  "llm_queue_enqueue_failed",
				"bot_id": bot.ID,
				"from":   fromNodeID,
				"text":   text,
				"error":  err.Error(),
			})
		}
	}

	return nil
}
