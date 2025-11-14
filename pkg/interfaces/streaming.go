package interfaces

import (
	"context"
	"time"
)

// StreamEventType represents the type of streaming event
type StreamEventType string

const (
	// Core streaming events
	StreamEventMessageStart    StreamEventType = "message_start"
	StreamEventContentDelta    StreamEventType = "content_delta"
	StreamEventContentComplete StreamEventType = "content_complete"
	StreamEventMessageStop     StreamEventType = "message_stop"
	StreamEventError           StreamEventType = "error"

	// Tool-related events
	StreamEventToolUse    StreamEventType = "tool_use"
	StreamEventToolResult StreamEventType = "tool_result"

	// Thinking/reasoning events
	StreamEventThinking StreamEventType = "thinking"
)

// StreamEvent represents a single event in a stream
type StreamEvent struct {
	Type      StreamEventType        `json:"type"`
	Content   string                 `json:"content,omitempty"`
	ToolCall  *ToolCall              `json:"tool_call,omitempty"`
	Error     error                  `json:"error,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// StreamingLLM extends LLM with streaming capabilities
type StreamingLLM interface {
	LLM

	// GenerateStream generates text with streaming response
	GenerateStream(ctx context.Context, prompt string, options ...GenerateOption) (<-chan StreamEvent, error)

	// GenerateWithToolsStream generates text with tools and streaming response
	GenerateWithToolsStream(ctx context.Context, prompt string, tools []Tool, options ...GenerateOption) (<-chan StreamEvent, error)
}

// StreamingAgent represents an agent with streaming capabilities
type StreamingAgent interface {
	// RunStream executes the agent with streaming response
	RunStream(ctx context.Context, input string) (<-chan AgentStreamEvent, error)
}

// AgentStreamEvent represents a streaming event from an agent
type AgentStreamEvent struct {
	Type         AgentEventType         `json:"type"`
	Content      string                 `json:"content,omitempty"`
	ToolCall     *ToolCallEvent         `json:"tool_call,omitempty"`
	ThinkingStep string                 `json:"thinking_step,omitempty"`
	Error        error                  `json:"error,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	Timestamp    time.Time              `json:"timestamp"`
}

// AgentEventType represents the type of agent streaming event
type AgentEventType string

const (
	AgentEventContent    AgentEventType = "content"
	AgentEventThinking   AgentEventType = "thinking"
	AgentEventToolCall   AgentEventType = "tool_call"
	AgentEventToolResult AgentEventType = "tool_result"
	AgentEventError      AgentEventType = "error"
	AgentEventComplete   AgentEventType = "complete"
)

// ToolCallEvent represents a tool call in streaming context
type ToolCallEvent struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Internal    bool   `json:"internal,omitempty"`
	Arguments   string `json:"arguments,omitempty"`
	Result      string `json:"result,omitempty"`
	Status      string `json:"status"` // "starting", "executing", "completed", "error"
}

// StreamConfig contains configuration for streaming behavior
type StreamConfig struct {
	// BufferSize determines the channel buffer size
	BufferSize int

	// IncludeThinking whether to include thinking events
	IncludeThinking bool

	// IncludeToolProgress whether to include tool execution progress
	IncludeToolProgress bool

	// IncludeIntermediateMessages whether to include intermediate messages between tool iterations
	IncludeIntermediateMessages bool
}

// DefaultStreamConfig returns default streaming configuration
func DefaultStreamConfig() StreamConfig {
	return StreamConfig{
		BufferSize:                  100,
		IncludeThinking:             true,
		IncludeToolProgress:         true,
		IncludeIntermediateMessages: false,
	}
}

// WithIncludeIntermediateMessages returns a StreamConfig option to include intermediate messages
func WithIncludeIntermediateMessages(include bool) func(*StreamConfig) {
	return func(cfg *StreamConfig) {
		cfg.IncludeIntermediateMessages = include
	}
}

// StreamForwarder is a function that forwards stream events to a parent stream
// This is used to enable nested streaming from sub-agents to parent agents
type StreamForwarder func(event AgentStreamEvent)

// streamForwarderContextKey is the context key for storing stream forwarders
type streamForwarderContextKey struct{}

// StreamForwarderKey is the exported context key for stream forwarders
var StreamForwarderKey = streamForwarderContextKey{}
