package a2a

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// mockAgent implements AgentAdapter for testing.
type mockAgent struct {
	name         string
	description  string
	runResult    string
	runErr       error
	streamEvents []interfaces.AgentStreamEvent
	streamDelay  time.Duration
}

func (m *mockAgent) GetName() string       { return m.name }
func (m *mockAgent) GetDescription() string { return m.description }

func (m *mockAgent) Run(_ context.Context, _ string) (string, error) {
	return m.runResult, m.runErr
}

func (m *mockAgent) RunStream(ctx context.Context, _ string) (<-chan interfaces.AgentStreamEvent, error) {
	if m.runErr != nil {
		return nil, m.runErr
	}
	ch := make(chan interfaces.AgentStreamEvent, len(m.streamEvents))
	go func() {
		defer close(ch)
		for _, ev := range m.streamEvents {
			if m.streamDelay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(m.streamDelay):
				}
			}
			select {
			case <-ctx.Done():
				return
			case ch <- ev:
			}
		}
	}()
	return ch, nil
}

// collectEvents implements a simple queue collector for testing.
type collectEvents struct {
	mu     sync.Mutex
	events []a2a.Event
}

func (q *collectEvents) Write(_ context.Context, event a2a.Event) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.events = append(q.events, event)
	return nil
}

func (q *collectEvents) WriteVersioned(_ context.Context, event a2a.Event, _ a2a.TaskVersion) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.events = append(q.events, event)
	return nil
}

func (q *collectEvents) Read(_ context.Context) (a2a.Event, a2a.TaskVersion, error) {
	return nil, a2a.TaskVersionMissing, nil
}

func (q *collectEvents) Close() error { return nil }

func (q *collectEvents) getEvents() []a2a.Event {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]a2a.Event, len(q.events))
	copy(out, q.events)
	return out
}

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

	events := queue.getEvents()

	// Expected events: working status, artifact(Hello ), artifact(world!), final artifact, completed status
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	// First event should be working status
	statusEvent, ok := events[0].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("expected TaskStatusUpdateEvent, got %T", events[0])
	}
	if statusEvent.Status.State != a2a.TaskStateWorking {
		t.Errorf("expected working state, got %s", statusEvent.Status.State)
	}

	// Last event should be completed status
	lastEvent, ok := events[len(events)-1].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("expected final TaskStatusUpdateEvent, got %T", events[len(events)-1])
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

	events := queue.getEvents()

	// Should have working + failed events
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	lastEvent, ok := events[len(events)-1].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("expected TaskStatusUpdateEvent, got %T", events[len(events)-1])
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

	events := queue.getEvents()

	// Last event should be failed
	lastEvent, ok := events[len(events)-1].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("expected TaskStatusUpdateEvent, got %T", events[len(events)-1])
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

	events := queue.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	statusEvent, ok := events[0].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("expected TaskStatusUpdateEvent, got %T", events[0])
	}
	if statusEvent.Status.State != a2a.TaskStateCanceled {
		t.Errorf("expected canceled state, got %s", statusEvent.Status.State)
	}
}

func TestExecutor_CancelStopsRunningAgent(t *testing.T) {
	// Agent that streams slowly so we can cancel mid-execution
	agent := &mockAgent{
		name: "slow-agent",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "chunk1", Timestamp: time.Now()},
			{Type: interfaces.AgentEventContent, Content: "chunk2", Timestamp: time.Now()},
			{Type: interfaces.AgentEventContent, Content: "chunk3", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
		streamDelay: 200 * time.Millisecond,
	}

	executor := newAgentExecutor(agent, logging.New())
	taskID := a2a.NewTaskID()

	execQueue := &collectEvents{}
	execReqCtx := &a2asrv.RequestContext{
		TaskID:    taskID,
		ContextID: a2a.NewContextID(),
		Message:   a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "slow request"}),
	}

	// Start execution in background
	execDone := make(chan error, 1)
	go func() {
		execDone <- executor.Execute(context.Background(), execReqCtx, execQueue)
	}()

	// Wait for execution to register its cancel function
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := executor.cancels.Load(taskID); ok {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Cancel the task
	cancelQueue := &collectEvents{}
	cancelReqCtx := &a2asrv.RequestContext{
		TaskID:    taskID,
		ContextID: a2a.NewContextID(),
	}

	err := executor.Cancel(context.Background(), cancelReqCtx, cancelQueue)
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	// Wait for execution to finish (should be fast due to cancellation)
	select {
	case <-execDone:
		// Execution completed (context was canceled, agent stopped)
	case <-time.After(3 * time.Second):
		t.Fatal("execution did not stop after cancellation")
	}

	// Verify cancel event was written
	cancelEvents := cancelQueue.getEvents()
	if len(cancelEvents) == 0 {
		t.Fatal("expected cancel event")
	}
	statusEvent, ok := cancelEvents[0].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("expected TaskStatusUpdateEvent, got %T", cancelEvents[0])
	}
	if statusEvent.Status.State != a2a.TaskStateCanceled {
		t.Errorf("expected canceled state, got %s", statusEvent.Status.State)
	}

	// Verify we got fewer events than a full run (cancellation worked)
	execEvents := execQueue.getEvents()
	contentEvents := 0
	for _, ev := range execEvents {
		if _, ok := ev.(*a2a.TaskArtifactUpdateEvent); ok {
			contentEvents++
		}
	}
	if contentEvents >= 3 {
		t.Errorf("expected fewer than 3 content events (cancellation should have stopped early), got %d", contentEvents)
	}
}

func TestExecutor_ToolCallEvents(t *testing.T) {
	agent := &mockAgent{
		name: "tool-agent",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventToolCall, ToolCall: &interfaces.ToolCallEvent{
				Name:   "web_search",
				Status: "starting",
			}, Timestamp: time.Now()},
			{Type: interfaces.AgentEventToolResult, ToolCall: &interfaces.ToolCallEvent{
				Name:   "web_search",
				Result: "search results here",
				Status: "completed",
			}, Timestamp: time.Now()},
			{Type: interfaces.AgentEventContent, Content: "Based on search results...", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	executor := newAgentExecutor(agent, logging.New())
	queue := &collectEvents{}
	reqCtx := &a2asrv.RequestContext{
		TaskID:    a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
		Message:   a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "search something"}),
	}

	err := executor.Execute(context.Background(), reqCtx, queue)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	events := queue.getEvents()

	// Count status events with working state (should have initial + tool call + tool result)
	workingStatuses := 0
	for _, ev := range events {
		if se, ok := ev.(*a2a.TaskStatusUpdateEvent); ok && se.Status.State == a2a.TaskStateWorking {
			workingStatuses++
		}
	}
	if workingStatuses < 3 {
		t.Errorf("expected at least 3 working status events (initial + tool call + tool result), got %d", workingStatuses)
	}
}

func TestExecutor_ThinkingEvents(t *testing.T) {
	agent := &mockAgent{
		name: "thinking-agent",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventThinking, ThinkingStep: "Let me think about this...", Timestamp: time.Now()},
			{Type: interfaces.AgentEventContent, Content: "My answer is 42.", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	executor := newAgentExecutor(agent, logging.New())
	queue := &collectEvents{}
	reqCtx := &a2asrv.RequestContext{
		TaskID:    a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
		Message:   a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "what is the meaning?"}),
	}

	err := executor.Execute(context.Background(), reqCtx, queue)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	events := queue.getEvents()

	if len(events) < 4 {
		t.Fatalf("expected at least 4 events, got %d", len(events))
	}

	// Find the thinking event
	found := false
	for _, ev := range events {
		if se, ok := ev.(*a2a.TaskStatusUpdateEvent); ok && se.Status.Message != nil {
			for _, p := range se.Status.Message.Parts {
				if tp, ok := p.(a2a.TextPart); ok && tp.Text == "Let me think about this..." {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("expected thinking step to appear in status events")
	}
}

func TestExtractTextFromMessage(t *testing.T) {
	logger := logging.New()
	ctx := context.Background()

	msg := a2a.NewMessage(a2a.MessageRoleUser,
		a2a.TextPart{Text: "Hello"},
		a2a.TextPart{Text: "World"},
	)
	text := extractTextFromMessage(ctx, logger, msg)
	if text != "Hello\nWorld" {
		t.Errorf("expected 'Hello\\nWorld', got %q", text)
	}

	// nil message
	if extractTextFromMessage(ctx, logger, nil) != "" {
		t.Error("expected empty string for nil message")
	}
}

func TestExtractTextFromMessage_DataPart(t *testing.T) {
	logger := logging.New()
	ctx := context.Background()

	msg := a2a.NewMessage(a2a.MessageRoleUser,
		a2a.TextPart{Text: "Here is data:"},
		a2a.DataPart{Data: map[string]any{"key": "value"}},
	)
	text := extractTextFromMessage(ctx, logger, msg)
	if text == "" {
		t.Error("expected non-empty text for message with DataPart")
	}
	if !strings.Contains(text, `"key"`) || !strings.Contains(text, `"value"`) {
		t.Errorf("expected JSON data in text, got %q", text)
	}
}

func TestExtractTextFromMessage_FilePart(t *testing.T) {
	logger := logging.New()
	ctx := context.Background()

	msg := a2a.NewMessage(a2a.MessageRoleUser,
		a2a.FilePart{File: a2a.FileURI{
			FileMeta: a2a.FileMeta{Name: "doc.pdf"},
			URI:      "https://example.com/doc.pdf",
		}},
	)
	text := extractTextFromMessage(ctx, logger, msg)
	if text == "" {
		t.Error("expected non-empty text for message with FilePart")
	}
	if !strings.Contains(text, "doc.pdf") {
		t.Errorf("expected file name in text, got %q", text)
	}
}

func TestExtractTextFromMessage_FilePartBytes(t *testing.T) {
	logger := logging.New()
	ctx := context.Background()

	msg := a2a.NewMessage(a2a.MessageRoleUser,
		a2a.FilePart{File: a2a.FileBytes{
			FileMeta: a2a.FileMeta{Name: "image.png"},
			Bytes:    "aGVsbG8=",
		}},
	)
	text := extractTextFromMessage(ctx, logger, msg)
	if !strings.Contains(text, "image.png") {
		t.Errorf("expected file name in text, got %q", text)
	}
	if !strings.Contains(text, "base64") {
		t.Errorf("expected 'base64' indicator in text, got %q", text)
	}
}
