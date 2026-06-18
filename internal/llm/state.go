package llm

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	ark "github.com/volcengine/volcengine-go-sdk/service/arkruntime"
)

// Profile represents an LLM provider configuration with client
type Profile struct {
	Config ProviderConfig
	Client *ark.Client
}

// ProviderConfig holds the configuration for an LLM provider
type ProviderConfig struct {
	Name               string
	Active             bool
	APIKey             string
	BaseURL            string
	Model              string
	Timeout            int
	ContextWindowTokens int
}

// State manages LLM profiles
type State struct {
	mu         sync.RWMutex
	profiles   map[string]*Profile
	order      []string
	activeName string
}

// NewState creates a new LLM state from provider configurations
func NewState(configs []ProviderConfig) (*State, error) {
	state := &State{
		profiles: make(map[string]*Profile, len(configs)),
		order:    make([]string, 0, len(configs)),
	}
	for _, item := range configs {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return nil, errors.New("llm provider name cannot be empty")
		}
		if strings.TrimSpace(item.APIKey) == "" {
			return nil, fmt.Errorf("llm provider %s api_key is required", name)
		}
		if strings.TrimSpace(item.Model) == "" {
			return nil, fmt.Errorf("llm provider %s model is required", name)
		}
		if strings.TrimSpace(item.BaseURL) == "" {
			return nil, fmt.Errorf("llm provider %s base_url is required", name)
		}
		if item.Timeout <= 0 {
			item.Timeout = 120
		}
		if _, ok := state.profiles[name]; ok {
			return nil, fmt.Errorf("duplicate llm provider name: %s", name)
		}
		state.profiles[name] = &Profile{
			Config: item,
			Client: ark.NewClientWithApiKey(
				item.APIKey,
				ark.WithBaseUrl(item.BaseURL),
				ark.WithTimeout(time.Duration(item.Timeout)*time.Second),
			),
		}
		state.order = append(state.order, name)
		if item.Active && state.activeName == "" {
			state.activeName = name
		}
	}
	if len(state.order) == 0 {
		return nil, errors.New("at least one llm provider is required")
	}
	if state.activeName == "" {
		state.activeName = state.order[0]
	}
	return state, nil
}

// ActiveProfile returns the currently active LLM profile
func (s *State) ActiveProfile() *Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.profiles[s.activeName]
}

// GetProfile returns a profile by name, or the active profile if name is empty
func (s *State) GetProfile(name string) (*Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return s.profiles[s.activeName], nil
	}
	profile, ok := s.profiles[name]
	if !ok {
		return nil, fmt.Errorf("llm provider not found: %s", name)
	}
	return profile, nil
}

// SwitchActive changes the active LLM profile
func (s *State) SwitchActive(name string) (*Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("llm provider name cannot be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.profiles[name]
	if !ok {
		return nil, fmt.Errorf("llm provider not found: %s", name)
	}
	s.activeName = name
	return profile, nil
}

// ListProfiles returns all LLM profiles
func (s *State) ListProfiles() []ProviderConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	profiles := make([]ProviderConfig, 0, len(s.order))
	for _, name := range s.order {
		profile := s.profiles[name]
		cfg := profile.Config
		cfg.APIKey = "" // Hide API key in listings
		cfg.Active = name == s.activeName
		profiles = append(profiles, cfg)
	}
	return profiles
}
