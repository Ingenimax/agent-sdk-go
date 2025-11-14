package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	streamEventChan   contextKey = "stream_event_chan" // For forwarding stream events

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
	GetName() string
	GetDescription() string
}

// StreamingSubAgent extends SubAgent with streaming capabilities
type StreamingSubAgent interface {
	SubAgent
	RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error)
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

	// Check if agent supports streaming and if parent has provided a stream event channel
	streamingAgent, supportsStreaming := at.agent.(StreamingSubAgent)
	parentEventChan, hasStreamContext := ctx.Value(streamEventChan).(chan<- interfaces.AgentStreamEvent)

	// Log detailed streaming capability check
	at.logger.Info(ctx, "Sub-agent streaming capability check", map[string]interface{}{
		"sub_agent":       agentName,
		"tool_name":       at.name,
		"supports_stream": supportsStreaming,
		"has_stream_ctx":  hasStreamContext,
	})

	var result string
	var err error

	// If both streaming is supported and parent is listening for stream events, use streaming
	if supportsStreaming && hasStreamContext {
		at.logger.Info(ctx, "Using STREAMING execution for sub-agent", map[string]interface{}{
			"sub_agent": agentName,
			"tool_name": at.name,
			"message":   "Sub-agent output will be streamed to parent in real-time",
		})

		result, err = at.runWithStreaming(ctx, input, streamingAgent, parentEventChan, agentName)
	} else {
		// Fall back to non-streaming execution
		fallbackReason := getFallbackReason(supportsStreaming, hasStreamContext)
		at.logger.Warn(ctx, "Using NON-STREAMING execution for sub-agent", map[string]interface{}{
			"sub_agent":       agentName,
			"tool_name":       at.name,
			"supports_stream": supportsStreaming,
			"has_stream_ctx":  hasStreamContext,
			"fallback_reason": fallbackReason,
			"impact":          "Sub-agent output will only be available at the end",
		})

		result, err = at.agent.Run(ctx, input)
	}

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
		"sub_agent":    agentName,
		"tool_name":    at.name,
		"input_prompt": input,
		"response":     result,
		"duration":     duration.String(),
		"response_len": len(result),
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

// WithStreamEventChan adds a stream event channel to the context for forwarding sub-agent events
func WithStreamEventChan(ctx context.Context, eventChan chan<- interfaces.AgentStreamEvent) context.Context {
	return context.WithValue(ctx, streamEventChan, eventChan)
}

// runWithStreaming executes the sub-agent with streaming and forwards events to parent
func (at *AgentTool) runWithStreaming(
	ctx context.Context,
	input string,
	streamingAgent StreamingSubAgent,
	parentEventChan chan<- interfaces.AgentStreamEvent,
	agentName string,
) (string, error) {
	at.logger.Info(ctx, "Starting sub-agent streaming execution", map[string]interface{}{
		"sub_agent": agentName,
		"tool_name": at.name,
	})

	// Start streaming
	eventChan, err := streamingAgent.RunStream(ctx, input)
	if err != nil {
		at.logger.Error(ctx, "Failed to start sub-agent streaming", map[string]interface{}{
			"sub_agent": agentName,
			"error":     err.Error(),
		})
		return "", fmt.Errorf("failed to start sub-agent streaming: %w", err)
	}

	var resultBuilder strings.Builder
	var finalError error
	eventCount := 0
	forwardedCount := 0
	droppedCount := 0

	// Forward all events from sub-agent to parent, adding metadata
	for event := range eventChan {
		eventCount++

		// Add sub-agent metadata to the event
		if event.Metadata == nil {
			event.Metadata = make(map[string]interface{})
		}
		event.Metadata["sub_agent"] = agentName
		event.Metadata["sub_agent_tool"] = at.name

		// Track content for final result
		if event.Type == interfaces.AgentEventContent {
			resultBuilder.WriteString(event.Content)
		}

		// Track errors
		if event.Error != nil {
			finalError = event.Error
		}

		// Forward event to parent (non-blocking)
		select {
		case parentEventChan <- event:
			forwardedCount++
			at.logger.Debug(ctx, "Forwarded sub-agent event to parent", map[string]interface{}{
				"sub_agent":  agentName,
				"event_type": event.Type,
				"event_num":  eventCount,
			})
		case <-ctx.Done():
			at.logger.Warn(ctx, "Context cancelled during sub-agent streaming", map[string]interface{}{
				"sub_agent":       agentName,
				"events_received": eventCount,
				"events_forward":  forwardedCount,
			})
			return "", ctx.Err()
		default:
			// If channel is full, log but don't block
			droppedCount++
			at.logger.Warn(ctx, "Parent event channel full, dropping sub-agent event", map[string]interface{}{
				"sub_agent":   agentName,
				"event_type":  event.Type,
				"event_num":   eventCount,
				"drop_reason": "channel buffer full",
			})
		}
	}

	// Log completion summary
	at.logger.Info(ctx, "Sub-agent streaming completed", map[string]interface{}{
		"sub_agent":       agentName,
		"events_received": eventCount,
		"events_forward":  forwardedCount,
		"events_dropped":  droppedCount,
		"result_length":   resultBuilder.Len(),
		"had_error":       finalError != nil,
	})

	// Return accumulated content or error
	if finalError != nil {
		return "", finalError
	}

	return resultBuilder.String(), nil
}

// getFallbackReason returns a human-readable reason for falling back to non-streaming
func getFallbackReason(supportsStreaming, hasStreamContext bool) string {
	if !supportsStreaming && !hasStreamContext {
		return "agent doesn't support streaming and no stream context"
	} else if !supportsStreaming {
		return "agent doesn't support streaming"
	} else if !hasStreamContext {
		return "no stream context from parent"
	}
	return "unknown"
}
