package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/tracing"
)

// AgentTool wraps an agent to make it callable as a tool
type AgentTool struct {
	agent       SubAgent
	name        string
	description string
	timeout     time.Duration
	logger      logging.Logger
	tracer      interfaces.Tracer
}

// SubAgent interface defines the minimal interface needed for a sub-agent
type SubAgent interface {
	Run(ctx context.Context, input string) (string, error)
	GetName() string
	GetDescription() string
}

// NewAgentTool creates a new agent tool wrapper
func NewAgentTool(agent SubAgent) *AgentTool {
	return &AgentTool{
		agent:       agent,
		name:        fmt.Sprintf("%s_agent", agent.GetName()),
		description: agent.GetDescription(),
		timeout:     30 * time.Second, // Default timeout
		logger:      logging.New(),    // Default logger
	}
}

// WithTimeout sets a custom timeout for the agent tool
func (at *AgentTool) WithTimeout(timeout time.Duration) *AgentTool {
	at.timeout = timeout
	return at
}

// WithLogger sets a custom logger for the agent tool
func (at *AgentTool) WithLogger(logger logging.Logger) *AgentTool {
	at.logger = logger
	return at
}

// WithTracer sets a custom tracer for the agent tool
func (at *AgentTool) WithTracer(tracer interfaces.Tracer) *AgentTool {
	at.tracer = tracer
	return at
}

// Name returns the name of the tool
func (at *AgentTool) Name() string {
	return at.name
}

// Description returns the description of what the tool does
func (at *AgentTool) Description() string {
	if at.description != "" {
		return at.description
	}
	return fmt.Sprintf("Delegate task to %s agent for specialized handling", at.agent.GetName())
}

// Parameters returns the parameters that the tool accepts
func (at *AgentTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"query": {
			Type:        "string",
			Description: fmt.Sprintf("The query or task to send to the %s agent", at.agent.GetName()),
			Required:    true,
		},
		"context": {
			Type:        "object",
			Description: "Optional context information for the sub-agent",
			Required:    false,
		},
	}
}

// Run executes the tool with the given input
func (at *AgentTool) Run(ctx context.Context, input string) (string, error) {
	startTime := time.Now()
	agentName := at.agent.GetName()
	
	// Start tracing span if tracer is available
	var span interfaces.Span
	if at.tracer != nil {
		ctx, span = at.tracer.StartSpan(ctx, fmt.Sprintf("sub_agent.%s", agentName))
		defer span.End()
		
		// Add span attributes
		span.SetAttribute("sub_agent.name", agentName)
		span.SetAttribute("sub_agent.input", input)
		span.SetAttribute("sub_agent.tool_name", at.name)
	}

	// Add agent name to context for tracing
	ctx = tracing.WithAgentName(ctx, agentName)
	
	// Check recursion depth
	depth := at.getRecursionDepth(ctx)
	if depth > 5 { // Maximum recursion depth
		err := fmt.Errorf("maximum sub-agent recursion depth exceeded")
		if span != nil {
			span.AddEvent("error", map[string]interface{}{
				"error": err.Error(),
			})
			span.SetAttribute("sub_agent.error", err.Error())
		}
		at.logger.Error(ctx, "Sub-agent recursion depth exceeded", map[string]interface{}{
			"sub_agent":       agentName,
			"recursion_depth": depth,
			"max_depth":       5,
		})
		return "", err
	}
	
	// Update context with sub-agent metadata
	ctx = context.WithValue(ctx, "sub_agent_name", agentName)
	ctx = context.WithValue(ctx, "parent_agent", "main")
	ctx = context.WithValue(ctx, "recursion_depth", depth+1)
	
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, at.timeout)
	defer cancel()

	// Log sub-agent invocation with debug details
	at.logger.Debug(ctx, "Invoking sub-agent", map[string]interface{}{
		"sub_agent":       agentName,
		"tool_name":       at.name,
		"input_prompt":    input,
		"recursion_depth": depth + 1,
		"timeout":         at.timeout.String(),
	})

	// Run the sub-agent
	result, err := at.agent.Run(ctx, input)
	duration := time.Since(startTime)
	
	if err != nil {
		// Log error details
		at.logger.Error(ctx, "Sub-agent execution failed", map[string]interface{}{
			"sub_agent": agentName,
			"tool_name": at.name,
			"error":     err.Error(),
			"duration":  duration.String(),
			"input":     input,
		})
		
		// Record error in span
		if span != nil {
			span.AddEvent("error", map[string]interface{}{
				"error": err.Error(),
			})
			span.SetAttribute("sub_agent.error", err.Error())
			span.SetAttribute("sub_agent.duration_ms", duration.Milliseconds())
		}
		
		return "", fmt.Errorf("sub-agent %s failed: %w", agentName, err)
	}

	// Log successful execution with response details
	at.logger.Debug(ctx, "Sub-agent execution completed", map[string]interface{}{
		"sub_agent":     agentName,
		"tool_name":     at.name,
		"input_prompt":  input,
		"response":      result,
		"duration":      duration.String(),
		"response_len":  len(result),
	})
	
	// Record success in span
	if span != nil {
		span.SetAttribute("sub_agent.response", result)
		span.SetAttribute("sub_agent.duration_ms", duration.Milliseconds())
		span.SetAttribute("sub_agent.response_length", len(result))
		span.SetAttribute("sub_agent.success", true)
	}

	return result, nil
}

// Execute implements interfaces.Tool.Execute
func (at *AgentTool) Execute(ctx context.Context, args string) (string, error) {
	agentName := at.agent.GetName()
	
	// Log the tool execution start
	at.logger.Debug(ctx, "Sub-agent tool execution started", map[string]interface{}{
		"sub_agent": agentName,
		"tool_name": at.name,
		"raw_args":  args,
	})
	
	// Parse the JSON arguments
	var params struct {
		Query   string                 `json:"query"`
		Context map[string]interface{} `json:"context,omitempty"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		at.logger.Error(ctx, "Failed to parse sub-agent tool arguments", map[string]interface{}{
			"sub_agent": agentName,
			"tool_name": at.name,
			"raw_args":  args,
			"error":     err.Error(),
		})
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if params.Query == "" {
		at.logger.Error(ctx, "Sub-agent tool called with empty query", map[string]interface{}{
			"sub_agent": agentName,
			"tool_name": at.name,
			"args":      args,
		})
		return "", fmt.Errorf("query parameter is required")
	}

	// Log parsed parameters
	at.logger.Debug(ctx, "Sub-agent tool parameters parsed", map[string]interface{}{
		"sub_agent":       agentName,
		"tool_name":       at.name,
		"parsed_query":    params.Query,
		"parsed_context":  params.Context,
	})

	// If context is provided, add it to the context
	if params.Context != nil {
		for key, value := range params.Context {
			ctx = context.WithValue(ctx, key, value)
		}
	}

	return at.Run(ctx, params.Query)
}

// getRecursionDepth retrieves the current recursion depth from context
func (at *AgentTool) getRecursionDepth(ctx context.Context) int {
	if depth, ok := ctx.Value("recursion_depth").(int); ok {
		return depth
	}
	return 0
}

// SetDescription allows updating the tool description
func (at *AgentTool) SetDescription(description string) {
	at.description = description
}