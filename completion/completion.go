package completion

import (
	"context"
	"fmt"
	"strings"
	"time"

	"meshtastic_mqtt_server/llm"
	"meshtastic_mqtt_server/message"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

// ChatCompleter is a function that completes a chat
type ChatCompleter func(ctx context.Context, profile *llm.Profile, req model.CreateChatCompletionRequest, timeout time.Duration) (model.ChatCompletionResponse, error)

// CompleteChat completes a chat conversation
func CompleteChat(ctx context.Context, profile *llm.Profile, req model.CreateChatCompletionRequest, timeout time.Duration) (model.ChatCompletionResponse, error) {
	if profile == nil || profile.Client == nil {
		return model.ChatCompletionResponse{}, fmt.Errorf("llm profile or client is nil")
	}

	// Use context with timeout if provided
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	resp, err := profile.Client.CreateChatCompletion(ctx, req)
	if err != nil {
		return model.ChatCompletionResponse{}, fmt.Errorf("chat completion failed: %w", err)
	}
	return resp, nil
}

// CompleteText completes a text prompt using conversation messages
// If systemPrompt is not empty, it will be added as the first message
func CompleteText(ctx context.Context, profile *llm.Profile, systemPrompt string, messages []message.ChatMessage, maxTokens int) (string, error) {
	if profile == nil || profile.Client == nil {
		return "", fmt.Errorf("llm profile or client is nil")
	}

	arkMessages := make([]*model.ChatCompletionMessage, 0, len(messages)+1)

	// Add system prompt if provided
	if strings.TrimSpace(systemPrompt) != "" {
		content := &model.ChatCompletionMessageContent{
			StringValue: &systemPrompt,
		}
		arkMessages = append(arkMessages, &model.ChatCompletionMessage{
			Role:    "system",
			Content: content,
		})
	}

	for _, msg := range messages {
		content := &model.ChatCompletionMessageContent{
			StringValue: &msg.Content,
		}
		arkMessages = append(arkMessages, &model.ChatCompletionMessage{
			Role:    msg.Role,
			Content: content,
		})
	}

	req := model.CreateChatCompletionRequest{
		Model:     profile.Config.Model,
		Messages:  arkMessages,
		MaxTokens: &maxTokens,
	}

	resp, err := profile.Client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("text completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no completion choices returned")
	}

	if resp.Choices[0].Message.Content != nil && resp.Choices[0].Message.Content.StringValue != nil {
		return *resp.Choices[0].Message.Content.StringValue, nil
	}
	return "", nil
}

// CompleteTextWithArkMessages completes a text prompt using already converted Ark messages
// This is used when messages have already been converted (e.g. after tool loop)
func CompleteTextWithArkMessages(ctx context.Context, profile *llm.Profile, arkMessages []*model.ChatCompletionMessage, maxTokens int) (string, error) {
	if profile == nil || profile.Client == nil {
		return "", fmt.Errorf("llm profile or client is nil")
	}

	req := model.CreateChatCompletionRequest{
		Model:     profile.Config.Model,
		Messages:  arkMessages,
		MaxTokens: &maxTokens,
	}

	resp, err := profile.Client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("text completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no completion choices returned")
	}

	if resp.Choices[0].Message.Content != nil && resp.Choices[0].Message.Content.StringValue != nil {
		return *resp.Choices[0].Message.Content.StringValue, nil
	}
	return "", nil
}
