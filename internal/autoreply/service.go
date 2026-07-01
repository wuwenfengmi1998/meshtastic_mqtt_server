package autoreply

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"meshtastic_mqtt_server/internal/agenttool"
	"meshtastic_mqtt_server/internal/completion"
	"meshtastic_mqtt_server/internal/conversation"
	"meshtastic_mqtt_server/internal/llm"
	"meshtastic_mqtt_server/internal/message"
	"meshtastic_mqtt_server/internal/stream"
	"meshtastic_mqtt_server/internal/toolmanager"
	"meshtastic_mqtt_server/internal/toolrouter"
	"meshtastic_mqtt_server/internal/topicrouter"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
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

// ToolConfigStore is the interface for getting tool configuration
type ToolConfigStore interface {
	GetLLMPrimaryConfigSystemPrompt() (string, error)
	GetLLMPrimaryConfigEnableTool() (bool, error)
}

// Service manages automatic AI replies for bots
type Service struct {
	llmState        *llm.State
	toolRouter      *toolrouter.State
	topicRouter     *topicrouter.State
	toolMgr         *toolmanager.Manager
	convStore       *conversation.Store
	msgQueue        MessageQueue
	botSender       BotSender
	toolConfigStore ToolConfigStore
	consoleLog      bool

	running bool
	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewService creates a new auto-reply service
func NewService(
	llmState *llm.State,
	toolRouter *toolrouter.State,
	topicRouter *topicrouter.State,
	toolMgr *toolmanager.Manager,
	convStore *conversation.Store,
	msgQueue MessageQueue,
	botSender BotSender,
	toolConfigStore ToolConfigStore,
	consoleLog bool,
) *Service {
	return &Service{
		llmState:        llmState,
		toolRouter:      toolRouter,
		topicRouter:     topicRouter,
		toolMgr:         toolMgr,
		convStore:       convStore,
		msgQueue:        msgQueue,
		botSender:       botSender,
		toolConfigStore: toolConfigStore,
		consoleLog:      consoleLog,
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

// logf 仅在 console_log.llm 开启时输出一行可读日志（带 [llm] 前缀）。
func (s *Service) logf(format string, args ...any) {
	if !s.consoleLog {
		return
	}
	fmt.Fprintf(os.Stderr, "[llm] "+format+"\n", args...)
}

// emit 把 toolrouter.Frame 转成单行日志，区分主 AI / 路由 AI / 工具调用。
func (s *Service) emit(msgID uint64, routerModel string) stream.EmitFunc {
	if !s.consoleLog {
		return nil
	}
	return func(f stream.Frame) {
		switch f.Stage {
		case "prepare":
			tools, _ := f.Data["tools"].([]string)
			s.logf("msg=%d router=%s prepare tools=%v", msgID, routerModel, tools)
		case "request":
			if f.Status == "success" {
				// 模型未请求工具
				s.logf("msg=%d router=%s decide → no_tool（直接生成回答）", msgID, routerModel)
				return
			}
			iter, _ := f.Data["iteration"].(int)
			s.logf("msg=%d router=%s decide iter=%d ...", msgID, routerModel, iter)
		case "tool_calls":
			calls, _ := f.Data["tools"].([]string)
			iter, _ := f.Data["iteration"].(int)
			s.logf("msg=%d router=%s decide iter=%d → call_tools=%v", msgID, routerModel, iter, calls)
		case "arguments":
			args, _ := f.Data["arguments"].(string)
			s.logf("msg=%d tool=%s args=%s", msgID, f.Tool, truncate(args, 200))
		case "result":
			dur, _ := f.Data["duration_ms"].(int64)
			preview, _ := f.Data["result_preview"].(string)
			s.logf("msg=%d tool=%s result(%dms)=%s", msgID, f.Tool, dur, truncate(preview, 200))
		case "execute":
			if f.Status == "error" {
				errStr, _ := f.Data["error"].(string)
				if errStr == "" {
					errStr = f.Message
				}
				s.logf("msg=%d tool=%s ERROR: %s", msgID, f.Tool, errStr)
			}
		case "decision":
			// 中间帧，已被 tool_calls / request(success) 覆盖，跳过
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ptrStringValue 安全取出 *string 的值，nil 返回空串。
func ptrStringValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// processMessage processes a single queued message
func (s *Service) processMessage(ctx context.Context, msg QueuedMessage) {
	// Mark message as processing
	if err := s.msgQueue.MarkAsProcessing(msg.ID); err != nil {
		s.logf("msg=%d FAIL step=mark_as_processing err=%v", msg.ID, err)
		return
	}

	s.logf("msg=%d from=%s start text=%q", msg.ID, msg.FromNodeID, msg.Text)

	// Create processing context with timeout
	procCtx, cancel := context.WithTimeout(ctx, MaxProcessingTime)
	defer cancel()

	// Get or create conversation for this bot
	conv, err := s.convStore.GetOrCreateForBot(msg.BotID, msg.BotNodeID, msg.FromNodeID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get conversation: %v", err)
		s.logf("msg=%d FAIL step=get_conversation err=%s", msg.ID, errMsg)
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
		s.logf("msg=%d FAIL step=add_message err=%s", msg.ID, errMsg)
		_ = s.msgQueue.MarkAsFailed(msg.ID, errMsg)
		return
	}

	// Get the LLM profile
	profile := s.llmState.ActiveProfile()
	if profile == nil {
		errMsg := "no active LLM profile - check if LLM providers are configured"
		s.logf("msg=%d FAIL step=get_profile err=%s", msg.ID, errMsg)
		_ = s.msgQueue.MarkAsFailed(msg.ID, errMsg)
		return
	}

	s.logf("msg=%d main_model=%s base=%s", msg.ID, profile.Config.Model, profile.Config.BaseURL)

	// Reload conversation to get updated messages
	conv, err = s.convStore.Get(conv.ID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reload conversation: %v", err)
		s.logf("msg=%d FAIL step=reload_conversation err=%s", msg.ID, errMsg)
		_ = s.msgQueue.MarkAsFailed(msg.ID, errMsg)
		return
	}

	// Get system prompt and tool enable flag from primary config
	var systemPrompt string
	enableTool := false
	if s.toolConfigStore != nil {
		systemPrompt, err = s.toolConfigStore.GetLLMPrimaryConfigSystemPrompt()
		if err != nil {
			s.logf("msg=%d WARN system_prompt err=%v", msg.ID, err)
		}
		enableTool, err = s.toolConfigStore.GetLLMPrimaryConfigEnableTool()
		if err != nil {
			s.logf("msg=%d WARN enable_tool err=%v", msg.ID, err)
		}
	}

	// Print tool manager status for debugging
	toolCount := 0
	if s.toolMgr != nil {
		tools := s.toolMgr.Tools()
		toolCount = len(tools)
		toolNames := make([]string, 0, toolCount)
		for _, t := range tools {
			toolNames = append(toolNames, t.Name())
		}
		s.logf("msg=%d tools_loaded=%v enable_tool=%t", msg.ID, toolNames, enableTool)
	}

	// Run the tool loop to get augmented messages - pass system prompt to tool router
	// Tool loop will handle system prompt and tool calling
	var augmentedMessages []*model.ChatCompletionMessage
	toolUsed := false
	if enableTool && toolCount > 0 {
		routerProfile := s.toolRouter.RouterProfile(profile)
		routerModel := profile.Config.Model
		if routerProfile != nil {
			routerModel = routerProfile.Config.Model
		}
		s.logf("msg=%d router_model=%s tool_loop start", msg.ID, routerModel)
		// 把发送节点身份注入 ctx，供需要识别节点的工具（如签到）使用
		nodeCtx := agenttool.WithNodeContext(procCtx, agenttool.NodeContext{
			NodeID:    msg.FromNodeID,
			LongName:  ptrStringValue(msg.LongName),
			ShortName: ptrStringValue(msg.ShortName),
		})
		augmentedMessages, toolUsed, err = toolrouter.RunAgentToolLoop(nodeCtx, s.toolRouter, profile, systemPrompt, conv.Messages, s.toolMgr, s.emit(msg.ID, routerModel))
		if err != nil {
			s.logf("msg=%d WARN tool_loop err=%v", msg.ID, err)
			// Continue with original messages if tool loop fails
		}
	}

	s.logf("msg=%d completion start has_system_prompt=%t augmented=%d", msg.ID, systemPrompt != "", len(augmentedMessages))

	// 若工具路由未实际调用任何工具，则进入话题选择判定：
	// 命中（REPLY/放行）才进入主回复，未命中则丢弃不回复。
	if !toolUsed {
		shouldReply, judgeErr := topicrouter.Judge(procCtx, s.topicRouter, profile, conv.Messages)
		if judgeErr != nil {
			s.logf("msg=%d WARN topic_judge err=%v (放行)", msg.ID, judgeErr)
		}
		if !shouldReply {
			s.logf("msg=%d topic_judge=IGNORE → 丢弃不回复", msg.ID)
			// 把刚加入会话的用户消息弹出，避免它残留在上下文里被下一次回复附带回答。
			if popped, popErr := s.convStore.PopLastMessage(conv.ID); popErr != nil {
				s.logf("msg=%d WARN pop_discarded_message err=%v", msg.ID, popErr)
			} else if popped.Content != "" {
				s.logf("msg=%d pop_discarded_message content=%q", msg.ID, truncate(popped.Content, 200))
			}
			_ = s.msgQueue.MarkAsProcessed(msg.ID, "")
			return
		}
		s.logf("msg=%d topic_judge=REPLY → 进入主回复", msg.ID)
	}

	// Use augmented messages from tool loop (already includes system prompt and tool results)
	// If augmented messages is empty or nil, fallback to original messages with system prompt
	var reply string
	if len(augmentedMessages) > 0 {
		// Use augmented messages from tool loop (already converted to model.ChatCompletionMessage)
		reply, err = completion.CompleteTextWithArkMessages(procCtx, profile, augmentedMessages, 512)
	} else {
		// Fallback to simple completion
		reply, err = completion.CompleteText(procCtx, profile, systemPrompt, conv.Messages, 512)
	}
	if err != nil {
		errMsg := fmt.Sprintf("LLM completion failed: %v", err)
		s.logf("msg=%d FAIL step=llm_completion err=%s", msg.ID, errMsg)
		_ = s.msgQueue.MarkAsFailed(msg.ID, errMsg)
		return
	}

	s.logf("msg=%d main=%s reply_len=%d reply=%q", msg.ID, profile.Config.Model, len(reply), truncate(reply, 200))

	// Clean and validate reply text
	reply = cleanReplyText(reply)

	// Truncate reply for Meshtastic (UTF-8 safe truncation)
	if len([]byte(reply)) > MaxReplyLength {
		reply = truncateUTF8(reply, MaxReplyLength-3) + "..."
		s.logf("msg=%d reply truncated to %d bytes", msg.ID, len(reply))
	}

	// Final UTF-8 validation before sending
	if !utf8.ValidString(reply) {
		s.logf("msg=%d WARN final text invalid utf8, using fallback", msg.ID)
		reply = "抱歉，我暂时无法回复。请稍后再试。"
	}

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
		s.logf("msg=%d send → channel=%s", msg.ID, *msg.ChannelID)
		sendErr = s.botSender.SendChannelText(procCtx, msg.BotID, *msg.ChannelID, reply)
	} else {
		// 私聊消息 - 回复给发送节点
		s.logf("msg=%d send → direct to_node_num=%d", msg.ID, msg.FromNodeNum)
		sendErr = s.botSender.SendDirectText(procCtx, msg.BotID, msg.FromNodeNum, reply)
	}
	if sendErr != nil {
		errMsg := fmt.Sprintf("failed to send reply: %v", sendErr)
		s.logf("msg=%d FAIL step=send_reply err=%s", msg.ID, errMsg)
		_ = s.msgQueue.MarkAsFailed(msg.ID, errMsg)
		return
	}

	// Mark message as processed
	_ = s.msgQueue.MarkAsProcessed(msg.ID, reply)
	s.logf("msg=%d done", msg.ID)
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
		case r >= 0x1F600 && r <= 0x1F64F: // Emoticons
			sb.WriteRune(r)
		case r >= 0x1F300 && r <= 0x1F5FF: // Misc Symbols and Pictographs
			sb.WriteRune(r)
		case r >= 0x1F680 && r <= 0x1F6FF: // Transport and Map Symbols
			sb.WriteRune(r)
		case r >= 0x1F900 && r <= 0x1F9FF: // Supplemental Symbols and Pictographs
			sb.WriteRune(r)
		case r >= 0x2600 && r <= 0x27BF: // Misc Symbols + Dingbats
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
