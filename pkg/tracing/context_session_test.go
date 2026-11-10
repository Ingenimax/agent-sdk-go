package tracing

import (
	"context"
	"errors"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func TestContextAccessors(t *testing.T) {
	ctx := context.Background()
	if _, ok := GetTraceID(ctx); ok {
		t.Error("GetTraceID found a value in an empty context")
	}

	ctx = WithTraceID(ctx, "trace-1")
	ctx = WithTraceName(ctx, "name-1")
	ctx = WithRequestID(ctx, "req-1")

	if got, ok := GetTraceID(ctx); !ok || got != "trace-1" {
		t.Errorf("GetTraceID = %q, %v", got, ok)
	}
	if got, ok := GetTraceName(ctx); !ok || got != "name-1" {
		t.Errorf("GetTraceName = %q, %v", got, ok)
	}
	if got, ok := GetRequestID(ctx); !ok || got != "req-1" {
		t.Errorf("GetRequestID = %q, %v", got, ok)
	}
}

func TestGetTraceNameOrDefault(t *testing.T) {
	bg := context.Background()
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{name: "empty context", ctx: bg, want: "default"},
		{name: "trace name wins", ctx: WithRequestID(WithTraceName(bg, "trace"), "req"), want: "trace"},
		{name: "request id when no trace name", ctx: WithRequestID(bg, "req"), want: "req"},
		{name: "empty trace name falls through", ctx: WithRequestID(WithTraceName(bg, ""), "req"), want: "req"},
		{name: "empty values use default", ctx: WithRequestID(WithTraceName(bg, ""), ""), want: "default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetTraceNameOrDefault(tt.ctx, "default"); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetSpanNameOrDefault(t *testing.T) {
	bg := context.Background()
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{name: "no agent", ctx: bg, want: "default"},
		{name: "empty agent", ctx: WithAgentName(bg, ""), want: "default"},
		{name: "agent name", ctx: WithAgentName(bg, "agent"), want: "agent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetSpanNameOrDefault(tt.ctx, "default"); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStartRequestTracing_NilTracer(t *testing.T) {
	ctx := context.Background()
	gotCtx, span := StartRequestTracing(ctx, nil, "req")
	if gotCtx != ctx {
		t.Error("context changed without a tracer")
	}
	if _, ok := span.(*NoOpSpan); !ok {
		t.Fatalf("span = %T, want *NoOpSpan", span)
	}
	// The no-op span accepts every call without effect.
	span.AddEvent("e", nil)
	span.SetAttribute("k", "v")
	span.RecordError(errors.New("x"))
	span.End()
}

func TestWithRequestTracing(t *testing.T) {
	tracer := &testutil.FakeTracer{}
	ctx, span := WithRequestTracing(context.Background(), tracer, "req-9", "org")
	if span == nil {
		t.Fatal("nil span")
	}
	if name, _ := onlySpan(t, tracer); name != "req-9" {
		t.Errorf("session span = %q", name)
	}
	if id, _ := GetRequestID(ctx); id != "req-9" {
		t.Errorf("request id = %q", id)
	}
	if name, _ := GetTraceName(ctx); name != "req-9" {
		t.Errorf("trace name = %q", name)
	}
}

func newInMemoryOTelTracer(t *testing.T) (*OTelTracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	tracer, err := NewOTelTracer(OTelConfig{Enabled: true, ServiceName: "svc", Tracer: provider.Tracer("test")})
	if err != nil {
		t.Fatalf("NewOTelTracer: %v", err)
	}
	return tracer, exporter
}

func TestOTelTracer_StartSpan(t *testing.T) {
	tracer, exporter := newInMemoryOTelTracer(t)
	if tracer.serviceName != "svc" {
		t.Errorf("serviceName = %q", tracer.serviceName)
	}

	ctx := multitenancy.WithOrgID(context.Background(), "org-1")
	ctx, span := tracer.StartSpan(ctx, "op")
	if !trace.SpanFromContext(ctx).SpanContext().IsValid() {
		t.Error("returned context carries no span")
	}
	span.SetAttribute("count", 3)
	span.AddEvent("evt", map[string]any{"k": 1})
	span.RecordError(errors.New("bad"))
	span.End()

	got := spanByName(t, exporter, "github.com/Ingenimax/agent-sdk-go/op")
	assertAttrs(t, attrMap(got.Attributes), map[string]string{"org_id": "org-1", "count": "3"})
	var names []string
	for _, e := range got.Events {
		names = append(names, e.Name)
	}
	if len(names) != 2 || names[0] != "evt" || names[1] != "exception" {
		t.Errorf("events = %v", names)
	}
	if attrMap(got.Events[0].Attributes)["k"] != "1" {
		t.Errorf("event attributes = %v", got.Events[0].Attributes)
	}
}

func TestOTelTracer_StartSpanWithoutOrg(t *testing.T) {
	tracer, exporter := newInMemoryOTelTracer(t)
	_, span := tracer.StartSpan(context.Background(), "op")
	span.End()

	got := spanByName(t, exporter, "github.com/Ingenimax/agent-sdk-go/op")
	if _, ok := attrMap(got.Attributes)["org_id"]; ok {
		t.Error("org_id set without an org in context")
	}
}

func TestOTelTracer_StartTraceSession(t *testing.T) {
	tracer, exporter := newInMemoryOTelTracer(t)
	ctx := multitenancy.WithOrgID(context.Background(), "org-2")
	_, span := tracer.StartTraceSession(ctx, "session-1")
	span.End()

	got := spanByName(t, exporter, "github.com/Ingenimax/agent-sdk-go/trace-session")
	assertAttrs(t, attrMap(got.Attributes), map[string]string{"trace.session_id": "session-1", "org_id": "org-2"})
}

func TestOTelTracer_Disabled(t *testing.T) {
	disabled, err := NewOTelTracer(OTelConfig{Enabled: false})
	if err != nil {
		t.Fatalf("NewOTelTracer: %v", err)
	}
	wrapped := NewOTelTracerWrapper(nil)

	for name, tracer := range map[string]*OTelTracer{"config": disabled, "nil wrapper": wrapped} {
		t.Run(name, func(t *testing.T) {
			if tracer.enabled {
				t.Fatal("tracer should be disabled")
			}
			ctx := context.Background()
			gotCtx, span := tracer.StartSpan(ctx, "op")
			if gotCtx != ctx {
				t.Error("disabled StartSpan changed the context")
			}
			span.End()
			gotCtx, span = tracer.StartTraceSession(ctx, "s")
			if gotCtx != ctx {
				t.Error("disabled StartTraceSession changed the context")
			}
			span.End()
		})
	}
}

func TestNewOTelTracerWrapper(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	tracer := NewOTelTracerWrapper(provider.Tracer("w"))
	if !tracer.enabled {
		t.Fatal("wrapper over a real tracer should be enabled")
	}
	_, span := tracer.StartSpan(context.Background(), "wrapped")
	span.End()
	spanByName(t, exporter, "github.com/Ingenimax/agent-sdk-go/wrapped")
}

func TestNewMemoryOTelMiddleware(t *testing.T) {
	tracer, _ := newInMemoryOTelTracer(t)
	mem := NewMemoryOTelMiddleware(plainMemory{}, tracer)
	traced, ok := mem.(*TracedMemory)
	if !ok {
		t.Fatalf("got %T, want *TracedMemory", mem)
	}
	if traced.tracer != tracer {
		t.Error("middleware does not use the given tracer")
	}
}
