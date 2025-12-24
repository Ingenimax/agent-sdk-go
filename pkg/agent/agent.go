package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/executionplan"
	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/client"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
	"github.com/Ingenimax/agent-sdk-go/pkg/tracing"
)

// LazyMCPConfig holds configuration for lazy MCP server initialization
type LazyMCPConfig struct {
	Name              string
	Type              string // "stdio" or "http"
	Command           string
	Args              []string
	Env               []string
	URL               string
	Token             string // Bearer token for HTTP authentication
	Tools             []LazyMCPToolConfig
	HttpTransportMode string // "sse" or "streamable"
}

// LazyMCPToolConfig holds configuration for a lazy MCP tool
type LazyMCPToolConfig struct {
	Name        string
	Description string
	Schema      interface{}
}

// CustomRunFunction represents a custom function that can replace the default Run behavior
type CustomRunFunction func(ctx context.Context, input string, agent *Agent) (string, error)

// CustomRunStreamFunction represents a custom function that can replace the default RunStream behavior
type CustomRunStreamFunction func(ctx context.Context, input string, agent *Agent) (<-chan interfaces.AgentStreamEvent, error)

// Agent represents an AI agent
type Agent struct {
	llm                  interfaces.LLM
	memory               interfaces.Memory
	datastore            interfaces.DataStore     // DataStore for persistent data storage (PostgreSQL, Supabase, etc.)
	graphRAGStore        interfaces.GraphRAGStore // GraphRAG store for knowledge graph operations
	tools                []interfaces.Tool
	subAgents            []*Agent // Sub-agents that can be called as tools
	orgID                string
	tracer               interfaces.Tracer
	guardrails           interfaces.Guardrails
	logger               logging.Logger // Logger for the agent
	systemPrompt         string
	name                 string                   // Name of the agent, e.g., "PlatformOps", "Math", "Research"
	description          string                   // Description of what the agent does
	requirePlanApproval  bool                     // New field to control whether execution plans require approval
	planStore            *executionplan.Store     // Store for execution plans
	planGenerator        *executionplan.Generator // Generator for execution plans
	planExecutor         *executionplan.Executor  // Executor for execution plans
	generatedAgentConfig *AgentConfig
	generatedTaskConfigs TaskConfigs
	responseFormat       *interfaces.ResponseFormat // Response format for the agent
	llmConfig            *interfaces.LLMConfig
	mcpServers           []interfaces.MCPServer   // MCP servers for the agent
	lazyMCPConfigs       []LazyMCPConfig          // Lazy MCP server configurations
	maxIterations        int                      // Maximum number of tool-calling iterations (default: 2)
	streamConfig         *interfaces.StreamConfig // Streaming configuration for the agent

	// Runtime configuration fields
	memoryConfig   map[string]interface{} // Memory configuration from YAML
	timeout        time.Duration          // Agent timeout from runtime config
	tracingEnabled bool                   // Whether tracing is enabled
	metricsEnabled bool                   // Whether metrics are enabled

	// Remote agent fields
	isRemote      bool                      // Whether this is a remote agent
	remoteURL     string                    // URL of the remote agent service
	remoteTimeout time.Duration             // Timeout for remote agent operations
	remoteClient  *client.RemoteAgentClient // gRPC client for remote communication

	// Custom function fields
	customRunFunc       CustomRunFunction       // Custom run function to replace default behavior
	customRunStreamFunc CustomRunStreamFunction // Custom stream function to replace default streaming behavior
}

// Option represents an option for configuring an agent
type Option func(*Agent)

// WithLLM sets the LLM for the agent
func WithLLM(llm interfaces.LLM) Option {
	return func(a *Agent) {
		a.llm = llm
	}
}

// WithMemory sets the memory for the agent
func WithMemory(memory interfaces.Memory) Option {
	return func(a *Agent) {
		a.memory = memory
	}
}

// WithDataStore sets the datastore for the agent
func WithDataStore(datastore interfaces.DataStore) Option {
	return func(a *Agent) {
		a.datastore = datastore
	}
}

// WithTools appends tools to the agent's tool list, deduplicating by name
func WithTools(tools ...interfaces.Tool) Option {
	return func(a *Agent) {
		a.tools = deduplicateTools(append(a.tools, tools...))
	}
}

// deduplicateTools removes duplicate tools based on their Name().
// When duplicates are found, the first occurrence is kept and subsequent duplicates are discarded.
// This ensures tools are added in order of priority (earlier = higher priority).
func deduplicateTools(tools []interfaces.Tool) []interfaces.Tool {
	if len(tools) == 0 {
		return tools
	}

	seen := make(map[string]bool, len(tools))
	result := make([]interfaces.Tool, 0, len(tools))

	for _, tool := range tools {
		if tool == nil {
			continue // Skip nil tools
		}
		name := tool.Name()
		if name == "" {
			continue // Skip tools with empty names
		}
		if !seen[name] {
			seen[name] = true
			result = append(result, tool)
		}
	}

	return result
}

// WithOrgID sets the organization ID for multi-tenancy
func WithOrgID(orgID string) Option {
	return func(a *Agent) {
		a.orgID = orgID
	}
}

// WithTracer sets the tracer for the agent
func WithTracer(tracer interfaces.Tracer) Option {
	return func(a *Agent) {
		a.tracer = tracer
	}
}

// WithLogger sets the logger for the agent
func WithLogger(logger logging.Logger) Option {
	return func(a *Agent) {
		a.logger = logger
	}
}

// WithGuardrails sets the guardrails for the agent
func WithGuardrails(guardrails interfaces.Guardrails) Option {
	return func(a *Agent) {
		a.guardrails = guardrails
	}
}

// WithSystemPrompt sets the system prompt for the agent
func WithSystemPrompt(prompt string) Option {
	return func(a *Agent) {
		a.systemPrompt = prompt
	}
}

// WithRequirePlanApproval sets whether execution plans require user approval
func WithRequirePlanApproval(require bool) Option {
	return func(a *Agent) {
		a.requirePlanApproval = require
	}
}

// WithName sets the name for the agent
func WithName(name string) Option {
	return func(a *Agent) {
		a.name = name
	}
}

// WithDescription sets the description for the agent
func WithDescription(description string) Option {
	return func(a *Agent) {
		a.description = description
	}
}

// WithAgentConfig sets the agent configuration from a YAML config
func WithAgentConfig(config AgentConfig, variables map[string]string) Option {
	return func(a *Agent) {
		// Expand environment variables in all config sections
		expandedConfig := ExpandAgentConfig(config)

		// Existing system prompt processing
		systemPrompt := FormatSystemPromptFromConfig(expandedConfig, variables)
		a.systemPrompt = systemPrompt

		// Existing response format and MCP config
		if expandedConfig.ResponseFormat != nil {
			responseFormat, err := ConvertYAMLSchemaToResponseFormat(expandedConfig.ResponseFormat)
			if err == nil && responseFormat != nil {
				a.responseFormat = responseFormat
			}
		}

		// Extract configVars for MCP configuration expansion
		var configVars map[string]string
		if expandedConfig.ConfigSource != nil && expandedConfig.ConfigSource.Variables != nil {
			configVars = expandedConfig.ConfigSource.Variables
		} else {
			configVars = make(map[string]string)
		}

		if expandedConfig.MCP != nil {
			applyMCPConfig(a, expandedConfig.MCP, configVars)
		}

		// Apply behavioral settings
		if expandedConfig.MaxIterations != nil {
			a.maxIterations = *expandedConfig.MaxIterations
		}
		if expandedConfig.RequirePlanApproval != nil {
			a.requirePlanApproval = *expandedConfig.RequirePlanApproval
		}

		// Apply complex configuration objects
		if expandedConfig.StreamConfig != nil {
			a.streamConfig = convertStreamConfigYAMLToInterface(expandedConfig.StreamConfig)
		}
		if expandedConfig.LLMConfig != nil {
			a.llmConfig = convertLLMConfigYAMLToInterface(expandedConfig.LLMConfig)
		}

		// Process LLM provider configuration
		if expandedConfig.LLMProvider != nil {
			if a.logger != nil {
				a.logger.Info(context.Background(), "Found LLM provider configuration in YAML", map[string]interface{}{
					"provider": expandedConfig.LLMProvider.Provider,
					"model":    expandedConfig.LLMProvider.Model,
					"has_llm":  a.llm != nil,
				})
			}
			if a.llm == nil {
				// Only create LLM from YAML if no LLM was provided programmatically
				llmClient, err := createLLMFromConfig(expandedConfig.LLMProvider)
				if err != nil {
					// Log warning but continue - don't fail agent creation for LLM issues
					if a.logger != nil {
						a.logger.Warn(context.Background(), "Failed to create LLM from YAML config", map[string]interface{}{
							"provider": expandedConfig.LLMProvider.Provider,
							"error":    err.Error(),
						})
					}
				} else {
					if a.logger != nil {
						a.logger.Info(context.Background(), "Successfully created LLM from YAML config", map[string]interface{}{
							"provider": expandedConfig.LLMProvider.Provider,
							"model":    expandedConfig.LLMProvider.Model,
						})
					}
					a.llm = llmClient
				}
			} else {
				if a.logger != nil {
					a.logger.Info(context.Background(), "LLM already provided programmatically, skipping YAML config", map[string]interface{}{
						"yaml_provider": expandedConfig.LLMProvider.Provider,
					})
				}
			}
		} else {
			if a.logger != nil {
				a.logger.Info(context.Background(), "No LLM provider configuration found in YAML", map[string]interface{}{
					"has_llm": a.llm != nil,
				})
			}
		}

		// Process tools
		if expandedConfig.Tools != nil {
			factory := NewToolFactory()
			toolsToAdd := make([]interfaces.Tool, 0)
			for _, toolConfig := range expandedConfig.Tools {
				if toolConfig.Enabled != nil && !*toolConfig.Enabled {
					continue // Skip disabled tools
				}
				tool, err := factory.CreateTool(toolConfig)
				if err != nil {
					// Log warning but continue - don't fail agent creation for tool issues
					if a.logger != nil {
						a.logger.Warn(context.Background(), "Failed to create tool from config", map[string]interface{}{
							"tool_name": toolConfig.Name,
							"tool_type": toolConfig.Type,
							"error":     err.Error(),
						})
					}
					continue
				}
				toolsToAdd = append(toolsToAdd, tool)
			}
			// Deduplicate before adding to agent
			a.tools = deduplicateTools(append(a.tools, toolsToAdd...))
		}

		// Store memory config for later instantiation (after LLM is set)
		if expandedConfig.Memory != nil {
			a.memoryConfig = convertMemoryConfigYAMLToInterface(expandedConfig.Memory)
		}

		// Apply runtime settings
		if expandedConfig.Runtime != nil {
			// TODO: Set log level if logger supports it when LogLevel is specified
			// Currently the logger interface doesn't support dynamic level setting
			if expandedConfig.Runtime.TimeoutDuration != "" {
				if timeout, err := time.ParseDuration(expandedConfig.Runtime.TimeoutDuration); err == nil {
					a.timeout = timeout
				}
			}
			if expandedConfig.Runtime.EnableTracing != nil && *expandedConfig.Runtime.EnableTracing {
				// Tracing enablement flag stored for later use
				a.tracingEnabled = true
			}
			if expandedConfig.Runtime.EnableMetrics != nil && *expandedConfig.Runtime.EnableMetrics {
				// Metrics enablement flag stored for later use
				a.metricsEnabled = true
			}
		}

		// Process sub-agents recursively
		if expandedConfig.SubAgents != nil {
			// Merge ConfigSource variables with OS env variables for sub-agents
			// ConfigSource variables take priority (they're from the config service)
			mergedVariables := make(map[string]string)
			// Start with OS env variables
			for k, v := range variables {
				mergedVariables[k] = v
			}
			// Override with ConfigSource variables if available
			if expandedConfig.ConfigSource != nil && expandedConfig.ConfigSource.Variables != nil {
				for k, v := range expandedConfig.ConfigSource.Variables {
					mergedVariables[k] = v
				}
			}

			subAgents, err := createSubAgentsFromConfig(expandedConfig.SubAgents, mergedVariables, a.llm, a.memory, a.tracer, a.logger)
			if err != nil {
				// Log error but don't fail agent creation
				if a.logger != nil {
					a.logger.Warn(context.Background(), "Failed to create some sub-agents from config", map[string]interface{}{
						"error": err.Error(),
					})
				}
			} else if len(subAgents) > 0 {
				// Add sub-agents using WithAgents
				a.subAgents = subAgents
				// Convert sub-agents to tools
				agentTools := make([]interfaces.Tool, 0, len(subAgents))
				for _, subAgent := range subAgents {
					agentTool := tools.NewAgentTool(subAgent)
					// Pass logger and tracer if available on parent agent
					if a.logger != nil {
						agentTool = agentTool.WithLogger(a.logger)
					}
					if a.tracer != nil {
						agentTool = agentTool.WithTracer(a.tracer)
					}
					agentTools = append(agentTools, agentTool)
				}
				// Deduplicate before adding to agent
				a.tools = deduplicateTools(append(a.tools, agentTools...))
			}
		}

		// Store the expanded configuration for later access
		a.generatedAgentConfig = &expandedConfig
	}
}

// WithResponseFormat sets the response format for the agent
func WithResponseFormat(formatType interfaces.ResponseFormat) Option {
	return func(a *Agent) {
		a.responseFormat = &formatType
	}
}

func WithLLMConfig(config interfaces.LLMConfig) Option {
	return func(a *Agent) {
		a.llmConfig = &config
	}
}

// WithMCPServers sets the MCP servers for the agent
func WithMCPServers(mcpServers []interfaces.MCPServer) Option {
	return func(a *Agent) {
		a.mcpServers = mcpServers
	}
}

// WithLazyMCPConfigs sets the lazy MCP server configurations for the agent
func WithLazyMCPConfigs(configs []LazyMCPConfig) Option {
	return func(a *Agent) {
		a.lazyMCPConfigs = configs
	}
}

// WithMCPURLs adds MCP servers from URL strings
// Supports formats:
// - stdio://command/path/to/executable
// - http://localhost:8080/mcp
// - https://api.example.com/mcp?token=xxx
// - mcp://preset-name (for presets)
func WithMCPURLs(urls ...string) Option {
	return func(a *Agent) {
		builder := mcp.NewBuilder()
		for _, url := range urls {
			builder.AddServer(url)
		}

		// Build lazy configurations
		lazyConfigs, err := builder.BuildLazy()
		if err != nil {
			// Log error but don't fail agent creation
			if a.logger != nil {
				a.logger.Warn(context.Background(), "Failed to parse some MCP URLs", map[string]interface{}{
					"error": err.Error(),
				})
			}
			return
		}

		// Convert mcp.LazyMCPServerConfig to agent.LazyMCPConfig
		for _, config := range lazyConfigs {
			agentConfig := LazyMCPConfig{
				Name:    config.Name,
				Type:    config.Type,
				Command: config.Command,
				Args:    config.Args,
				Env:     config.Env,
				URL:     config.URL,
			}
			a.lazyMCPConfigs = append(a.lazyMCPConfigs, agentConfig)
		}
	}
}

// WithMCPPresets adds predefined MCP server configurations
func WithMCPPresets(presetNames ...string) Option {
	return func(a *Agent) {
		builder := mcp.NewBuilder()
		for _, preset := range presetNames {
			builder.AddPreset(preset)
		}

		// Build lazy configurations
		lazyConfigs, err := builder.BuildLazy()
		if err != nil {
			// Log error but don't fail agent creation
			if a.logger != nil {
				a.logger.Warn(context.Background(), "Failed to load some MCP presets", map[string]interface{}{
					"error": err.Error(),
				})
			}
			return
		}

		// Convert mcp.LazyMCPServerConfig to agent.LazyMCPConfig
		for _, config := range lazyConfigs {
			agentConfig := LazyMCPConfig{
				Name:    config.Name,
				Type:    config.Type,
				Command: config.Command,
				Args:    config.Args,
				Env:     config.Env,
				URL:     config.URL,
			}
			a.lazyMCPConfigs = append(a.lazyMCPConfigs, agentConfig)
		}
	}
}

// WithMaxIterations sets the maximum number of tool-calling iterations for the agent
func WithMaxIterations(maxIterations int) Option {
	return func(a *Agent) {
		a.maxIterations = maxIterations
	}
}

// WithStreamConfig sets the streaming configuration for the agent
func WithStreamConfig(config *interfaces.StreamConfig) Option {
	return func(a *Agent) {
		a.streamConfig = config
	}
}

// WithURL creates a remote agent that communicates via gRPC
func WithURL(url string) Option {
	return func(a *Agent) {
		a.isRemote = true
		a.remoteURL = url
		// For remote agents, LLM is not required locally
		a.llm = nil
	}
}

// WithRemoteTimeout sets the timeout for remote agent operations
func WithRemoteTimeout(timeout time.Duration) Option {
	return func(a *Agent) {
		a.remoteTimeout = timeout
	}
}

// WithAgents sets the sub-agents that can be called as tools
func WithAgents(subAgents ...*Agent) Option {
	return func(a *Agent) {
		a.subAgents = subAgents
		// Automatically wrap sub-agents as tools
		for _, subAgent := range subAgents {
			agentTool := tools.NewAgentTool(subAgent)

			// Pass logger and tracer if available on parent agent
			// Note: This will be set later in NewAgent after the agent is fully constructed
			a.tools = append(a.tools, agentTool)
		}
	}
}

// WithCustomRunFunction sets a custom run function that replaces the default Run behavior
func WithCustomRunFunction(fn CustomRunFunction) Option {
	return func(a *Agent) {
		a.customRunFunc = fn
	}
}

// WithCustomRunStreamFunction sets a custom streaming run function that replaces the default RunStream behavior
func WithCustomRunStreamFunction(fn CustomRunStreamFunction) Option {
	return func(a *Agent) {
		a.customRunStreamFunc = fn
	}
}

// NewAgent creates a new agent with the given options
func NewAgent(options ...Option) (*Agent, error) {
	agent := &Agent{
		requirePlanApproval: true, // Default to requiring approval
		maxIterations:       2,    // Default to 2 iterations (current behavior)
	}

	for _, option := range options {
		option(agent)
	}

	// Initialize default logger if none provided
	if agent.logger == nil {
		agent.logger = logging.New()
	}

	// Create memory from config if specified and LLM is available
	if agent.memoryConfig != nil && agent.llm != nil && agent.memory == nil {
		memoryInstance, err := CreateMemoryFromConfig(agent.memoryConfig, agent.llm)
		if err != nil {
			// Log warning but don't fail agent creation
			if agent.logger != nil {
				agent.logger.Warn(context.Background(), "Failed to create memory from config, using default", map[string]interface{}{
					"error": err.Error(),
					"type":  agent.memoryConfig["type"],
				})
			}
		} else {
			// Apply the memory instance
			agent.memory = memoryInstance
		}
	}

	// Different validation for local vs remote agents
	if agent.isRemote {
		return validateRemoteAgent(agent)
	} else {
		return validateLocalAgent(agent)
	}
}

// validateLocalAgent validates a local agent
func validateLocalAgent(agent *Agent) (*Agent, error) {
	// Validate required fields for local agents
	if agent.llm == nil {
		return nil, fmt.Errorf("LLM is required for local agents")
	}

	// Validate sub-agents if present
	if len(agent.subAgents) > 0 {
		// Check for circular dependencies
		if err := agent.validateSubAgents(); err != nil {
			return nil, fmt.Errorf("sub-agent validation failed: %w", err)
		}

		// Validate agent tree depth (max 5 levels)
		if err := validateAgentTree(agent, 5); err != nil {
			return nil, fmt.Errorf("agent tree validation failed: %w", err)
		}
	}

	// Configure sub-agent tools with logger and tracer
	agent.configureSubAgentTools()

	// Eagerly load MCP tools during initialization to combine with manual tools
	if err := agent.initializeMCPTools(); err != nil {
		// Log warning but continue - MCP tools are optional
		fmt.Printf("Warning: Failed to initialize MCP tools: %v\n", err)
	}

	// Get all tools (manual + MCP) for execution plan components
	allTools := agent.getAllToolsSync()

	// Initialize execution plan components
	agent.planStore = executionplan.NewStore()
	agent.planGenerator = executionplan.NewGenerator(agent.llm, allTools, agent.systemPrompt, agent.requirePlanApproval)
	agent.planExecutor = executionplan.NewExecutor(allTools)

	return agent, nil
}

// validateRemoteAgent validates a remote agent
func validateRemoteAgent(agent *Agent) (*Agent, error) {
	// Validate required fields for remote agents
	if agent.remoteURL == "" {
		return nil, fmt.Errorf("URL is required for remote agents")
	}

	// Initialize remote client
	config := client.RemoteAgentConfig{
		URL: agent.remoteURL,
	}
	// Use custom timeout if specified, otherwise the default 5 minutes will be used
	// Special case: 0 means infinite timeout (no timeout)
	if agent.remoteTimeout >= 0 {
		config.Timeout = agent.remoteTimeout
	}
	agent.remoteClient = client.NewRemoteAgentClient(config)

	// Test connection and fetch metadata
	if err := agent.initializeRemoteAgent(); err != nil {
		return nil, fmt.Errorf("failed to initialize remote agent: %w", err)
	}

	return agent, nil
}

// NewAgentWithAutoConfig creates a new agent with automatic configuration generation
// based on the system prompt if explicit configuration is not provided
func NewAgentWithAutoConfig(ctx context.Context, options ...Option) (*Agent, error) {
	// First create an agent with the provided options
	agent, err := NewAgent(options...)
	if err != nil {
		return nil, err
	}

	// If the agent doesn't have a name, set a default one
	if agent.name == "" {
		agent.name = "Auto-Configured Agent"
	}

	// If the system prompt is provided but no configuration was explicitly set,
	// generate configuration using the LLM
	if agent.systemPrompt != "" {
		// Generate agent and task configurations from the system prompt
		agentConfig, taskConfigs, err := GenerateConfigFromSystemPrompt(ctx, agent.llm, agent.systemPrompt)
		if err != nil {
			// If we fail to generate configs, just continue with the manual system prompt
			// We don't want to fail agent creation just because auto-config failed
			return agent, nil
		}

		// Create a task configuration map
		taskConfigMap := make(TaskConfigs)
		for i, taskConfig := range taskConfigs {
			taskName := fmt.Sprintf("auto_task_%d", i+1)
			taskConfig.Agent = agent.name // Set the task to use this agent
			taskConfigMap[taskName] = taskConfig
		}

		// Store generated configurations in agent so they can be accessed later
		agent.generatedAgentConfig = &agentConfig
		agent.generatedTaskConfigs = taskConfigMap
	}

	return agent, nil
}

// NewAgentFromConfig creates a new agent from a YAML configuration
func NewAgentFromConfig(agentName string, configs AgentConfigs, variables map[string]string, options ...Option) (*Agent, error) {
	config, exists := configs[agentName]
	if !exists {
		return nil, fmt.Errorf("agent configuration for %s not found", agentName)
	}

	// Add the agent config option
	configOption := WithAgentConfig(config, variables)
	nameOption := WithName(agentName)

	// Combine all options
	allOptions := append([]Option{configOption, nameOption}, options...)

	return NewAgent(allOptions...)
}

// CreateAgentForTask creates a new agent for a specific task
func CreateAgentForTask(taskName string, agentConfigs AgentConfigs, taskConfigs TaskConfigs, variables map[string]string, options ...Option) (*Agent, error) {
	agentName, err := GetAgentForTask(taskConfigs, taskName)
	if err != nil {
		return nil, err
	}

	// Check if task has its own response format
	taskConfig := taskConfigs[taskName]
	if taskConfig.ResponseFormat != nil {
		responseFormat, err := ConvertYAMLSchemaToResponseFormat(taskConfig.ResponseFormat)
		if err == nil && responseFormat != nil {
			options = append(options, WithResponseFormat(*responseFormat))
		}
	}

	return NewAgentFromConfig(agentName, agentConfigs, variables, options...)
}

// Run runs the agent with the given input
func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	response, err := a.runInternal(ctx, input, false)
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

func (a *Agent) RunDetailed(ctx context.Context, input string) (*interfaces.AgentResponse, error) {
	return a.runInternal(ctx, input, true)
}

func (a *Agent) runInternal(ctx context.Context, input string, detailed bool) (*interfaces.AgentResponse, error) {
	startTime := time.Now()

	tracker := newUsageTracker(detailed)
	ctx = withUsageTracker(ctx, tracker)

	var response string
	var err error

	if a.customRunFunc != nil {
		response, err = a.customRunFunc(ctx, input, a)
		if err != nil {
			return nil, err
		}
	} else if a.isRemote {
		response, err = a.runRemoteWithTracking(ctx, input)
		if err != nil {
			return nil, err
		}
	} else {
		response, err = a.runLocalWithTracking(ctx, input)
		if err != nil {
			return nil, err
		}
	}

	tracker.setExecutionTime(time.Since(startTime).Milliseconds())
	usage, execSummary, primaryModel := tracker.getResults()

	var execSum interfaces.ExecutionSummary
	if execSummary != nil {
		execSum = *execSummary
	}

	// Log detailed execution information for all agent calls
	if detailed {
		executionDetails := map[string]interface{}{
			"agent_name":        a.name,
			"input_length":      len(input),
			"response_length":   len(response),
			"model_used":        primaryModel,
			"execution_time_ms": time.Since(startTime).Milliseconds(),
		}
		if execSummary != nil {
			executionDetails["llm_calls"] = execSummary.LLMCalls
			executionDetails["tool_calls"] = execSummary.ToolCalls
			executionDetails["sub_agent_calls"] = execSummary.SubAgentCalls
			executionDetails["used_tools"] = execSummary.UsedTools
			executionDetails["used_sub_agents"] = execSummary.UsedSubAgents
		}
		if usage != nil {
			executionDetails["input_tokens"] = usage.InputTokens
			executionDetails["output_tokens"] = usage.OutputTokens
			executionDetails["total_tokens"] = usage.TotalTokens
			executionDetails["reasoning_tokens"] = usage.ReasoningTokens
		}
		log.Printf("[Agent SDK] Agent execution completed: %+v", executionDetails)
	}

	return &interfaces.AgentResponse{
		Content:          response,
		Usage:            usage,
		AgentName:        a.name,
		Model:            primaryModel,
		ExecutionSummary: execSum,
		Metadata: map[string]interface{}{
			"agent_name":            a.name,
			"execution_timestamp":   startTime.Unix(),
			"execution_duration_ms": time.Since(startTime).Milliseconds(),
		},
	}, nil
}

func (a *Agent) runLocalWithTracking(ctx context.Context, input string) (string, error) {
	ctx = tracing.WithAgentName(ctx, a.name)

	if a.orgID != "" {
		ctx = multitenancy.WithOrgID(ctx, a.orgID)
	}

	var span interfaces.Span
	if a.tracer != nil {
		ctx, span = a.tracer.StartSpan(ctx, "agent.Run")
		defer span.End()
	}

	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    interfaces.MessageRoleUser,
			Content: input,
		}); err != nil {
			return "", fmt.Errorf("failed to add user message to memory: %w", err)
		}
	}

	if a.guardrails != nil {
		guardedInput, err := a.guardrails.ProcessInput(ctx, input)
		if err != nil {
			return "", fmt.Errorf("guardrails error: %w", err)
		}
		input = guardedInput
	}

	taskID, action, planInput := a.extractPlanAction(input)
	if taskID != "" {
		return a.handlePlanAction(ctx, taskID, action, planInput)
	}

	if a.systemPrompt != "" && a.isAskingAboutRole(input) {
		response := a.generateRoleResponse()

		if a.memory != nil {
			if err := a.memory.AddMessage(ctx, interfaces.Message{
				Role:    interfaces.MessageRoleAssistant,
				Content: response,
			}); err != nil {
				return "", fmt.Errorf("failed to add role response to memory: %w", err)
			}
		}

		return response, nil
	}

	// Use pre-initialized tools (manual + MCP tools already combined during agent creation)
	allTools := a.tools

	if len(a.mcpServers) > 0 {
		mcpTools, err := a.collectMCPTools(ctx)
		if err != nil {
			fmt.Printf("Failed to collect MCP tools: %v\n", err)
		} else if len(mcpTools) > 0 {
			allTools = append(allTools, mcpTools...)
		}
	}

	if len(a.lazyMCPConfigs) > 0 {
		lazyMCPTools := a.createLazyMCPTools()
		allTools = append(allTools, lazyMCPTools...)
	}

	if (len(allTools) > 0) && a.requirePlanApproval {
		a.planGenerator = executionplan.NewGenerator(a.llm, allTools, a.systemPrompt, a.requirePlanApproval)
		return a.runWithExecutionPlan(ctx, input)
	}

	return a.runWithoutExecutionPlanWithToolsTracked(ctx, input, allTools)
}

func (a *Agent) RunWithAuth(ctx context.Context, input string, authToken string) (string, error) {
	response, err := a.runWithAuthInternal(ctx, input, authToken, false)
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

func (a *Agent) RunWithAuthDetailed(ctx context.Context, input string, authToken string) (*interfaces.AgentResponse, error) {
	return a.runWithAuthInternal(ctx, input, authToken, true)
}

func (a *Agent) runWithAuthInternal(ctx context.Context, input string, authToken string, detailed bool) (*interfaces.AgentResponse, error) {
	startTime := time.Now()

	tracker := newUsageTracker(detailed)
	ctx = withUsageTracker(ctx, tracker)

	var response string
	var err error

	if a.isRemote {
		response, err = a.runRemoteWithAuthTracking(ctx, input, authToken)
		if err != nil {
			return nil, err
		}
	} else {
		response, err = a.runLocalWithTracking(ctx, input)
		if err != nil {
			return nil, err
		}
	}

	tracker.setExecutionTime(time.Since(startTime).Milliseconds())
	usage, execSummary, primaryModel := tracker.getResults()

	var execSum interfaces.ExecutionSummary
	if execSummary != nil {
		execSum = *execSummary
	}

	return &interfaces.AgentResponse{
		Content:          response,
		Usage:            usage,
		AgentName:        a.name,
		Model:            primaryModel,
		ExecutionSummary: execSum,
		Metadata: map[string]interface{}{
			"agent_name":            a.name,
			"execution_timestamp":   startTime.Unix(),
			"execution_duration_ms": time.Since(startTime).Milliseconds(),
			"auth_enabled":          true,
		},
	}, nil
}

// RunStreamWithAuth executes the agent with streaming response and explicit auth token
func (a *Agent) RunStreamWithAuth(ctx context.Context, input string, authToken string) (<-chan interfaces.AgentStreamEvent, error) {
	// If this is a remote agent, delegate to remote streaming execution with auth token
	if a.isRemote {
		return a.runRemoteStreamWithAuth(ctx, input, authToken)
	}

	// For local agents, the auth token isn't used but we maintain compatibility
	return a.RunStream(ctx, input)
}

func (a *Agent) runRemoteWithTracking(ctx context.Context, input string) (string, error) {
	if a.remoteClient == nil {
		return "", fmt.Errorf("remote client not initialized")
	}

	if a.orgID != "" {
		ctx = multitenancy.WithOrgID(ctx, a.orgID)
	}

	tracker := getUsageTracker(ctx)

	if tracker != nil && tracker.detailed {
		tracker.execSummary.SubAgentCalls++

		if a.name != "" {
			found := false
			for _, used := range tracker.execSummary.UsedSubAgents {
				if used == a.name {
					found = true
					break
				}
			}
			if !found {
				tracker.execSummary.UsedSubAgents = append(tracker.execSummary.UsedSubAgents, a.name)
			}
		}
	}

	return a.remoteClient.Run(ctx, input)
}

func (a *Agent) runRemoteWithAuthTracking(ctx context.Context, input string, authToken string) (string, error) {
	if a.remoteClient == nil {
		return "", fmt.Errorf("remote client not initialized")
	}

	if a.orgID != "" {
		ctx = multitenancy.WithOrgID(ctx, a.orgID)
	}

	tracker := getUsageTracker(ctx)

	if tracker != nil && tracker.detailed {
		tracker.execSummary.SubAgentCalls++

		if a.name != "" {
			found := false
			for _, used := range tracker.execSummary.UsedSubAgents {
				if used == a.name {
					found = true
					break
				}
			}
			if !found {
				tracker.execSummary.UsedSubAgents = append(tracker.execSummary.UsedSubAgents, a.name)
			}
		}
	}

	return a.remoteClient.RunWithAuth(ctx, input, authToken)
}

// runRemoteStreamWithAuth executes a remote agent via gRPC with streaming response and explicit auth token
func (a *Agent) runRemoteStreamWithAuth(ctx context.Context, input string, authToken string) (<-chan interfaces.AgentStreamEvent, error) {
	if a.remoteClient == nil {
		return nil, fmt.Errorf("remote client not initialized")
	}

	// If orgID is set on the agent, add it to the context
	if a.orgID != "" {
		ctx = multitenancy.WithOrgID(ctx, a.orgID)
	}

	return a.remoteClient.RunStreamWithAuth(ctx, input, authToken)
}

// collectMCPTools collects tools from all MCP servers
func (a *Agent) collectMCPTools(ctx context.Context) ([]interfaces.Tool, error) {
	var mcpTools []interfaces.Tool

	for _, server := range a.mcpServers {
		// List tools from this server
		tools, err := server.ListTools(ctx)
		if err != nil {
			fmt.Printf("Failed to list tools from MCP server: %v\n", err)
			continue
		}

		// Convert MCP tools to agent tools
		for _, mcpTool := range tools {
			// Create a new MCPTool
			tool := mcp.NewMCPTool(mcpTool.Name, mcpTool.Description, mcpTool.Schema, server)
			mcpTools = append(mcpTools, tool)
		}
	}

	return mcpTools, nil
}

// createLazyMCPTools creates lazy MCP tools from configurations
func (a *Agent) createLazyMCPTools() []interfaces.Tool {
	var lazyTools []interfaces.Tool

	fmt.Printf("Creating lazy MCP tools from %d configs...\n", len(a.lazyMCPConfigs))

	for _, config := range a.lazyMCPConfigs {
		fmt.Printf("Processing MCP config: %s (type: %s)\n", config.Name, config.Type)

		// Create lazy server config
		lazyServerConfig := mcp.LazyMCPServerConfig{
			Name:              config.Name,
			Type:              config.Type,
			Command:           config.Command,
			Args:              config.Args,
			Env:               config.Env,
			URL:               config.URL,
			Token:             config.Token,
			HttpTransportMode: config.HttpTransportMode,
		}

		// If no specific tools are defined, discover all tools from the server
		if len(config.Tools) == 0 {
			fmt.Printf("No tools specified for %s, discovering tools from server\n", config.Name)

			// Create a temporary server instance to discover tools
			ctx := context.Background()
			server, err := mcp.GetOrCreateServerFromCache(ctx, lazyServerConfig)
			if err != nil {
				fmt.Printf("Failed to create server for tool discovery: %v\n", err)
				continue
			}

			// Log discovered server metadata
			if serverInfo, err := server.GetServerInfo(); err == nil && serverInfo != nil {
				fmt.Printf("Discovered MCP server metadata for %s:\n", config.Name)
				fmt.Printf("  Name: %s\n", serverInfo.Name)
				if serverInfo.Title != "" {
					fmt.Printf("  Title: %s\n", serverInfo.Title)
				}
				if serverInfo.Version != "" {
					fmt.Printf("  Version: %s\n", serverInfo.Version)
				}
			}

			// Discover available tools from the server
			discoveredTools, err := server.ListTools(ctx)
			if err != nil {
				fmt.Printf("Failed to discover tools from %s: %v\n", config.Name, err)
				continue
			}

			fmt.Printf("Discovered %d tools from %s server\n", len(discoveredTools), config.Name)

			// Create lazy tools for each discovered tool
			for _, discoveredTool := range discoveredTools {
				fmt.Printf("Creating lazy tool: %s\n", discoveredTool.Name)
				fmt.Printf("  Description: '%s'\n", discoveredTool.Description)
				fmt.Printf("  Schema: %+v\n", discoveredTool.Schema)

				lazyTool := mcp.NewLazyMCPTool(
					discoveredTool.Name,
					discoveredTool.Description,
					discoveredTool.Schema,
					lazyServerConfig,
				)
				lazyTools = append(lazyTools, lazyTool)
			}
		} else {
			// Create a temporary server instance to discover metadata even for configured tools
			ctx := context.Background()
			server, err := mcp.GetOrCreateServerFromCache(ctx, lazyServerConfig)
			if err != nil {
				fmt.Printf("Warning: Failed to create server for metadata discovery: %v\n", err)
			} else {
				// Log discovered server metadata
				if serverInfo, err := server.GetServerInfo(); err == nil && serverInfo != nil {
					fmt.Printf("Discovered MCP server metadata for %s:\n", config.Name)
					fmt.Printf("  Name: %s\n", serverInfo.Name)
					if serverInfo.Title != "" {
						fmt.Printf("  Title: %s\n", serverInfo.Title)
					}
					if serverInfo.Version != "" {
						fmt.Printf("  Version: %s\n", serverInfo.Version)
					}
				}
			}

			// Create lazy tools for each configured tool
			for _, toolConfig := range config.Tools {
				fmt.Printf("Creating tool: %s\n", toolConfig.Name)
				lazyTool := mcp.NewLazyMCPTool(
					toolConfig.Name,
					toolConfig.Description,
					toolConfig.Schema,
					lazyServerConfig,
				)
				lazyTools = append(lazyTools, lazyTool)
			}
		}
	}

	fmt.Printf("Created %d lazy MCP tools\n", len(lazyTools))
	return lazyTools
}

func (a *Agent) runWithoutExecutionPlanWithToolsTracked(ctx context.Context, input string, tools []interfaces.Tool) (string, error) {
	prompt := input

	var response string
	var err error

	generateOptions := []interfaces.GenerateOption{}
	if a.systemPrompt != "" {
		fmt.Printf("[DEBUG] Using system prompt (length=%d):\n%s\n", len(a.systemPrompt), a.systemPrompt)
		generateOptions = append(generateOptions, openai.WithSystemMessage(a.systemPrompt))
	} else {
		fmt.Printf("[DEBUG] WARNING: No system prompt set for agent %s\n", a.name)
	}

	if a.responseFormat != nil {
		generateOptions = append(generateOptions, openai.WithResponseFormat(*a.responseFormat))
	}

	if a.llmConfig != nil {
		generateOptions = append(generateOptions, func(options *interfaces.GenerateOptions) {
			options.LLMConfig = a.llmConfig
		})
	}

	generateOptions = append(generateOptions, interfaces.WithMaxIterations(a.maxIterations))

	if a.memory != nil {
		generateOptions = append(generateOptions, interfaces.WithMemory(a.memory))
	}

	tracker := getUsageTracker(ctx)

	if len(tools) > 0 {
		if tracker != nil {
			for _, tool := range tools {
				tracker.addToolCall(tool.Name())
			}
		}

		if tracker != nil && tracker.detailed {
			llmResp, err := a.llm.GenerateWithToolsDetailed(ctx, prompt, tools, generateOptions...)
			if err != nil {
				return "", fmt.Errorf("failed to generate response: %w", err)
			}
			response = llmResp.Content
			tracker.addLLMUsage(llmResp.Usage, llmResp.Model)
		} else {
			response, err = a.llm.GenerateWithTools(ctx, prompt, tools, generateOptions...)
			if err != nil {
				return "", fmt.Errorf("failed to generate response: %w", err)
			}
		}
	} else {
		if tracker != nil && tracker.detailed {
			llmResp, err := a.llm.GenerateDetailed(ctx, prompt, generateOptions...)
			if err != nil {
				return "", fmt.Errorf("failed to generate response: %w", err)
			}
			response = llmResp.Content
			tracker.addLLMUsage(llmResp.Usage, llmResp.Model)
		} else {
			response, err = a.llm.Generate(ctx, prompt, generateOptions...)
			if err != nil {
				return "", fmt.Errorf("failed to generate response: %w", err)
			}
		}
	}

	// Apply guardrails to output if available
	if a.guardrails != nil {
		guardedResponse, err := a.guardrails.ProcessOutput(ctx, response)
		if err != nil {
			return "", fmt.Errorf("guardrails error: %w", err)
		}
		response = guardedResponse
	}

	// Add agent message to memory
	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    interfaces.MessageRoleAssistant,
			Content: response,
		}); err != nil {
			return "", fmt.Errorf("failed to add agent message to memory: %w", err)
		}
	}

	return response, nil
}

// extractPlanAction attempts to extract a plan action from the user input
// Returns taskID, action, and remaining input
func (a *Agent) extractPlanAction(input string) (string, string, string) {
	// This is a placeholder implementation
	// In a real implementation, you would use NLP or pattern matching to extract plan actions
	return "", "", input
}

// handlePlanAction handles actions related to an existing plan
func (a *Agent) handlePlanAction(ctx context.Context, taskID, action, input string) (string, error) {
	plan, exists := a.planStore.GetPlanByTaskID(taskID)
	if !exists {
		return "", fmt.Errorf("plan with task ID %s not found", taskID)
	}

	switch action {
	case "approve":
		return a.approvePlan(ctx, plan)
	case "modify":
		return a.modifyPlan(ctx, plan, input)
	case "cancel":
		return a.cancelPlan(plan)
	case "status":
		return a.getPlanStatus(plan)
	default:
		return "", fmt.Errorf("unknown plan action: %s", action)
	}
}

// approvePlan approves and executes a plan
func (a *Agent) approvePlan(ctx context.Context, plan *executionplan.ExecutionPlan) (string, error) {
	plan.UserApproved = true
	plan.Status = executionplan.StatusApproved

	// Add the approval to memory
	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    interfaces.MessageRoleUser,
			Content: "I approve the plan. Please proceed with execution.",
		}); err != nil {
			return "", fmt.Errorf("failed to add approval to memory: %w", err)
		}
	}

	// Execute the plan
	result, err := a.planExecutor.ExecutePlan(ctx, plan)
	if err != nil {
		return "", fmt.Errorf("failed to execute plan: %w", err)
	}

	// Add the execution result to memory
	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    interfaces.MessageRoleAssistant,
			Content: result,
		}); err != nil {
			return "", fmt.Errorf("failed to add execution result to memory: %w", err)
		}
	}

	return result, nil
}

// modifyPlan modifies a plan based on user input
func (a *Agent) modifyPlan(ctx context.Context, plan *executionplan.ExecutionPlan, input string) (string, error) {
	// Add the modification request to memory
	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    interfaces.MessageRoleUser,
			Content: "I'd like to modify the plan: " + input,
		}); err != nil {
			return "", fmt.Errorf("failed to add modification request to memory: %w", err)
		}
	}

	// Modify the plan
	modifiedPlan, err := a.planGenerator.ModifyExecutionPlan(ctx, plan, input)
	if err != nil {
		return "", fmt.Errorf("failed to modify plan: %w", err)
	}

	// Update the plan in the store
	a.planStore.StorePlan(modifiedPlan)

	// Format the modified plan
	formattedPlan := executionplan.FormatExecutionPlan(modifiedPlan)

	// Add the modified plan to memory
	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    interfaces.MessageRoleAssistant,
			Content: "I've updated the execution plan based on your feedback:\n\n" + formattedPlan + "\nDo you approve this plan? You can modify it further if needed.",
		}); err != nil {
			return "", fmt.Errorf("failed to add modified plan to memory: %w", err)
		}
	}

	return "I've updated the execution plan based on your feedback:\n\n" + formattedPlan + "\nDo you approve this plan? You can modify it further if needed.", nil
}

// cancelPlan cancels a plan
func (a *Agent) cancelPlan(plan *executionplan.ExecutionPlan) (string, error) {
	a.planExecutor.CancelPlan(plan)

	return "Plan cancelled. What would you like to do instead?", nil
}

// getPlanStatus returns the status of a plan
func (a *Agent) getPlanStatus(plan *executionplan.ExecutionPlan) (string, error) {
	status := a.planExecutor.GetPlanStatus(plan)
	formattedPlan := executionplan.FormatExecutionPlan(plan)

	return fmt.Sprintf("Current plan status: %s\n\n%s", status, formattedPlan), nil
}

// runWithExecutionPlan runs the agent with an execution plan
func (a *Agent) runWithExecutionPlan(ctx context.Context, input string) (string, error) {
	// Generate an execution plan
	plan, err := a.planGenerator.GenerateExecutionPlan(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to generate execution plan: %w", err)
	}

	// Store the plan
	a.planStore.StorePlan(plan)

	// Format the plan for display
	formattedPlan := executionplan.FormatExecutionPlan(plan)

	// Add the plan to memory
	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    interfaces.MessageRoleAssistant,
			Content: "I've created an execution plan for your request:\n\n" + formattedPlan + "\nDo you approve this plan? You can modify it if needed.",
		}); err != nil {
			return "", fmt.Errorf("failed to add plan to memory: %w", err)
		}
	}

	// Return the plan for user approval
	return "I've created an execution plan for your request:\n\n" + formattedPlan + "\nDo you approve this plan? You can modify it if needed.", nil
}

// isStructuredJSONResponse checks if a message content is a structured JSON response
func isStructuredJSONResponse(content string) bool {
	trimmed := strings.TrimSpace(content)
	return strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")
}

// convertToHumanReadable converts a JSON response to a human-readable format
// to avoid confusing the LLM with raw JSON in conversation history
func convertToHumanReadable(jsonContent string) string {
	// Try to parse the JSON to extract key information
	var jsonMap map[string]interface{}
	if err := json.Unmarshal([]byte(jsonContent), &jsonMap); err != nil {
		// If parsing fails, return a generic summary
		return "[Generated structured response]"
	}

	// Convert JSON to human-readable format - works with any JSON structure
	var parts []string

	for key, value := range jsonMap {
		switch v := value.(type) {
		case string:
			if v != "" && v != "null" {
				parts = append(parts, fmt.Sprintf("%s: %s", key, v))
			}
		case []interface{}:
			if len(v) > 0 {
				if str, ok := v[0].(string); ok && str != "" {
					parts = append(parts, fmt.Sprintf("%s: %s", key, str))
				}
			}
		case bool:
			parts = append(parts, fmt.Sprintf("%s: %t", key, v))
		case float64, int:
			parts = append(parts, fmt.Sprintf("%s: %v", key, v))
		}
	}

	if len(parts) == 0 {
		return "[Generated structured response]"
	}

	// Limit to most important parts to keep summary concise
	if len(parts) > 3 {
		parts = parts[:3]
	}

	return "[AI: " + strings.Join(parts, ", ") + "]"
}

// ApproveExecutionPlan approves an execution plan for execution
func (a *Agent) ApproveExecutionPlan(ctx context.Context, plan *executionplan.ExecutionPlan) (string, error) {
	return a.approvePlan(ctx, plan)
}

// ModifyExecutionPlan modifies an execution plan based on user input
func (a *Agent) ModifyExecutionPlan(ctx context.Context, plan *executionplan.ExecutionPlan, modifications string) (*executionplan.ExecutionPlan, error) {
	return a.planGenerator.ModifyExecutionPlan(ctx, plan, modifications)
}

// GenerateExecutionPlan generates an execution plan
func (a *Agent) GenerateExecutionPlan(ctx context.Context, input string) (*executionplan.ExecutionPlan, error) {
	return a.planGenerator.GenerateExecutionPlan(ctx, input)
}

// isAskingAboutRole determines if the user is asking about the agent's role or identity
func (a *Agent) isAskingAboutRole(input string) bool {
	// Convert input to lowercase for case-insensitive matching
	lowerInput := strings.ToLower(input)

	// Common phrases that indicate a user asking about the agent's role
	roleQueries := []string{
		"what are you",
		"who are you",
		"what is your role",
		"what do you do",
		"what can you do",
		"what is your purpose",
		"what is your function",
		"tell me about yourself",
		"introduce yourself",
		"what are your capabilities",
		"what are you designed to do",
		"what's your job",
		"what kind of assistant are you",
		"your role",
		"your expertise",
		"what are you expert in",
		"what are you specialized in",
		"your specialty",
		"what's your specialty",
	}

	// Check if any of the role query phrases are in the input
	for _, query := range roleQueries {
		if strings.Contains(lowerInput, query) {
			return true
		}
	}

	return false
}

// generateRoleResponse creates a response based on the agent's system prompt
func (a *Agent) generateRoleResponse() string {
	// If the prompt is empty, return a generic response
	if a.systemPrompt == "" || a.llm == nil {
		return "I'm an AI assistant designed to help you with various tasks and answer your questions. How can I assist you today?"
	}

	// Create a prompt that asks the LLM to generate a role description based on the system prompt
	agentName := "an AI assistant"
	if a.name != "" {
		agentName = a.name
	}

	prompt := fmt.Sprintf(`Based on the following system prompt that defines your role and capabilities,
generate a brief, natural-sounding response (3-5 sentences) introducing yourself to a user who asked what you can do.
You are named "%s".
Do not directly quote from the system prompt, but create a conversational first-person response that captures your
purpose, expertise, and how you can help. The response should feel like a natural conversation, not like reading documentation.

System prompt:
%s

Your response should:
1. Introduce yourself using first-person perspective, mentioning your name ("%s")
2. Briefly explain your specialization or purpose
3. Mention 2-3 key areas you can help with
4. End with a friendly question about how you can assist the user

Response:`, agentName, a.systemPrompt, agentName)

	// Generate a response using the LLM with the system prompt as context
	generateOptions := []interfaces.GenerateOption{}

	// Use the same system prompt to ensure consistent persona
	generateOptions = append(generateOptions, openai.WithSystemMessage(a.systemPrompt))

	// Generate the response
	response, err := a.llm.Generate(context.Background(), prompt, generateOptions...)
	if err != nil {
		// Fallback to a simple response in case of errors
		if a.name != "" {
			return fmt.Sprintf("I'm %s, an AI assistant based on the role defined in my system prompt. How can I help you today?", a.name)
		}
		return "I'm an AI assistant based on the role defined in my system prompt. How can I help you today?"
	}

	return response
}

// ExecuteTaskFromConfig executes a task using its YAML configuration
func (a *Agent) ExecuteTaskFromConfig(ctx context.Context, taskName string, taskConfigs TaskConfigs, variables map[string]string) (string, error) {
	taskConfig, exists := taskConfigs[taskName]
	if !exists {
		return "", fmt.Errorf("task configuration for %s not found", taskName)
	}

	// Replace variables in the task description
	description := taskConfig.Description
	for key, value := range variables {
		placeholder := fmt.Sprintf("{%s}", key)
		description = strings.ReplaceAll(description, placeholder, value)
	}

	// Run the agent with the task description
	result, err := a.Run(ctx, description)
	if err != nil {
		return "", fmt.Errorf("failed to execute task %s: %w", taskName, err)
	}

	// If an output file is specified, write the result to the file
	if taskConfig.OutputFile != "" {
		outputPath := taskConfig.OutputFile
		for key, value := range variables {
			placeholder := fmt.Sprintf("{%s}", key)
			outputPath = strings.ReplaceAll(outputPath, placeholder, value)
		}

		err := os.WriteFile(outputPath, []byte(result), 0600)
		if err != nil {
			return result, fmt.Errorf("failed to write output to file %s: %w", outputPath, err)
		}
	}

	return result, nil
}

// GetGeneratedAgentConfig returns the automatically generated agent configuration, if any
func (a *Agent) GetGeneratedAgentConfig() *AgentConfig {
	return a.generatedAgentConfig
}

// GetGeneratedTaskConfigs returns the automatically generated task configurations, if any
func (a *Agent) GetGeneratedTaskConfigs() TaskConfigs {
	return a.generatedTaskConfigs
}

// GetTaskByID returns a task by its ID
func (a *Agent) GetTaskByID(taskID string) (*executionplan.ExecutionPlan, bool) {
	return a.planStore.GetPlanByTaskID(taskID)
}

// ListTasks returns a list of all tasks
func (a *Agent) ListTasks() []*executionplan.ExecutionPlan {
	return a.planStore.ListPlans()
}

// GetName returns the agent's name
func (a *Agent) GetName() string {
	return a.name
}

// GetDescription returns the description of the agent
func (a *Agent) GetDescription() string {
	return a.description
}

// GetCapabilities returns a description of what the agent can do
func (a *Agent) GetCapabilities() string {
	if a.description != "" {
		return a.description
	}

	// If no description is set, generate one based on the system prompt
	if a.systemPrompt != "" {
		return fmt.Sprintf("Agent with system prompt: %s", a.systemPrompt)
	}

	return "A general-purpose AI agent"
}

// GetLLM returns the LLM instance (for use in custom functions)
func (a *Agent) GetLLM() interfaces.LLM {
	return a.llm
}

// GetMemory returns the memory instance (for use in custom functions)
func (a *Agent) GetMemory() interfaces.Memory {
	return a.memory
}

// GetDataStore returns the datastore instance
func (a *Agent) GetDataStore() interfaces.DataStore {
	return a.datastore
}

// SetDataStore sets the datastore for the agent
func (a *Agent) SetDataStore(datastore interfaces.DataStore) {
	a.datastore = datastore
}

// GetAllConversations returns all conversation IDs from memory
func (a *Agent) GetAllConversations(ctx context.Context) ([]string, error) {
	if a.memory == nil {
		return []string{}, nil
	}

	// Check if memory supports conversation operations
	if convMem, ok := a.memory.(interfaces.ConversationMemory); ok {
		return convMem.GetAllConversations(ctx)
	}

	// Fallback: return empty list for memories that don't support conversations
	return []string{}, nil
}

// GetConversationMessages gets all messages for a specific conversation
func (a *Agent) GetConversationMessages(ctx context.Context, conversationID string) ([]interfaces.Message, error) {
	if a.memory == nil {
		return []interfaces.Message{}, nil
	}

	// Check if memory supports conversation operations
	if convMem, ok := a.memory.(interfaces.ConversationMemory); ok {
		return convMem.GetConversationMessages(ctx, conversationID)
	}

	// Fallback: return empty list for memories that don't support conversations
	return []interfaces.Message{}, nil
}

// GetMemoryStatistics returns basic memory statistics
func (a *Agent) GetMemoryStatistics(ctx context.Context) (totalConversations, totalMessages int, err error) {
	if a.memory == nil {
		return 0, 0, nil
	}

	// Check if memory supports conversation operations
	if convMem, ok := a.memory.(interfaces.ConversationMemory); ok {
		return convMem.GetMemoryStatistics(ctx)
	}

	// Fallback: return basic stats for memories that don't support conversations
	return 0, 0, nil
}

// GetTools returns the tools slice (for use in custom functions)
func (a *Agent) GetTools() []interfaces.Tool {
	// Return pre-initialized tools (manual + MCP tools already combined during agent creation)
	return a.tools
}

// GetSubAgents returns the sub-agents slice
func (a *Agent) GetSubAgents() []*Agent {
	return a.subAgents
}

// GetLogger returns the logger instance (for use in custom functions)
func (a *Agent) GetLogger() logging.Logger {
	return a.logger
}

// GetTracer returns the tracer instance (for use in custom functions)
func (a *Agent) GetTracer() interfaces.Tracer {
	return a.tracer
}

// GetSystemPrompt returns the system prompt (for use in custom functions)
func (a *Agent) GetSystemPrompt() string {
	return a.systemPrompt
}

// configureSubAgentTools configures sub-agent tools with logger and tracer from parent agent
func (a *Agent) configureSubAgentTools() {
	for _, tool := range a.tools {
		// Check if this is an AgentTool by trying to cast it
		if agentTool, ok := tool.(*tools.AgentTool); ok {
			// Configure with parent agent's logger and tracer
			if a.tracer != nil {
				agentTool.WithTracer(a.tracer)
			}
			if a.logger != nil {
				agentTool.WithLogger(a.logger)
			}
		}
	}
}

// initializeRemoteAgent initializes the remote agent connection and fetches metadata
func (a *Agent) initializeRemoteAgent() error {
	// Connect to the remote agent
	// NOTE: Connection failures are non-fatal during initialization
	// This allows agents to be created even if the remote service is temporarily unavailable
	// The SDK will automatically retry connection on first actual use
	if err := a.remoteClient.Connect(); err != nil {
		// Log warning but don't fail initialization - connection will be retried on first use
		fmt.Printf("Warning: failed to connect to remote agent %s during initialization: %v (will retry on first use)\n", a.remoteURL, err)
		// Return early - skip metadata fetch since connection is not available yet
		// Set default name if not provided
		if a.name == "" {
			a.name = "Remote-Agent"
		}
		return nil // Return nil to allow agent creation despite connection failure
	}

	// Fetch metadata if agent name or description is not set
	if a.name == "" || a.description == "" {
		metadata, err := a.remoteClient.GetMetadata(context.Background())
		if err != nil {
			// Don't fail if metadata fetch fails, just log and continue
			fmt.Printf("Warning: failed to fetch metadata from remote agent %s: %v\n", a.remoteURL, err)
		} else {
			if a.name == "" {
				a.name = metadata.Name
			}
			if a.description == "" {
				a.description = metadata.Description
			}
		}
	}

	return nil
}

// IsRemote returns true if this is a remote agent
func (a *Agent) IsRemote() bool {
	return a.isRemote
}

// GetRemoteURL returns the URL of the remote agent (empty string if not remote)
func (a *Agent) GetRemoteURL() string {
	return a.remoteURL
}

// Disconnect closes the connection to a remote agent
func (a *Agent) Disconnect() error {
	if a.isRemote && a.remoteClient != nil {
		return a.remoteClient.Disconnect()
	}
	return nil
}

// GetRemoteMetadata returns metadata for remote agents, nil for local agents
func (a *Agent) GetRemoteMetadata() (map[string]string, error) {
	if !a.isRemote || a.remoteClient == nil {
		return nil, fmt.Errorf("not a remote agent")
	}

	metadata, err := a.remoteClient.GetMetadata(context.Background())
	if err != nil {
		return nil, err
	}

	// Convert to a simple map for easier access
	result := make(map[string]string)
	result["name"] = metadata.GetName()
	result["description"] = metadata.GetDescription()
	result["system_prompt"] = metadata.GetSystemPrompt()

	// Include properties
	for k, v := range metadata.GetProperties() {
		result[k] = v
	}

	return result, nil
}

// initializeMCPTools eagerly initializes MCP tools during agent creation
func (a *Agent) initializeMCPTools() error {
	ctx := context.Background()

	// Initialize regular MCP tools if available
	if len(a.mcpServers) > 0 {
		mcpTools, err := a.collectMCPTools(ctx)
		if err != nil {
			return fmt.Errorf("failed to collect MCP server tools: %w", err)
		}
		// Add MCP tools to the main tools slice with deduplication
		a.tools = deduplicateTools(append(a.tools, mcpTools...))
		fmt.Printf("Initialized %d MCP server tools\n", len(mcpTools))
	}

	// Initialize lazy MCP tools if available
	if len(a.lazyMCPConfigs) > 0 {
		lazyMCPTools := a.createLazyMCPTools()
		// Add lazy MCP tools to the main tools slice with deduplication
		a.tools = deduplicateTools(append(a.tools, lazyMCPTools...))
		fmt.Printf("Initialized %d lazy MCP tools\n", len(lazyMCPTools))
	}

	return nil
}

// getAllToolsSync returns all tools (manual + MCP) synchronously for use during initialization
func (a *Agent) getAllToolsSync() []interfaces.Tool {
	// At this point, a.tools already contains manual tools + initialized MCP tools
	return a.tools
}

// createSubAgentsFromConfig recursively creates sub-agents from YAML configuration
func createSubAgentsFromConfig(subAgentConfigs map[string]AgentConfig, variables map[string]string, llm interfaces.LLM, memory interfaces.Memory, tracer interfaces.Tracer, logger logging.Logger) ([]*Agent, error) {
	if len(subAgentConfigs) == 0 {
		return nil, nil
	}

	var subAgents []*Agent
	var errors []string

	for name, config := range subAgentConfigs {
		// Create agent options for this sub-agent
		agentOptions := []Option{
			WithAgentConfig(config, variables), // This will recursively process sub-agents of sub-agents
			WithName(name),
		}

		// Inherit infrastructure dependencies (LLM, memory, tracer, logger) but NOT tools
		if llm != nil {
			agentOptions = append(agentOptions, WithLLM(llm))
		}
		if memory != nil {
			agentOptions = append(agentOptions, WithMemory(memory))
		}
		if tracer != nil {
			agentOptions = append(agentOptions, WithTracer(tracer))
		}
		if logger != nil {
			agentOptions = append(agentOptions, WithLogger(logger))
		}
		// NOTE: Tools are NOT inherited - each sub-agent defines its own tools through MCP/YAML config

		// Create the sub-agent (this will recursively create its own sub-agents)
		subAgent, err := NewAgent(agentOptions...)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to create sub-agent '%s': %v", name, err))
			if logger != nil {
				logger.Error(context.Background(), "Failed to create sub-agent", map[string]interface{}{
					"sub_agent_name": name,
					"error":          err.Error(),
				})
			}
			continue
		}

		// Set the agent name if not already set
		if subAgent.name == "" {
			subAgent.name = name
		}

		subAgents = append(subAgents, subAgent)

		if logger != nil {
			logger.Info(context.Background(), "Successfully created sub-agent from YAML config", map[string]interface{}{
				"sub_agent_name":        name,
				"require_plan_approval": config.RequirePlanApproval,
				"has_sub_agents":        len(config.SubAgents) > 0,
			})
		}
	}

	// Return error if any sub-agents failed to create
	if len(errors) > 0 {
		return subAgents, fmt.Errorf("some sub-agents failed to create: %s", strings.Join(errors, "; "))
	}

	return subAgents, nil
}

// CreateMemoryFromConfig creates a memory instance from YAML configuration
// This function is intended to be used by agent-blueprint applications that need
// to instantiate memory from YAML config stored in the agent
func CreateMemoryFromConfig(memoryConfig map[string]interface{}, llmClient interfaces.LLM) (interfaces.Memory, error) {
	if memoryConfig == nil {
		return nil, fmt.Errorf("memory config is nil")
	}

	factory := memory.NewMemoryFactory()
	return factory.CreateMemory(memoryConfig, llmClient)
}

// GetMemoryConfig returns the stored memory configuration from YAML
// This allows agent-blueprint to access the memory config for instantiation
func (a *Agent) GetMemoryConfig() map[string]interface{} {
	return a.memoryConfig
}

// GetConfig returns the agent's configuration for inspection
func (a *Agent) GetConfig() *AgentConfig {
	if a.generatedAgentConfig == nil {
		return &AgentConfig{}
	}
	return a.generatedAgentConfig
}

// NewAgentFromConfigObject creates an agent from a pre-loaded AgentConfig object
// This is useful when you already have a loaded configuration from any source
func NewAgentFromConfigObject(ctx context.Context, config *AgentConfig, variables map[string]string, options ...Option) (*Agent, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Create options from config
	configOption := WithAgentConfig(*config, variables)

	// Extract agent name from config source or use a default
	agentName := "agent"
	if config.ConfigSource != nil && config.ConfigSource.AgentName != "" {
		agentName = config.ConfigSource.AgentName
	}
	nameOption := WithName(agentName)

	// Combine all options
	allOptions := append([]Option{configOption, nameOption}, options...)

	return NewAgent(allOptions...)
}
