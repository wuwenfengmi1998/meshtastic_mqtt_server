package agenttool

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

// LoadOptions contains options for loading a tool
type LoadOptions struct {
	Values map[string]any
}

// Value returns a value from the load options
func (o LoadOptions) Value(key string) any {
	if o.Values == nil {
		return nil
	}
	return o.Values[key]
}

// Frame represents an event frame emitted by a tool
type Frame struct {
	Type    string
	Tool    string
	Stage   string
	Status  string
	Message string
	Data    map[string]any
	Error   string
	Text    string
}

// EmitFunc is a function that emits frames
type EmitFunc func(frame any)

// Runtime provides context for tool execution
type Runtime struct {
	Profile      any
	CompleteText func(context.Context, string, int) (string, error)
	Emit         EmitFunc
	Now          time.Time
}

// LoadedTool is the interface that all tools must implement
type LoadedTool interface {
	Name() string
	Enabled() bool
	ToolDefinition(description string) *model.Tool
	Execute(context.Context, string, Runtime) (string, error)
	RawState() any
}

// Descriptor describes a tool that can be loaded
type Descriptor struct {
	Name string
	Load func(path string, options LoadOptions) (LoadedTool, error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Descriptor{}
)

// Register registers a tool descriptor
func Register(descriptor Descriptor) {
	name := strings.ToLower(strings.TrimSpace(descriptor.Name))
	if name == "" {
		panic("agenttool: tool name is empty")
	}
	if descriptor.Load == nil {
		panic(fmt.Sprintf("agenttool: %s load function is nil", name))
	}
	descriptor.Name = name

	registryMu.Lock()
	defer registryMu.Unlock()
	if _, ok := registry[name]; ok {
		panic(fmt.Sprintf("agenttool: tool %s already registered", name))
	}
	registry[name] = descriptor
}

// Lookup finds a tool descriptor by name
func Lookup(name string) (Descriptor, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	descriptor, ok := registry[strings.ToLower(strings.TrimSpace(name))]
	return descriptor, ok
}

// Names returns all registered tool names
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
