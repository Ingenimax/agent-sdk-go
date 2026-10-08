package microservice

import (
	"context"
	"errors"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// scopeRecordingMemory records the context it was read with, so a test can tell
// whether the request scope actually reached the memory layer.
type scopeRecordingMemory struct {
	messages []interfaces.Message
	gotOrg   string
	gotConv  string
	readErr  error
	reads    int
}

func (m *scopeRecordingMemory) AddMessage(context.Context, interfaces.Message) error { return nil }

func (m *scopeRecordingMemory) GetMessages(ctx context.Context, _ ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	m.reads++
	// Mirror the real implementations: both IDs are required.
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return nil, errors.New("memory: org ID missing from context")
	}
	convID, ok := memory.GetConversationID(ctx)
	if !ok {
		return nil, errors.New("memory: conversation ID missing from context")
	}
	m.gotOrg, m.gotConv = orgID, convID
	if m.readErr != nil {
		return nil, m.readErr
	}
	return m.messages, nil
}

func (m *scopeRecordingMemory) Clear(context.Context) error { return nil }

func scopedContext() context.Context {
	ctx := multitenancy.WithOrgID(context.Background(), "org-7")
	return memory.WithConversationID(ctx, "conv-42")
}

// TestGetMemoryFromAgentUsesRequestScope is the core of #330. getMemoryFromAgent
// built its own context.Background(), which no Memory implementation can satisfy
// because they all require an org and conversation ID. The durable read
// therefore always failed and the in-process buffer was always served in its
// place, indistinguishably.
func TestGetMemoryFromAgentUsesRequestScope(t *testing.T) {
	mem := &scopeRecordingMemory{messages: []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Content: "durable question"},
		{Role: interfaces.MessageRoleAssistant, Content: "durable answer"},
	}}

	server := &HTTPServerWithUI{conversationHistory: []MemoryEntry{
		{Role: "user", Content: "local-only entry"},
	}}

	entries, durable := server.getMemoryFromAgent(scopedContext(), mem, 10, 0)

	if mem.reads != 1 {
		t.Fatalf("memory read %d times, want 1", mem.reads)
	}
	if mem.gotOrg != "org-7" || mem.gotConv != "conv-42" {
		t.Errorf("memory saw org=%q conv=%q, want org-7/conv-42: the request scope did not reach it",
			mem.gotOrg, mem.gotConv)
	}
	if !durable {
		t.Error("durable = false, want true: a successful read must not be reported as local")
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 from durable memory", len(entries))
	}
	for _, entry := range entries {
		if entry.Content == "local-only entry" {
			t.Fatal("served the in-process buffer despite durable memory succeeding")
		}
	}
}

// TestGetMemoryFromAgentReportsLocalFallback keeps the fallback usable but
// honest: a failed durable read still returns something, and says it is local.
func TestGetMemoryFromAgentReportsLocalFallback(t *testing.T) {
	mem := &scopeRecordingMemory{readErr: errors.New("redis unreachable")}
	server := &HTTPServerWithUI{conversationHistory: []MemoryEntry{
		{Role: "user", Content: "local-only entry"},
	}}

	entries, durable := server.getMemoryFromAgent(scopedContext(), mem, 10, 0)

	if durable {
		t.Error("durable = true, want false: the durable read failed")
	}
	if len(entries) != 1 || entries[0].Content != "local-only entry" {
		t.Errorf("entries = %+v, want the local buffer", entries)
	}
}

// TestGetMemoryFromAgentUnscopedContextIsNotDurable pins the old behaviour as a
// non-durable answer rather than a silent success.
func TestGetMemoryFromAgentUnscopedContextIsNotDurable(t *testing.T) {
	mem := &scopeRecordingMemory{messages: []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Content: "durable question"},
	}}
	server := &HTTPServerWithUI{conversationHistory: []MemoryEntry{
		{Role: "user", Content: "local-only entry"},
	}}

	// context.Background() is what the function used to build for itself.
	entries, durable := server.getMemoryFromAgent(context.Background(), mem, 10, 0)

	if durable {
		t.Error("durable = true for an unscoped context, want false")
	}
	if len(entries) != 1 || entries[0].Content != "local-only entry" {
		t.Errorf("entries = %+v, want the local buffer", entries)
	}
}

// TestWithSourceLabelsResponses covers the wire-level signal that lets an
// operator tell a durable answer from a best-effort one.
func TestWithSourceLabelsResponses(t *testing.T) {
	if got := withSource(MemoryResponse{Mode: "messages"}, memorySourceDurable); got.Source != "memory" {
		t.Errorf("Source = %q, want memory", got.Source)
	}
	if got := withSource(MemoryResponse{Mode: "conversations"}, memorySourceLocal); got.Source != "local" {
		t.Errorf("Source = %q, want local", got.Source)
	}
	// Tagging must not disturb the payload.
	original := MemoryResponse{Mode: "messages", Total: 3, Limit: 10, ConversationID: "c1"}
	tagged := withSource(original, memorySourceDurable)
	if tagged.Mode != "messages" || tagged.Total != 3 || tagged.Limit != 10 || tagged.ConversationID != "c1" {
		t.Errorf("tagging altered the response: %+v", tagged)
	}
}
