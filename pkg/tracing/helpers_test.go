package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
)

// onlySpan returns the name and span of the single span tr started.
func onlySpan(t *testing.T, tr *testutil.FakeTracer) (string, *testutil.FakeSpan) {
	t.Helper()
	names, spans := tr.SpanNames(), tr.Spans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	return names[0], spans[0]
}

// attr returns a span attribute, or nil when it is not set.
func attr(s *testutil.FakeSpan, key string) any {
	v, _ := s.Attribute(key)
	return v
}

// hasAttr reports whether the span has the attribute.
func hasAttr(s *testutil.FakeSpan, key string) bool {
	_, ok := s.Attribute(key)
	return ok
}

// keepGlobalTracerProvider restores the global OpenTelemetry tracer provider
// when the test ends, for tests whose code under test replaces it.
func keepGlobalTracerProvider(t *testing.T) {
	t.Helper()
	prev := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
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
