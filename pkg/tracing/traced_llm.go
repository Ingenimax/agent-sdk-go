package tracing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// TracedLLM implements middleware for LLM calls with unified tracing
type TracedLLM struct {
	llm    interfaces.LLM
	tracer interfaces.Tracer
}

// NewTracedLLM creates a new LLM middleware with unified tracing
func NewTracedLLM(llm interfaces.LLM, tracer interfaces.Tracer) interfaces.LLM {
	return &TracedLLM{
		llm:    llm,
		tracer: tracer,
	}
}

// Generate generates text from a prompt with tracing
func (m *TracedLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	startTime := time.Now()

	// Start span
	ctx, span := m.tracer.StartSpan(ctx, "llm.generate")
	defer span.End()

	// Add attributes
	span.SetAttribute("prompt.length", len(prompt))
	span.SetAttribute("prompt.hash", hashString(prompt))

	// Extract model name from LLM client
	model := "unknown"
	if modelProvider, ok := m.llm.(interface{ GetModel() string }); ok {
		model = modelProvider.GetModel()
	}
	if model == "" {
		model = m.llm.Name() // fallback to provider name
	}
	span.SetAttribute("model", model)

	// Call the underlying LLM
	response, err := m.llm.Generate(ctx, prompt, options...)

	endTime := time.Now()
	duration := endTime.Sub(startTime)

	// Add response attributes
	if err == nil {
		span.SetAttribute("response.length", len(response))
		span.SetAttribute("response.hash", hashString(response))
		span.SetAttribute("duration_ms", duration.Milliseconds())
	} else {
		span.RecordError(err)
	}

	return response, err
}

// GenerateWithTools generates text from a prompt with tools using unified tracing
func (m *TracedLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	// First check if underlying LLM supports GenerateWithTools
	if llmWithTools, ok := m.llm.(interface {
		GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error)
	}); ok {
		startTime := time.Now()

		// Start span
		ctx, span := m.tracer.StartSpan(ctx, "llm.generate_with_tools")
		defer span.End()

		// Add attributes
		span.SetAttribute("prompt.length", len(prompt))
		span.SetAttribute("prompt.hash", hashString(prompt))
		span.SetAttribute("tools.count", len(tools))

		// Extract model name from LLM client
		model := "unknown"
		if modelProvider, ok := m.llm.(interface{ GetModel() string }); ok {
			model = modelProvider.GetModel()
		}
		if model == "" {
			model = m.llm.Name() // fallback to provider name
		}
		span.SetAttribute("model", model)

		// Add tool names if available
		if len(tools) > 0 {
			toolNames := make([]string, len(tools))
			for i, tool := range tools {
				toolNames[i] = tool.Name()
			}
			span.SetAttribute("tools", strings.Join(toolNames, ","))
		}

		// Initialize tool calls collection in context for tracing
		ctx = WithToolCallsCollection(ctx)

		// Call the underlying LLM's GenerateWithTools method
		response, err := llmWithTools.GenerateWithTools(ctx, prompt, tools, options...)

		endTime := time.Now()
		duration := endTime.Sub(startTime)

		// Add response attributes
		if err == nil {
			span.SetAttribute("response.length", len(response))
			span.SetAttribute("response.hash", hashString(response))
			span.SetAttribute("duration_ms", duration.Milliseconds())
		} else {
			span.RecordError(err)
		}

		return response, err
	}

	// Fallback to regular Generate if GenerateWithTools is not supported
	return m.Generate(ctx, prompt, options...)
}

// Name implements interfaces.LLM.Name
func (m *TracedLLM) Name() string {
	return m.llm.Name()
}

// SupportsStreaming implements interfaces.LLM.SupportsStreaming
func (m *TracedLLM) SupportsStreaming() bool {
	return m.llm.SupportsStreaming()
}

// GenerateStream implements interfaces.StreamingLLM.GenerateStream
func (m *TracedLLM) GenerateStream(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	// Check if underlying LLM supports streaming
	streamingLLM, ok := m.llm.(interfaces.StreamingLLM)
	if !ok {
		return nil, fmt.Errorf("underlying LLM does not support streaming")
	}

	// Start span
	ctx, span := m.tracer.StartSpan(ctx, "llm.generate_stream")
	defer span.End()

	// Add attributes
	span.SetAttribute("prompt.length", len(prompt))
	span.SetAttribute("prompt.hash", hashString(prompt))
	span.SetAttribute("streaming", true)

	// Extract model name from LLM client
	model := "unknown"
	if modelProvider, ok := m.llm.(interface{ GetModel() string }); ok {
		model = modelProvider.GetModel()
	}
	if model == "" {
		model = m.llm.Name() // fallback to provider name
	}
	span.SetAttribute("model", model)

	return streamingLLM.GenerateStream(ctx, prompt, options...)
}

// GenerateWithToolsStream implements interfaces.StreamingLLM.GenerateWithToolsStream
func (m *TracedLLM) GenerateWithToolsStream(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	// Check if underlying LLM supports streaming with tools
	streamingLLM, ok := m.llm.(interfaces.StreamingLLM)
	if !ok {
		return nil, fmt.Errorf("underlying LLM does not support streaming")
	}

	// Start span
	startTime := time.Now()
	ctx, span := m.tracer.StartSpan(ctx, "llm.generate_with_tools_stream")

	// Add attributes
	span.SetAttribute("prompt.length", len(prompt))
	span.SetAttribute("prompt.hash", hashString(prompt))
	span.SetAttribute("streaming", true)
	span.SetAttribute("tools.count", len(tools))

	// Extract model name from LLM client
	model := "unknown"
	if modelProvider, ok := m.llm.(interface{ GetModel() string }); ok {
		model = modelProvider.GetModel()
	}
	if model == "" {
		model = m.llm.Name() // fallback to provider name
	}
	span.SetAttribute("model", model)

	// Add tool names if available
	if len(tools) > 0 {
		toolNames := make([]string, len(tools))
		for i, tool := range tools {
			toolNames[i] = tool.Name()
		}
		span.SetAttribute("tools", strings.Join(toolNames, ","))
	}

	// Initialize tool calls collection in context for tracing
	ctx = WithToolCallsCollection(ctx)

	// Get the original stream
	originalChan, err := streamingLLM.GenerateWithToolsStream(ctx, prompt, tools, options...)
	if err != nil {
		span.RecordError(err)
		span.End()
		return nil, err
	}

	// Create a new channel to wrap the original
	wrappedChan := make(chan interfaces.StreamEvent, 10)

	// Start a goroutine to proxy events and handle span completion
	go func() {
		defer close(wrappedChan)
		defer func() {
			// When streaming is complete, create tool call spans and end main span
			endTime := time.Now()
			duration := endTime.Sub(startTime)
			span.SetAttribute("duration_ms", duration.Milliseconds())

			// Get tool calls from context and create spans using TraceGeneration if any exist
			toolCalls := GetToolCallsFromContext(ctx)

			if len(toolCalls) > 0 {
				// Extract model name
				model := "unknown"
				if modelProvider, ok := streamingLLM.(interface{ GetModel() string }); ok {
					model = modelProvider.GetModel()
				}
				if model == "" {
					model = streamingLLM.Name()
				}

				// Create spans using TraceGeneration which handles tool calls correctly
				if adapter, ok := m.tracer.(*OTELTracerAdapter); ok {
					adapter.otelTracer.TraceGeneration(ctx, model, prompt, "streaming_response", startTime, endTime, map[string]any{
						"streaming": true,
						"tools":     len(tools),
					})
				} else if tracer, ok := m.tracer.(*OTELLangfuseTracer); ok {
					tracer.TraceGeneration(ctx, model, prompt, "streaming_response", startTime, endTime, map[string]any{
						"streaming": true,
						"tools":     len(tools),
					})
				}
			}

			span.End()
		}()

		// Proxy all events from the original channel
		for event := range originalChan {
			wrappedChan <- event
		}
	}()

	return wrappedChan, nil
}
