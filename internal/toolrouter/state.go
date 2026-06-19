package toolrouter

import (
	"errors"
	"fmt"
	"strings"

	"meshtastic_mqtt_server/internal/llm"
)

// Config holds the tool router configuration
type Config struct {
	Enabled      bool
	OpenAIName   string
	Timeout      int
	MaxTokens    int
	SystemPrompt string
}

// ConfigStore 定义从持久化层读取最新 ToolRouter 配置的能力。
// 每次 RunAgentToolLoop 都会调用 LoadToolRouterConfig，从而保证管理员
// 在 /admin/llm/api 修改配置后立即生效，无需重启。
type ConfigStore interface {
	LoadToolRouterConfig() (*Config, error)
}

// State manages the tool router state
type State struct {
	cfg   *Config
	ai    *llm.State
	store ConfigStore
}

// Option is a function that configures the State
type Option func(*State)

// WithConfigStore 注入运行时配置加载器，State 会在每次需要时拉取最新配置。
func WithConfigStore(store ConfigStore) Option {
	return func(s *State) {
		s.store = store
	}
}

// NewState creates a new tool router state
func NewState(cfg *Config, ai *llm.State, options ...Option) (*State, error) {
	if cfg == nil {
		cfg = &Config{
			Enabled:      true,
			Timeout:      30,
			MaxTokens:    512,
			SystemPrompt: "你可以按需直接调用可用工具来回答用户问题。\n每个工具的 description 描述了它的适用场景和调用条件。\n工具结果优先于模型内置知识；工具失败时必须如实说明，不要编造结果。\n只调用确实必要的工具。",
		}
	}
	if ai == nil {
		return nil, errors.New("tool router requires an LLM state")
	}
	if cfg.Enabled && strings.TrimSpace(cfg.OpenAIName) != "" {
		if _, err := ai.GetProfile(cfg.OpenAIName); err != nil {
			return nil, fmt.Errorf("invalid LLM provider name in tool router: %w", err)
		}
	}
	state := &State{cfg: cfg, ai: ai}
	for _, option := range options {
		option(state)
	}
	return state, nil
}

// effectiveConfig 返回当前生效的配置：优先从 store 加载最新值，加载失败时回退到内存 cfg。
// 调用方拿到的永远是非 nil 指针；内存 cfg 也保持同步以便其它读取点。
func (s *State) effectiveConfig() *Config {
	if s == nil {
		return &Config{}
	}
	if s.store != nil {
		if latest, err := s.store.LoadToolRouterConfig(); err == nil && latest != nil {
			s.cfg = latest
			return latest
		}
	}
	if s.cfg == nil {
		return &Config{}
	}
	return s.cfg
}

// RouterProfile returns the LLM profile configured for the tool router
func (s *State) RouterProfile(fallback *llm.Profile) *llm.Profile {
	if s == nil || s.ai == nil {
		return fallback
	}
	cfg := s.effectiveConfig()
	name := strings.TrimSpace(cfg.OpenAIName)
	if name == "" {
		return fallback
	}
	profile, err := s.ai.GetProfile(name)
	if err != nil {
		return fallback
	}
	return profile
}

// Config returns a copy of the current configuration
func (s *State) Config() Config {
	if s == nil {
		return Config{}
	}
	return *s.effectiveConfig()
}
