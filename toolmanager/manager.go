package toolmanager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"meshtastic_mqtt_server/agenttool"
)

// Manager manages loaded AI tools
type Manager struct {
	tools map[string]agenttool.LoadedTool
	order []string
}

// Load loads tools from the given directory
func Load(root string, options agenttool.LoadOptions) (*Manager, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		// Directory doesn't exist, create empty manager
		if os.IsNotExist(err) {
			return &Manager{tools: map[string]agenttool.LoadedTool{}}, nil
		}
		return nil, fmt.Errorf("failed to read tools directory: %w", err)
	}

	manager := &Manager{tools: map[string]agenttool.LoadedTool{}}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(entry.Name()))
		descriptor, ok := agenttool.Lookup(name)
		if !ok {
			continue
		}
		tool, err := descriptor.Load(filepath.Join(root, entry.Name()), options)
		if err != nil {
			manager.Close()
			return nil, fmt.Errorf("failed to load tool %s: %w", name, err)
		}
		if tool == nil {
			continue
		}
		toolName := strings.ToLower(strings.TrimSpace(tool.Name()))
		if toolName == "" {
			toolName = name
		}
		if _, ok := manager.tools[toolName]; ok {
			manager.Close()
			return nil, fmt.Errorf("duplicate tool name: %s", toolName)
		}
		manager.tools[toolName] = tool
		manager.order = append(manager.order, toolName)
	}
	return manager, nil
}

// NewForTest creates a manager with preloaded tools for testing
func NewForTest(tools ...agenttool.LoadedTool) *Manager {
	manager := &Manager{tools: map[string]agenttool.LoadedTool{}}
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(tool.Name()))
		if name == "" {
			continue
		}
		if _, ok := manager.tools[name]; !ok {
			manager.order = append(manager.order, name)
		}
		manager.tools[name] = tool
	}
	return manager
}

// Tools returns all loaded tools
func (m *Manager) Tools() []agenttool.LoadedTool {
	if m == nil {
		return nil
	}
	tools := make([]agenttool.LoadedTool, 0, len(m.order))
	for _, name := range m.order {
		if tool := m.tools[name]; tool != nil {
			tools = append(tools, tool)
		}
	}
	return tools
}

// Get returns a tool by name
func (m *Manager) Get(name string) (agenttool.LoadedTool, bool) {
	if m == nil {
		return nil, false
	}
	tool, ok := m.tools[strings.ToLower(strings.TrimSpace(name))]
	return tool, ok
}

// RawState returns the raw state of a tool
func (m *Manager) RawState(name string) (any, bool) {
	tool, ok := m.Get(name)
	if !ok || tool == nil {
		return nil, false
	}
	return tool.RawState(), true
}

// Close closes all tools
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	var errs []string
	for _, tool := range m.Tools() {
		if closer, ok := tool.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
