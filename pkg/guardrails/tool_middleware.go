package guardrails

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// ToolMiddleware implements middleware for tool calls
type ToolMiddleware struct {
	tool     interfaces.Tool
	pipeline *Pipeline
}

// NewToolMiddleware creates a new tool middleware
func NewToolMiddleware(tool interfaces.Tool, pipeline *Pipeline) *ToolMiddleware {
	return &ToolMiddleware{
		tool:     tool,
		pipeline: pipeline,
	}
}

// Name returns the name of the tool
func (m *ToolMiddleware) Name() string {
	return m.tool.Name()
}

// Description returns a description of what the tool does
func (m *ToolMiddleware) Description() string {
	return m.tool.Description()
}

// Parameters returns the parameters that the tool accepts
func (m *ToolMiddleware) Parameters() map[string]interfaces.ParameterSpec {
	return m.tool.Parameters()
}

// Run executes the tool with the given input
func (m *ToolMiddleware) Run(ctx context.Context, input string) (string, error) {
	// Process request through guardrails
	processedInput, err := m.pipeline.ProcessRequest(ctx, input)
	if err != nil {
		return "", err
	}

	// Call the underlying tool
	output, err := m.tool.Run(ctx, processedInput)
	if err != nil {
		return "", err
	}

	// Process response through guardrails
	processedOutput, err := m.pipeline.ProcessResponse(ctx, output)
	if err != nil {
		return "", err
	}

	return processedOutput, nil
}

// Execute executes the tool with the given arguments, applying guardrails
func (m *ToolMiddleware) Execute(ctx context.Context, args string) (string, error) {
	// Process request through guardrails
	processedInput, err := m.pipeline.ProcessRequest(ctx, args)
	if err != nil {
		return "", err
	}

	// Call the underlying tool
	output, err := m.tool.Execute(ctx, processedInput)
	if err != nil {
		return "", err
	}

	// Process response through guardrails
	processedOutput, err := m.pipeline.ProcessResponse(ctx, output)
	if err != nil {
		return "", err
	}

	return processedOutput, nil
}
