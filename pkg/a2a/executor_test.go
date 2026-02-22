package a2a

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// mockAgent implements AgentAdapter for testing.
type mockAgent struct {
	name        string
	description string
	runResult   string
	runErr      error
	streamEvents []interfaces.AgentStreamEvent
}

func (m *mockAgent) GetName() string        { return m.name }
func (m *mockAgent) GetDescription() string  { return m.description }

func (m *mockAgent) Run(_ context.Context, _ string) (string, error) {
	return m.runResult, m.runErr
}

func (m *mockAgent) RunStream(_ context.Context, _ string) (<-chan interfaces.AgentStreamEvent, error) {
	if m.runErr != nil {
		return nil, m.runErr
	}
	ch := make(chan interfaces.AgentStreamEvent, len(m.streamEvents))
	for _, ev := range m.streamEvents {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

// collectEvents implements a simple queue collector for testing.
type collectEvents struct {
	events []a2a.Event
}

func (q *collectEvents) Write(_ context.Context, event a2a.Event) error {
	q.events = append(q.events, event)
	return nil
}

func (q *collectEvents) WriteVersioned(_ context.Context, _ a2a.Event, _ a2a.TaskVersion) error {
	return nil
}

func (q *collectEvents) Read(_ context.Context) (a2a.Event, a2a.TaskVersion, error) {
	return nil, a2a.TaskVersionMissing, nil
}

func (q *collectEvents) Close() error { return nil }

func TestExecutor_ExecuteSuccess(t *testing.T) {
	agent := &mockAgent{
		name:        "test-agent",
		description: "test agent",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "Hello ", Timestamp: time.Now()},
			{Type: interfaces.AgentEventContent, Content: "world!", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	executor := newAgentExecutor(agent, logging.New())
	queue := &collectEvents{}
	reqCtx := &a2asrv.RequestContext{
		TaskID:    a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
		Message:   a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "hi"}),
	}

	err := executor.Execute(context.Background(), reqCtx, queue)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Expected events: working status, artifact(Hello ), artifact(world!), final artifact, completed status
	if len(queue.events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(queue.events))
	}

	// First event should be working status
	statusEvent, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("expected TaskStatusUpdateEvent, got %T", queue.events[0])
	}
	if statusEvent.Status.State != a2a.TaskStateWorking {
		t.Errorf("expected working state, got %s", statusEvent.Status.State)
	}

	// Last event should be completed status
	lastEvent, ok := queue.events[len(queue.events)-1].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("expected final TaskStatusUpdateEvent, got %T", queue.events[len(queue.events)-1])
	}
	if lastEvent.Status.State != a2a.TaskStateCompleted {
		t.Errorf("expected completed state, got %s", lastEvent.Status.State)
	}
	if !lastEvent.Final {
		t.Error("expected final flag on completed event")
	}
}

func TestExecutor_ExecuteStreamError(t *testing.T) {
	agent := &mockAgent{
		name:   "failing-agent",
		runErr: errors.New("stream init failed"),
	}

	executor := newAgentExecutor(agent, logging.New())
	queue := &collectEvents{}
	reqCtx := &a2asrv.RequestContext{
		TaskID:    a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
		Message:   a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "hi"}),
	}

	err := executor.Execute(context.Background(), reqCtx, queue)
	if err != nil {
		t.Fatalf("Execute should not return error (should write fail event): %v", err)
	}

	// Should have working + failed events
	if len(queue.events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(queue.events))
	}

	lastEvent, ok := queue.events[len(queue.events)-1].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("expected TaskStatusUpdateEvent, got %T", queue.events[len(queue.events)-1])
	}
	if lastEvent.Status.State != a2a.TaskStateFailed {
		t.Errorf("expected failed state, got %s", lastEvent.Status.State)
	}
}

func TestExecutor_ExecuteAgentError(t *testing.T) {
	agent := &mockAgent{
		name: "error-agent",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "partial", Timestamp: time.Now()},
			{Type: interfaces.AgentEventError, Error: errors.New("agent crashed"), Timestamp: time.Now()},
		},
	}

	executor := newAgentExecutor(agent, logging.New())
	queue := &collectEvents{}
	reqCtx := &a2asrv.RequestContext{
		TaskID:    a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
		Message:   a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "hi"}),
	}

	err := executor.Execute(context.Background(), reqCtx, queue)
	if err != nil {
		t.Fatalf("Execute should not return error: %v", err)
	}

	// Last event should be failed
	lastEvent, ok := queue.events[len(queue.events)-1].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("expected TaskStatusUpdateEvent, got %T", queue.events[len(queue.events)-1])
	}
	if lastEvent.Status.State != a2a.TaskStateFailed {
		t.Errorf("expected failed state, got %s", lastEvent.Status.State)
	}
}

func TestExecutor_Cancel(t *testing.T) {
	agent := &mockAgent{name: "cancel-agent"}
	executor := newAgentExecutor(agent, logging.New())
	queue := &collectEvents{}
	reqCtx := &a2asrv.RequestContext{
		TaskID:    a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
	}

	err := executor.Cancel(context.Background(), reqCtx, queue)
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	if len(queue.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(queue.events))
	}

	statusEvent, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("expected TaskStatusUpdateEvent, got %T", queue.events[0])
	}
	if statusEvent.Status.State != a2a.TaskStateCanceled {
		t.Errorf("expected canceled state, got %s", statusEvent.Status.State)
	}
}

func TestExtractTextFromMessage(t *testing.T) {
	msg := a2a.NewMessage(a2a.MessageRoleUser,
		a2a.TextPart{Text: "Hello"},
		a2a.TextPart{Text: "World"},
	)
	text := extractTextFromMessage(msg)
	if text != "Hello\nWorld" {
		t.Errorf("expected 'Hello\\nWorld', got %q", text)
	}

	// nil message
	if extractTextFromMessage(nil) != "" {
		t.Error("expected empty string for nil message")
	}
}
