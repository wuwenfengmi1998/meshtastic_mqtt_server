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

// NodeContext carries the originating node identity for a tool execution.
// 它由 autoreply 在处理一条队列消息时注入到 ctx 中，供需要识别发送节点的
// 工具（如签到工具）使用，避免依赖 LLM 从文本里回填节点 ID。
type NodeContext struct {
	NodeID    string
	LongName  string
	ShortName string
}

type nodeContextKey struct{}

// WithNodeContext 把节点身份信息挂到 ctx 上。
func WithNodeContext(ctx context.Context, nc NodeContext) context.Context {
	return context.WithValue(ctx, nodeContextKey{}, nc)
}

// NodeContextFromContext 从 ctx 中取出节点身份信息；不存在时第二个返回值为 false。
func NodeContextFromContext(ctx context.Context) (NodeContext, bool) {
	nc, ok := ctx.Value(nodeContextKey{}).(NodeContext)
	return nc, ok
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
