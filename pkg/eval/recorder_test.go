package eval

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestRecorderCorrelatesPairedLayersAndShortCircuits(t *testing.T) {
	recorder := NewRecorder(RecorderOptions{})
	recorder.EnablePairedLayers()

	attemptContext, attempt := recorder.Begin(context.Background(), SpanStart{
		AgentPath: RootAgentPath,
		Layer:     ToolLayerAttempt,
		Tool:      "weather",
		Method:    "Execute",
		Arguments: `{"city":"madrid"}`,
	})
	_, execution := recorder.Begin(attemptContext, SpanStart{
		AgentPath: RootAgentPath,
		Layer:     ToolLayerExecution,
		Tool:      "weather",
		Method:    "Execute",
		Arguments: `{"city":"Madrid"}`,
	})
	recorder.End(execution, SpanEnd{Err: errors.New("upstream unavailable")})
	recorder.End(attempt, SpanEnd{Result: "fallback"})

	_, denied := recorder.Begin(context.Background(), SpanStart{
		AgentPath: RootAgentPath,
		Layer:     ToolLayerAttempt,
		Tool:      "deploy",
		Method:    "Execute",
		Arguments: `{}`,
	})
	recorder.End(denied, SpanEnd{Result: "denied"})

	trace := recorder.Snapshot()
	if trace.Incomplete || len(trace.Spans) != 3 {
		t.Fatalf("trace = %#v", trace)
	}
	if trace.Spans[1].ParentID != trace.Spans[0].ID {
		t.Fatalf("execution parent = %q, want %q", trace.Spans[1].ParentID, trace.Spans[0].ID)
	}
	if trace.Spans[0].Status != ToolSpanCompleted || trace.Spans[1].Status != ToolSpanError {
		t.Fatalf("paired statuses = %s, %s", trace.Spans[0].Status, trace.Spans[1].Status)
	}
	if trace.Spans[2].Status != ToolSpanShortCircuited {
		t.Fatalf("denied attempt status = %s", trace.Spans[2].Status)
	}
}

func TestRecorderMarksTruncatedTraceIncomplete(t *testing.T) {
	recorder := NewRecorder(RecorderOptions{MaxSpans: 1, MaxContentBytes: 5})
	_, handle := recorder.Begin(context.Background(), SpanStart{
		Layer:     ToolLayerExecution,
		Tool:      "large",
		Method:    "Run",
		Arguments: "123456",
	})
	recorder.End(handle, SpanEnd{Result: "result"})
	recorder.Begin(context.Background(), SpanStart{Tool: "overflow"})

	trace := recorder.Snapshot()
	if !trace.Incomplete || len(trace.Spans) != 1 {
		t.Fatalf("trace = %#v", trace)
	}
	if !trace.Spans[0].ContentTruncated || trace.Spans[0].Arguments != "12345" {
		t.Fatalf("span = %#v", trace.Spans[0])
	}
}

func TestRecorderSupportsConcurrentIdenticalCalls(t *testing.T) {
	recorder := NewRecorder(RecorderOptions{})
	const calls = 100
	var group sync.WaitGroup
	group.Add(calls)
	for i := 0; i < calls; i++ {
		go func() {
			defer group.Done()
			_, handle := recorder.Begin(context.Background(), SpanStart{
				Layer:     ToolLayerExecution,
				Tool:      "same",
				Method:    "Execute",
				Arguments: `{}`,
			})
			recorder.End(handle, SpanEnd{Result: "ok"})
		}()
	}
	group.Wait()

	trace := recorder.Snapshot()
	if len(trace.Spans) != calls {
		t.Fatalf("got %d spans, want %d", len(trace.Spans), calls)
	}
	ids := make(map[string]struct{}, calls)
	for _, span := range trace.Spans {
		if _, duplicate := ids[span.ID]; duplicate {
			t.Fatalf("duplicate span id %q", span.ID)
		}
		ids[span.ID] = struct{}{}
		if span.Status != ToolSpanCompleted {
			t.Fatalf("span %s status = %s", span.ID, span.Status)
		}
	}
}
