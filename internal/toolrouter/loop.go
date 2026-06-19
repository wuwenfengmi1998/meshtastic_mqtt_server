package toolrouter

import (
	"context"
	"encoding/json"
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
// The third return value toolUsed indicates whether at least one tool was actually
// invoked during the loop (i.e. the model selected a tool). Callers use it to decide
// whether to skip downstream gating (e.g. topic selection).
func RunAgentToolLoop(ctx context.Context, state *State, profile *llm.Profile, systemPrompt string, chatMessages []message.ChatMessage, manager *toolmanager.Manager, emit stream.EmitFunc) ([]*model.ChatCompletionMessage, bool, error) {
	finalMessages, err := buildArkMessages(chatMessages)
	if err != nil {
		return nil, false, err
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
		return finalMessages, false, nil
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
		return finalMessages, false, nil
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
	// toolUsed 记录本轮是否真的执行了至少一次工具调用，供调用方决定是否跳过话题选择等后续门控。
	toolUsed := false
	// 签到意图强制调用：用户明确想签到（「签到/打卡/上台」等）时，模型却不主动调
	// sign 工具的话，会被下游话题判定当成噪音丢弃。这里在循环外预判意图，待模型
	// 该轮未请求任何工具时强制注入一次 sign 调用（用用户原文作为 raw_text），
	// 保证签到一定落库、且不会被话题判定丢弃。
	forceSignText := detectSignIntent(chatMessages)
	_, signAvailable := toolByName["sign"]
	signInvoked := false // 本轮循环中是否已经调用过 sign（含模型主动调与强制调）
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
			return finalMessages, toolUsed, err
		}
		if tracker := stream.TrackerFromContext(ctx); tracker != nil {
			tracker.AddTool(resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
		}
		if len(resp.Choices) == 0 {
			return finalMessages, toolUsed, nil
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
			// 模型本轮未请求任何工具。若检测到签到意图且 sign 工具可用、本次循环尚未调过 sign，
			// 则强制注入一次 sign 调用（以用户原文作为 raw_text），保证签到一定落库。
			if forced := buildForcedSignCall(forceSignText, signAvailable, signInvoked); forced != nil {
				if emit != nil {
					emit(stream.Frame{Type: "trace", Tool: "sign", Stage: "tool_calls", Status: "running", Message: "检测到签到意图，模型未调用签到工具，强制调用 sign", Data: map[string]any{"tools": []string{"sign"}, "forced": true, "iteration": i + 1}})
				}
				calls = []*model.ToolCall{forced}
			} else {
				if emit != nil {
					emit(stream.Frame{Type: "trace", Tool: "agent_tools", Stage: "request", Status: "success", Message: "模型未请求工具，进入回答生成"})
				}
				return finalMessages, toolUsed, nil
			}
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
		// 模型确实请求了工具调用，标记 toolUsed=true
		toolUsed = true
		assistantMessage := &model.ChatCompletionMessage{Role: "assistant", ToolCalls: calls, Content: choice.Message.Content}
		finalMessages = append(finalMessages, assistantMessage)
		decisionMessages = append(decisionMessages, assistantMessage)
		for _, call := range calls {
			if call != nil && call.Function.Name == "sign" {
				signInvoked = true
			}
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
	return finalMessages, toolUsed, nil
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

// signIntentKeywords 是判定签到意图的关键词。命中任一即认为用户想签到。
var signIntentKeywords = []string{"签到", "打卡", "上台"}

// detectSignIntent 取最后一条用户消息，若包含签到意图关键词则返回该消息原文
//（去除前缀的「[来自 ...]」等格式化包装），否则返回空串。
func detectSignIntent(chatMessages []message.ChatMessage) string {
	userText := lastUserMessageText(chatMessages)
	if strings.TrimSpace(userText) == "" {
		return ""
	}
	for _, kw := range signIntentKeywords {
		if strings.Contains(userText, kw) {
			return stripFromPrefix(userText)
		}
	}
	return ""
}

// stripFromPrefix 去掉 autoreply.formatUserMessage 加上的「[来自 ...] 」前缀，
// 让签到正文只保留用户实际发送的内容。
func stripFromPrefix(s string) string {
	if idx := strings.Index(s, "]"); idx >= 0 && strings.HasPrefix(strings.TrimSpace(s), "[") {
		return strings.TrimSpace(s[idx+1:])
	}
	return s
}

// buildForcedSignCall 在满足条件时构造一次强制 sign 调用。条件：
//   - 有签到意图原文（signText 非空）
//   - sign 工具可用
//   - 本次循环尚未调过 sign（避免重复签到）
//
// 调用参数仅含 raw_text（用户原文），由 sign 工具回退为签到正文。
func buildForcedSignCall(signText string, signAvailable, signInvoked bool) *model.ToolCall {
	if strings.TrimSpace(signText) == "" || !signAvailable || signInvoked {
		return nil
	}
	args, _ := json.Marshal(map[string]string{"raw_text": signText})
	argsStr := string(args)
	return &model.ToolCall{
		ID:   "forced_sign",
		Type: model.ToolTypeFunction,
		Function: model.FunctionCall{
			Name:      "sign",
			Arguments: argsStr,
		},
	}
}

// lastUserMessageText 返回消息列表中最后一条 role 为 user 的消息内容。
func lastUserMessageText(messages []message.ChatMessage) string {
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
