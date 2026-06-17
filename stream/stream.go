package stream

import "context"

// Frame represents an event in the stream
type Frame struct {
	Type    string         `json:"type,omitempty"`
	Tool    string         `json:"tool,omitempty"`
	Stage   string         `json:"stage,omitempty"`
	Status  string         `json:"status,omitempty"`
	Message string         `json:"message,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
	Error   string         `json:"error,omitempty"`
	Text    string         `json:"text,omitempty"`
}

// EmitFunc is a function that emits frames
type EmitFunc func(frame Frame)

// ContextKey type for context keys
type contextKey string

const (
	// TrackerContextKey is the key for the stream tracker in context
	TrackerContextKey contextKey = "stream_tracker"
)

// Tracker tracks token usage during streaming
type Tracker struct {
	PromptTokens     int
	CompletionTokens int
	ToolCalls        int
}

// NewTracker creates a new stream tracker
func NewTracker() *Tracker {
	return &Tracker{}
}

// AddTool adds tool call token usage
func (t *Tracker) AddTool(promptTokens, completionTokens int) {
	t.PromptTokens += promptTokens
	t.CompletionTokens += completionTokens
	t.ToolCalls++
}

// TrackerFromContext retrieves the tracker from context
func TrackerFromContext(ctx context.Context) *Tracker {
	if tracker, ok := ctx.Value(TrackerContextKey).(*Tracker); ok {
		return tracker
	}
	return nil
}

// WithTracker adds a tracker to the context
func WithTracker(ctx context.Context, tracker *Tracker) context.Context {
	return context.WithValue(ctx, TrackerContextKey, tracker)
}
