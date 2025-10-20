package tracing

import (
	"context"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

// OTelTracer implements tracing using OpenTelemetry
type OTelTracer struct {
	tracer      trace.Tracer
	enabled     bool
	serviceName string
}

// OTelSpan wraps an OpenTelemetry span to implement interfaces.Span
type OTelSpan struct {
	span trace.Span
}

// End implements interfaces.Span
func (s *OTelSpan) End() {
	s.span.End()
}

// AddEvent implements interfaces.Span
func (s *OTelSpan) AddEvent(name string, attributes map[string]interface{}) {
	attrs := make([]attribute.KeyValue, 0, len(attributes))
	for k, v := range attributes {
		attrs = append(attrs, attribute.String(k, fmt.Sprintf("%v", v)))
	}
	s.span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetAttribute implements interfaces.Span
func (s *OTelSpan) SetAttribute(key string, value interface{}) {
	s.span.SetAttributes(attribute.String(key, fmt.Sprintf("%v", value)))
}

// OTelConfig contains configuration for OpenTelemetry
type OTelConfig struct {
	// Enabled determines whether OpenTelemetry tracing is enabled
	Enabled bool

	// ServiceName is the name of the service
	ServiceName string

	// CollectorEndpoint is the endpoint of the OpenTelemetry collector
	CollectorEndpoint string

	// Tracer allows passing a pre-built tracer instead of creating one
	Tracer trace.Tracer
}

// NewOTelTracer creates a new OpenTelemetry tracer
func NewOTelTracer(config OTelConfig) (*OTelTracer, error) {
	if !config.Enabled {
		return &OTelTracer{
			enabled: false,
		}, nil
	}

	var tracer trace.Tracer

	// Use provided tracer or create a new one
	if config.Tracer != nil {
		tracer = config.Tracer
	} else {
		// Create exporter
		ctx := context.Background()
		exporter, err := otlptrace.New(
			ctx,
			otlptracegrpc.NewClient(
				otlptracegrpc.WithEndpoint(config.CollectorEndpoint),
				otlptracegrpc.WithInsecure(),
			),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
		}

		// Create resource
		res, err := resource.New(ctx,
			resource.WithAttributes(
				semconv.ServiceNameKey.String(config.ServiceName),
			),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create resource: %w", err)
		}

		// Create trace provider
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exporter),
			sdktrace.WithResource(res),
		)
		otel.SetTracerProvider(tp)

		// Create tracer
		tracer = tp.Tracer(config.ServiceName)
	}

	return &OTelTracer{
		tracer:      tracer,
		enabled:     true,
		serviceName: config.ServiceName,
	}, nil
}

// StartSpan implements interfaces.Tracer
func (t *OTelTracer) StartSpan(ctx context.Context, name string) (context.Context, interfaces.Span) {
	if !t.enabled {
		return ctx, &OTelSpan{span: trace.SpanFromContext(ctx)}
	}

	// Get organization ID from context
	orgID, _ := multitenancy.GetOrgID(ctx)

	attrs := []attribute.KeyValue{}
	if orgID != "" {
		attrs = append(attrs, attribute.String("org_id", orgID))
	}

	// Namespace the span name with the library name
	namespacedName := "github.com/Ingenimax/agent-sdk-go/" + name

	// Start span
	ctx, span := t.tracer.Start(ctx, namespacedName, trace.WithAttributes(attrs...))
	return ctx, &OTelSpan{span: span}
}

// StartTraceSession implements interfaces.Tracer
func (t *OTelTracer) StartTraceSession(ctx context.Context, contextID string) (context.Context, interfaces.Span) {
	if !t.enabled {
		return ctx, &OTelSpan{span: trace.SpanFromContext(ctx)}
	}

	// Get organization ID from context
	orgID, _ := multitenancy.GetOrgID(ctx)

	attrs := []attribute.KeyValue{
		attribute.String("trace.session_id", contextID),
	}
	if orgID != "" {
		attrs = append(attrs, attribute.String("org_id", orgID))
	}

	// Namespace the span name with the library name
	namespacedName := "github.com/Ingenimax/agent-sdk-go/trace-session"

	// Start root span for the session
	ctx, span := t.tracer.Start(ctx, namespacedName, trace.WithAttributes(attrs...))
	return ctx, &OTelSpan{span: span}
}

// @deprecated Use NewTracedLLM - removing in v1.0.0
func NewMemoryOTelMiddleware(memory interfaces.Memory, tracer *OTelTracer) interfaces.Memory {
	return NewTracedMemory(memory, tracer)
}
