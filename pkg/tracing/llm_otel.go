package tracing

import (
	"context"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"go.opentelemetry.io/otel/attribute"
)

// LLMOTelMiddleware wraps an LLM with OpenTelemetry tracing
type LLMOTelMiddleware struct {
	llm    interfaces.LLM
	tracer *OTelTracer
}

// NewLLMOTelMiddleware creates a new LLMOTelMiddleware
func NewLLMOTelMiddleware(llm interfaces.LLM, tracer *OTelTracer) *LLMOTelMiddleware {
	return &LLMOTelMiddleware{
		llm:    llm,
		tracer: tracer,
	}
}

// Generate implements interfaces.LLM.Generate
func (m *LLMOTelMiddleware) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	// Create attributes
	attributes := map[string]string{
		"prompt.length": fmt.Sprintf("%d", len(prompt)),
		"model":         "unknown", // We can't easily extract the model from options anymore
	}

	// Start span
	ctx, span := m.tracer.StartSpan(ctx, "llm.generate", attributes)
	defer func() {
		m.tracer.EndSpan(span, nil)
	}()

	// Call the underlying LLM
	response, err := m.llm.Generate(ctx, prompt, options...)

	// Record response attributes
	if err == nil {
		span.SetAttributes(attribute.Int("response.length", len(response)))
	} else {
		span.RecordError(err)
	}

	return response, err
}

// GenerateWithTools implements interfaces.LLM.GenerateWithTools
func (m *LLMOTelMiddleware) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	// Create attributes
	attributes := map[string]string{
		"prompt.length": fmt.Sprintf("%d", len(prompt)),
		"tools.count":   fmt.Sprintf("%d", len(tools)),
	}

	// Start span
	ctx, span := m.tracer.StartSpan(ctx, "llm.generate_with_tools", attributes)
	defer func() {
		m.tracer.EndSpan(span, nil)
	}()

	// Call the underlying LLM
	response, err := m.llm.GenerateWithTools(ctx, prompt, tools, options...)

	// Record response attributes
	if err == nil {
		span.SetAttributes(attribute.Int("response.length", len(response)))
	} else {
		span.RecordError(err)
	}

	return response, err
}

// Name implements interfaces.LLM.Name
func (m *LLMOTelMiddleware) Name() string {
	return m.llm.Name()
}

// SupportsStreaming implements interfaces.LLM.SupportsStreaming
func (m *LLMOTelMiddleware) SupportsStreaming() bool {
	return m.llm.SupportsStreaming()
}
