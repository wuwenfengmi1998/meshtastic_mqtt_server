package autoreply

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"meshtastic_mqtt_server/completion"
	"meshtastic_mqtt_server/conversation"
	"meshtastic_mqtt_server/llm"
	"meshtastic_mqtt_server/message"
	"meshtastic_mqtt_server/toolmanager"
	"meshtastic_mqtt_server/toolrouter"
)

const (
	// MaxReplyLength is the maximum length for Meshtastic messages
	MaxReplyLength = 200
	// PollInterval is how often to check the queue for new messages
	PollInterval = 5 * time.Second
	// MaxProcessingTime is the maximum time to spend processing a single message
	MaxProcessingTime = 120 * time.Second
)

// MessageQueue is the interface for accessing the LLM message queue
type MessageQueue interface {
	// GetPendingMessages returns pending messages for a bot
	GetPendingMessages(botID uint64, limit int) ([]QueuedMessage, error)
	// MarkAsProcessing marks a message as being processed
	MarkAsProcessing(id uint64) error
	// MarkAsProcessed marks a message as successfully processed
	MarkAsProcessed(id uint64, reply string) error
	// MarkAsFailed marks a message as failed
	MarkAsFailed(id uint64, error string) error
}

// QueuedMessage represents a message in the LLM queue
type QueuedMessage struct {
	ID          uint64
	BotID       uint64
	BotNodeID   string
	BotNodeNum  int64
	FromNodeID  string
	FromNodeNum int64
	LongName    *string
	ShortName   *string
	Text        string
	PacketID    int64
	ChannelID   *string
	Topic       string
	MessageType string // "channel" 或 "direct"
	ReceivedAt  time.Time
}

// BotSender is the interface for sending bot messages
type BotSender interface {
	// SendDirectText 发送私聊消息给指定节点
	SendDirectText(ctx context.Context, botID uint64, toNodeNum int64, text string) error
	// SendChannelText 发送频道消息到指定频道
	SendChannelText(ctx context.Context, botID uint64, channelID string, text string) error
}

// SystemPromptStore is the interface for getting the system prompt
type SystemPromptStore interface {
	GetLLMPrimaryConfigSystemPrompt() (string, error)
}

// Service manages automatic AI replies for bots
type Service struct {
	llmState          *llm.State
	toolRouter        *toolrouter.State
	toolMgr           *toolmanager.Manager
	convStore         *conversation.Store
	msgQueue          MessageQueue
	botSender         BotSender
	systemPromptStore SystemPromptStore

	running bool
	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewService creates a new auto-reply service
func NewService(
	llmState *llm.State,
	toolRouter *toolrouter.State,
	toolMgr *toolmanager.Manager,
	convStore *conversation.Store,
	msgQueue MessageQueue,
	botSender BotSender,
	systemPromptStore SystemPromptStore,
) *Service {
	return &Service{
		llmState:          llmState,
		toolRouter:        toolRouter,
		toolMgr:           toolMgr,
		convStore:         convStore,
		msgQueue:          msgQueue,
		botSender:         botSender,
		systemPromptStore: systemPromptStore,
	}
}

// Start starts the auto-reply service
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("auto-reply service is already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.running = true

	s.wg.Add(1)
	go s.run(ctx)

	return nil
}

// Stop stops the auto-reply service
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.running = false
}

// run is the main processing loop
func (s *Service) run(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processQueue(ctx)
		}
	}
}

// processQueue processes pending messages in the queue
func (s *Service) processQueue(ctx context.Context) {
	// Get all bots (we'd typically get this from bot store, but for now
	// we'll rely on the queue to provide messages per bot)
	// For now, process up to 10 messages at a time
	messages, err := s.msgQueue.GetPendingMessages(0, 10)
	if err != nil {
		return
	}

	for _, msg := range messages {
		select {
		case <-ctx.Done():
			return
		default:
			s.processMessage(ctx, msg)
		}
	}
}

// printJSON outputs a structured log message (imported from main package pattern)
func printJSON(v any) {
	fmt.Printf("%+v\n", v)
}

// processMessage processes a single queued message
func (s *Service) processMessage(ctx context.Context, msg QueuedMessage) {
	// Mark message as processing
	if err := s.msgQueue.MarkAsProcessing(msg.ID); err != nil {
		printJSON(map[string]any{
			"event": "llm_process_failed",
			"msg_id": msg.ID,
			"step":  "mark_as_processing",
			"error": err.Error(),
		})
		return
	}

	printJSON(map[string]any{
		"event":        "llm_process_start",
		"msg_id":       msg.ID,
		"bot_id":       msg.BotID,
		"from_node_id": msg.FromNodeID,
		"text":         msg.Text,
	})

	// Create processing context with timeout
	procCtx, cancel := context.WithTimeout(ctx, MaxProcessingTime)
	defer cancel()

	// Get or create conversation for this bot
	conv, err := s.convStore.GetOrCreateForBot(msg.BotID, msg.BotNodeID, msg.FromNodeID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get conversation: %v", err)
		printJSON(map[string]any{"event": "llm_process_failed", "msg_id": msg.ID, "step": "get_conversation", "error": errMsg})
		_ = s.msgQueue.MarkAsFailed(msg.ID, errMsg)
		return
	}

	// Add the user message to the conversation
	userMsg := message.ChatMessage{
		Role:    "user",
		Content: s.formatUserMessage(msg),
	}
	if err := s.convStore.AddMessage(conv.ID, userMsg); err != nil {
		errMsg := fmt.Sprintf("failed to add message: %v", err)
		printJSON(map[string]any{"event": "llm_process_failed", "msg_id": msg.ID, "step": "add_message", "error": errMsg})
		_ = s.msgQueue.MarkAsFailed(msg.ID, errMsg)
		return
	}

	// Get the LLM profile
	profile := s.llmState.ActiveProfile()
	if profile == nil {
		errMsg := "no active LLM profile - check if LLM providers are configured"
		printJSON(map[string]any{"event": "llm_process_failed", "msg_id": msg.ID, "step": "get_profile", "error": errMsg})
		_ = s.msgQueue.MarkAsFailed(msg.ID, errMsg)
		return
	}

	printJSON(map[string]any{
		"event":       "llm_process_profile",
		"msg_id":      msg.ID,
		"model":       profile.Config.Model,
		"base_url":    profile.Config.BaseURL,
	})

	// Reload conversation to get updated messages
	conv, err = s.convStore.Get(conv.ID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reload conversation: %v", err)
		printJSON(map[string]any{"event": "llm_process_failed", "msg_id": msg.ID, "step": "reload_conversation", "error": errMsg})
		_ = s.msgQueue.MarkAsFailed(msg.ID, errMsg)
		return
	}

	// Get system prompt from primary config
	var systemPrompt string
	if s.systemPromptStore != nil {
		systemPrompt, err = s.systemPromptStore.GetLLMPrimaryConfigSystemPrompt()
		if err != nil {
			printJSON(map[string]any{"event": "llm_system_prompt_warning", "msg_id": msg.ID, "error": err.Error()})
		}
	}

	// Run the tool loop to get augmented messages
	augmentedMessages, err := toolrouter.RunAgentToolLoop(procCtx, s.toolRouter, profile, conv.Messages, s.toolMgr, nil)
	_ = augmentedMessages // We'll use this in the future with proper tool support

	// For now, use simple completion since we don't have tools registered yet
	printJSON(map[string]any{"event": "llm_process_completion_start", "msg_id": msg.ID, "has_system_prompt": systemPrompt != ""})
	reply, err := completion.CompleteText(procCtx, profile, systemPrompt, conv.Messages, 512)
	if err != nil {
		errMsg := fmt.Sprintf("LLM completion failed: %v", err)
		printJSON(map[string]any{"event": "llm_process_failed", "msg_id": msg.ID, "step": "llm_completion", "error": errMsg})
		_ = s.msgQueue.MarkAsFailed(msg.ID, errMsg)
		return
	}

	printJSON(map[string]any{
		"event":      "llm_process_completion_success",
		"msg_id":     msg.ID,
		"reply_len":  len(reply),
	})

	// Clean and validate reply text
	reply = cleanReplyText(reply)
	printJSON(map[string]any{
		"event":       "llm_process_text_cleaned",
		"msg_id":      msg.ID,
		"cleaned_len": len(reply),
	})

	// Truncate reply for Meshtastic (UTF-8 safe truncation)
	if len([]byte(reply)) > MaxReplyLength {
		reply = truncateUTF8(reply, MaxReplyLength-3) + "..."
		printJSON(map[string]any{
			"event":       "llm_process_text_truncated",
			"msg_id":      msg.ID,
			"truncated_len": len(reply),
		})
	}

	// Final UTF-8 validation before sending
	if !utf8.ValidString(reply) {
		printJSON(map[string]any{
			"event":   "llm_process_utf8_warning",
			"msg_id":  msg.ID,
			"message": "final text still invalid, using fallback",
		})
		reply = "抱歉，我暂时无法回复。请稍后再试。"
	}
	printJSON(map[string]any{
		"event":    "llm_process_final_check",
		"msg_id":   msg.ID,
		"valid_utf8": utf8.ValidString(reply),
		"final_len":  len(reply),
	})

	// Add assistant reply to conversation
	assistantMsg := message.ChatMessage{
		Role:    "assistant",
		Content: reply,
	}
	if err := s.convStore.AddMessage(conv.ID, assistantMsg); err != nil {
		// Non-fatal, continue
	}

	// Send the reply via the bot - 根据消息类型决定发送方式
	// 频道消息：回复到原频道；私聊消息：回复给发送节点
	var sendErr error
	if msg.MessageType == "channel" && msg.ChannelID != nil && *msg.ChannelID != "" {
		// 频道消息 - 回复到原频道
		printJSON(map[string]any{"event": "llm_process_send_start", "msg_id": msg.ID, "channel_id": *msg.ChannelID, "message_type": "channel"})
		sendErr = s.botSender.SendChannelText(procCtx, msg.BotID, *msg.ChannelID, reply)
	} else {
		// 私聊消息 - 回复给发送节点
		printJSON(map[string]any{"event": "llm_process_send_start", "msg_id": msg.ID, "to_node_num": msg.FromNodeNum, "message_type": "direct"})
		sendErr = s.botSender.SendDirectText(procCtx, msg.BotID, msg.FromNodeNum, reply)
	}
	if sendErr != nil {
		errMsg := fmt.Sprintf("failed to send reply: %v", sendErr)
		printJSON(map[string]any{"event": "llm_process_failed", "msg_id": msg.ID, "step": "send_reply", "error": errMsg})
		_ = s.msgQueue.MarkAsFailed(msg.ID, errMsg)
		return
	}

	// Mark message as processed
	_ = s.msgQueue.MarkAsProcessed(msg.ID, reply)
	printJSON(map[string]any{
		"event":    "llm_process_success",
		"msg_id":   msg.ID,
		"reply":    reply,
	})
}

// formatUserMessage formats the incoming message for the LLM
func (s *Service) formatUserMessage(msg QueuedMessage) string {
	var sb strings.Builder

	if msg.LongName != nil && *msg.LongName != "" {
		sb.WriteString(fmt.Sprintf("[来自 %s (%s)] ", *msg.LongName, msg.FromNodeID))
	} else if msg.ShortName != nil && *msg.ShortName != "" {
		sb.WriteString(fmt.Sprintf("[来自 %s (%s)] ", *msg.ShortName, msg.FromNodeID))
	} else {
		sb.WriteString(fmt.Sprintf("[来自 %s] ", msg.FromNodeID))
	}

	sb.WriteString(msg.Text)
	return sb.String()
}

// cleanReplyText cleans LLM reply text to ensure it's valid for Meshtastic
func cleanReplyText(text string) string {
	// Force convert to valid UTF-8 using rune-by-rune processing
	v := make([]rune, 0, len(text))
	for i, r := range text {
		if r == utf8.RuneError {
			_, size := utf8.DecodeRuneInString(text[i:])
			if size == 1 {
				continue // Skip invalid runes
			}
		}
		// Skip all problematic characters
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			continue
		}
		if r == 65533 || r == 0xfffd { // Unicode replacement character
			continue
		}
		v = append(v, r)
	}
	text = string(v)

	// Additional cleanup - use only explicitly allowed ASCII printable + CJK
	var sb strings.Builder
	for _, r := range text {
		switch {
		case r == '\n':
			sb.WriteRune(' ') // Replace newlines with space for Meshtastic
		case r == '\r' || r == '\t':
			continue
		case r >= 32 && r <= 126: // Printable ASCII
			sb.WriteRune(r)
		case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
			sb.WriteRune(r)
		case r >= 0x3400 && r <= 0x4DBF: // CJK Extension A
			sb.WriteRune(r)
		case r >= 0x20000 && r <= 0x2A6DF: // CJK Extension B
			sb.WriteRune(r)
		case r >= 0xFF01 && r <= 0xFF5E: // Fullwidth ASCII variants
			sb.WriteRune(r)
		case r == 0x3002 || r == 0xFF1F || r == 0xFF01 || r == 0xFF0C || r == 0xFF1A: // Fullwidth punctuation
			sb.WriteRune(r)
		default:
			continue // Skip all other characters
		}
	}

	result := strings.TrimSpace(sb.String())
	if result == "" {
		result = "抱歉，我无法回复此消息。"
	}
	return result
}

// truncateUTF8 safely truncates a UTF-8 string to max bytes without breaking in the middle of a rune
func truncateUTF8(s string, maxBytes int) string {
	if len([]byte(s)) <= maxBytes {
		return s
	}
	bytes := []byte(s)
	for i := maxBytes; i > 0; i-- {
		if utf8.RuneStart(bytes[i]) {
			return string(bytes[:i])
		}
	}
	return ""
}
