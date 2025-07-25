package tracing

import (
	"context"
)

// Context keys for tracing
type contextKey string

const (
	// TraceNameKey is used to store the trace name in context
	TraceNameKey contextKey = "trace_name"

	// TraceIDKey is used to store the trace ID in context
	TraceIDKey contextKey = "trace_id"

	// RequestIDKey is used to store the request ID in context
	RequestIDKey contextKey = "request_id"

	// AgentNameKey is used to store the current agent name in context
	AgentNameKey contextKey = "agent_name"
)

// WithTraceName adds a trace name to the context
func WithTraceName(ctx context.Context, traceName string) context.Context {
	return context.WithValue(ctx, TraceNameKey, traceName)
}

// GetTraceName retrieves the trace name from context
func GetTraceName(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(TraceNameKey).(string)
	return name, ok
}

// WithTraceID adds a trace ID to the context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// GetTraceID retrieves the trace ID from context
func GetTraceID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(TraceIDKey).(string)
	return id, ok
}

// WithRequestID adds a request ID to the context for tracing
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(RequestIDKey).(string)
	return id, ok
}

// WithAgentName adds an agent name to the context for span naming
func WithAgentName(ctx context.Context, agentName string) context.Context {
	return context.WithValue(ctx, AgentNameKey, agentName)
}

// GetAgentName retrieves the agent name from context
func GetAgentName(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(AgentNameKey).(string)
	return name, ok
}

// GetTraceNameOrDefault gets trace name from context or returns a default
func GetTraceNameOrDefault(ctx context.Context, defaultName string) string {
	if name, ok := GetTraceName(ctx); ok && name != "" {
		return name
	}
	if requestID, ok := GetRequestID(ctx); ok && requestID != "" {
		return requestID
	}
	return defaultName
}

// GetSpanNameOrDefault gets span name based on agent name or returns default
func GetSpanNameOrDefault(ctx context.Context, defaultName string) string {
	if agentName, ok := GetAgentName(ctx); ok && agentName != "" {
		return agentName
	}
	return defaultName
}
