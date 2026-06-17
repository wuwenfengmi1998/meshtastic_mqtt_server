package autoreply

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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
	ReceivedAt  time.Time
}

// BotSender is the interface for sending bot messages
type BotSender interface {
	SendText(ctx context.Context, botID uint64, toNodeNum int64, text string) error
}

// Service manages automatic AI replies for bots
type Service struct {
	llmState   *llm.State
	toolRouter *toolrouter.State
	toolMgr    *toolmanager.Manager
	convStore  *conversation.Store
	msgQueue   MessageQueue
	botSender  BotSender

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
) *Service {
	return &Service{
		llmState:  llmState,
		toolRouter: toolRouter,
		toolMgr:   toolMgr,
		convStore: convStore,
		msgQueue:  msgQueue,
		botSender: botSender,
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

// processMessage processes a single queued message
func (s *Service) processMessage(ctx context.Context, msg QueuedMessage) {
	// Mark message as processing
	if err := s.msgQueue.MarkAsProcessing(msg.ID); err != nil {
		return
	}

	// Create processing context with timeout
	procCtx, cancel := context.WithTimeout(ctx, MaxProcessingTime)
	defer cancel()

	// Get or create conversation for this bot
	conv, err := s.convStore.GetOrCreateForBot(msg.BotID, msg.BotNodeID, msg.FromNodeID)
	if err != nil {
		_ = s.msgQueue.MarkAsFailed(msg.ID, fmt.Sprintf("failed to get conversation: %v", err))
		return
	}

	// Add the user message to the conversation
	userMsg := message.ChatMessage{
		Role:    "user",
		Content: s.formatUserMessage(msg),
	}
	if err := s.convStore.AddMessage(conv.ID, userMsg); err != nil {
		_ = s.msgQueue.MarkAsFailed(msg.ID, fmt.Sprintf("failed to add message: %v", err))
		return
	}

	// Get the LLM profile
	profile := s.llmState.ActiveProfile()
	if profile == nil {
		_ = s.msgQueue.MarkAsFailed(msg.ID, "no active LLM profile")
		return
	}

	// Reload conversation to get updated messages
	conv, err = s.convStore.Get(conv.ID)
	if err != nil {
		_ = s.msgQueue.MarkAsFailed(msg.ID, fmt.Sprintf("failed to reload conversation: %v", err))
		return
	}

	// Run the tool loop to get augmented messages
	augmentedMessages, err := toolrouter.RunAgentToolLoop(procCtx, s.toolRouter, profile, conv.Messages, s.toolMgr, nil)
	_ = augmentedMessages // We'll use this in the future with proper tool support

	// For now, use simple completion since we don't have tools registered yet
	reply, err := completion.CompleteText(procCtx, profile, conv.Messages, 512)
	if err != nil {
		_ = s.msgQueue.MarkAsFailed(msg.ID, fmt.Sprintf("LLM completion failed: %v", err))
		return
	}

	// Truncate reply for Meshtastic
	if len([]byte(reply)) > MaxReplyLength {
		reply = string([]byte(reply)[:MaxReplyLength-3]) + "..."
	}

	// Add assistant reply to conversation
	assistantMsg := message.ChatMessage{
		Role:    "assistant",
		Content: reply,
	}
	if err := s.convStore.AddMessage(conv.ID, assistantMsg); err != nil {
		// Non-fatal, continue
	}

	// Send the reply via the bot
	if err := s.botSender.SendText(procCtx, msg.BotID, msg.FromNodeNum, reply); err != nil {
		_ = s.msgQueue.MarkAsFailed(msg.ID, fmt.Sprintf("failed to send reply: %v", err))
		return
	}

	// Mark message as processed
	_ = s.msgQueue.MarkAsProcessed(msg.ID, reply)
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
