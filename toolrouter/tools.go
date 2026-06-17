package toolrouter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"meshtastic_mqtt_server/agenttool"
	"meshtastic_mqtt_server/llm"
	"meshtastic_mqtt_server/stream"
	"meshtastic_mqtt_server/toolmanager"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

// AgentTool represents a tool that can be called by the LLM
type AgentTool struct {
	name       string
	definition *model.Tool
	execute    func(context.Context, string) (string, error)
}

// NewAgentTool creates a new agent tool
func NewAgentTool(name string, definition *model.Tool, execute func(context.Context, string) (string, error)) AgentTool {
	return AgentTool{name: name, definition: definition, execute: execute}
}

// Name returns the tool name
func (t AgentTool) Name() string { return t.name }

// Definition returns the tool definition
func (t AgentTool) Definition() *model.Tool { return t.definition }

// AvailableAgentTools returns all available agent tools
func AvailableAgentTools(state *State, profile *llm.Profile, manager *toolmanager.Manager, emit stream.EmitFunc) []AgentTool {
	return availableAgentTools(state, profile, manager, emit)
}

func availableAgentTools(state *State, profile *llm.Profile, manager *toolmanager.Manager, emit stream.EmitFunc) []AgentTool {
	if state == nil || state.cfg == nil || !state.cfg.Enabled || manager == nil {
		return nil
	}

	loaded := manager.Tools()
	tools := make([]AgentTool, 0, len(loaded))
	for _, tool := range loaded {
		if tool == nil {
			continue
		}
		if agentTool, ok := buildAgentTool(tool, "", profile, emit); ok {
			tools = append(tools, agentTool)
		}
	}
	return tools
}

func buildAgentTool(tool agenttool.LoadedTool, description string, profile *llm.Profile, emit stream.EmitFunc) (AgentTool, bool) {
	if tool == nil || !tool.Enabled() {
		return AgentTool{}, false
	}
	definition := tool.ToolDefinition(description)
	if definition == nil || definition.Function == nil {
		return AgentTool{}, false
	}
	name := tool.Name()
	return AgentTool{
		name:       name,
		definition: definition,
		execute: func(ctx context.Context, args string) (string, error) {
			runtime := agenttool.Runtime{
				Profile: profile,
				Now:     time.Now(),
				Emit:    wrapAgentEmit(emit),
			}
			return tool.Execute(ctx, args, runtime)
		},
	}, true
}

func wrapAgentEmit(emit stream.EmitFunc) agenttool.EmitFunc {
	if emit == nil {
		return nil
	}
	return func(frame any) {
		switch value := frame.(type) {
		case agenttool.Frame:
			emit(stream.Frame{Type: value.Type, Tool: value.Tool, Stage: value.Stage, Status: value.Status, Message: value.Message, Data: value.Data, Error: value.Error, Text: value.Text})
		case stream.Frame:
			emit(value)
		}
	}
}

// ExecuteAgentToolCall executes a tool call and returns the result
func ExecuteAgentToolCall(ctx context.Context, call *model.ToolCall, tools map[string]AgentTool, emit stream.EmitFunc) string {
	if call == nil || call.Type != model.ToolTypeFunction {
		result := "工具调用无效：仅支持 function 类型工具。"
		if emit != nil {
			emit(stream.Frame{Type: "trace", Tool: "agent_tools", Stage: "execute", Status: "error", Message: result})
		}
		return result
	}
	toolName := call.Function.Name
	if emit != nil {
		emit(stream.Frame{Type: "trace", Tool: toolName, Stage: "arguments", Status: "running", Message: "准备执行工具", Data: map[string]any{"tool_call_id": call.ID, "arguments": call.Function.Arguments}})
	}
	tool, ok := tools[toolName]
	if !ok {
		result := fmt.Sprintf("工具调用失败：未知工具 %s。", toolName)
		if emit != nil {
			emit(stream.Frame{Type: "trace", Tool: toolName, Stage: "execute", Status: "error", Message: result})
		}
		return result
	}
	started := time.Now()
	result, err := tool.execute(ctx, call.Function.Arguments)
	durationMs := time.Since(started).Milliseconds()
	if err != nil {
		messageText := fmt.Sprintf("工具 %s 执行失败：%v", tool.name, err)
		if emit != nil {
			emit(stream.Frame{Type: "trace", Tool: tool.name, Stage: "execute", Status: "error", Message: "工具执行失败", Data: map[string]any{"tool_call_id": call.ID, "duration_ms": durationMs, "error": err.Error()}})
		}
		return messageText
	}
	if strings.TrimSpace(result) == "" {
		result = fmt.Sprintf("工具 %s 执行完成，但没有返回内容。", tool.name)
	}
	if emit != nil {
		emit(stream.Frame{Type: "trace", Tool: tool.name, Stage: "result", Status: "success", Message: "工具执行完成", Data: map[string]any{"tool_call_id": call.ID, "duration_ms": durationMs, "result_preview": truncateString(result, 1200)}})
	}
	return result
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
