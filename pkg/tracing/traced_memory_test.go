package tracing

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// noopTracer satisfies interfaces.Tracer without emitting anything, so these
// tests exercise decorator behaviour rather than tracing behaviour.
type noopTracer struct{}

func (noopTracer) StartSpan(ctx context.Context, _ string) (context.Context, interfaces.Span) {
	return ctx, &NoOpSpan{}
}

func (noopTracer) StartTraceSession(ctx context.Context, _ string) (context.Context, interfaces.Span) {
	return ctx, &NoOpSpan{}
}

// conversationCapableMemory implements the optional ConversationMemory
// interface, standing in for RedisMemory in these tests.
type conversationCapableMemory struct {
	conversations []string
}

func (m *conversationCapableMemory) AddMessage(context.Context, interfaces.Message) error {
	return nil
}

func (m *conversationCapableMemory) GetMessages(context.Context, ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return nil, nil
}

func (m *conversationCapableMemory) Clear(context.Context) error { return nil }

func (m *conversationCapableMemory) GetAllConversations(context.Context) ([]string, error) {
	return m.conversations, nil
}

func (m *conversationCapableMemory) GetConversationMessages(context.Context, string) ([]interfaces.Message, error) {
	return []interfaces.Message{{Role: interfaces.MessageRoleUser, Content: "hello"}}, nil
}

func (m *conversationCapableMemory) GetMemoryStatistics(context.Context) (int, int, error) {
	return len(m.conversations), 1, nil
}

// plainMemory implements only the three core Memory methods.
type plainMemory struct{}

func (plainMemory) AddMessage(context.Context, interfaces.Message) error { return nil }
func (plainMemory) GetMessages(context.Context, ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return nil, nil
}
func (plainMemory) Clear(context.Context) error { return nil }

// TestTracedMemory_PreservesConversationCapability guards a live bug.
//
// Agent.GetAllConversations, GetConversationMessages and GetMemoryStatistics
// each did a bare a.memory.(interfaces.ConversationMemory) assertion.
// TracedMemory implements only the three core Memory methods, so wrapping a
// RedisMemory in tracing made that assertion miss and the three accessors
// return empty results -- no error, no warning, just silently no data.
func TestTracedMemory_PreservesConversationCapability(t *testing.T) {
	inner := &conversationCapableMemory{conversations: []string{"conv-a", "conv-b"}}
	traced := NewTracedMemory(inner, noopTracer{})

	convMem, ok := interfaces.AsConversationMemory(traced)
	if !ok {
		t.Fatal("ConversationMemory capability was lost through the tracing decorator; " +
			"Agent.GetAllConversations and friends would silently return empty")
	}

	got, err := convMem.GetAllConversations(context.Background())
	if err != nil {
		t.Fatalf("GetAllConversations() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("GetAllConversations() returned %d conversations, want 2", len(got))
	}
}

// TestTracedMemory_BareAssertionStillMisses documents precisely why the helper
// is needed: the assertion the agent used to perform genuinely does not see
// through the decorator. If this ever starts passing, TracedMemory has grown
// the optional methods itself and the helper indirection could be revisited.
func TestTracedMemory_BareAssertionStillMisses(t *testing.T) {
	inner := &conversationCapableMemory{conversations: []string{"conv-a"}}
	var traced interfaces.Memory = NewTracedMemory(inner, noopTracer{})

	if _, ok := traced.(interfaces.ConversationMemory); ok {
		t.Skip("TracedMemory now implements ConversationMemory directly")
	}

	if _, ok := interfaces.AsConversationMemory(traced); !ok {
		t.Error("AsConversationMemory must see through the decorator that the bare assertion misses")
	}
}

// TestAsConversationMemory_PlainMemoryReportsFalse asserts the helper does not
// manufacture a capability that is genuinely absent.
func TestAsConversationMemory_PlainMemoryReportsFalse(t *testing.T) {
	traced := NewTracedMemory(plainMemory{}, noopTracer{})

	if _, ok := interfaces.AsConversationMemory(traced); ok {
		t.Error("AsConversationMemory reported a capability the wrapped memory does not have")
	}
}

// TestUnwrapMemory_WalksNestedDecorators asserts the walk handles more than one
// layer, since nothing prevents a caller stacking decorators.
func TestUnwrapMemory_WalksNestedDecorators(t *testing.T) {
	inner := &conversationCapableMemory{conversations: []string{"conv-a"}}
	doubled := NewTracedMemory(NewTracedMemory(inner, noopTracer{}), noopTracer{})

	if got := interfaces.UnwrapMemory(doubled); got != interfaces.Memory(inner) {
		t.Errorf("UnwrapMemory() = %T, want the innermost *conversationCapableMemory", got)
	}

	if _, ok := interfaces.AsConversationMemory(doubled); !ok {
		t.Error("capability must survive two layers of decoration")
	}
}
