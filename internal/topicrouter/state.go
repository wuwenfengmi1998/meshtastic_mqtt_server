package topicrouter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"meshtastic_mqtt_server/internal/completion"
	"meshtastic_mqtt_server/internal/llm"
	"meshtastic_mqtt_server/internal/message"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

// DefaultSystemPrompt 是话题选择判定模型的默认系统提示词。
// 模型被要求只输出 REPLY 或 IGNORE：REPLY 表示应当回复，IGNORE 表示应当丢弃。
const DefaultSystemPrompt = "你是一个话题过滤器，判断用户最新消息是否属于应当回复的话题范围。\n如果应当回复，请输出 REPLY；如果不应当回复，请输出 IGNORE。\n只输出 REPLY 或 IGNORE，不要输出任何其他内容。"

// Config holds the topic selection configuration
type Config struct {
	Enabled      bool
	OpenAIName   string
	Timeout      int
	MaxTokens    int
	SystemPrompt string
}

// ConfigStore 定义从持久化层读取最新话题选择配置的能力。
// 每次 Judge 都会调用 LoadTopicConfig，从而保证管理员在 /admin/llm/api
// 修改配置后立即生效，无需重启。
type ConfigStore interface {
	LoadTopicConfig() (*Config, error)
}

// State manages the topic router state
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

// NewState creates a new topic router state
func NewState(cfg *Config, ai *llm.State, options ...Option) (*State, error) {
	if cfg == nil {
		cfg = &Config{
			Enabled:      false,
			Timeout:      30,
			MaxTokens:    512,
			SystemPrompt: DefaultSystemPrompt,
		}
	}
	if ai == nil {
		return nil, errors.New("topic router requires an LLM state")
	}
	if cfg.Enabled && strings.TrimSpace(cfg.OpenAIName) != "" {
		if _, err := ai.GetProfile(cfg.OpenAIName); err != nil {
			return nil, fmt.Errorf("invalid LLM provider name in topic router: %w", err)
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
		if latest, err := s.store.LoadTopicConfig(); err == nil && latest != nil {
			s.cfg = latest
			return latest
		}
	}
	if s.cfg == nil {
		return &Config{}
	}
	return s.cfg
}

// Profile 返回话题选择使用的 LLM profile，OpenAIName 为空时回退到 fallback（主 profile）。
func (s *State) Profile(fallback *llm.Profile) *llm.Profile {
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

// Judge 对最近一条用户消息做话题判定。
// 返回值 shouldReply：true 表示命中/放行（应进入主回复），false 表示应丢弃不回复。
// 当话题选择未启用、未配置提供商，或判定调用失败时，一律放行（返回 true），
// 避免判定接口故障导致所有未命中工具的消息被丢弃。
func Judge(ctx context.Context, state *State, fallback *llm.Profile, messages []message.ChatMessage) (bool, error) {
	if state == nil {
		return true, nil
	}
	cfg := state.effectiveConfig()
	if !cfg.Enabled {
		return true, nil
	}

	profile := state.Profile(fallback)
	if profile == nil || profile.Client == nil {
		// 未配置话题选择的 AI 提供商，回退到放行
		return true, nil
	}

	// 取最后一条用户消息作为判定输入
	userText := lastUserMessage(messages)
	if strings.TrimSpace(userText) == "" {
		return true, nil
	}

	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}

	arkMessages := make([]*model.ChatCompletionMessage, 0, 2)
	arkMessages = append(arkMessages, &model.ChatCompletionMessage{
		Role: "system",
		Content: &model.ChatCompletionMessageContent{
			StringValue: &systemPrompt,
		},
	})
	arkMessages = append(arkMessages, &model.ChatCompletionMessage{
		Role: "user",
		Content: &model.ChatCompletionMessageContent{
			StringValue: &userText,
		},
	})

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 512
	}
	timeout := time.Duration(cfg.Timeout) * time.Second
	if cfg.Timeout <= 0 {
		timeout = 30 * time.Second
	}

	resp, err := completion.CompleteChat(ctx, profile, model.CreateChatCompletionRequest{
		Model:     profile.Config.Model,
		Messages:  arkMessages,
		MaxTokens: &maxTokens,
	}, timeout)
	if err != nil {
		// 判定调用失败时放行，避免接口故障导致全部丢消息
		return true, err
	}
	if len(resp.Choices) == 0 {
		return true, nil
	}

	text := ""
	if resp.Choices[0].Message.Content != nil && resp.Choices[0].Message.Content.StringValue != nil {
		text = *resp.Choices[0].Message.Content.StringValue
	}
	// 解析模型输出：包含 REPLY 即命中（忽略大小写）
	upper := strings.ToUpper(strings.TrimSpace(text))
	if strings.Contains(upper, "REPLY") {
		return true, nil
	}
	return false, nil
}

// lastUserMessage 返回消息列表中最后一条 role 为 user 的消息内容。
func lastUserMessage(messages []message.ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		role := msg.Role
		if role == "" {
			role = "user"
		}
		if role == "user" {
			return msg.Content
		}
	}
	return ""
}
