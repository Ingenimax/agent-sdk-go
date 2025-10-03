package tracing

import (
	"context"
	"fmt"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// LangfuseTracer implements tracing using Langfuse via OTEL (backward compatibility wrapper)
// This replaces the old buggy henomis/langfuse-go implementation with our reliable OTEL-based one
type LangfuseTracer struct {
	otelTracer *OTELLangfuseTracer
	enabled    bool
}

// LangfuseConfig contains configuration for Langfuse
type LangfuseConfig struct {
	// Enabled determines whether Langfuse tracing is enabled
	Enabled bool

	// SecretKey is the Langfuse secret key
	SecretKey string

	// PublicKey is the Langfuse public key
	PublicKey string

	// Host is the Langfuse host (optional)
	Host string

	// Environment is the environment name (e.g., "production", "staging")
	Environment string
}

// NewLangfuseTracer creates a new Langfuse tracer (backward compatibility wrapper)
// This now uses the reliable OTEL-based implementation internally
func NewLangfuseTracer(customConfig ...LangfuseConfig) (*LangfuseTracer, error) {
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
		return &LangfuseTracer{
			enabled: false,
		}, nil
	}

	// Create the new OTEL-based Langfuse tracer internally
	otelTracer, err := NewOTELLangfuseTracer(tracerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTEL Langfuse tracer: %w", err)
	}

	return &LangfuseTracer{
		otelTracer: otelTracer,
		enabled:    true,
	}, nil
}

// TraceGeneration traces an LLM generation (delegates to OTEL implementation)
func (t *LangfuseTracer) TraceGeneration(ctx context.Context, modelName string, prompt string, response string, startTime time.Time, endTime time.Time, metadata map[string]interface{}) (string, error) {
	if !t.enabled || t.otelTracer == nil {
		return "", nil
	}
	return t.otelTracer.TraceGeneration(ctx, modelName, prompt, response, startTime, endTime, metadata)
}

// TraceSpan traces a span of execution (delegates to OTEL implementation)
func (t *LangfuseTracer) TraceSpan(ctx context.Context, name string, startTime time.Time, endTime time.Time, metadata map[string]interface{}, parentID string) (string, error) {
	if !t.enabled || t.otelTracer == nil {
		return "", nil
	}
	return t.otelTracer.TraceSpan(ctx, name, startTime, endTime, metadata, parentID)
}

// TraceEvent traces an event (delegates to OTEL implementation)
func (t *LangfuseTracer) TraceEvent(ctx context.Context, name string, input interface{}, output interface{}, level string, metadata map[string]interface{}, parentID string) (string, error) {
	if !t.enabled || t.otelTracer == nil {
		return "", nil
	}
	return t.otelTracer.TraceEvent(ctx, name, input, output, level, metadata, parentID)
}

// Flush flushes the Langfuse tracer (delegates to OTEL implementation)
func (t *LangfuseTracer) Flush() error {
	if !t.enabled || t.otelTracer == nil {
		return nil
	}
	return t.otelTracer.Flush()
}

// Shutdown shuts down the tracer (delegates to OTEL implementation)
func (t *LangfuseTracer) Shutdown() error {
	if !t.enabled || t.otelTracer == nil {
		return nil
	}
	return t.otelTracer.Shutdown()
}

// AsInterfaceTracer returns an interfaces.Tracer compatible adapter
// This allows the backward-compatible tracer to work with Agents
func (t *LangfuseTracer) AsInterfaceTracer() interfaces.Tracer {
	if !t.enabled || t.otelTracer == nil {
		return nil
	}
	return NewOTELTracerAdapter(t.otelTracer)
}

// LLMMiddleware implements middleware for LLM calls with Langfuse tracing (backward compatibility)
// This now uses the reliable OTEL-based implementation internally
type LLMMiddleware struct {
	llm    interfaces.LLM
	tracer *LangfuseTracer
}

// NewLLMMiddleware creates a new LLM middleware with Langfuse tracing (backward compatibility wrapper)
// This now uses the reliable OTEL-based implementation internally
func NewLLMMiddleware(llm interfaces.LLM, tracer *LangfuseTracer) *LLMMiddleware {
	return &LLMMiddleware{
		llm:    llm,
		tracer: tracer,
	}
}

// Generate generates text from a prompt with Langfuse tracing (delegates to OTEL implementation)
func (m *LLMMiddleware) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	if !m.tracer.enabled || m.tracer.otelTracer == nil {
		// If tracing is disabled, just call the underlying LLM
		return m.llm.Generate(ctx, prompt, options...)
	}

	// Use the OTEL-based LLM middleware internally
	otelMiddleware := NewOTELLLMMiddleware(m.llm, m.tracer.otelTracer)
	return otelMiddleware.Generate(ctx, prompt, options...)
}

// GenerateWithTools generates text from a prompt with tools (delegates to OTEL implementation)
func (m *LLMMiddleware) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	if !m.tracer.enabled || m.tracer.otelTracer == nil {
		// If tracing is disabled, call the underlying LLM if it supports GenerateWithTools
		if llmWithTools, ok := m.llm.(interface {
			GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error)
		}); ok {
			return llmWithTools.GenerateWithTools(ctx, prompt, tools, options...)
		}
		return m.llm.Generate(ctx, prompt, options...)
	}

	// Use the OTEL-based LLM middleware internally
	otelMiddleware := NewOTELLLMMiddleware(m.llm, m.tracer.otelTracer)
	return otelMiddleware.GenerateWithTools(ctx, prompt, tools, options...)
}

// Name implements interfaces.LLM.Name
func (m *LLMMiddleware) Name() string {
	return m.llm.Name()
}

// SupportsStreaming implements interfaces.LLM.SupportsStreaming
func (m *LLMMiddleware) SupportsStreaming() bool {
	return m.llm.SupportsStreaming()
}

// GenerateStream implements interfaces.StreamingLLM.GenerateStream
func (m *LLMMiddleware) GenerateStream(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	if !m.tracer.enabled || m.tracer.otelTracer == nil {
		// If tracing is disabled, call the underlying LLM if it supports streaming
		if streamingLLM, ok := m.llm.(interfaces.StreamingLLM); ok {
			return streamingLLM.GenerateStream(ctx, prompt, options...)
		}
		return nil, fmt.Errorf("underlying LLM does not support streaming")
	}

	// Use the OTEL-based LLM middleware for streaming
	otelMiddleware := NewOTELLLMMiddleware(m.llm, m.tracer.otelTracer)
	return otelMiddleware.GenerateStream(ctx, prompt, options...)
}

// GenerateWithToolsStream implements interfaces.StreamingLLM.GenerateWithToolsStream
func (m *LLMMiddleware) GenerateWithToolsStream(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	if !m.tracer.enabled || m.tracer.otelTracer == nil {
		// If tracing is disabled, call the underlying LLM if it supports streaming
		if streamingLLM, ok := m.llm.(interfaces.StreamingLLM); ok {
			return streamingLLM.GenerateWithToolsStream(ctx, prompt, tools, options...)
		}
		return nil, fmt.Errorf("underlying LLM does not support streaming")
	}

	// Use the OTEL-based LLM middleware for streaming
	otelMiddleware := NewOTELLLMMiddleware(m.llm, m.tracer.otelTracer)
	return otelMiddleware.GenerateWithToolsStream(ctx, prompt, tools, options...)
}
