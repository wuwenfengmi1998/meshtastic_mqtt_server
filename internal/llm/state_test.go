package llm

import (
	"testing"
)

func TestUpdateProvider(t *testing.T) {
	// Create initial state with one provider
	configs := []ProviderConfig{
		{
			Name:                "test-provider",
			Active:              true,
			APIKey:              "test-key",
			BaseURL:             "https://test.example.com",
			Model:               "test-model",
			Timeout:             120,
			ContextWindowTokens: 4096,
		},
	}

	state, err := NewState(configs)
	if err != nil {
		t.Fatalf("failed to create state: %v", err)
	}

	// Get the initial profile
	profile := state.ActiveProfile()
	if profile.Config.APIKey != "test-key" {
		t.Errorf("expected APIKey 'test-key', got '%s'", profile.Config.APIKey)
	}

	// Update the provider with new config
	updatedConfig := ProviderConfig{
		Name:                "test-provider",
		Active:              true,
		APIKey:              "new-key",
		BaseURL:             "https://new.example.com",
		Model:               "new-model",
		Timeout:             60,
		ContextWindowTokens: 8192,
	}

	err = state.UpdateProvider(updatedConfig)
	if err != nil {
		t.Fatalf("failed to update provider: %v", err)
	}

	// Verify the update
	profile = state.ActiveProfile()
	if profile.Config.APIKey != "new-key" {
		t.Errorf("expected updated APIKey 'new-key', got '%s'", profile.Config.APIKey)
	}
	if profile.Config.BaseURL != "https://new.example.com" {
		t.Errorf("expected updated BaseURL 'https://new.example.com', got '%s'", profile.Config.BaseURL)
	}
	if profile.Config.Model != "new-model" {
		t.Errorf("expected updated Model 'new-model', got '%s'", profile.Config.Model)
	}
	if profile.Config.Timeout != 60 {
		t.Errorf("expected updated Timeout 60, got %d", profile.Config.Timeout)
	}
}

func TestAddProvider(t *testing.T) {
	// Create initial state with one provider
	configs := []ProviderConfig{
		{
			Name:                "provider1",
			Active:              true,
			APIKey:              "key1",
			BaseURL:             "https://example1.com",
			Model:               "model1",
			Timeout:             120,
			ContextWindowTokens: 4096,
		},
	}

	state, err := NewState(configs)
	if err != nil {
		t.Fatalf("failed to create state: %v", err)
	}

	// Add a second provider
	newConfig := ProviderConfig{
		Name:                "provider2",
		Active:              false,
		APIKey:              "key2",
		BaseURL:             "https://example2.com",
		Model:               "model2",
		Timeout:             60,
		ContextWindowTokens: 8192,
	}

	err = state.AddProvider(newConfig)
	if err != nil {
		t.Fatalf("failed to add provider: %v", err)
	}

	// Verify the new provider exists
	profile, err := state.GetProfile("provider2")
	if err != nil {
		t.Fatalf("failed to get provider2: %v", err)
	}
	if profile.Config.Name != "provider2" {
		t.Errorf("expected name 'provider2', got '%s'", profile.Config.Name)
	}

	// Verify active provider is still provider1
	activeProfile := state.ActiveProfile()
	if activeProfile.Config.Name != "provider1" {
		t.Errorf("expected active provider 'provider1', got '%s'", activeProfile.Config.Name)
	}
}

func TestRemoveProvider(t *testing.T) {
	// Create initial state with two providers
	configs := []ProviderConfig{
		{
			Name:                "provider1",
			Active:              true,
			APIKey:              "key1",
			BaseURL:             "https://example1.com",
			Model:               "model1",
			Timeout:             120,
			ContextWindowTokens: 4096,
		},
		{
			Name:                "provider2",
			Active:              false,
			APIKey:              "key2",
			BaseURL:             "https://example2.com",
			Model:               "model2",
			Timeout:             60,
			ContextWindowTokens: 8192,
		},
	}

	state, err := NewState(configs)
	if err != nil {
		t.Fatalf("failed to create state: %v", err)
	}

	// Remove provider2
	err = state.RemoveProvider("provider2")
	if err != nil {
		t.Fatalf("failed to remove provider: %v", err)
	}

	// Verify provider2 is gone
	_, err = state.GetProfile("provider2")
	if err == nil {
		t.Error("expected error when getting removed provider, got nil")
	}

	// Try to remove the last provider (should fail)
	err = state.RemoveProvider("provider1")
	if err == nil {
		t.Error("expected error when removing last provider, got nil")
	}
}
