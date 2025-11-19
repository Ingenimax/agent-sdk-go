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

// Context keys for sub-agent metadata
type contextKey string

const (
	recursionDepthKey contextKey = "recursion_depth"
	subAgentNameKey   contextKey = "sub_agent_name"
	parentAgentKey    contextKey = "parent_agent"
	invocationIDKey   contextKey = "invocation_id"

	// MaxRecursionDepth is the maximum allowed recursion depth
	MaxRecursionDepth = 5
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
	RunDetailed(ctx context.Context, input string) (*interfaces.AgentResponse, error)
	GetName() string
	GetDescription() string
}

// NewAgentTool creates a new agent tool wrapper
func NewAgentTool(agent SubAgent) *AgentTool {
	return &AgentTool{
		agent:       agent,
		name:        fmt.Sprintf("%s_agent", agent.GetName()),
		description: agent.GetDescription(),
		timeout:     12 * time.Minute, // 12 minutes - shorter than HTTP client timeout
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

// DisplayName implements interfaces.ToolWithDisplayName.DisplayName
func (at *AgentTool) DisplayName() string {
	return fmt.Sprintf("%s Agent", at.agent.GetName())
}

// Description returns the description of what the tool does
func (at *AgentTool) Description() string {
	if at.description != "" {
		return at.description
	}
	return fmt.Sprintf("Delegate task to %s agent for specialized handling", at.agent.GetName())
}

// Internal implements interfaces.InternalTool.Internal
func (at *AgentTool) Internal() bool {
	return false
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
	depth := getRecursionDepth(ctx)
	if depth > MaxRecursionDepth {
		err := fmt.Errorf("maximum recursion depth %d exceeded (current: %d)", MaxRecursionDepth, depth)
		if span != nil {
			span.AddEvent("error", map[string]interface{}{
				"error": err.Error(),
			})
			span.SetAttribute("sub_agent.error", err.Error())
		}
		at.logger.Error(ctx, "Sub-agent recursion depth exceeded", map[string]interface{}{
			"sub_agent":       agentName,
			"recursion_depth": depth,
			"max_depth":       MaxRecursionDepth,
		})
		return "", err
	}

	// Update context with sub-agent metadata
	ctx = context.WithValue(ctx, subAgentNameKey, agentName)
	ctx = context.WithValue(ctx, parentAgentKey, "main")
	ctx = context.WithValue(ctx, recursionDepthKey, depth+1)

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

	// Run the sub-agent with detailed tracking
	response, err := at.agent.RunDetailed(ctx, input)
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

	// Log comprehensive execution details
	executionDetails := map[string]interface{}{
		"sub_agent":         agentName,
		"tool_name":         at.name,
		"input_prompt":      input,
		"response_content":  response.Content,
		"response_length":   len(response.Content),
		"duration":          duration.String(),
		"agent_name":        response.AgentName,
		"model_used":        response.Model,
		"llm_calls":         response.ExecutionSummary.LLMCalls,
		"tool_calls":        response.ExecutionSummary.ToolCalls,
		"sub_agent_calls":   response.ExecutionSummary.SubAgentCalls,
		"execution_time_ms": response.ExecutionSummary.ExecutionTimeMs,
		"used_tools":        response.ExecutionSummary.UsedTools,
		"used_sub_agents":   response.ExecutionSummary.UsedSubAgents,
	}

	// Add token usage details if available
	if response.Usage != nil {
		executionDetails["input_tokens"] = response.Usage.InputTokens
		executionDetails["output_tokens"] = response.Usage.OutputTokens
		executionDetails["total_tokens"] = response.Usage.TotalTokens
		executionDetails["reasoning_tokens"] = response.Usage.ReasoningTokens
	}

	// Add metadata if available
	if response.Metadata != nil {
		executionDetails["metadata"] = response.Metadata
	}

	at.logger.Info(ctx, "Sub-agent execution completed with detailed tracking", executionDetails)

	// Record detailed success information in span
	if span != nil {
		span.SetAttribute("sub_agent.response", response.Content)
		span.SetAttribute("sub_agent.duration_ms", duration.Milliseconds())
		span.SetAttribute("sub_agent.response_length", len(response.Content))
		span.SetAttribute("sub_agent.success", true)
		span.SetAttribute("sub_agent.agent_name", response.AgentName)
		span.SetAttribute("sub_agent.model_used", response.Model)
		span.SetAttribute("sub_agent.llm_calls", response.ExecutionSummary.LLMCalls)
		span.SetAttribute("sub_agent.tool_calls", response.ExecutionSummary.ToolCalls)
		span.SetAttribute("sub_agent.sub_agent_calls", response.ExecutionSummary.SubAgentCalls)
		span.SetAttribute("sub_agent.execution_time_ms", response.ExecutionSummary.ExecutionTimeMs)

		// Add token usage to span if available
		if response.Usage != nil {
			span.SetAttribute("sub_agent.input_tokens", response.Usage.InputTokens)
			span.SetAttribute("sub_agent.output_tokens", response.Usage.OutputTokens)
			span.SetAttribute("sub_agent.total_tokens", response.Usage.TotalTokens)
			span.SetAttribute("sub_agent.reasoning_tokens", response.Usage.ReasoningTokens)
		}
	}

	return response.Content, nil
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
		"sub_agent":      agentName,
		"tool_name":      at.name,
		"parsed_query":   params.Query,
		"parsed_context": params.Context,
	})

	// If context is provided, add it to the context
	if params.Context != nil {
		for key, value := range params.Context {
			ctx = context.WithValue(ctx, contextKey(key), value)
		}
	}

	return at.Run(ctx, params.Query)
}

// SetDescription allows updating the tool description
func (at *AgentTool) SetDescription(description string) {
	at.description = description
}

// getRecursionDepth retrieves the current recursion depth from context
func getRecursionDepth(ctx context.Context) int {
	if depth, ok := ctx.Value(recursionDepthKey).(int); ok {
		return depth
	}
	return 0
}

// withSubAgentContext adds sub-agent context information for testing purposes
func withSubAgentContext(ctx context.Context, parentAgent, subAgentName string) context.Context {
	depth := getRecursionDepth(ctx)
	ctx = context.WithValue(ctx, subAgentNameKey, subAgentName)
	ctx = context.WithValue(ctx, parentAgentKey, parentAgent)
	ctx = context.WithValue(ctx, recursionDepthKey, depth+1)
	return ctx
}
