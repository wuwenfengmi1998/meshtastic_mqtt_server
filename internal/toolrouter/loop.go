package toolrouter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"meshtastic_mqtt_server/internal/completion"
	"meshtastic_mqtt_server/internal/llm"
	"meshtastic_mqtt_server/internal/message"
	"meshtastic_mqtt_server/internal/stream"
	"meshtastic_mqtt_server/internal/toolmanager"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

const maxAgentToolIterations = 6

// RunAgentToolLoop runs the agent tool calling loop
// systemPrompt is the primary system prompt from LLM config
func RunAgentToolLoop(ctx context.Context, state *State, profile *llm.Profile, systemPrompt string, chatMessages []message.ChatMessage, manager *toolmanager.Manager, emit stream.EmitFunc) ([]*model.ChatCompletionMessage, error) {
	finalMessages, err := buildArkMessages(chatMessages)
	if err != nil {
		return nil, err
	}
	routerProfile := profile
	if state != nil {
		routerProfile = state.RouterProfile(profile)
	}
	tools := availableAgentTools(state, routerProfile, manager, emit)
	if len(tools) == 0 {
		// No tools available, add system prompt and return
		if strings.TrimSpace(systemPrompt) != "" {
			systemMessage := &model.ChatCompletionMessage{
				Role: "system",
				Content: &model.ChatCompletionMessageContent{
					StringValue: &systemPrompt,
				},
			}
			finalMessages = append([]*model.ChatCompletionMessage{systemMessage}, finalMessages...)
		}
		return finalMessages, nil
	}

	decisionMessages := append([]*model.ChatCompletionMessage(nil), finalMessages...)

	toolByName := make(map[string]AgentTool, len(tools))
	definitions := make([]*model.Tool, 0, len(tools))
	availableNames := make([]string, 0, len(tools))
	toolDescriptions := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolByName[tool.name] = tool
		definitions = append(definitions, tool.definition)
		availableNames = append(availableNames, tool.name)
		if tool.definition != nil && tool.definition.Function != nil {
			toolDescriptions = append(toolDescriptions, fmt.Sprintf("%s: %s", tool.name, tool.definition.Function.Description))
		}
	}
	if emit != nil {
		emit(stream.Frame{Type: "trace", Tool: "agent_tools", Stage: "prepare", Status: "success", Message: "已准备可用工具", Data: map[string]any{"tools": availableNames, "tool_descriptions": toolDescriptions}})
	}
	if state == nil {
		// No tool router state, but we have tools - use primary system prompt
		if strings.TrimSpace(systemPrompt) != "" {
			systemMessage := &model.ChatCompletionMessage{
				Role: "system",
				Content: &model.ChatCompletionMessageContent{
					StringValue: &systemPrompt,
				},
			}
			finalMessages = append([]*model.ChatCompletionMessage{systemMessage}, finalMessages...)
			decisionMessages = append([]*model.ChatCompletionMessage{systemMessage}, decisionMessages...)
		}
		return finalMessages, nil
	}
	// 每轮调用都重新加载最新配置，确保管理员在 /admin/llm/api 保存后立即生效
	cfg := state.effectiveConfig()
	// 最终回复使用主回复配置的 system prompt（机器人人设/回复指引）；
	// 工具路由决策使用工具路由的 system prompt（指导如何调用工具），
	// 为空时回退到主回复 prompt。两者分离，避免工具路由 prompt 覆盖主回复 prompt。
	primaryPrompt := strings.TrimSpace(systemPrompt)
	routerPrompt := strings.TrimSpace(cfg.SystemPrompt)
	if routerPrompt == "" {
		routerPrompt = primaryPrompt
	}
	if primaryPrompt != "" {
		primarySystemMessage := &model.ChatCompletionMessage{
			Role: "system",
			Content: &model.ChatCompletionMessageContent{
				StringValue: &primaryPrompt,
			},
		}
		finalMessages = append([]*model.ChatCompletionMessage{primarySystemMessage}, finalMessages...)
	}
	if routerPrompt != "" {
		routerSystemMessage := &model.ChatCompletionMessage{
			Role: "system",
			Content: &model.ChatCompletionMessageContent{
				StringValue: &routerPrompt,
			},
		}
		decisionMessages = append([]*model.ChatCompletionMessage{routerSystemMessage}, decisionMessages...)
	}
	for i := 0; i < maxAgentToolIterations; i++ {
		if emit != nil {
			emit(stream.Frame{Type: "trace", Tool: "agent_tools", Stage: "request", Status: "running", Message: fmt.Sprintf("正在进行第 %d 轮工具判断", i+1), Data: map[string]any{"iteration": i + 1, "max_iterations": maxAgentToolIterations, "tools": availableNames}})
		}
		resp, err := completion.CompleteChat(ctx, routerProfile, model.CreateChatCompletionRequest{
			Model:             routerProfile.Config.Model,
			Messages:          decisionMessages,
			MaxTokens:         &cfg.MaxTokens,
			Tools:             definitions,
			ToolChoice:        model.ToolChoiceStringTypeAuto,
			ParallelToolCalls: BoolPtr(false),
		}, time.Duration(cfg.Timeout)*time.Second)
		if err != nil {
			return finalMessages, err
		}
		if tracker := stream.TrackerFromContext(ctx); tracker != nil {
			tracker.AddTool(resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
		}
		if len(resp.Choices) == 0 {
			return finalMessages, nil
		}
		choice := resp.Choices[0]
		if emit != nil {
			emit(stream.Frame{Type: "trace", Tool: "agent_tools", Stage: "decision", Status: "success", Message: "工具判断响应已返回", Data: map[string]any{"iteration": i + 1}})
		}
		calls := choice.Message.ToolCalls
		if len(calls) == 0 && choice.Message.FunctionCall != nil {
			calls = []*model.ToolCall{{ID: "legacy_function_call", Type: model.ToolTypeFunction, Function: *choice.Message.FunctionCall}}
		}
		if len(calls) == 0 {
			if emit != nil {
				emit(stream.Frame{Type: "trace", Tool: "agent_tools", Stage: "request", Status: "success", Message: "模型未请求工具，进入回答生成"})
			}
			return finalMessages, nil
		}
		callNames := make([]string, 0, len(calls))
		for _, call := range calls {
			if call != nil {
				callNames = append(callNames, call.Function.Name)
			}
		}
		if emit != nil {
			emit(stream.Frame{Type: "trace", Tool: "agent_tools", Stage: "tool_calls", Status: "running", Message: fmt.Sprintf("模型请求调用 %d 个工具", len(calls)), Data: map[string]any{"tools": callNames, "iteration": i + 1}})
		}
		assistantMessage := &model.ChatCompletionMessage{Role: "assistant", ToolCalls: calls, Content: choice.Message.Content}
		finalMessages = append(finalMessages, assistantMessage)
		decisionMessages = append(decisionMessages, assistantMessage)
		for _, call := range calls {
			result := ExecuteAgentToolCall(ctx, call, toolByName, emit)
			resultContent := &model.ChatCompletionMessageContent{StringValue: &result}
			toolMessage := &model.ChatCompletionMessage{Role: "tool", ToolCallID: call.ID, Content: resultContent}
			finalMessages = append(finalMessages, toolMessage)
			decisionMessages = append(decisionMessages, toolMessage)
		}
	}
	limitText := "工具调用轮数已达到上限。请基于已有工具结果回答，并说明可能未完成全部工具调用。"
	limitMessage := &model.ChatCompletionMessage{Role: "system", Content: &model.ChatCompletionMessageContent{StringValue: &limitText}}
	finalMessages = append(finalMessages, limitMessage)
	return finalMessages, nil
}

func buildArkMessages(chatMessages []message.ChatMessage) ([]*model.ChatCompletionMessage, error) {
	messages := make([]*model.ChatCompletionMessage, 0, len(chatMessages))
	for _, msg := range chatMessages {
		role := msg.Role
		if role == "" {
			role = "user"
		}
		content := &model.ChatCompletionMessageContent{StringValue: &msg.Content}
		messages = append(messages, &model.ChatCompletionMessage{
			Role:    role,
			Content: content,
		})
	}
	return messages, nil
}

// BoolPtr returns a pointer to the given bool
func BoolPtr(b bool) *bool {
	return &b
}

// IntPtr returns a pointer to the given int
func IntPtr(i int) *int {
	return &i
}
