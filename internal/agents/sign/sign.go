// Package sign 提供签到工具。当用户想签到时，把签到信息写入 signs 表，
// 每个节点每天只能签到一次。
//
// 节点身份（node_id / long_name / short_name）由 autoreply 在处理队列消息时
// 通过 ctx 注入（见 agenttool.NodeContext）；text_message 包本身不含名字，
// 队列记录里的 long_name/short_name 经常为空，因此签到工具会在名字缺失时
// 用 node_id 查 nodeinfo 表补全。签到正文里的地区、名字、设备等字段则由
// LLM 从用户消息中提取后作为工具参数传入。
package sign

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"meshtastic_mqtt_server/internal/agenttool"
	storepkg "meshtastic_mqtt_server/internal/store"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

// SignStore 定义签到工具所需的持久化能力，通常由 *store.Store 实现。
type SignStore interface {
	CreateSign(nodeID string, longName, shortName *string, signText string, signTime time.Time) (*storepkg.SignRecord, error)
	HasSignedOnDay(nodeID string, day time.Time) (bool, error)
	GetNodeInfo(nodeID string) (*storepkg.NodeInfoRecord, error)
}

// Tool 是签到工具。
type Tool struct {
	enabled bool
	store   SignStore
}

// Name returns the tool name
func (t *Tool) Name() string { return "sign" }

// Enabled returns whether the tool is enabled
func (t *Tool) Enabled() bool { return t.enabled && t.store != nil }

// ToolDefinition returns the OpenAI tool definition
func (t *Tool) ToolDefinition(description string) *model.Tool {
	desc := "签到工具。当用户想要签到/打卡/上台时调用。会记录该节点今日的签到信息，每个节点每天只能签到一次。" +
		"必填参数：地区、名字、设备；可选参数：发射功率、天线长度、身处高度。"
	if description != "" {
		desc = description
	}
	return &model.Tool{
		Type: model.ToolTypeFunction,
		Function: &model.FunctionDefinition{
			Name:        "sign",
			Description: desc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"region": map[string]any{
						"type":        "string",
						"description": "地区，例如 \"上海闵行\"、\"安徽\"、\"广东深圳\"",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "签到用户的名字/呼号，例如 \"Kevin\"、\"TaoEngine\"",
					},
					"device": map[string]any{
						"type":        "string",
						"description": "使用的设备型号，例如 \"GAT562\"、\"EBYTE_EoRa_S3\"",
					},
					"tx_power": map[string]any{
						"type":        "string",
						"description": "可选：发射功率，例如 \"25mW\"、\"100mW\"",
					},
					"antenna_length": map[string]any{
						"type":        "string",
						"description": "可选：天线长度，例如 \"5dBi\"、\"1.2m\"",
					},
					"altitude": map[string]any{
						"type":        "string",
						"description": "可选：身处高度，例如 \"30m\"、\"海拔500m\"",
					},
					"raw_text": map[string]any{
						"type":        "string",
						"description": "可选：用户原始签到文本。当无法准确拆分地区/名字/设备时，传入用户原文作为签到正文",
					},
				},
				"required": []string{"region", "name", "device"},
			},
		},
	}
}

// Execute executes the sign tool
func (t *Tool) Execute(ctx context.Context, args string, runtime agenttool.Runtime) (string, error) {
	if t.store == nil {
		return "", fmt.Errorf("sign store is not configured")
	}

	var params signParams
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	params.Region = strings.TrimSpace(params.Region)
	params.Name = strings.TrimSpace(params.Name)
	params.Device = strings.TrimSpace(params.Device)
	params.RawText = strings.TrimSpace(params.RawText)
	if params.Region == "" || params.Name == "" || params.Device == "" {
		if params.RawText == "" {
			return "", fmt.Errorf("region, name, device 都是必填参数")
		}
	}

	// 节点身份来自消息上下文，而非 LLM 回填，保证「每节点每天一次」判定可靠。
	node, ok := agenttool.NodeContextFromContext(ctx)
	if !ok || strings.TrimSpace(node.NodeID) == "" {
		return "", fmt.Errorf("缺少发送节点上下文，无法签到")
	}

	now := runtime.Now
	if now.IsZero() {
		now = time.Now()
	}

	// 每个节点每天只能签到一次
	signed, err := t.store.HasSignedOnDay(node.NodeID, now)
	if err != nil {
		return fmt.Sprintf("签到失败：检查今日签到状态时出错：%v", err), nil
	}
	if signed {
		return fmt.Sprintf("%s 今天已经签到过了，每个节点每天只能签到一次。", displayName(node)), nil
	}

	signText := buildSignText(params)
	if signText == "" {
		// 结构化字段缺失时回退到用户原始文本
		signText = params.RawText
	}
	longName, shortName := resolveNodeNames(t, node)

	record, err := t.store.CreateSign(node.NodeID, longName, shortName, signText, now)
	if err != nil {
		return fmt.Sprintf("签到失败：%v", err), nil
	}

	return fmt.Sprintf("签到成功！%s\n签到内容：%s", displayName(node), record.SignText), nil
}

// resolveNodeNames 取出节点的 long_name / short_name。
// text_message 包本身不含名字，队列记录里的 long_name/short_name 经常为空；
// 此时用 node_id 查 nodeinfo 表补全，保证签到记录里能看到节点名。
// 仍查不到则返回两个 nil，签到照常进行（仅名字字段为空）。
func resolveNodeNames(t *Tool, node agenttool.NodeContext) (*string, *string) {
	var longName, shortName *string
	if ln := strings.TrimSpace(node.LongName); ln != "" {
		longName = &ln
	}
	if sn := strings.TrimSpace(node.ShortName); sn != "" {
		shortName = &sn
	}
	// 队列上下文已有名字就直接用，无需查库
	if longName != nil && shortName != nil {
		return longName, shortName
	}
	if t.store == nil {
		return longName, shortName
	}
	info, err := t.store.GetNodeInfo(node.NodeID)
	if err != nil {
		return longName, shortName
	}
	if info == nil {
		return longName, shortName
	}
	if longName == nil && info.LongName != nil {
		if v := strings.TrimSpace(*info.LongName); v != "" {
			longName = &v
		}
	}
	if shortName == nil && info.ShortName != nil {
		if v := strings.TrimSpace(*info.ShortName); v != "" {
			shortName = &v
		}
	}
	return longName, shortName
}

// signParams 是签到工具的入参。
type signParams struct {
	Region        string `json:"region"`
	Name          string `json:"name"`
	Device        string `json:"device"`
	TxPower       string `json:"tx_power"`
	AntennaLength string `json:"antenna_length"`
	Altitude      string `json:"altitude"`
	// RawText 是用户原始签到文本，结构化字段缺失时作为签到正文回退使用。
	RawText string `json:"raw_text"`
}

// buildSignText 按参考格式拼装签到正文：地区-名字-设备签到，可选信息附在括号内。
// 当 region/name/device 任一缺失时返回空串，由调用方回退到 RawText。
func buildSignText(p signParams) string {
	if strings.TrimSpace(p.Region) == "" || strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Device) == "" {
		return ""
	}
	text := fmt.Sprintf("%s-%s-%s签到", p.Region, p.Name, p.Device)

	var extras []string
	if v := strings.TrimSpace(p.TxPower); v != "" {
		extras = append(extras, "发射功率 "+v)
	}
	if v := strings.TrimSpace(p.AntennaLength); v != "" {
		extras = append(extras, "天线 "+v)
	}
	if v := strings.TrimSpace(p.Altitude); v != "" {
		extras = append(extras, "高度 "+v)
	}
	if len(extras) > 0 {
		text += "（" + strings.Join(extras, "，") + "）"
	}
	return text
}

func displayName(node agenttool.NodeContext) string {
	if node.LongName != "" {
		return node.LongName
	}
	if node.ShortName != "" {
		return node.ShortName
	}
	return node.NodeID
}

// RawState returns the tool state
func (t *Tool) RawState() any {
	return map[string]any{"enabled": t.enabled, "has_store": t.store != nil}
}

func init() {
	agenttool.Register(agenttool.Descriptor{
		Name: "sign",
		Load: func(path string, options agenttool.LoadOptions) (agenttool.LoadedTool, error) {
			tool := &Tool{enabled: true}
			if store, ok := options.Value("store").(SignStore); ok && store != nil {
				tool.store = store
			}
			return tool, nil
		},
	})
}
