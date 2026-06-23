package active

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"meshtastic_mqtt_server/internal/agenttool"
)

// mockActiveStore 是用于测试的 mock store
type mockActiveStore struct {
	activeNodeCount int64
	activeUserCount int64
}

func (m *mockActiveStore) CountActiveNodes(since time.Time) (int64, error) {
	return m.activeNodeCount, nil
}

func (m *mockActiveStore) CountActiveUsers(since time.Time) (int64, error) {
	return m.activeUserCount, nil
}

func TestActiveTool_Query(t *testing.T) {
	now := time.Date(2024, 6, 23, 12, 0, 0, 0, time.UTC)

	store := &mockActiveStore{
		activeNodeCount: 25,
		activeUserCount: 15,
	}

	tool := &Tool{
		enabled: true,
		store:   store,
	}

	tests := []struct {
		name        string
		hours       float64
		queryType   string
		expectNodes bool
		expectUsers bool
		expectError bool
	}{
		{
			name:        "默认查询1小时（both）",
			hours:       0, // 0 表示使用默认值
			queryType:   "",
			expectNodes: true,
			expectUsers: true,
			expectError: false,
		},
		{
			name:        "查询6小时",
			hours:       6,
			queryType:   "both",
			expectNodes: true,
			expectUsers: true,
			expectError: false,
		},
		{
			name:        "仅查询节点",
			hours:       1,
			queryType:   "nodes",
			expectNodes: true,
			expectUsers: false,
			expectError: false,
		},
		{
			name:        "仅查询人数",
			hours:       1,
			queryType:   "users",
			expectNodes: false,
			expectUsers: true,
			expectError: false,
		},
		{
			name:        "查询24小时（最大值）",
			hours:       24,
			queryType:   "both",
			expectNodes: true,
			expectUsers: true,
			expectError: false,
		},
		{
			name:        "超过24小时应限制到24小时",
			hours:       48,
			queryType:   "both",
			expectNodes: true,
			expectUsers: true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := activeParams{
				Hours:     tt.hours,
				QueryType: tt.queryType,
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

				// 验证结果包含预期的内容
				if tt.expectNodes && result != "" {
					// 应该包含节点统计
					if !contains(result, "活跃节点") {
						t.Errorf("Expected result to contain node count")
					}
				}
				if tt.expectUsers && result != "" {
					// 应该包含人数统计
					if !contains(result, "活跃人数") {
						t.Errorf("Expected result to contain user count")
					}
				}
			}
		})
	}
}

func TestActiveTool_Enabled(t *testing.T) {
	// 测试工具启用状态
	tests := []struct {
		name    string
		enabled bool
		store   ActiveStore
		expect  bool
	}{
		{
			name:    "启用且有store",
			enabled: true,
			store:   &mockActiveStore{},
			expect:  true,
		},
		{
			name:    "启用但无store",
			enabled: true,
			store:   nil,
			expect:  false,
		},
		{
			name:    "禁用且有store",
			enabled: false,
			store:   &mockActiveStore{},
			expect:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &Tool{
				enabled: tt.enabled,
				store:   tt.store,
			}
			if tool.Enabled() != tt.expect {
				t.Errorf("Expected Enabled() = %v, got %v", tt.expect, tool.Enabled())
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
