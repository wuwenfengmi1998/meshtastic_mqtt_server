package sign

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"meshtastic_mqtt_server/internal/agenttool"
	storepkg "meshtastic_mqtt_server/internal/store"
)

// mockSignStore 是用于测试的 mock store
type mockSignStore struct {
	signs       []storepkg.SignRecord
	nodeInfoMap map[string]*storepkg.NodeInfoRecord
}

func (m *mockSignStore) CreateSign(nodeID string, longName, shortName *string, signText string, signTime time.Time) (*storepkg.SignRecord, error) {
	record := storepkg.SignRecord{
		NodeID:    nodeID,
		LongName:  longName,
		ShortName: shortName,
		SignText:  signText,
		SignTime:  signTime,
	}
	m.signs = append(m.signs, record)
	return &record, nil
}

func (m *mockSignStore) HasSignedOnDay(nodeID string, day time.Time) (bool, error) {
	loc := day.Location()
	if loc == nil {
		loc = time.Local
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)

	for _, sign := range m.signs {
		if sign.NodeID == nodeID && sign.SignTime.After(start) && sign.SignTime.Before(end) {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockSignStore) GetNodeInfo(nodeID string) (*storepkg.NodeInfoRecord, error) {
	return m.nodeInfoMap[nodeID], nil
}

func (m *mockSignStore) CountSigns(opts storepkg.ListOptions) (int64, error) {
	count := int64(0)
	for _, sign := range m.signs {
		if opts.Since != nil && sign.SignTime.Before(*opts.Since) {
			continue
		}
		if opts.Until != nil && sign.SignTime.After(*opts.Until) {
			continue
		}
		count++
	}
	return count, nil
}

func (m *mockSignStore) CountSignsByDay(opts storepkg.ListOptions) ([]storepkg.SignDayCount, error) {
	dayCounts := make(map[string]int64)
	for _, sign := range m.signs {
		if opts.Since != nil && sign.SignTime.Before(*opts.Since) {
			continue
		}
		if opts.Until != nil && sign.SignTime.After(*opts.Until) {
			continue
		}
		dateStr := sign.SignTime.Format("2006-01-02")
		dayCounts[dateStr]++
	}

	var result []storepkg.SignDayCount
	for date, count := range dayCounts {
		result = append(result, storepkg.SignDayCount{Date: date, Count: count})
	}
	return result, nil
}

func (m *mockSignStore) ListSigns(opts storepkg.ListOptions) ([]storepkg.SignRecord, error) {
	var result []storepkg.SignRecord
	for _, sign := range m.signs {
		if opts.Since != nil && sign.SignTime.Before(*opts.Since) {
			continue
		}
		if opts.Until != nil && sign.SignTime.After(*opts.Until) {
			continue
		}
		result = append(result, sign)
	}
	return result, nil
}

func TestSignTool_Query(t *testing.T) {
	// 创建 mock store 并添加测试数据
	now := time.Date(2024, 6, 23, 12, 0, 0, 0, time.UTC)
	yesterday := now.AddDate(0, 0, -1)
	twoDaysAgo := now.AddDate(0, 0, -2)

	store := &mockSignStore{
		signs: []storepkg.SignRecord{
			{NodeID: "node1", SignText: "上海-Alice-Device1签到", SignTime: now},
			{NodeID: "node2", SignText: "北京-Bob-Device2签到", SignTime: now},
			{NodeID: "node3", SignText: "深圳-Charlie-Device3签到", SignTime: yesterday},
			{NodeID: "node4", SignText: "广州-David-Device4签到", SignTime: twoDaysAgo},
		},
		nodeInfoMap: make(map[string]*storepkg.NodeInfoRecord),
	}

	tool := &Tool{
		enabled: true,
		store:   store,
	}

	tests := []struct {
		name        string
		action      string
		date        string
		days        int
		expectCount int64
		expectError bool
	}{
		{
			name:        "查询今天",
			action:      "query",
			date:        "2024-06-23",
			days:        0,
			expectCount: 2,
			expectError: false,
		},
		{
			name:        "查询最近3天",
			action:      "query",
			date:        "2024-06-23",
			days:        3,
			expectCount: 4,
			expectError: false,
		},
		{
			name:        "查询昨天",
			action:      "query",
			date:        "2024-06-22",
			days:        0,
			expectCount: 1,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := signParams{
				Action: tt.action,
				Date:   tt.date,
				Days:   tt.days,
			}
			argsJSON, _ := json.Marshal(params)

			runtime := agenttool.Runtime{Now: now}
			result, err := tool.Execute(context.Background(), string(argsJSON), runtime)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectError {
				t.Logf("Query result:\n%s", result)
			}
		})
	}
}

func TestSignTool_SignAction(t *testing.T) {
	now := time.Date(2024, 6, 23, 12, 0, 0, 0, time.UTC)
	store := &mockSignStore{
		signs:       []storepkg.SignRecord{},
		nodeInfoMap: make(map[string]*storepkg.NodeInfoRecord),
	}

	tool := &Tool{
		enabled: true,
		store:   store,
	}

	// 测试签到功能
	params := signParams{
		Action: "sign",
		Region: "上海闵行",
		Name:   "TestUser",
		Device: "TestDevice",
	}
	argsJSON, _ := json.Marshal(params)

	// 创建带节点上下文的 context
	nodeCtx := agenttool.NodeContext{
		NodeID:    "test_node_123",
		LongName:  "Test Node",
		ShortName: "TN",
	}
	ctx := agenttool.WithNodeContext(context.Background(), nodeCtx)

	runtime := agenttool.Runtime{Now: now}
	result, err := tool.Execute(ctx, string(argsJSON), runtime)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	t.Logf("Sign result: %s", result)

	// 验证签到记录已创建
	if len(store.signs) != 1 {
		t.Errorf("Expected 1 sign record, got %d", len(store.signs))
	}
}
