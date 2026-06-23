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
	CountSigns(opts storepkg.ListOptions) (int64, error)
	CountSignsByDay(opts storepkg.ListOptions) ([]storepkg.SignDayCount, error)
	ListSigns(opts storepkg.ListOptions) ([]storepkg.SignRecord, error)
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
	desc := "签到工具。支持签到和查询两种操作：\n" +
		"1. 签到操作(action=sign)：记录节点今日签到信息，每个节点每天只能签到一次。必填参数：地区、名字、设备\n" +
		"2. 查询操作(action=query)：查询签到统计。可按日期范围查询，默认查询今天。返回签到总数和按天的统计数据"
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
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"sign", "query"},
						"description": "操作类型：sign=签到，query=查询签到统计",
					},
					"region": map[string]any{
						"type":        "string",
						"description": "签到时必填：地区，例如 \"上海闵行\"、\"安徽\"、\"广东深圳\"",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "签到时必填：签到用户的名字/呼号，例如 \"Kevin\"、\"TaoEngine\"",
					},
					"device": map[string]any{
						"type":        "string",
						"description": "签到时必填：使用的设备型号，例如 \"GAT562\"、\"EBYTE_EoRa_S3\"",
					},
					"tx_power": map[string]any{
						"type":        "string",
						"description": "签到时可选：发射功率，例如 \"25mW\"、\"100mW\"",
					},
					"antenna_length": map[string]any{
						"type":        "string",
						"description": "签到时可选：天线长度，例如 \"5dBi\"、\"1.2m\"",
					},
					"altitude": map[string]any{
						"type":        "string",
						"description": "签到时可选：身处高度，例如 \"30m\"、\"海拔500m\"",
					},
					"raw_text": map[string]any{
						"type":        "string",
						"description": "签到时可选：用户原始签到文本。当无法准确拆分地区/名字/设备时，传入用户原文作为签到正文",
					},
					"date": map[string]any{
						"type":        "string",
						"description": "查询时可选：查询日期，格式 YYYY-MM-DD，例如 \"2024-06-23\"。不填则查询今天",
					},
					"days": map[string]any{
						"type":        "integer",
						"description": "查询时可选：查询最近N天的数据，例如 7 表示最近7天。不填则只查询date指定的那一天",
					},
				},
				"required": []string{"action"},
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

	// 根据 action 参数路由到不同的操作
	switch strings.ToLower(strings.TrimSpace(params.Action)) {
	case "query":
		return t.executeQuery(ctx, params, runtime)
	case "sign", "":
		return t.executeSign(ctx, params, runtime)
	default:
		return "", fmt.Errorf("无效的操作类型：%s，只支持 sign 或 query", params.Action)
	}
}

// executeSign 执行签到操作
func (t *Tool) executeSign(ctx context.Context, params signParams, runtime agenttool.Runtime) (string, error) {
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

// executeQuery 执行查询操作
func (t *Tool) executeQuery(ctx context.Context, params signParams, runtime agenttool.Runtime) (string, error) {
	now := runtime.Now
	if now.IsZero() {
		now = time.Now()
	}

	// 解析查询日期
	var targetDate time.Time
	if params.Date != "" {
		var err error
		targetDate, err = time.Parse("2006-01-02", params.Date)
		if err != nil {
			return "", fmt.Errorf("日期格式错误，应为 YYYY-MM-DD：%v", err)
		}
	} else {
		// 默认查询今天
		targetDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}

	// 构建查询选项
	var opts storepkg.ListOptions
	if params.Days > 0 {
		// 查询最近N天
		since := targetDate.AddDate(0, 0, -params.Days+1)
		until := targetDate.Add(24*time.Hour - time.Nanosecond)
		opts.Since = &since
		opts.Until = &until
	} else {
		// 只查询指定的那一天
		since := targetDate
		until := targetDate.Add(24*time.Hour - time.Nanosecond)
		opts.Since = &since
		opts.Until = &until
	}

	// 获取总数
	total, err := t.store.CountSigns(opts)
	if err != nil {
		return "", fmt.Errorf("查询签到总数失败：%w", err)
	}

	// 获取按天统计
	dayCounts, err := t.store.CountSignsByDay(opts)
	if err != nil {
		return "", fmt.Errorf("查询按天统计失败：%w", err)
	}

	// 构建返回消息
	var result strings.Builder
	if params.Days > 0 {
		result.WriteString(fmt.Sprintf("最近 %d 天的签到统计：\n", params.Days))
	} else {
		result.WriteString(fmt.Sprintf("%s 的签到统计：\n", targetDate.Format("2006-01-02")))
	}
	result.WriteString(fmt.Sprintf("总计：%d 人次\n\n", total))

	if len(dayCounts) > 0 {
		result.WriteString("按天统计：\n")
		for _, dc := range dayCounts {
			result.WriteString(fmt.Sprintf("- %s: %d 人\n", dc.Date, dc.Count))
		}
	} else {
		result.WriteString("该时间段内没有签到记录")
	}

	return result.String(), nil
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
	Action        string `json:"action"`         // 操作类型：sign=签到，query=查询
	Region        string `json:"region"`         // 签到时使用
	Name          string `json:"name"`           // 签到时使用
	Device        string `json:"device"`         // 签到时使用
	TxPower       string `json:"tx_power"`       // 签到时使用
	AntennaLength string `json:"antenna_length"` // 签到时使用
	Altitude      string `json:"altitude"`       // 签到时使用
	RawText       string `json:"raw_text"`       // 签到时使用：用户原始签到文本
	Date          string `json:"date"`           // 查询时使用：日期 YYYY-MM-DD
	Days          int    `json:"days"`           // 查询时使用：查询最近N天
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
