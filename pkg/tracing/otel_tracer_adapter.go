package tracing

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// OTELTracerAdapter adapts OTELLangfuseTracer to implement interfaces.Tracer
// This allows the OTEL-based Langfuse tracer to be used with Agents
type OTELTracerAdapter struct {
	otelTracer *OTELLangfuseTracer
}

// NewOTELTracerAdapter creates a new adapter for OTELLangfuseTracer
func NewOTELTracerAdapter(otelTracer *OTELLangfuseTracer) interfaces.Tracer {
	return &OTELTracerAdapter{
		otelTracer: otelTracer,
	}
}

// StartSpan implements interfaces.Tracer by delegating to OTELLangfuseTracer
func (a *OTELTracerAdapter) StartSpan(ctx context.Context, name string) (context.Context, interfaces.Span) {
	return a.otelTracer.StartSpan(ctx, name)
}

// StartTraceSession implements interfaces.Tracer by delegating to OTELLangfuseTracer
func (a *OTELTracerAdapter) StartTraceSession(ctx context.Context, contextID string) (context.Context, interfaces.Span) {
	return a.otelTracer.StartTraceSession(ctx, contextID)
}

// Helper function to create and return the adapter in one call
// This makes it easy to migrate existing code
func NewOTELLangfuseTracerAsInterface(customConfig ...LangfuseConfig) (interfaces.Tracer, error) {
	otelTracer, err := NewOTELLangfuseTracer(customConfig...)
	if err != nil {
		return nil, err
	}

	return NewOTELTracerAdapter(otelTracer), nil
}
