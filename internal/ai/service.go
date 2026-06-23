package ai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "meshtastic_mqtt_server/internal/agents/active"
	_ "meshtastic_mqtt_server/internal/agents/calculator"
	_ "meshtastic_mqtt_server/internal/agents/sign"
	_ "meshtastic_mqtt_server/internal/agents/time"
	"meshtastic_mqtt_server/internal/agenttool"
	"meshtastic_mqtt_server/internal/autoreply"
	"meshtastic_mqtt_server/internal/conversation"
	"meshtastic_mqtt_server/internal/llm"
	storepkg "meshtastic_mqtt_server/internal/store"
	"meshtastic_mqtt_server/internal/toolmanager"
	"meshtastic_mqtt_server/internal/toolrouter"
	"meshtastic_mqtt_server/internal/topicrouter"

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

// TopicRouterStore 是 ai 服务依赖的话题选择持久化接口；
// 通常由 *store.Store 实现（GetLLMTopicConfig）。
type TopicRouterStore interface {
	GetLLMTopicConfig() (*storepkg.LLMTopicConfigRecord, error)
}

// Config holds the AI service configuration
type Config struct {
	LLMProviders     []llm.ProviderConfig
	DataDir          string
	Enabled          bool
	ConsoleLog       bool
	ToolConfigStore  ToolConfigStore
	ToolRouterStore  ToolRouterStore
	TopicRouterStore TopicRouterStore
	// Store 注入持久化层，供需要 DB 访问的 agent 工具（如签到工具）使用。
	Store *storepkg.Store
}

// Service manages all AI-related components
type Service struct {
	LLMState    *llm.State
	ToolRouter  *toolrouter.State
	TopicRouter *topicrouter.State
	ToolMgr     *toolmanager.Manager
	ConvStore   *conversation.Store
	AutoReply   *autoreply.Service
	MsgQueue    *autoreply.DBMessageQueue

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

// topicRouterConfigAdapter 把 TopicRouterStore 适配成 topicrouter.ConfigStore，
// 每次 LoadTopicConfig 都从 DB 拉取最新一行 llm_topic_config。
type topicRouterConfigAdapter struct {
	store TopicRouterStore
}

// LoadTopicConfig 实现 topicrouter.ConfigStore。
// 当 DB 没有记录时返回 nil + nil，由 topicrouter 内部回退到内存默认值。
func (a *topicRouterConfigAdapter) LoadTopicConfig() (*topicrouter.Config, error) {
	if a == nil || a.store == nil {
		return nil, errors.New("topic router store is not configured")
	}
	record, err := a.store.GetLLMTopicConfig()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return topicRouterConfigFromRecord(record), nil
}

func topicRouterConfigFromRecord(r *storepkg.LLMTopicConfigRecord) *topicrouter.Config {
	if r == nil {
		return nil
	}
	return &topicrouter.Config{
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

	// 初始化话题选择 router：同样优先从 DB 读取已保存配置，支持保存即生效。
	var (
		topicRouterCfg     *topicrouter.Config
		topicRouterOptions []topicrouter.Option
	)
	if cfg.TopicRouterStore != nil {
		topicAdapter := &topicRouterConfigAdapter{store: cfg.TopicRouterStore}
		if loaded, loadErr := topicAdapter.LoadTopicConfig(); loadErr == nil && loaded != nil {
			topicRouterCfg = loaded
		}
		topicRouterOptions = append(topicRouterOptions, topicrouter.WithConfigStore(topicAdapter))
	}
	topicRouter, err := topicrouter.NewState(topicRouterCfg, llmState, topicRouterOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize topic router: %w", err)
	}

	// Load tools
	loadOptions := agenttool.LoadOptions{Values: map[string]any{}}
	if cfg.Store != nil {
		loadOptions.Values["store"] = cfg.Store
	}
	toolMgr, err := toolmanager.Load(agentsDir, loadOptions)
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
		topicRouter,
		toolMgr,
		convStore,
		msgQueue,
		botSender,
		cfg.ToolConfigStore,
		cfg.ConsoleLog,
	)

	return &Service{
		LLMState:    llmState,
		ToolRouter:  toolRouter,
		TopicRouter: topicRouter,
		ToolMgr:     toolMgr,
		ConvStore:   convStore,
		AutoReply:   autoReply,
		MsgQueue:    msgQueue,
		enabled:     true,
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

// ReloadLLMProvider reloads a specific LLM provider configuration
func (s *Service) ReloadLLMProvider(config interface{}) error {
	if !s.enabled || s.LLMState == nil {
		return nil
	}
	providerConfig, err := convertToProviderConfig(config)
	if err != nil {
		return err
	}
	return s.LLMState.UpdateProvider(providerConfig)
}

// AddLLMProvider adds a new LLM provider
func (s *Service) AddLLMProvider(config interface{}) error {
	if !s.enabled || s.LLMState == nil {
		return nil
	}
	providerConfig, err := convertToProviderConfig(config)
	if err != nil {
		return err
	}
	return s.LLMState.AddProvider(providerConfig)
}

// RemoveLLMProvider removes an LLM provider
func (s *Service) RemoveLLMProvider(name string) error {
	if !s.enabled || s.LLMState == nil {
		return nil
	}
	return s.LLMState.RemoveProvider(name)
}

// convertToProviderConfig converts a map to llm.ProviderConfig
func convertToProviderConfig(config interface{}) (llm.ProviderConfig, error) {
	m, ok := config.(map[string]interface{})
	if !ok {
		return llm.ProviderConfig{}, fmt.Errorf("invalid config type: expected map[string]interface{}")
	}

	pc := llm.ProviderConfig{}

	if v, ok := m["Name"].(string); ok {
		pc.Name = v
	}
	if v, ok := m["Active"].(bool); ok {
		pc.Active = v
	}
	if v, ok := m["APIKey"].(string); ok {
		pc.APIKey = v
	}
	if v, ok := m["BaseURL"].(string); ok {
		pc.BaseURL = v
	}
	if v, ok := m["Model"].(string); ok {
		pc.Model = v
	}
	if v, ok := m["Timeout"].(int); ok {
		pc.Timeout = v
	}
	if v, ok := m["ContextWindowTokens"].(int); ok {
		pc.ContextWindowTokens = v
	}

	return pc, nil
}
