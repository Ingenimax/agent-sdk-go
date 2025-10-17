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

// OTelConfig contains configuration for OpenTelemetry
type OTelConfig struct {
	// Enabled determines whether OpenTelemetry tracing is enabled
	Enabled bool

	// ServiceName is the name of the service
	ServiceName string

	// CollectorEndpoint is the endpoint of the OpenTelemetry collector
	CollectorEndpoint string
}

// NewOTelTracer creates a new OpenTelemetry tracer
func NewOTelTracer(config OTelConfig) (*OTelTracer, error) {
	if !config.Enabled {
		return &OTelTracer{
			enabled: false,
		}, nil
	}

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
	tracer := tp.Tracer(config.ServiceName)

	return &OTelTracer{
		tracer:      tracer,
		enabled:     true,
		serviceName: config.ServiceName,
	}, nil
}

// NewOTelTracerWrapper creates a new OpenTelemetry tracer wrapper from existing tracer
func NewOTelTracerWrapper(tracer trace.Tracer) *OTelTracer {
	if tracer == nil {
		return &OTelTracer{
			enabled: false,
		}
	}

	return &OTelTracer{
		tracer:  tracer,
		enabled: true,
	}
}

// StartSpan starts a new span
func (t *OTelTracer) StartSpan(ctx context.Context, name string, attributes map[string]string) (context.Context, trace.Span) {
	if !t.enabled {
		return ctx, trace.SpanFromContext(ctx)
	}

	// Convert attributes to OpenTelemetry attributes
	attrs := make([]attribute.KeyValue, 0, len(attributes))
	for k, v := range attributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	// Get organization ID from context
	orgID, _ := multitenancy.GetOrgID(ctx)
	if orgID != "" {
		attrs = append(attrs, attribute.String("org_id", orgID))
	}

	// Start span
	return t.tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

// EndSpan ends a span
func (t *OTelTracer) EndSpan(span trace.Span, err error) {
	if !t.enabled {
		return
	}

	if err != nil {
		span.RecordError(err)
	}
	span.End()
}

// MemoryOTelMiddleware implements middleware for memory operations with OpenTelemetry tracing
type MemoryOTelMiddleware struct {
	memory interfaces.Memory
	tracer *OTelTracer
}

// NewMemoryOTelMiddleware creates a new memory middleware with OpenTelemetry tracing
func NewMemoryOTelMiddleware(memory interfaces.Memory, tracer *OTelTracer) *MemoryOTelMiddleware {
	return &MemoryOTelMiddleware{
		memory: memory,
		tracer: tracer,
	}
}

// AddMessage adds a message to memory with OpenTelemetry tracing
func (m *MemoryOTelMiddleware) AddMessage(ctx context.Context, message interfaces.Message) error {
	// Create attributes
	attributes := map[string]string{
		"message.role":    string(message.Role),
		"message.content": fmt.Sprintf("%d bytes", len(message.Content)),
	}

	// Start span
	ctx, span := m.tracer.StartSpan(ctx, "memory.add_message", attributes)
	defer func() {
		m.tracer.EndSpan(span, nil)
	}()

	// Call the underlying memory
	err := m.memory.AddMessage(ctx, message)
	if err != nil {
		span.RecordError(err)
	}

	return err
}

// GetMessages gets messages from memory with OpenTelemetry tracing
func (m *MemoryOTelMiddleware) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	// Start span
	ctx, span := m.tracer.StartSpan(ctx, "memory.get_messages", nil)
	defer func() {
		m.tracer.EndSpan(span, nil)
	}()

	// Call the underlying memory
	messages, err := m.memory.GetMessages(ctx, options...)
	if err != nil {
		span.RecordError(err)
	} else {
		span.SetAttributes(attribute.Int("messages.count", len(messages)))
	}

	return messages, err
}

// Clear clears memory with OpenTelemetry tracing
func (m *MemoryOTelMiddleware) Clear(ctx context.Context) error {
	// Start span
	ctx, span := m.tracer.StartSpan(ctx, "memory.clear", nil)
	defer func() {
		m.tracer.EndSpan(span, nil)
	}()

	// Call the underlying memory
	err := m.memory.Clear(ctx)
	if err != nil {
		span.RecordError(err)
	}

	return err
}
