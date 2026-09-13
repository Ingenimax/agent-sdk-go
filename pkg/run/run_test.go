package run

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// blockingAgent runs until released or its context ends.
type blockingAgent struct {
	name    string
	release chan struct{}
	started chan struct{}
	reply   string
	err     error

	mu     sync.Mutex
	ctxErr error
}

func newBlockingAgent(name string) *blockingAgent {
	return &blockingAgent{
		name:    name,
		release: make(chan struct{}),
		started: make(chan struct{}, 1),
		reply:   "done",
	}
}

func (a *blockingAgent) GetName() string { return a.name }

func (a *blockingAgent) Run(ctx context.Context, _ string) (string, error) {
	select {
	case a.started <- struct{}{}:
	default:
	}
	select {
	case <-a.release:
		return a.reply, a.err
	case <-ctx.Done():
		a.mu.Lock()
		a.ctxErr = ctx.Err()
		a.mu.Unlock()
		return "", ctx.Err()
	}
}

func (a *blockingAgent) RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error) {
	ch := make(chan interfaces.AgentStreamEvent, 4)
	go func() {
		defer close(ch)
		result, err := a.Run(ctx, input)
		if err != nil {
			ch <- interfaces.AgentStreamEvent{Type: interfaces.AgentEventError, Error: err}
			return
		}
		ch <- interfaces.AgentStreamEvent{Type: interfaces.AgentEventContent, Content: result}
		ch <- interfaces.AgentStreamEvent{Type: interfaces.AgentEventComplete}
	}()
	return ch, nil
}

func (a *blockingAgent) observedCtxErr() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ctxErr
}

func TestStartReturnsImmediatelyAndWaitDelivers(t *testing.T) {
	m := NewManager()
	a := newBlockingAgent("worker")

	h := m.Start(context.Background(), a, "go")

	// Start must not block on the agent.
	select {
	case <-a.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the agent never started")
	}
	if got := h.Info().Status; got != StatusRunning {
		t.Errorf("status = %q, want %q while in flight", got, StatusRunning)
	}

	close(a.release)

	result, err := h.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if result != "done" {
		t.Errorf("result = %q, want %q", result, "done")
	}
	if got := h.Info().Status; got != StatusSucceeded {
		t.Errorf("status = %q, want %q", got, StatusSucceeded)
	}
}

// TestCancelByIDStopsTheRun is the capability the SDK had no form of: nothing
// could stop one specific run.
func TestCancelByIDStopsTheRun(t *testing.T) {
	m := NewManager()
	a := newBlockingAgent("worker")

	h := m.Start(context.Background(), a, "go")
	<-a.started

	if err := m.Cancel(h.ID()); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the run did not finish after being cancelled")
	}

	if got := h.Info().Status; got != StatusCanceled {
		t.Errorf("status = %q, want %q", got, StatusCanceled)
	}
	if !errors.Is(a.observedCtxErr(), context.Canceled) {
		t.Errorf("the agent's context ended with %v, want context.Canceled", a.observedCtxErr())
	}
}

func TestCancelUnknownRunIsAnError(t *testing.T) {
	m := NewManager()
	if err := m.Cancel("run_nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Cancel(unknown) error = %v, want ErrNotFound", err)
	}
}

// TestDetachedRunSurvivesTheCallersContext guards the property that makes a
// background run useful: it must outlive the request that started it.
func TestDetachedRunSurvivesTheCallersContext(t *testing.T) {
	m := NewManager()
	a := newBlockingAgent("worker")

	ctx, cancel := context.WithCancel(context.Background())
	h := m.Start(ctx, a, "go", WithDetached(), WithTimeout(30*time.Second))
	<-a.started

	// The caller gives up -- as an HTTP handler returning would.
	cancel()

	// The run must still be in flight.
	time.Sleep(200 * time.Millisecond)
	if got := h.Info().Status; got != StatusRunning {
		t.Fatalf("status = %q after the caller's context was cancelled, want %q: "+
			"a detached run must outlive the request that started it", got, StatusRunning)
	}

	close(a.release)
	if _, err := h.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

// TestAttachedRunDiesWithTheCaller is the complementary default: without
// WithDetached, cancelling the caller cancels the run.
func TestAttachedRunDiesWithTheCaller(t *testing.T) {
	m := NewManager()
	a := newBlockingAgent("worker")

	ctx, cancel := context.WithCancel(context.Background())
	h := m.Start(ctx, a, "go")
	<-a.started

	cancel()

	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("an attached run did not stop when its caller was cancelled")
	}
	if got := h.Info().Status; got != StatusCanceled {
		t.Errorf("status = %q, want %q", got, StatusCanceled)
	}
}

func TestTimeoutCancelsTheRun(t *testing.T) {
	m := NewManager()
	a := newBlockingAgent("worker")

	h := m.Start(context.Background(), a, "go", WithTimeout(150*time.Millisecond))

	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the run did not stop at its timeout")
	}
	if got := h.Info().Status; got != StatusCanceled {
		t.Errorf("status = %q, want %q", got, StatusCanceled)
	}
}

func TestListAndActiveReflectState(t *testing.T) {
	m := NewManager()
	a1 := newBlockingAgent("one")
	a2 := newBlockingAgent("two")

	h1 := m.Start(context.Background(), a1, "go")
	h2 := m.Start(context.Background(), a2, "go")
	<-a1.started
	<-a2.started

	if got := len(m.Active()); got != 2 {
		t.Errorf("Active() = %d runs, want 2", got)
	}

	close(a1.release)
	<-h1.Done()

	if got := len(m.Active()); got != 1 {
		t.Errorf("Active() = %d runs after one finished, want 1", got)
	}
	if got := len(m.List()); got != 2 {
		t.Errorf("List() = %d runs, want 2 (finished runs are retained)", got)
	}

	close(a2.release)
	<-h2.Done()
}

// TestRetentionIsBounded guards against the registry becoming a memory leak in
// a long-lived service.
func TestRetentionIsBounded(t *testing.T) {
	m := NewManager(WithRetainedRuns(3))

	for i := 0; i < 10; i++ {
		a := newBlockingAgent("worker")
		close(a.release)
		h := m.Start(context.Background(), a, "go")
		<-h.Done()
	}

	if got := len(m.List()); got > 3 {
		t.Errorf("List() = %d runs, want at most 3: finished runs must not accumulate", got)
	}
}

func TestRunIDIsOnTheContext(t *testing.T) {
	m := NewManager()

	var seen string
	var mu sync.Mutex
	a := &funcAgent{name: "worker", fn: func(ctx context.Context, _ string) (string, error) {
		id, _ := FromContext(ctx)
		mu.Lock()
		seen = id
		mu.Unlock()
		return "ok", nil
	}}

	h := m.Start(context.Background(), a, "go")
	if _, err := h.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if seen == "" {
		t.Error("no run ID reached the agent's context")
	}
	if seen != h.ID() {
		t.Errorf("context run ID = %q, want the handle's ID %q", seen, h.ID())
	}
}

func TestStreamDeliversEventsAndResult(t *testing.T) {
	m := NewManager()
	a := newBlockingAgent("worker")
	close(a.release)

	h := m.Stream(context.Background(), a, "go")

	var content string
	for event := range h.Events() {
		if event.Type == interfaces.AgentEventContent {
			content += event.Content
		}
	}

	result, err := h.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if content != "done" {
		t.Errorf("streamed content = %q, want %q", content, "done")
	}
	if result != "done" {
		t.Errorf("result = %q, want the accumulated content", result)
	}
}

func TestStreamOnNonStreamingAgentFails(t *testing.T) {
	m := NewManager()
	a := &funcAgent{name: "plain", fn: func(context.Context, string) (string, error) {
		return "ok", nil
	}}

	h := m.Stream(context.Background(), a, "go")
	if _, err := h.Wait(context.Background()); err == nil {
		t.Error("Stream on a non-streaming agent should fail")
	}
	if got := h.Info().Status; got != StatusFailed {
		t.Errorf("status = %q, want %q", got, StatusFailed)
	}
}

func TestCancelAllStopsEveryLiveRun(t *testing.T) {
	m := NewManager()
	agents := make([]*blockingAgent, 3)
	handles := make([]*Handle, 3)

	for i := range agents {
		agents[i] = newBlockingAgent("worker")
		handles[i] = m.Start(context.Background(), agents[i], "go")
		<-agents[i].started
	}

	if got := m.CancelAll(); got != 3 {
		t.Errorf("CancelAll() signalled %d runs, want 3", got)
	}

	for _, h := range handles {
		select {
		case <-h.Done():
		case <-time.After(5 * time.Second):
			t.Fatal("a run did not stop after CancelAll")
		}
	}
}

// funcAgent is a minimal Agent backed by a function.
type funcAgent struct {
	name string
	fn   func(context.Context, string) (string, error)
}

func (a *funcAgent) GetName() string { return a.name }
func (a *funcAgent) Run(ctx context.Context, input string) (string, error) {
	return a.fn(ctx, input)
}
