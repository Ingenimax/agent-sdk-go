package tracing

import (
	"context"
	"fmt"
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
