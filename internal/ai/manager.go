package ai

import (
	"context"
	"fmt"
	"sync"

	"meshtastic_mqtt_server/internal/autoreply"
	"meshtastic_mqtt_server/internal/llm"
	storepkg "meshtastic_mqtt_server/internal/store"

	"gorm.io/gorm"
)

// AIServiceStatus reports the current state of the AI service
type AIServiceStatus struct {
	Running       bool   `json:"running"`
	Enabled       bool   `json:"enabled"`
	ProviderCount int    `json:"provider_count"`
	Message       string `json:"message,omitempty"`
}

// AIManager manages the lifecycle of the AI service, supporting restart
type AIManager struct {
	mu        sync.Mutex
	service   *Service
	cfg       Config
	db        *gorm.DB
	botSender autoreply.BotSender
	ctx       context.Context
	store     *storepkg.Store
}

// NewAIManager creates a new AIManager
func NewAIManager(cfg Config, db *gorm.DB, botSender autoreply.BotSender, ctx context.Context, store *storepkg.Store) *AIManager {
	return &AIManager{
		cfg:       cfg,
		db:        db,
		botSender: botSender,
		ctx:       ctx,
		store:     store,
	}
}

// SetConfigEnabled sets the enabled flag on the config
func (m *AIManager) SetConfigEnabled(enabled bool) {
	m.cfg.Enabled = enabled
}

// SetProviderConfigs sets the LLM provider configs
func (m *AIManager) SetProviderConfigs(configs []llm.ProviderConfig) {
	m.cfg.LLMProviders = configs
}

// Init creates and starts the AI service
func (m *AIManager) Init() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	svc, err := NewService(m.cfg, m.db, m.botSender)
	if err != nil {
		return fmt.Errorf("failed to create AI service: %w", err)
	}
	if err := svc.Start(m.ctx); err != nil {
		return fmt.Errorf("failed to start AI service: %w", err)
	}
	m.service = svc
	return nil
}

// Stop stops the currently running AI service
func (m *AIManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.service != nil {
		m.service.Stop()
		m.service = nil
	}
}

// Status returns the current AI service status
func (m *AIManager) Status() AIServiceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	providerCount := len(m.cfg.LLMProviders)
	if m.service == nil {
		msg := "AI 服务未运行，可在配置提供商后点击重启"
		if providerCount == 0 {
			msg = "尚未配置 AI 提供商，请先添加提供商配置"
		}
		return AIServiceStatus{
			Running:       false,
			Enabled:       m.cfg.Enabled,
			ProviderCount: providerCount,
			Message:       msg,
		}
	}
	enabled := m.service.Enabled()
	if !enabled {
		return AIServiceStatus{
			Running:       false,
			Enabled:       false,
			ProviderCount: providerCount,
			Message:       "AI 服务未启用",
		}
	}
	return AIServiceStatus{
		Running:       true,
		Enabled:       true,
		ProviderCount: providerCount,
	}
}

// Restart stops the current AI service and creates a new one from DB
func (m *AIManager) Restart() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.service != nil {
		m.service.Stop()
		m.service = nil
	}

	providers, err := m.store.ListLLMProviders(true)
	if err != nil {
		return fmt.Errorf("加载 LLM 提供商列表失败: %w", err)
	}
	if len(providers) == 0 {
		return fmt.Errorf("没有配置任何 LLM 提供商，请先添加配置")
	}

	providerConfigs := make([]llm.ProviderConfig, 0, len(providers))
	for _, p := range providers {
		providerConfigs = append(providerConfigs, llm.ProviderConfig{
			Name:                p.Name,
			Active:              p.Active,
			APIKey:              p.APIKey,
			BaseURL:             p.BaseURL,
			Model:               p.Model,
			Timeout:             p.Timeout,
			ContextWindowTokens: p.ContextWindowTokens,
		})
	}
	m.cfg.LLMProviders = providerConfigs

	svc, err := NewService(m.cfg, m.db, m.botSender)
	if err != nil {
		return fmt.Errorf("创建 AI 服务失败: %w", err)
	}
	if err := svc.Start(m.ctx); err != nil {
		return fmt.Errorf("启动 AI 服务失败: %w", err)
	}

	m.service = svc
	return nil
}

// ReloadLLMProvider delegates to the current service
func (m *AIManager) ReloadLLMProvider(config interface{}) error {
	m.mu.Lock()
	svc := m.service
	m.mu.Unlock()

	if svc == nil {
		return nil
	}
	return svc.ReloadLLMProvider(config)
}

// AddLLMProvider delegates to the current service
func (m *AIManager) AddLLMProvider(config interface{}) error {
	m.mu.Lock()
	svc := m.service
	m.mu.Unlock()

	if svc == nil {
		return nil
	}
	return svc.AddLLMProvider(config)
}

// RemoveLLMProvider delegates to the current service
func (m *AIManager) RemoveLLMProvider(name string) error {
	m.mu.Lock()
	svc := m.service
	m.mu.Unlock()

	if svc == nil {
		return nil
	}
	return svc.RemoveLLMProvider(name)
}

// AIServiceStatus returns the AI service status for the web interface
func (m *AIManager) AIServiceStatus() AIServiceStatus {
	return m.Status()
}

// RestartAIService restarts the AI service for the web interface
func (m *AIManager) RestartAIService() error {
	return m.Restart()
}