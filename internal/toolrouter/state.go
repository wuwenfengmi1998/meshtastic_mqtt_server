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

// State manages the tool router state
type State struct {
	cfg *Config
	ai  *llm.State
}

// Option is a function that configures the State
type Option func(*State)

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

// RouterProfile returns the LLM profile configured for the tool router
func (s *State) RouterProfile(fallback *llm.Profile) *llm.Profile {
	if s == nil || s.cfg == nil || s.ai == nil {
		return fallback
	}
	name := strings.TrimSpace(s.cfg.OpenAIName)
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
	if s == nil || s.cfg == nil {
		return Config{}
	}
	return *s.cfg
}
