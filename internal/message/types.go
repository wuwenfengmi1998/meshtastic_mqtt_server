package message

import "time"

// ChatMessage represents a single message in a conversation
type ChatMessage struct {
	Role          string `json:"role"`
	Content       string `json:"content"`
	ImageURL      string `json:"image_url,omitempty"`
	ImageURLAlias string `json:"image_url,omitempty"`
	Hidden        bool   `json:"hidden,omitempty"`
}

// Conversation represents a chat conversation with a bot
type Conversation struct {
	ID           string        `json:"id"`
	BotID        uint64        `json:"bot_id"`
	BotNodeID    string        `json:"bot_node_id"`
	Title        string        `json:"title"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	PresetPrompt string        `json:"preset_prompt,omitempty"`
	Messages     []ChatMessage `json:"messages,omitempty"`
}

// ConversationPreset represents a preset configuration for a bot's conversation
type ConversationPreset struct {
	ID          uint64    `json:"id"`
	BotID       uint64    `json:"bot_id"`
	Name        string    `json:"name"`
	SystemPrompt string    `json:"system_prompt"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
