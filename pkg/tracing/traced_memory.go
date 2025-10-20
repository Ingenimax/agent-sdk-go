package tracing

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// TracedMemory implements middleware for memory operations with unified tracing
type TracedMemory struct {
	memory interfaces.Memory
	tracer interfaces.Tracer
}

// NewTracedMemory creates a new memory middleware with unified tracing
func NewTracedMemory(memory interfaces.Memory, tracer interfaces.Tracer) *TracedMemory {
	return &TracedMemory{
		memory: memory,
		tracer: tracer,
	}
}

// AddMessage adds a message to memory with tracing
func (m *TracedMemory) AddMessage(ctx context.Context, message interfaces.Message) error {
	// Start span
	ctx, span := m.tracer.StartSpan(ctx, "memory.add_message")
	defer span.End()

	// Add attributes
	span.SetAttribute("message.role", string(message.Role))
	span.SetAttribute("message.content_length", len(message.Content))
	span.SetAttribute("message.content_hash", hashString(message.Content))
	if len(message.ToolCalls) > 0 {
		span.SetAttribute("message.tool_calls_count", len(message.ToolCalls))
	}

	// Call the underlying memory
	err := m.memory.AddMessage(ctx, message)
	if err != nil {
		span.RecordError(err)
	}

	return err
}

// GetMessages gets messages from memory with tracing
func (m *TracedMemory) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	// Start span
	ctx, span := m.tracer.StartSpan(ctx, "memory.get_messages")
	defer span.End()

	// Call the underlying memory
	messages, err := m.memory.GetMessages(ctx, options...)
	if err != nil {
		span.RecordError(err)
	} else {
		span.SetAttribute("messages.count", len(messages))
	}

	return messages, err
}

// Clear clears memory with tracing
func (m *TracedMemory) Clear(ctx context.Context) error {
	// Start span
	ctx, span := m.tracer.StartSpan(ctx, "memory.clear")
	defer span.End()

	// Call the underlying memory
	err := m.memory.Clear(ctx)
	if err != nil {
		span.RecordError(err)
	}

	return err
}
