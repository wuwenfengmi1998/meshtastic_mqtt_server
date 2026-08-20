package conversation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"meshtastic_mqtt_server/internal/message"
)

// Store manages conversations stored as JSON files
type Store struct {
	dir string
	mu  sync.Mutex
}

// NewStore creates a new conversation store
func NewStore(dir string) *Store {
	os.MkdirAll(dir, 0755)
	return &Store{dir: dir}
}

// path returns the file path for a conversation ID
func (s *Store) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// botPath returns the directory path for a specific bot
func (s *Store) botPath(botID uint64) string {
	return filepath.Join(s.dir, fmt.Sprintf("bot_%d", botID))
}

// Create creates a new conversation for a bot
func (s *Store) Create(botID uint64, botNodeID string) (*message.Conversation, error) {
	return s.CreateWithPreset(botID, botNodeID, "")
}

// CreateWithPreset creates a new conversation with a preset prompt
func (s *Store) CreateWithPreset(botID uint64, botNodeID string, preset string) (*message.Conversation, error) {
	conv := &message.Conversation{
		ID:           generateConversationID(botID),
		BotID:        botID,
		BotNodeID:    botNodeID,
		Title:        "新对话",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		PresetPrompt: strings.TrimSpace(preset),
	}
	if err := s.Save(conv); err != nil {
		return nil, err
	}
	return conv, nil
}

// Save saves a conversation to disk
func (s *Store) Save(conv *message.Conversation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	conv.UpdatedAt = time.Now()
	return atomicWriteJSON(s.path(conv.ID), conv)
}

// Get retrieves a conversation by ID
func (s *Store) Get(id string) (*message.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("对话不存在")
		}
		return nil, fmt.Errorf("读取对话失败: %w", err)
	}
	var conv message.Conversation
	if err := json.Unmarshal(data, &conv); err != nil {
		return nil, fmt.Errorf("解析对话失败: %w", err)
	}
	return &conv, nil
}

// GetOrCreateForBot gets or creates a conversation for a bot.
// peerNodeID 非空时按 (bot, peer) 隔离会话:不同对端互不可见上下文,
// 防止 A 的私聊内容/提示注入影响 B;peerNodeID 为空时维持旧行为(取最近会话)。
func (s *Store) GetOrCreateForBot(botID uint64, botNodeID string, peerNodeID string) (*message.Conversation, error) {
	peerNodeID = strings.TrimSpace(peerNodeID)
	convs, err := s.ListForBot(botID)
	if err == nil && len(convs) > 0 {
		if peerNodeID != "" {
			// List 按 UpdatedAt 倒序,取该对端最近一次会话。
			for _, conv := range convs {
				if conv.PeerNodeID == peerNodeID {
					return s.Get(conv.ID)
				}
			}
		} else {
			return s.Get(convs[0].ID)
		}
	}
	conv, err := s.Create(botID, botNodeID)
	if err != nil {
		return nil, err
	}
	if peerNodeID != "" {
		conv.PeerNodeID = peerNodeID
		if err := s.Save(conv); err != nil {
			return nil, err
		}
	}
	return conv, nil
}

// List returns all conversations
func (s *Store) List() ([]message.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("读取对话目录失败: %w", err)
	}

	var list []message.Conversation
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var conv message.Conversation
		if err := json.Unmarshal(data, &conv); err != nil {
			continue
		}
		conv.Messages = nil // Don't return messages in list
		list = append(list, conv)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].UpdatedAt.After(list[j].UpdatedAt)
	})
	return list, nil
}

// ListForBot returns all conversations for a specific bot
func (s *Store) ListForBot(botID uint64) ([]message.Conversation, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var filtered []message.Conversation
	for _, conv := range all {
		if conv.BotID == botID {
			filtered = append(filtered, conv)
		}
	}
	return filtered, nil
}

// Delete deletes a conversation by ID
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除对话失败: %w", err)
	}
	return nil
}

// DeleteForBot deletes all conversations for a bot
func (s *Store) DeleteForBot(botID uint64) error {
	convs, err := s.ListForBot(botID)
	if err != nil {
		return err
	}
	for _, conv := range convs {
		_ = s.Delete(conv.ID)
	}
	return nil
}

// maxConversationMessages 限制单会话保留的历史消息数,防止上下文无限增长。
const maxConversationMessages = 50

// AddMessage adds a message to a conversation
func (s *Store) AddMessage(convID string, msg message.ChatMessage) error {
	conv, err := s.Get(convID)
	if err != nil {
		return err
	}
	conv.Messages = append(conv.Messages, msg)
	if len(conv.Messages) > maxConversationMessages {
		conv.Messages = conv.Messages[len(conv.Messages)-maxConversationMessages:]
	}
	if conv.Title == "" || conv.Title == "新对话" {
		conv.Title = GenerateTitle(conv.Messages)
	}
	return s.Save(conv)
}

// PopLastMessage 移除会话中最后一条消息（例如被话题选择丢弃、不应保留在上下文里的用户消息）。
// 若会话没有消息则不做任何改动。返回移除的消息内容，便于调用方记录日志。
func (s *Store) PopLastMessage(convID string) (message.ChatMessage, error) {
	conv, err := s.Get(convID)
	if err != nil {
		return message.ChatMessage{}, err
	}
	if len(conv.Messages) == 0 {
		return message.ChatMessage{}, nil
	}
	last := conv.Messages[len(conv.Messages)-1]
	conv.Messages = conv.Messages[:len(conv.Messages)-1]
	// 若弹出后没有消息了，重置标题，避免残留被丢弃消息的文本。
	if len(conv.Messages) == 0 {
		conv.Title = "新对话"
	}
	return last, s.Save(conv)
}

// atomicWriteJSON writes JSON to a file atomically
func atomicWriteJSON(path string, v any) error {
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// GenerateTitle generates a title from conversation messages
func GenerateTitle(messages []message.ChatMessage) string {
	for _, m := range messages {
		if m.Hidden {
			continue
		}
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			title := strings.TrimSpace(m.Content)
			title = strings.ReplaceAll(title, "\r\n", " ")
			title = strings.ReplaceAll(title, "\n", " ")
			runes := []rune(title)
			if len(runes) > 30 {
				return string(runes[:30]) + "..."
			}
			return title
		}
	}
	return "新对话"
}

// generateConversationID generates a unique conversation ID
func generateConversationID(botID uint64) string {
	return fmt.Sprintf("bot_%d_%d", botID, time.Now().UnixNano())
}
