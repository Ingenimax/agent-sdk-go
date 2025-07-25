package tracing

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// StartRequestTracing starts a trace session for a request and returns the updated context
// This should be called at the beginning of each request to group all operations under one trace
func StartRequestTracing(ctx context.Context, tracer interfaces.Tracer, requestID string) (context.Context, interfaces.Span) {
	if tracer == nil {
		// Return original context with no-op span if no tracer is available
		return ctx, &NoOpSpan{}
	}

	// Start the trace session with the requestID
	return tracer.StartTraceSession(ctx, requestID)
}

// NoOpSpan is a no-operation span implementation for when tracing is disabled
type NoOpSpan struct{}

func (s *NoOpSpan) End()                                                    {}
func (s *NoOpSpan) AddEvent(name string, attributes map[string]interface{}) {}
func (s *NoOpSpan) SetAttribute(key string, value interface{})              {}

// WithRequestTracing is a convenience function that combines context setup and trace session creation
func WithRequestTracing(ctx context.Context, tracer interfaces.Tracer, requestID string, orgID string) (context.Context, interfaces.Span) {
	// Add the requestID to context for tracing
	ctx = WithRequestID(ctx, requestID)
	ctx = WithTraceName(ctx, requestID)

	// Start the trace session
	return StartRequestTracing(ctx, tracer, requestID)
}
