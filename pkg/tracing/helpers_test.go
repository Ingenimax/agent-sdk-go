package tracing

import (
	"context"
	"sync"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// recordedSpan captures everything a caller does with a span.
type recordedSpan struct {
	mu     sync.Mutex
	name   string
	attrs  map[string]any
	events []string
	errs   []error
	ended  bool
}

func (s *recordedSpan) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended = true
}

func (s *recordedSpan) AddEvent(name string, _ map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, name)
}

func (s *recordedSpan) SetAttribute(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attrs[key] = value
}

func (s *recordedSpan) RecordError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errs = append(s.errs, err)
}

// recordingTracer is an interfaces.Tracer that keeps every span it starts.
type recordingTracer struct {
	mu    sync.Mutex
	spans []*recordedSpan
}

func (r *recordingTracer) start(name string) *recordedSpan {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := &recordedSpan{name: name, attrs: map[string]any{}}
	r.spans = append(r.spans, s)
	return s
}

func (r *recordingTracer) StartSpan(ctx context.Context, name string) (context.Context, interfaces.Span) {
	return ctx, r.start(name)
}

func (r *recordingTracer) StartTraceSession(ctx context.Context, contextID string) (context.Context, interfaces.Span) {
	return ctx, r.start("session:" + contextID)
}

func (r *recordingTracer) only(t *testing.T) *recordedSpan {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(r.spans))
	}
	return r.spans[0]
}

// newTestLangfuseTracer returns an enabled tracer that exports to memory.
func newTestLangfuseTracer(t *testing.T, includeContent bool) (*OTELLangfuseTracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return &OTELLangfuseTracer{
		tracerProvider: provider,
		tracer:         provider.Tracer("test"),
		enabled:        true,
		config:         LangfuseConfig{Environment: "test-env"},
		IncludeContent: includeContent,
	}, exporter
}

// attrMap flattens span attributes to strings for easy comparison.
func attrMap(kvs []attribute.KeyValue) map[string]string {
	m := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		m[string(kv.Key)] = kv.Value.String()
	}
	return m
}

// spanByName returns the first exported span with the given name.
func spanByName(t *testing.T, exporter *tracetest.InMemoryExporter, name string) tracetest.SpanStub {
	t.Helper()
	for _, s := range exporter.GetSpans() {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("span %q not exported; have %d spans", name, len(exporter.GetSpans()))
	return tracetest.SpanStub{}
}

// assertAttrs checks that every wanted attribute is present with its value.
func assertAttrs(t *testing.T, got map[string]string, want map[string]string) {
	t.Helper()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("attribute %q = %q, want %q", k, got[k], v)
		}
	}
}
