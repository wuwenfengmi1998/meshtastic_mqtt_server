// Package active 提供活跃度查询工具。当用户想查询活跃节点数或活跃人数时调用。
//
// 活跃节点：查询 nodeinfo 表的 updated_at 字段，统计指定时间范围内更新过的节点数
// 活跃人数：查询 text_message 表的 created_at 字段，统计指定时间范围内发过消息的唯一用户数（按 from_id 去重）
package active

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"meshtastic_mqtt_server/internal/agenttool"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

// ActiveStore 定义活跃度查询工具所需的持久化能力，通常由 *store.Store 实现。
type ActiveStore interface {
	CountActiveNodes(since time.Time) (int64, error)
	CountActiveUsers(since time.Time) (int64, error)
}

// Tool 是活跃度查询工具。
type Tool struct {
	enabled bool
	store   ActiveStore
}

// Name returns the tool name
func (t *Tool) Name() string { return "active" }

// Enabled returns whether the tool is enabled
func (t *Tool) Enabled() bool { return t.enabled && t.store != nil }

// ToolDefinition returns the OpenAI tool definition
func (t *Tool) ToolDefinition(description string) *model.Tool {
	desc := "活跃度查询工具。查询指定时间范围内的活跃节点数和活跃人数。\n" +
		"活跃节点：在指定时间内有更新记录的节点数量\n" +
		"活跃人数：在指定时间内发送过消息的唯一用户数（按 from_id 去重）\n" +
		"默认查询最近1小时，最大支持24小时"
	if description != "" {
		desc = description
	}
	return &model.Tool{
		Type: model.ToolTypeFunction,
		Function: &model.FunctionDefinition{
			Name:        "active",
			Description: desc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hours": map[string]any{
						"type":        "number",
						"description": "查询最近多少小时内的活跃数据，默认1小时，最大24小时。例如：1、2、6、12、24",
					},
					"query_type": map[string]any{
						"type":        "string",
						"enum":        []string{"both", "nodes", "users"},
						"description": "查询类型：both=同时查询节点和人数（默认），nodes=仅查询节点，users=仅查询人数",
					},
				},
			},
		},
	}
}

// Execute executes the active query tool
func (t *Tool) Execute(ctx context.Context, args string, runtime agenttool.Runtime) (string, error) {
	if t.store == nil {
		return "", fmt.Errorf("active store is not configured")
	}

	var params activeParams
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// 默认查询1小时
	hours := params.Hours
	if hours <= 0 {
		hours = 1
	}
	// 最大24小时
	if hours > 24 {
		hours = 24
	}

	// 默认查询类型为 both
	queryType := strings.ToLower(strings.TrimSpace(params.QueryType))
	if queryType == "" {
		queryType = "both"
	}

	now := runtime.Now
	if now.IsZero() {
		now = time.Now()
	}

	// 计算时间范围
	since := now.Add(-time.Duration(hours * float64(time.Hour)))

	var result strings.Builder
	result.WriteString(fmt.Sprintf("最近 %.1f 小时的活跃统计：\n\n", hours))

	// 查询活跃节点
	if queryType == "both" || queryType == "nodes" {
		nodeCount, err := t.store.CountActiveNodes(since)
		if err != nil {
			return "", fmt.Errorf("查询活跃节点失败：%w", err)
		}
		result.WriteString(fmt.Sprintf("活跃节点：%d 个\n", nodeCount))
	}

	// 查询活跃人数
	if queryType == "both" || queryType == "users" {
		userCount, err := t.store.CountActiveUsers(since)
		if err != nil {
			return "", fmt.Errorf("查询活跃人数失败：%w", err)
		}
		result.WriteString(fmt.Sprintf("活跃人数：%d 人\n", userCount))
	}

	return result.String(), nil
}

// activeParams 是活跃度查询工具的入参。
type activeParams struct {
	Hours     float64 `json:"hours"`      // 查询最近多少小时，默认1小时
	QueryType string  `json:"query_type"` // 查询类型：both/nodes/users
}

// RawState returns the tool state
func (t *Tool) RawState() any {
	return map[string]any{"enabled": t.enabled, "has_store": t.store != nil}
}

func init() {
	agenttool.Register(agenttool.Descriptor{
		Name: "active",
		Load: func(path string, options agenttool.LoadOptions) (agenttool.LoadedTool, error) {
			tool := &Tool{enabled: true}
			if store, ok := options.Value("store").(ActiveStore); ok && store != nil {
				tool.store = store
			}
			return tool, nil
		},
	})
}
