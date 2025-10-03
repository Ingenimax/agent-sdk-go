package tracing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// OTELLLMMiddleware implements middleware for LLM calls with OTEL-based Langfuse tracing
type OTELLLMMiddleware struct {
	llm    interfaces.LLM
	tracer *OTELLangfuseTracer
}

// NewOTELLLMMiddleware creates a new LLM middleware with OTEL-based Langfuse tracing
func NewOTELLLMMiddleware(llm interfaces.LLM, tracer *OTELLangfuseTracer) *OTELLLMMiddleware {
	return &OTELLLMMiddleware{
		llm:    llm,
		tracer: tracer,
	}
}

// Generate generates text from a prompt with OTEL-based Langfuse tracing
func (m *OTELLLMMiddleware) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	startTime := time.Now()

	// Initialize tool calls collection in context (even for regular generation, in case tools are used internally)
	ctx = WithToolCallsCollection(ctx)

	// Call the underlying LLM
	response, err := m.llm.Generate(ctx, prompt, options...)

	endTime := time.Now()

	// Extract model name from LLM client
	model := "unknown"
	if modelProvider, ok := m.llm.(interface{ GetModel() string }); ok {
		model = modelProvider.GetModel()
	}
	if model == "" {
		model = m.llm.Name() // fallback to provider name
	}
	// Create metadata from options
	metadata := map[string]interface{}{
		"options": fmt.Sprintf("%v", options),
	}

	// Trace the generation
	if err == nil {
		_, traceErr := m.tracer.TraceGeneration(ctx, model, prompt, response, startTime, endTime, metadata)
		if traceErr != nil {
			// Log the error but don't fail the request
			fmt.Printf("Failed to trace generation: %v\n", traceErr)
		}
	} else {
		// Trace error
		errorMetadata := map[string]interface{}{
			"options": fmt.Sprintf("%v", options),
			"error":   err.Error(),
		}
		_, traceErr := m.tracer.TraceEvent(ctx, "llm_error", prompt, nil, "error", errorMetadata, "")
		if traceErr != nil {
			// Log the error but don't fail the request
			fmt.Printf("Failed to trace error: %v\n", traceErr)
		}
	}

	return response, err
}

// GenerateWithTools generates text from a prompt with tools using OTEL-based tracing
func (m *OTELLLMMiddleware) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	// First check if underlying LLM supports GenerateWithTools
	if llmWithTools, ok := m.llm.(interface {
		GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error)
	}); ok {
		startTime := time.Now()

		// Initialize tool calls collection in context
		ctx = WithToolCallsCollection(ctx)

		// Call the underlying LLM's GenerateWithTools method
		response, err := llmWithTools.GenerateWithTools(ctx, prompt, tools, options...)

		endTime := time.Now()

		// Extract model name from LLM client
		model := "unknown"
		if modelProvider, ok := m.llm.(interface{ GetModel() string }); ok {
			model = modelProvider.GetModel()
		}
		if model == "" {
			model = m.llm.Name() // fallback to provider name
		}
		// Create metadata including tool information
		metadata := map[string]interface{}{
			"options":    fmt.Sprintf("%v", options),
			"tool_count": len(tools),
		}
		if len(tools) > 0 {
			toolNames := make([]string, len(tools))
			for i, tool := range tools {
				toolNames[i] = tool.Name()
			}
			metadata["tools"] = toolNames
		}

		// Trace the generation
		if err == nil {
			_, traceErr := m.tracer.TraceGeneration(ctx, model, prompt, response, startTime, endTime, metadata)
			if traceErr != nil {
				// Log the error but don't fail the request
				fmt.Printf("Failed to trace generation with tools: %v\n", traceErr)
			}
		} else {
			// Trace error
			errorMetadata := map[string]interface{}{
				"options":    fmt.Sprintf("%v", options),
				"tool_count": len(tools),
				"error":      err.Error(),
			}
			_, traceErr := m.tracer.TraceEvent(ctx, "llm_tools_error", prompt, nil, "error", errorMetadata, "")
			if traceErr != nil {
				// Log the error but don't fail the request
				fmt.Printf("Failed to trace tools error: %v\n", traceErr)
			}
		}

		return response, err
	}

	// Fallback to regular Generate if GenerateWithTools is not supported
	return m.Generate(ctx, prompt, options...)
}

// Name implements interfaces.LLM.Name
func (m *OTELLLMMiddleware) Name() string {
	return m.llm.Name()
}

// SupportsStreaming implements interfaces.LLM.SupportsStreaming
func (m *OTELLLMMiddleware) SupportsStreaming() bool {
	return m.llm.SupportsStreaming()
}

// GenerateStream implements interfaces.StreamingLLM.GenerateStream
func (m *OTELLLMMiddleware) GenerateStream(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	// Check if underlying LLM supports streaming
	streamingLLM, ok := m.llm.(interfaces.StreamingLLM)
	if !ok {
		return nil, fmt.Errorf("underlying LLM does not support streaming")
	}

	startTime := time.Now()

	// Initialize tool calls collection in context
	ctx = WithToolCallsCollection(ctx)

	// Create channel for tracing stream events
	originalChan, err := streamingLLM.GenerateStream(ctx, prompt, options...)
	if err != nil {
		return nil, err
	}

	// Create a new channel to forward events while tracing
	tracedChan := make(chan interfaces.StreamEvent, 100)

	go func() {
		defer close(tracedChan)

		var responseBuilder strings.Builder
		var lastError error

		for event := range originalChan {
			// Forward the event first
			select {
			case tracedChan <- event:
			case <-ctx.Done():
				return
			}

			// Collect response content for tracing
			if event.Type == interfaces.StreamEventContentDelta && event.Content != "" {
				responseBuilder.WriteString(event.Content)
			}

			// Track errors
			if event.Error != nil {
				lastError = event.Error
			}
		}

		// Trace the complete generation when streaming is done
		endTime := time.Now()
		response := responseBuilder.String()

		// Extract model name from LLM client
		model := "unknown"
		if modelProvider, ok := m.llm.(interface{ GetModel() string }); ok {
			model = modelProvider.GetModel()
		}
		if model == "" {
			model = m.llm.Name() // fallback to provider name
		}

		// Create metadata from options
		metadata := map[string]interface{}{
			"options":   fmt.Sprintf("%v", options),
			"streaming": true,
		}

		// Trace the generation
		if lastError == nil && response != "" {
			_, traceErr := m.tracer.TraceGeneration(ctx, model, prompt, response, startTime, endTime, metadata)
			if traceErr != nil {
				// Log the error but don't fail the request
				fmt.Printf("Failed to trace streaming generation: %v\n", traceErr)
			}
		} else if lastError != nil {
			// Trace error
			errorMetadata := map[string]interface{}{
				"options":   fmt.Sprintf("%v", options),
				"streaming": true,
				"error":     lastError.Error(),
			}
			_, traceErr := m.tracer.TraceEvent(ctx, "llm_stream_error", prompt, nil, "error", errorMetadata, "")
			if traceErr != nil {
				// Log the error but don't fail the request
				fmt.Printf("Failed to trace streaming error: %v\n", traceErr)
			}
		}
	}()

	return tracedChan, nil
}

// GenerateWithToolsStream implements interfaces.StreamingLLM.GenerateWithToolsStream
func (m *OTELLLMMiddleware) GenerateWithToolsStream(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	// Check if underlying LLM supports streaming with tools
	streamingLLM, ok := m.llm.(interfaces.StreamingLLM)
	if !ok {
		return nil, fmt.Errorf("underlying LLM does not support streaming")
	}

	startTime := time.Now()

	// Initialize tool calls collection in context
	ctx = WithToolCallsCollection(ctx)

	// Create channel for tracing stream events
	originalChan, err := streamingLLM.GenerateWithToolsStream(ctx, prompt, tools, options...)
	if err != nil {
		return nil, err
	}

	// Create a new channel to forward events while tracing
	tracedChan := make(chan interfaces.StreamEvent, 100)

	go func() {
		defer close(tracedChan)

		var responseBuilder strings.Builder
		var lastError error

		for event := range originalChan {
			// Forward the event first
			select {
			case tracedChan <- event:
			case <-ctx.Done():
				return
			}

			// Collect response content for tracing
			if event.Type == interfaces.StreamEventContentDelta && event.Content != "" {
				responseBuilder.WriteString(event.Content)
			}

			// Track errors
			if event.Error != nil {
				lastError = event.Error
			}
		}

		// Trace the complete generation when streaming is done
		endTime := time.Now()
		response := responseBuilder.String()

		// Extract model name from LLM client
		model := "unknown"
		if modelProvider, ok := m.llm.(interface{ GetModel() string }); ok {
			model = modelProvider.GetModel()
		}
		if model == "" {
			model = m.llm.Name() // fallback to provider name
		}

		// Create metadata including tool information
		metadata := map[string]interface{}{
			"options":    fmt.Sprintf("%v", options),
			"streaming":  true,
			"tool_count": len(tools),
		}
		if len(tools) > 0 {
			toolNames := make([]string, len(tools))
			for i, tool := range tools {
				toolNames[i] = tool.Name()
			}
			metadata["tools"] = toolNames
		}

		// Trace the generation
		if lastError == nil && response != "" {
			_, traceErr := m.tracer.TraceGeneration(ctx, model, prompt, response, startTime, endTime, metadata)
			if traceErr != nil {
				// Log the error but don't fail the request
				fmt.Printf("Failed to trace streaming generation with tools: %v\n", traceErr)
			}
		} else if lastError != nil {
			// Trace error
			errorMetadata := map[string]interface{}{
				"options":    fmt.Sprintf("%v", options),
				"streaming":  true,
				"tool_count": len(tools),
				"error":      lastError.Error(),
			}
			_, traceErr := m.tracer.TraceEvent(ctx, "llm_stream_tools_error", prompt, nil, "error", errorMetadata, "")
			if traceErr != nil {
				// Log the error but don't fail the request
				fmt.Printf("Failed to trace streaming tools error: %v\n", traceErr)
			}
		}
	}()

	return tracedChan, nil
}
