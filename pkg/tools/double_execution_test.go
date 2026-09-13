package tools

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// countingSubAgent records how many times it was actually executed, by any
// entry point.
type countingSubAgent struct {
	runs        atomic.Int64
	streamRuns  atomic.Int64
	detailRuns  atomic.Int64
	replyText   string
	reportUsage *interfaces.TokenUsage
}

func (s *countingSubAgent) GetName() string        { return "counter" }
func (s *countingSubAgent) GetDescription() string { return "counts its own executions" }

func (s *countingSubAgent) Run(_ context.Context, _ string) (string, error) {
	s.runs.Add(1)
	return s.replyText, nil
}

func (s *countingSubAgent) RunDetailed(_ context.Context, _ string) (*interfaces.AgentResponse, error) {
	s.runs.Add(1)
	s.detailRuns.Add(1)
	return &interfaces.AgentResponse{
		Content:   s.replyText,
		AgentName: "counter",
		Model:     "counter-model",
		Usage:     s.reportUsage,
	}, nil
}

func (s *countingSubAgent) RunStream(ctx context.Context, _ string) (<-chan interfaces.AgentStreamEvent, error) {
	s.runs.Add(1)
	s.streamRuns.Add(1)

	ch := make(chan interfaces.AgentStreamEvent, 3)
	ch <- interfaces.AgentStreamEvent{
		Type:      interfaces.AgentEventContent,
		Content:   s.replyText,
		Timestamp: time.Now(),
	}
	// The completion event carries usage, exactly as pkg/agent's streaming path
	// now emits it.
	ch <- interfaces.AgentStreamEvent{
		Type:      interfaces.AgentEventComplete,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			interfaces.MetadataKeyUsage: s.reportUsage,
			interfaces.MetadataKeyModel: "counter-model",
		},
	}
	close(ch)
	return ch, nil
}

// TestStreamedSubAgentExecutesExactlyOnce guards a defect that charged users
// twice for every streamed sub-agent call.
//
// AgentTool.Execute used to do this when a stream forwarder was present:
//
//	result, streamErr := at.runWithStreaming(...)   // runs the sub-agent
//	...
//	response, err = at.agent.RunDetailed(ctx, input) // runs it AGAIN
//
// The second call existed purely to recover *AgentResponse.Usage for logging
// and span attributes -- it was never recorded into the parent's usage tracker.
// So every parent that streamed executed and paid for each sub-agent twice, and
// any side effect the sub-agent had happened twice too.
func TestStreamedSubAgentExecutesExactlyOnce(t *testing.T) {
	sub := &countingSubAgent{
		replyText:   "sub-agent answer",
		reportUsage: &interfaces.TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
	}
	tool := NewAgentTool(sub)

	var forwarded atomic.Int64
	ctx := WithStreamForwarder(context.Background(), func(interfaces.AgentStreamEvent) {
		forwarded.Add(1)
	})

	out, err := tool.Execute(ctx, `{"query":"do the thing"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := sub.runs.Load(); got != 1 {
		t.Errorf("sub-agent executed %d times, want exactly 1: a streamed sub-agent "+
			"must not be run again to recover usage -- that bills the caller twice "+
			"and repeats any side effect", got)
	}
	if got := sub.detailRuns.Load(); got != 0 {
		t.Errorf("RunDetailed was called %d times on the streaming path, want 0", got)
	}
	if got := sub.streamRuns.Load(); got != 1 {
		t.Errorf("RunStream called %d times, want 1", got)
	}
	if out != "sub-agent answer" {
		t.Errorf("Execute() = %q, want the streamed content", out)
	}
	if forwarded.Load() == 0 {
		t.Error("no events were forwarded to the parent")
	}
}

// TestStreamedSubAgentRecoversUsageWithoutRerunning asserts the accounting the
// second execution used to provide still arrives -- via the completion event's
// metadata rather than a second run.
func TestStreamedSubAgentRecoversUsageWithoutRerunning(t *testing.T) {
	want := &interfaces.TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}
	sub := &countingSubAgent{replyText: "answer", reportUsage: want}
	tool := NewAgentTool(sub)

	ctx := WithStreamForwarder(context.Background(), func(interfaces.AgentStreamEvent) {})

	resp, err := tool.runWithStreaming(ctx, "input", func(interfaces.AgentStreamEvent) {}, nil, "counter")
	if err != nil {
		t.Fatalf("runWithStreaming() error = %v", err)
	}

	if resp.Usage == nil {
		t.Fatal("usage was not recovered from the completion event; removing the " +
			"second execution would then lose token accounting entirely")
	}
	if resp.Usage.TotalTokens != want.TotalTokens {
		t.Errorf("TotalTokens = %d, want %d", resp.Usage.TotalTokens, want.TotalTokens)
	}
	if resp.Model != "counter-model" {
		t.Errorf("Model = %q, want %q", resp.Model, "counter-model")
	}
	if resp.Content != "answer" {
		t.Errorf("Content = %q, want %q", resp.Content, "answer")
	}
	if got := sub.runs.Load(); got != 1 {
		t.Errorf("sub-agent executed %d times, want 1", got)
	}
}

// TestNonStreamedSubAgentStillExecutesOnce pins the non-streaming path, which
// was already correct and must stay that way.
func TestNonStreamedSubAgentStillExecutesOnce(t *testing.T) {
	sub := &countingSubAgent{replyText: "answer"}
	tool := NewAgentTool(sub)

	if _, err := tool.Execute(context.Background(), `{"query":"go"}`); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := sub.runs.Load(); got != 1 {
		t.Errorf("sub-agent executed %d times, want 1", got)
	}
	if got := sub.detailRuns.Load(); got != 1 {
		t.Errorf("RunDetailed called %d times, want 1 on the non-streaming path", got)
	}
}
