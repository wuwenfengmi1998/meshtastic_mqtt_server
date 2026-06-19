package ai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"meshtastic_mqtt_server/internal/agenttool"
	_ "meshtastic_mqtt_server/internal/agents/calculator"
	_ "meshtastic_mqtt_server/internal/agents/time"
	"meshtastic_mqtt_server/internal/autoreply"
	"meshtastic_mqtt_server/internal/conversation"
	"meshtastic_mqtt_server/internal/llm"
	storepkg "meshtastic_mqtt_server/internal/store"
	"meshtastic_mqtt_server/internal/toolmanager"
	"meshtastic_mqtt_server/internal/toolrouter"

	"gorm.io/gorm"
)

// ToolConfigStore is the interface for getting tool configuration
type ToolConfigStore interface {
	GetLLMPrimaryConfigSystemPrompt() (string, error)
	GetLLMPrimaryConfigEnableTool() (bool, error)
}

// ToolRouterStore 是 ai 服务依赖的 ToolRouter 持久化接口；
// 通常由 *store.Store 实现（GetLLMToolRouter）。
// 通过本接口我们可以让 toolrouter 在每轮调用时拉取最新配置，
// 让 /admin/llm/api 中的修改在保存后立即生效（无需重启）。
type ToolRouterStore interface {
	GetLLMToolRouter() (*storepkg.LLMToolRouterRecord, error)
}

// Config holds the AI service configuration
type Config struct {
	LLMProviders    []llm.ProviderConfig
	DataDir         string
	Enabled         bool
	ConsoleLog      bool
	ToolConfigStore ToolConfigStore
	ToolRouterStore ToolRouterStore
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

// toolRouterConfigAdapter 把 ToolRouterStore 适配成 toolrouter.ConfigStore，
// 每次 LoadToolRouterConfig 都从 DB 拉取最新一行 llm_tool_router。
type toolRouterConfigAdapter struct {
	store ToolRouterStore
}

// LoadToolRouterConfig 实现 toolrouter.ConfigStore。
// 当 DB 没有记录时返回 nil + nil，由 toolrouter 内部回退到内存默认值。
func (a *toolRouterConfigAdapter) LoadToolRouterConfig() (*toolrouter.Config, error) {
	if a == nil || a.store == nil {
		return nil, errors.New("tool router store is not configured")
	}
	record, err := a.store.GetLLMToolRouter()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toolRouterConfigFromRecord(record), nil
}

func toolRouterConfigFromRecord(r *storepkg.LLMToolRouterRecord) *toolrouter.Config {
	if r == nil {
		return nil
	}
	return &toolrouter.Config{
		Enabled:      r.Enabled,
		OpenAIName:   r.OpenAIName,
		Timeout:      r.Timeout,
		MaxTokens:    r.MaxTokens,
		SystemPrompt: r.SystemPrompt,
	}
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

	// 初始化 tool router：优先从 DB 读取已保存的配置，避免硬编码 prompt 把
	// 用户在 /admin/llm/api 配置好的内容覆盖掉。
	var (
		toolRouterCfg     *toolrouter.Config
		toolRouterOptions []toolrouter.Option
	)
	if cfg.ToolRouterStore != nil {
		adapter := &toolRouterConfigAdapter{store: cfg.ToolRouterStore}
		// 启动时拉一次作为初始 cfg；失败或为空时让 toolrouter.NewState 走内置默认值。
		if loaded, loadErr := adapter.LoadToolRouterConfig(); loadErr == nil && loaded != nil {
			toolRouterCfg = loaded
		}
		toolRouterOptions = append(toolRouterOptions, toolrouter.WithConfigStore(adapter))
	}
	toolRouter, err := toolrouter.NewState(toolRouterCfg, llmState, toolRouterOptions...)
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
		cfg.ToolConfigStore,
		cfg.ConsoleLog,
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
