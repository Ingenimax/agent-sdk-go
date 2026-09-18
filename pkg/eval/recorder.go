package eval

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const (
	DefaultMaxSpans        = 10_000
	DefaultMaxContentBytes = 10 << 20
)

// RecorderOptions bounds trace retention. Now exists to make recorder tests
// deterministic; production callers normally leave it nil.
type RecorderOptions struct {
	MaxSpans        int
	MaxContentBytes int
	Now             func() time.Time
}

// SpanStart describes a tool invocation at decorator entry.
type SpanStart struct {
	AgentPath string
	Layer     ToolLayer
	Tool      string
	Method    string
	Arguments string
}

// SpanEnd describes a tool invocation at decorator exit.
type SpanEnd struct {
	Result     string
	Err        error
	PanicValue any
}

// SpanHandle identifies a span allocated by Recorder.Begin.
type SpanHandle struct{ id string }

// Valid reports whether the recorder retained this span.
func (h SpanHandle) Valid() bool { return h.id != "" }

type parentSpanContextKey struct{}

// Recorder captures correlated tool spans safely across concurrent calls.
type Recorder struct {
	mu              sync.Mutex
	spans           []ToolSpan
	spanIndex       map[string]int
	incomplete      bool
	incompleteCause string
	retainedBytes   int
	maxSpans        int
	maxContentBytes int
	now             func() time.Time
	sequence        atomic.Uint64
	pairedLayers    atomic.Bool
}

// NewRecorder creates an empty recorder with bounded retention.
func NewRecorder(options RecorderOptions) *Recorder {
	maxSpans := options.MaxSpans
	if maxSpans <= 0 {
		maxSpans = DefaultMaxSpans
	}
	maxContentBytes := options.MaxContentBytes
	if maxContentBytes <= 0 {
		maxContentBytes = DefaultMaxContentBytes
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Recorder{
		spanIndex:       make(map[string]int),
		maxSpans:        maxSpans,
		maxContentBytes: maxContentBytes,
		now:             now,
	}
}

// EnablePairedLayers tells Snapshot that attempt spans should have execution
// children. The SDK adapter enables this when it installs both decorators.
func (r *Recorder) EnablePairedLayers() {
	if r != nil {
		r.pairedLayers.Store(true)
	}
}

// Begin records tool entry and returns a context carrying the new span as the
// parent for nested decorators and sub-agents.
func (r *Recorder) Begin(ctx context.Context, start SpanStart) (context.Context, SpanHandle) {
	if r == nil {
		return ctx, SpanHandle{}
	}
	sequence := r.sequence.Add(1)
	id := fmt.Sprintf("tool-%d", sequence)
	parentID, _ := ctx.Value(parentSpanContextKey{}).(string)
	if start.AgentPath == "" {
		start.AgentPath = RootAgentPath
	}

	r.mu.Lock()
	if len(r.spans) >= r.maxSpans {
		r.markIncompleteLocked("tool span limit exceeded")
		r.mu.Unlock()
		return ctx, SpanHandle{}
	}
	arguments, truncated := r.retainLocked(start.Arguments)
	span := ToolSpan{
		ID:               id,
		ParentID:         parentID,
		AgentPath:        start.AgentPath,
		Layer:            start.Layer,
		Sequence:         sequence,
		Tool:             start.Tool,
		Method:           start.Method,
		Arguments:        arguments,
		StartedAt:        r.now(),
		Status:           ToolSpanIncomplete,
		ContentTruncated: truncated,
	}
	r.spanIndex[id] = len(r.spans)
	r.spans = append(r.spans, span)
	r.mu.Unlock()

	return context.WithValue(ctx, parentSpanContextKey{}, id), SpanHandle{id: id}
}

// End completes exactly the span identified by handle.
func (r *Recorder) End(handle SpanHandle, end SpanEnd) {
	if r == nil || !handle.Valid() {
		return
	}
	endedAt := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	index, ok := r.spanIndex[handle.id]
	if !ok {
		return
	}
	span := &r.spans[index]
	if span.EndedAt != nil {
		return
	}
	span.EndedAt = &endedAt
	span.Result, span.ContentTruncated = r.retainWithExistingFlagLocked(end.Result, span.ContentTruncated)
	switch {
	case end.PanicValue != nil:
		span.Status = ToolSpanPanicked
		span.Error, span.ContentTruncated = r.retainWithExistingFlagLocked(fmt.Sprint(end.PanicValue), span.ContentTruncated)
	case end.Err != nil:
		span.Status = ToolSpanError
		span.Error, span.ContentTruncated = r.retainWithExistingFlagLocked(end.Err.Error(), span.ContentTruncated)
	default:
		span.Status = ToolSpanCompleted
	}
}

// MarkIncomplete marks the trace unusable for checks requiring complete tool
// capture. It is useful when an adapter detects a boundary it cannot observe.
func (r *Recorder) MarkIncomplete(reason string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.markIncompleteLocked(reason)
}

// Snapshot returns a deep copy ordered by invocation start sequence.
func (r *Recorder) Snapshot() Trace {
	if r == nil {
		return Trace{}
	}
	r.mu.Lock()
	spans := append([]ToolSpan(nil), r.spans...)
	incomplete := r.incomplete
	reason := r.incompleteCause
	r.mu.Unlock()

	for i := range spans {
		if spans[i].EndedAt != nil {
			ended := *spans[i].EndedAt
			spans[i].EndedAt = &ended
		}
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].Sequence < spans[j].Sequence })
	if r.pairedLayers.Load() {
		executionParents := make(map[string]struct{})
		for _, span := range spans {
			if span.Layer == ToolLayerExecution && span.ParentID != "" {
				executionParents[span.ParentID] = struct{}{}
			}
		}
		for i := range spans {
			if spans[i].Layer != ToolLayerAttempt || spans[i].Status != ToolSpanCompleted {
				continue
			}
			if _, executed := executionParents[spans[i].ID]; !executed {
				spans[i].Status = ToolSpanShortCircuited
			}
		}
	}
	return Trace{Spans: spans, Incomplete: incomplete, Reason: reason}
}

func (r *Recorder) retainWithExistingFlagLocked(value string, existing bool) (string, bool) {
	retained, truncated := r.retainLocked(value)
	return retained, existing || truncated
}

func (r *Recorder) retainLocked(value string) (string, bool) {
	if value == "" {
		return "", false
	}
	remaining := r.maxContentBytes - r.retainedBytes
	if remaining <= 0 {
		r.markIncompleteLocked("retained trace content limit exceeded")
		return "", true
	}
	if len(value) <= remaining {
		r.retainedBytes += len(value)
		return value, false
	}
	bytes := []byte(value[:remaining])
	for len(bytes) > 0 && !utf8.Valid(bytes) {
		bytes = bytes[:len(bytes)-1]
	}
	r.retainedBytes += len(bytes)
	r.markIncompleteLocked("retained trace content limit exceeded")
	return string(bytes), true
}

func (r *Recorder) markIncompleteLocked(reason string) {
	r.incomplete = true
	if reason == "" {
		return
	}
	if r.incompleteCause == "" {
		r.incompleteCause = reason
		return
	}
	for _, existing := range strings.Split(r.incompleteCause, "; ") {
		if existing == reason {
			return
		}
	}
	r.incompleteCause += "; " + reason
}
