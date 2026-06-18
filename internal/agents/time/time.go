package time

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"meshtastic_mqtt_server/internal/agenttool"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

// Tool is a time tool that can get current time and format dates
type Tool struct {
	enabled bool
}

// Name returns the tool name
func (t *Tool) Name() string {
	return "time"
}

// Enabled returns whether the tool is enabled
func (t *Tool) Enabled() bool {
	return t.enabled
}

// ToolDefinition returns the OpenAI tool definition
func (t *Tool) ToolDefinition(description string) *model.Tool {
	desc := "一个时间工具，可以获取当前时间、格式化日期时间、计算时间差等。当用户询问时间或需要处理日期时间相关问题时使用此工具。"
	if description != "" {
		desc = description
	}
	return &model.Tool{
		Type: model.ToolTypeFunction,
		Function: &model.FunctionDefinition{
			Name:        "time",
			Description: desc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type": "string",
						"enum": []string{"now", "format", "parse"},
						"description": "执行的操作: now(获取当前时间), format(格式化时间), parse(解析时间)",
					},
					"format": map[string]any{
						"type":        "string",
						"description": "时间格式字符串，例如 \"2006-01-02 15:04:05\"",
					},
					"time": map[string]any{
						"type":        "string",
						"description": "要解析的时间字符串",
					},
					"timezone": map[string]any{
						"type":        "string",
						"description": "时区，例如 \"Asia/Shanghai\" 或 \"Local\"",
					},
				},
				"required": []string{"action"},
			},
		},
	}
}

// Execute executes the time tool
func (t *Tool) Execute(ctx context.Context, args string, runtime agenttool.Runtime) (string, error) {
	var params struct {
		Action   string `json:"action"`
		Format   string `json:"format"`
		Time     string `json:"time"`
		Timezone string `json:"timezone"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Get timezone
	loc := time.Local
	if params.Timezone != "" {
		var err error
		loc, err = time.LoadLocation(params.Timezone)
		if err != nil {
			loc = time.Local
		}
	}

	now := time.Now().In(loc)

	switch params.Action {
	case "now":
		formatStr := time.RFC3339
		if params.Format != "" {
			formatStr = params.Format
		}
		return fmt.Sprintf("当前时间: %s", now.Format(formatStr)), nil

	case "format":
		formatStr := time.RFC3339
		if params.Format != "" {
			formatStr = params.Format
		}
		return fmt.Sprintf("格式化时间: %s", now.Format(formatStr)), nil

	case "parse":
		if params.Time == "" {
			return "", fmt.Errorf("time 参数是必需的")
		}
		formatStr := time.RFC3339
		if params.Format != "" {
			formatStr = params.Format
		}
		parsed, err := time.ParseInLocation(formatStr, params.Time, loc)
		if err != nil {
			return fmt.Sprintf("解析时间失败: %v", err), nil
		}
		return fmt.Sprintf("解析结果: %s (Unix 时间戳: %d)", parsed.Format(time.RFC3339), parsed.Unix()), nil

	default:
		return fmt.Sprintf("不支持的操作: %s", params.Action), nil
	}
}

// RawState returns the tool state
func (t *Tool) RawState() any {
	return map[string]any{"enabled": t.enabled}
}

func init() {
	agenttool.Register(agenttool.Descriptor{
		Name: "time",
		Load: func(path string, options agenttool.LoadOptions) (agenttool.LoadedTool, error) {
			return &Tool{enabled: true}, nil
		},
	})
}
