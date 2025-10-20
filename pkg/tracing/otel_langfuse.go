package tracing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

// OTELLangfuseTracer implements tracing using OpenTelemetry sending to Langfuse
type OTELLangfuseTracer struct {
	tracerProvider *sdktrace.TracerProvider
	tracer         trace.Tracer
	exporter       *otlptrace.Exporter
	enabled        bool
	config         LangfuseConfig
}

// OTELLangfuseSpan wraps an OTEL span to implement the interfaces.Span interface
type OTELLangfuseSpan struct {
	span trace.Span
}

// End implements interfaces.Span
func (s *OTELLangfuseSpan) End() {
	s.span.End()
}

// AddEvent implements interfaces.Span
func (s *OTELLangfuseSpan) AddEvent(name string, attributes map[string]interface{}) {
	attrs := make([]attribute.KeyValue, 0, len(attributes))
	for k, v := range attributes {
		attrs = append(attrs, attribute.String(k, fmt.Sprintf("%v", v)))
	}
	s.span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetAttribute implements interfaces.Span
func (s *OTELLangfuseSpan) SetAttribute(key string, value interface{}) {
	s.span.SetAttributes(attribute.String(key, fmt.Sprintf("%v", value)))
}

func (s *OTELLangfuseSpan) RecordError(err error) {
	s.span.RecordError(err)
}

// NewOTELLangfuseTracer creates a new OTEL-based Langfuse tracer
func NewOTELLangfuseTracer(customConfig ...LangfuseConfig) (*OTELLangfuseTracer, error) {
	// Get global configuration
	cfg := config.Get()

	// Use custom config if provided, otherwise use global config
	var tracerConfig LangfuseConfig
	if len(customConfig) > 0 {
		tracerConfig = customConfig[0]
	} else {
		tracerConfig = LangfuseConfig{
			Enabled:     cfg.Tracing.Langfuse.Enabled,
			SecretKey:   cfg.Tracing.Langfuse.SecretKey,
			PublicKey:   cfg.Tracing.Langfuse.PublicKey,
			Host:        cfg.Tracing.Langfuse.Host,
			Environment: cfg.Tracing.Langfuse.Environment,
		}
	}

	if !tracerConfig.Enabled {
		return &OTELLangfuseTracer{
			enabled: false,
		}, nil
	}

	// Validate required configuration
	if tracerConfig.SecretKey == "" || tracerConfig.PublicKey == "" {
		return nil, fmt.Errorf("langfuse secret key and public key are required")
	}

	if tracerConfig.Host == "" {
		tracerConfig.Host = "https://cloud.langfuse.com"
	}

	// Build Basic Auth header for Langfuse
	auth := base64.StdEncoding.EncodeToString([]byte(tracerConfig.PublicKey + ":" + tracerConfig.SecretKey))

	// Create OTLP HTTP exporter pointing to Langfuse
	ctx := context.Background()

	// Configure endpoint URL properly
	endpointURL := tracerConfig.Host + "/api/public/otel/v1/traces"

	exporterOptions := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(endpointURL),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Basic " + auth,
		}),
	}

	// Only use insecure if explicitly using HTTP
	if len(tracerConfig.Host) >= 7 && tracerConfig.Host[:7] == "http://" {
		exporterOptions = append(exporterOptions, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, exporterOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("agent-sdk-go"),
			semconv.ServiceVersionKey.String("1.0.0"),
			attribute.String("langfuse.environment", tracerConfig.Environment),
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

	// Set as global tracer provider
	otel.SetTracerProvider(tp)

	// Create tracer
	tracer := tp.Tracer("agent-sdk-go")

	return &OTELLangfuseTracer{
		tracerProvider: tp,
		tracer:         tracer,
		exporter:       exporter,
		enabled:        true,
		config:         tracerConfig,
	}, nil
}

// StartSpan implements interfaces.Tracer
func (t *OTELLangfuseTracer) StartSpan(ctx context.Context, name string) (context.Context, interfaces.Span) {
	if !t.enabled {
		// Return a no-op span if tracing is disabled
		return ctx, &OTELLangfuseSpan{span: trace.SpanFromContext(ctx)}
	}

	// Get organization ID from context if available
	orgID, _ := multitenancy.GetOrgID(ctx)

	// Get agent name from context if available
	agentName, _ := GetAgentName(ctx)

	// Create attributes using proper Langfuse namespace
	attrs := []attribute.KeyValue{
		// Trace-level attributes (for list view)
		attribute.String("langfuse.trace.name", GetTraceNameOrDefault(ctx, name)),

		// Observation-level attributes (for detailed view)
		attribute.String("langfuse.environment", t.config.Environment),
	}
	if orgID != "" {
		attrs = append(attrs, attribute.String("langfuse.user.id", orgID))
	}

	// Add agent name if available
	if agentName != "" {
		// Use the correct Langfuse observation metadata format
		attrs = append(attrs, attribute.String("langfuse.observation.metadata.agent_name", agentName))
		// Also try as trace metadata
		attrs = append(attrs, attribute.String("langfuse.trace.metadata.agent_name", agentName))
		// Standard service name (common in observability)
		attrs = append(attrs, attribute.String("service.name", agentName))
		// User-friendly name
		attrs = append(attrs, attribute.String("component.name", agentName))
	}

	// Start OTEL span
	ctx, span := t.tracer.Start(ctx, name, trace.WithAttributes(attrs...))

	// Return wrapped span
	return ctx, &OTELLangfuseSpan{span: span}
}

// promptToAttributes converts a prompt string to GenAI semantic convention attributes
func (t *OTELLangfuseTracer) promptToAttributes(prompt string) []attribute.KeyValue {
	var attrs []attribute.KeyValue

	// For simple string prompts, assume it's a user message
	// In the future, this could be enhanced to parse structured prompts
	attrs = append(attrs,
		attribute.String("gen_ai.prompt.0.role", "user"),
		attribute.String("gen_ai.prompt.0.content", prompt),
	)

	return attrs
}

// responseToAttributes converts a response string to GenAI semantic convention attributes
func (t *OTELLangfuseTracer) responseToAttributes(response string) []attribute.KeyValue {
	var attrs []attribute.KeyValue

	attrs = append(attrs,
		attribute.String("gen_ai.completion.0.role", "assistant"),
		attribute.String("gen_ai.completion.0.content", response),
	)

	return attrs
}

// extractLastUserMessage extracts the last user message from a formatted conversation string
func extractLastUserMessage(conversationText string) string {
	// Handle empty or whitespace-only input
	if strings.TrimSpace(conversationText) == "" {
		return ""
	}

	lines := strings.Split(conversationText, "\n")

	// Look for the last line that starts with "user:"
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "user:") {
			// Remove the "user:" prefix and return the content
			userMessage := strings.TrimSpace(strings.TrimPrefix(line, "user:"))
			if userMessage != "" {
				return userMessage
			}
		}
	}

	// If no user message found, check if the entire text is a single user message
	// (this handles cases where the prompt is just the user input without formatting)
	trimmedText := strings.TrimSpace(conversationText)
	if trimmedText != "" {
		// If the text doesn't contain any role prefixes, assume it's a user message
		if !strings.Contains(trimmedText, "user:") &&
			!strings.Contains(trimmedText, "assistant:") &&
			!strings.Contains(trimmedText, "system:") {
			return trimmedText
		}
	}

	// If still no user message found, return the original text as fallback
	return conversationText
}

// TraceGeneration traces an LLM generation using OTEL spans
func (t *OTELLangfuseTracer) TraceGeneration(ctx context.Context, modelName string, prompt string, response string, startTime time.Time, endTime time.Time, metadata map[string]interface{}) (string, error) {
	if !t.enabled {
		return "", nil
	}

	// Get organization ID from context
	orgID, _ := multitenancy.GetOrgID(ctx)

	// Get agent name from context if available
	agentName, _ := GetAgentName(ctx)

	// Get span name from agent context or use default
	spanName := GetSpanNameOrDefault(ctx, "llm.generation")

	// Check for tool calls from context
	toolCalls := GetToolCallsFromContext(ctx)

	var outputWithToolCalls string
	if len(toolCalls) > 0 {
		// Try to parse response as JSON and add tool_calls field
		var responseObj map[string]interface{}
		if err := json.Unmarshal([]byte(response), &responseObj); err == nil {
			// Successfully parsed as JSON, add tool_calls field
			responseObj["tool_calls"] = toolCalls
			if modifiedJSON, err := json.Marshal(responseObj); err == nil {
				outputWithToolCalls = string(modifiedJSON)
			} else {
				// Fallback to original response if marshaling fails
				outputWithToolCalls = response
			}
		} else {
			// Not valid JSON, fallback to text concatenation
			toolCallsJSON, _ := json.MarshalIndent(toolCalls, "", "  ")
			outputWithToolCalls = fmt.Sprintf("%s\n\n**Tool Calls:**\n```json\n%s\n```", response, string(toolCallsJSON))
		}
	} else {
		outputWithToolCalls = response
	}

	// Build attributes for the generation span
	attrs := []attribute.KeyValue{
		// GenAI semantic conventions that Langfuse expects
		attribute.String("gen_ai.request.model", modelName),
		attribute.String("gen_ai.system", "openai"), // Can be made configurable

		// Langfuse-specific trace-level attributes (for list view)
		attribute.String("langfuse.trace.name", GetTraceNameOrDefault(ctx, spanName)),
		attribute.String("langfuse.trace.input", prompt),
		attribute.String("langfuse.trace.output", outputWithToolCalls),

		// Langfuse-specific observation-level attributes (for detailed view)
		attribute.String("langfuse.environment", t.config.Environment),
		attribute.String("langfuse.observation.type", "generation"),
		attribute.String("langfuse.observation.input", prompt),
		attribute.String("langfuse.observation.output", outputWithToolCalls),

		// Token usage with proper GenAI attributes (based on last user message only)
		attribute.Int64("gen_ai.usage.prompt_tokens", int64(len(prompt)/4)), // Rough estimate
		attribute.Int64("gen_ai.usage.completion_tokens", int64(len(response)/4)),
		attribute.Int64("gen_ai.usage.total_tokens", int64((len(prompt)+len(response))/4)),
	}

	// Add organization ID if available
	if orgID != "" {
		attrs = append(attrs, attribute.String("langfuse.user.id", orgID))
	}

	// Add agent name if available
	if agentName != "" {
		// Use the correct Langfuse observation metadata format
		attrs = append(attrs, attribute.String("langfuse.observation.metadata.agent_name", agentName))
		// Also try as trace metadata
		attrs = append(attrs, attribute.String("langfuse.trace.metadata.agent_name", agentName))
		// Standard service name (common in observability)
		attrs = append(attrs, attribute.String("service.name", agentName))
		// User-friendly name
		attrs = append(attrs, attribute.String("component.name", agentName))

	}

	// Add prompt attributes using the last user message
	promptAttrs := t.promptToAttributes(prompt)
	attrs = append(attrs, promptAttrs...)

	// Add response attributes
	responseAttrs := t.responseToAttributes(response)
	attrs = append(attrs, responseAttrs...)

	// Create LLM generation span (will be child of existing span if one exists)
	ctx, span := t.tracer.Start(ctx, spanName,
		trace.WithTimestamp(startTime),
		trace.WithAttributes(attrs...),
	)
	defer span.End(trace.WithTimestamp(endTime))

	// Create individual spans for each tool call at the trace level (not as children)
	if len(toolCalls) > 0 {
		// Create tool call spans using the root context to make them appear as separate timeline items
		t.createToolCallSpansAsTraceItems(ctx, toolCalls)

		// Also add tool calls to metadata for backward compatibility
		for i, toolCall := range toolCalls {
			prefix := fmt.Sprintf("tool_call_%d", i)
			span.SetAttributes(
				attribute.String("langfuse.observation.metadata."+prefix+".name", toolCall.Name),
				attribute.String("langfuse.observation.metadata."+prefix+".arguments", toolCall.Arguments),
				attribute.String("langfuse.observation.metadata."+prefix+".result", toolCall.Result),
			)
			if toolCall.Error != "" {
				span.SetAttributes(attribute.String("langfuse.observation.metadata."+prefix+".error", toolCall.Error))
			}
		}
		span.SetAttributes(attribute.Int("langfuse.observation.metadata.tool_calls_count", len(toolCalls)))
	}

	// Add metadata as span attributes using proper Langfuse namespace
	for k, v := range metadata {
		span.SetAttributes(attribute.String("langfuse.observation.metadata."+k, fmt.Sprintf("%v", v)))
	}

	return span.SpanContext().SpanID().String(), nil
}

// Message represents a chat message with role and content
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolCall represents a tool call made by the LLM
type ToolCall struct {
	Name       string        `json:"name"`
	Arguments  string        `json:"arguments"`
	ID         string        `json:"id,omitempty"`
	Result     string        `json:"result,omitempty"`
	Error      string        `json:"error,omitempty"`
	Timestamp  string        `json:"timestamp"`
	DurationMs int64         `json:"duration_ms,omitempty"` // Duration in milliseconds for JSON
	StartTime  time.Time     `json:"-"`                     // Not included in JSON, used for span timing
	Duration   time.Duration `json:"-"`                     // Execution duration, not directly serialized
}

// toolCallsKey is the context key for collecting tool calls
type toolCallsKey struct{}

// WithToolCallsCollection adds a tool calls collector to the context
func WithToolCallsCollection(ctx context.Context) context.Context {
	toolCalls := make([]ToolCall, 0)
	return context.WithValue(ctx, toolCallsKey{}, &toolCalls)
}

// AddToolCallToContext adds a tool call to the context for tracing
func AddToolCallToContext(ctx context.Context, toolCall ToolCall) {
	if toolCalls, ok := ctx.Value(toolCallsKey{}).(*[]ToolCall); ok {
		*toolCalls = append(*toolCalls, toolCall)
	}
}

// GetToolCallsFromContext retrieves tool calls from the context
func GetToolCallsFromContext(ctx context.Context) []ToolCall {
	if toolCalls, ok := ctx.Value(toolCallsKey{}).(*[]ToolCall); ok {
		return *toolCalls
	}
	return nil
}

// createToolCallSpansAsTraceItems creates individual spans for each tool call at the trace root level
func (t *OTELLangfuseTracer) createToolCallSpansAsTraceItems(ctx context.Context, toolCalls []ToolCall) {
	if !t.enabled || len(toolCalls) == 0 {
		fmt.Printf("DEBUG: Tool call spans not created - enabled: %v, toolCalls count: %d\n", t.enabled, len(toolCalls))
		return
	}

	fmt.Printf("DEBUG: Creating %d tool call spans\n", len(toolCalls))

	// Get organization ID from context
	orgID, _ := multitenancy.GetOrgID(ctx)

	for _, toolCall := range toolCalls {
		// Use actual start time if available, otherwise parse timestamp
		var startTime time.Time
		if !toolCall.StartTime.IsZero() {
			startTime = toolCall.StartTime
		} else if toolCall.Timestamp != "" {
			if parsed, err := time.Parse(time.RFC3339, toolCall.Timestamp); err == nil {
				startTime = parsed
			} else {
				startTime = time.Now().Add(-time.Second) // Fallback to 1 second ago
			}
		} else {
			startTime = time.Now().Add(-time.Second)
		}

		// Use actual duration if available, otherwise estimate
		var endTime time.Time
		if toolCall.Duration > 0 {
			endTime = startTime.Add(toolCall.Duration)
		} else {
			endTime = startTime.Add(500 * time.Millisecond) // Default 500ms execution time
		}

		// Create span name that will appear in timeline
		spanName := toolCall.Name

		// Build attributes for the tool call span to appear as a separate timeline item
		attrs := []attribute.KeyValue{
			// Langfuse-specific trace-level attributes (for list view)
			attribute.String("langfuse.trace.name", GetTraceNameOrDefault(ctx, spanName)),
			attribute.String("langfuse.trace.input", toolCall.Arguments),
			attribute.String("langfuse.trace.output", toolCall.Result),

			// Langfuse-specific observation-level attributes (for detailed view)
			attribute.String("langfuse.environment", t.config.Environment),
			attribute.String("langfuse.observation.type", "span"),
			attribute.String("langfuse.observation.name", toolCall.Name),
			attribute.String("langfuse.observation.input", toolCall.Arguments),
			attribute.String("langfuse.observation.output", toolCall.Result),

			// Tool-specific attributes
			attribute.String("tool.name", toolCall.Name),
			attribute.String("tool.arguments", toolCall.Arguments),
			attribute.String("tool.result", toolCall.Result),
			attribute.Int64("tool.duration_ms", endTime.Sub(startTime).Milliseconds()),
		}

		// Add organization ID if available
		if orgID != "" {
			attrs = append(attrs, attribute.String("langfuse.user.id", orgID))
		}

		// Add tool call ID if available
		if toolCall.ID != "" {
			attrs = append(attrs, attribute.String("tool.call_id", toolCall.ID))
		}

		// Add error information if present
		if toolCall.Error != "" {
			attrs = append(attrs,
				attribute.String("tool.error", toolCall.Error),
				attribute.String("langfuse.observation.level", "error"),
				attribute.String("langfuse.trace.output", fmt.Sprintf("Error: %s", toolCall.Error)),
				attribute.String("langfuse.observation.output", fmt.Sprintf("Error: %s", toolCall.Error)),
			)
		} else {
			attrs = append(attrs, attribute.String("langfuse.observation.level", "info"))
		}

		// Create tool call span using original context for now
		// Note: These will appear as child spans under LLM generation, but they will be visible
		_, span := t.tracer.Start(ctx, spanName,
			trace.WithTimestamp(startTime),
			trace.WithAttributes(attrs...),
		)

		// Record error if present
		if toolCall.Error != "" {
			span.RecordError(fmt.Errorf("tool execution error: %s", toolCall.Error))
		}

		// End the span with the calculated end time
		span.End(trace.WithTimestamp(endTime))
	}
}

// TraceSpan traces a span of execution
func (t *OTELLangfuseTracer) TraceSpan(ctx context.Context, name string, startTime time.Time, endTime time.Time, metadata map[string]interface{}, parentID string) (string, error) {
	if !t.enabled {
		return "", nil
	}

	// Get organization ID from context
	orgID, _ := multitenancy.GetOrgID(ctx)

	// Get agent name from context if available
	agentName, _ := GetAgentName(ctx)

	// Create span
	_, span := t.tracer.Start(ctx, name,
		trace.WithTimestamp(startTime),
		trace.WithAttributes(
			// Trace-level attributes (for list view)
			attribute.String("langfuse.trace.name", GetTraceNameOrDefault(ctx, name)),

			// Observation-level attributes (for detailed view)
			attribute.String("langfuse.environment", t.config.Environment),
			attribute.String("langfuse.user.id", orgID),
		),
	)
	defer span.End(trace.WithTimestamp(endTime))

	// Add agent name if available
	if agentName != "" {
		span.SetAttributes(attribute.String("langfuse.observation.metadata.agent_name", agentName))
	}

	// Add metadata as span attributes
	for k, v := range metadata {
		span.SetAttributes(attribute.String(k, fmt.Sprintf("%v", v)))
	}

	return span.SpanContext().SpanID().String(), nil
}

// TraceEvent traces an event
func (t *OTELLangfuseTracer) TraceEvent(ctx context.Context, name string, input interface{}, output interface{}, level string, metadata map[string]interface{}, parentID string) (string, error) {
	if !t.enabled {
		return "", nil
	}

	// Get organization ID from context
	orgID, _ := multitenancy.GetOrgID(ctx)

	// Get agent name from context if available
	agentName, _ := GetAgentName(ctx)

	// Create span for the event
	_, span := t.tracer.Start(ctx, name,
		trace.WithAttributes(
			// Trace-level attributes (for list view)
			attribute.String("langfuse.trace.name", GetTraceNameOrDefault(ctx, name)),

			// Observation-level attributes (for detailed view)
			attribute.String("langfuse.observation.level", level),
			attribute.String("langfuse.environment", t.config.Environment),
			attribute.String("langfuse.user.id", orgID),
		),
	)
	defer span.End()

	// Add agent name if available
	if agentName != "" {
		span.SetAttributes(attribute.String("langfuse.observation.metadata.agent_name", agentName))
	}

	// Add trace-level input/output if provided
	if input != nil {
		inputStr := fmt.Sprintf("%v", input)
		span.SetAttributes(
			attribute.String("langfuse.trace.input", inputStr),
			attribute.String("langfuse.observation.input", inputStr),
		)
		span.AddEvent("input", trace.WithAttributes(
			attribute.String("content", inputStr),
		))
	}

	if output != nil {
		outputStr := fmt.Sprintf("%v", output)
		span.SetAttributes(
			attribute.String("langfuse.trace.output", outputStr),
			attribute.String("langfuse.observation.output", outputStr),
		)
		span.AddEvent("output", trace.WithAttributes(
			attribute.String("content", outputStr),
		))
	}

	// Add metadata as span attributes
	for k, v := range metadata {
		span.SetAttributes(attribute.String(k, fmt.Sprintf("%v", v)))
	}

	return span.SpanContext().SpanID().String(), nil
}

// StartTraceSession starts a root trace session for a request with the given contextID/requestID
// This creates a root span that will group all subsequent LLM calls and operations
func (t *OTELLangfuseTracer) StartTraceSession(ctx context.Context, contextID string) (context.Context, interfaces.Span) {
	if !t.enabled {
		// Return a no-op span if tracing is disabled
		return ctx, &OTELLangfuseSpan{span: trace.SpanFromContext(ctx)}
	}

	// Get organization ID from context if available
	orgID, _ := multitenancy.GetOrgID(ctx)

	// Get agent name from context if available
	agentName, _ := GetAgentName(ctx)

	// Create root span for the entire request session
	attrs := []attribute.KeyValue{
		// Trace-level attributes (for list view)
		attribute.String("langfuse.trace.name", contextID),

		// Observation-level attributes (for detailed view)
		attribute.String("langfuse.environment", t.config.Environment),
		attribute.String("langfuse.observation.type", "span"),
	}

	if orgID != "" {
		attrs = append(attrs, attribute.String("langfuse.user.id", orgID))
	}

	// Add agent name if available
	if agentName != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.metadata.agent_name", agentName))
	}

	// Start root OTEL span for the session
	ctx, span := t.tracer.Start(ctx, "request-session", trace.WithAttributes(attrs...))

	// Add contextID to the context for subsequent spans
	ctx = WithTraceName(ctx, contextID)
	ctx = WithRequestID(ctx, contextID)

	// Return wrapped span
	return ctx, &OTELLangfuseSpan{span: span}
}

// Flush flushes the OTEL tracer provider
func (t *OTELLangfuseTracer) Flush() error {
	if !t.enabled || t.tracerProvider == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return t.tracerProvider.ForceFlush(ctx)
}

// Shutdown shuts down the tracer provider
func (t *OTELLangfuseTracer) Shutdown() error {
	if !t.enabled || t.tracerProvider == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return t.tracerProvider.Shutdown(ctx)
}
