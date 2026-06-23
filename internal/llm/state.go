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

// UpdateProvider updates an existing provider's configuration
func (s *State) UpdateProvider(config ProviderConfig) error {
	name := strings.TrimSpace(config.Name)
	if name == "" {
		return errors.New("llm provider name cannot be empty")
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return fmt.Errorf("llm provider %s api_key is required", name)
	}
	if strings.TrimSpace(config.Model) == "" {
		return fmt.Errorf("llm provider %s model is required", name)
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return fmt.Errorf("llm provider %s base_url is required", name)
	}
	if config.Timeout <= 0 {
		config.Timeout = 120
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.profiles[name]; !ok {
		return fmt.Errorf("llm provider not found: %s", name)
	}

	// Create new client with updated config
	s.profiles[name] = &Profile{
		Config: config,
		Client: ark.NewClientWithApiKey(
			config.APIKey,
			ark.WithBaseUrl(config.BaseURL),
			ark.WithTimeout(time.Duration(config.Timeout)*time.Second),
		),
	}

	// Update active status if needed
	if config.Active && s.activeName != name {
		s.activeName = name
	} else if !config.Active && s.activeName == name {
		// If we're deactivating the current active provider, switch to the first available
		for _, otherName := range s.order {
			if otherName != name {
				s.activeName = otherName
				break
			}
		}
	}

	return nil
}

// AddProvider adds a new provider to the state
func (s *State) AddProvider(config ProviderConfig) error {
	name := strings.TrimSpace(config.Name)
	if name == "" {
		return errors.New("llm provider name cannot be empty")
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return fmt.Errorf("llm provider %s api_key is required", name)
	}
	if strings.TrimSpace(config.Model) == "" {
		return fmt.Errorf("llm provider %s model is required", name)
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return fmt.Errorf("llm provider %s base_url is required", name)
	}
	if config.Timeout <= 0 {
		config.Timeout = 120
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.profiles[name]; ok {
		return fmt.Errorf("llm provider already exists: %s", name)
	}

	s.profiles[name] = &Profile{
		Config: config,
		Client: ark.NewClientWithApiKey(
			config.APIKey,
			ark.WithBaseUrl(config.BaseURL),
			ark.WithTimeout(time.Duration(config.Timeout)*time.Second),
		),
	}
	s.order = append(s.order, name)

	// Set as active if it's the first one or explicitly marked active
	if len(s.profiles) == 1 || config.Active {
		s.activeName = name
	}

	return nil
}

// RemoveProvider removes a provider from the state
func (s *State) RemoveProvider(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("llm provider name cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.profiles[name]; !ok {
		return fmt.Errorf("llm provider not found: %s", name)
	}

	// Don't allow removing the last provider
	if len(s.profiles) == 1 {
		return errors.New("cannot remove the last llm provider")
	}

	delete(s.profiles, name)

	// Remove from order
	newOrder := make([]string, 0, len(s.order)-1)
	for _, n := range s.order {
		if n != name {
			newOrder = append(newOrder, n)
		}
	}
	s.order = newOrder

	// If we removed the active provider, switch to the first available
	if s.activeName == name {
		s.activeName = s.order[0]
	}

	return nil
}
