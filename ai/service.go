package ai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"meshtastic_mqtt_server/agenttool"
	_ "meshtastic_mqtt_server/agents/calculator"
	_ "meshtastic_mqtt_server/agents/time"
	"meshtastic_mqtt_server/autoreply"
	"meshtastic_mqtt_server/conversation"
	"meshtastic_mqtt_server/llm"
	"meshtastic_mqtt_server/toolmanager"
	"meshtastic_mqtt_server/toolrouter"

	"gorm.io/gorm"
)

// SystemPromptStore is the interface for getting the system prompt
type SystemPromptStore interface {
	GetLLMPrimaryConfigSystemPrompt() (string, error)
}

// Config holds the AI service configuration
type Config struct {
	LLMProviders      []llm.ProviderConfig
	DataDir           string
	Enabled           bool
	SystemPromptStore SystemPromptStore
}

// Service manages all AI-related components
type Service struct {
	LLMState   *llm.State
	ToolRouter *toolrouter.State
	ToolMgr    *toolmanager.Manager
	ConvStore  *conversation.Store
	AutoReply  *autoreply.Service
	MsgQueue   *autoreply.DBMessageQueue

	enabled bool
}

// NewService creates a new AI service
func NewService(cfg Config, db *gorm.DB, botSender autoreply.BotSender) (*Service, error) {
	if !cfg.Enabled {
		return &Service{enabled: false}, nil
	}

	// Create data directories
	agentsDir := filepath.Join(cfg.DataDir, "agents")
	convDir := filepath.Join(cfg.DataDir, "conversations")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create agents directory: %w", err)
	}
	if err := os.MkdirAll(convDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create conversations directory: %w", err)
	}

	// Initialize LLM state
	llmState, err := llm.NewState(cfg.LLMProviders)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LLM state: %w", err)
	}

	// Initialize tool router
	toolRouterCfg := &toolrouter.Config{
		Enabled:      true,
		Timeout:      30,
		MaxTokens:    512,
		SystemPrompt: "你是一个智能助手，可以调用工具来回答用户问题。\n用户正在通过 Mesh 网络与你对话，请保持回答简洁明了。\n工具结果优先于模型内置知识。",
	}
	toolRouter, err := toolrouter.NewState(toolRouterCfg, llmState)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tool router: %w", err)
	}

	// Load tools
	toolMgr, err := toolmanager.Load(agentsDir, agenttool.LoadOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to load tools: %w", err)
	}

	// Initialize conversation store
	convStore := conversation.NewStore(convDir)

	// Initialize message queue
	msgQueue := autoreply.NewDBMessageQueue(db)

	// Initialize auto-reply service
	autoReply := autoreply.NewService(
		llmState,
		toolRouter,
		toolMgr,
		convStore,
		msgQueue,
		botSender,
		cfg.SystemPromptStore,
	)

	return &Service{
		LLMState:   llmState,
		ToolRouter: toolRouter,
		ToolMgr:    toolMgr,
		ConvStore:  convStore,
		AutoReply:  autoReply,
		MsgQueue:   msgQueue,
		enabled:    true,
	}, nil
}

// Start starts the AI service
func (s *Service) Start(ctx context.Context) error {
	if !s.enabled {
		return nil
	}
	return s.AutoReply.Start(ctx)
}

// Stop stops the AI service
func (s *Service) Stop() {
	if !s.enabled {
		return
	}
	s.AutoReply.Stop()
	s.ToolMgr.Close()
}

// Enabled returns whether the AI service is enabled
func (s *Service) Enabled() bool {
	return s.enabled
}
