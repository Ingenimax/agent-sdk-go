package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

// ToolFactory creates tools from YAML configuration
type ToolFactory struct {
	builtinFactories map[string]func(map[string]interface{}) (interfaces.Tool, error)
	customFactories  map[string]func(map[string]interface{}) (interfaces.Tool, error)
}

// NewToolFactory creates a new tool factory with builtin tools registered
func NewToolFactory() *ToolFactory {
	tf := &ToolFactory{
		builtinFactories: make(map[string]func(map[string]interface{}) (interfaces.Tool, error)),
		customFactories:  make(map[string]func(map[string]interface{}) (interfaces.Tool, error)),
	}

	// Register builtin tools
	tf.registerBuiltinTools()

	return tf
}

// registerBuiltinTools registers all available builtin tools
func (tf *ToolFactory) registerBuiltinTools() {
	// Calculator tool
	tf.builtinFactories["calculator"] = func(config map[string]interface{}) (interfaces.Tool, error) {
		return calculator.New(), nil
	}

	// Add other builtin tools as they become available
	// tf.builtinFactories["web_search"] = func(config map[string]interface{}) (interfaces.Tool, error) {
	//     // Implementation depends on available web search tool
	//     return nil, fmt.Errorf("web_search tool not implemented yet")
	// }
}

// CreateTool creates a tool from YAML configuration
func (tf *ToolFactory) CreateTool(config ToolConfigYAML) (interfaces.Tool, error) {
	return tf.CreateToolWithParentConfig(config, nil)
}

// CreateToolWithParentConfig creates a tool from YAML configuration with access to parent agent config
func (tf *ToolFactory) CreateToolWithParentConfig(config ToolConfigYAML, parentConfig *AgentConfig) (interfaces.Tool, error) {
	switch config.Type {
	case "builtin":
		return tf.createBuiltinTool(config)
	case "custom":
		return tf.createCustomTool(config)
	case "agent":
		return tf.createAgentToolWithParentConfig(config, parentConfig)
	case "mcp":
		return tf.createMCPTool(config)
	default:
		return nil, fmt.Errorf("unknown tool type: %s", config.Type)
	}
}

// createBuiltinTool creates a builtin tool
func (tf *ToolFactory) createBuiltinTool(config ToolConfigYAML) (interfaces.Tool, error) {
	factory, exists := tf.builtinFactories[config.Name]
	if !exists {
		return nil, fmt.Errorf("unknown builtin tool: %s", config.Name)
	}

	return factory(config.Config)
}

// createCustomTool creates a custom tool
func (tf *ToolFactory) createCustomTool(config ToolConfigYAML) (interfaces.Tool, error) {
	factory, exists := tf.customFactories[config.Name]
	if !exists {
		return nil, fmt.Errorf("unknown custom tool: %s. Register it first using RegisterCustomTool()", config.Name)
	}

	return factory(config.Config)
}

// createAgentTool creates a tool that wraps a remote agent (backward compatibility)
func (tf *ToolFactory) createAgentTool(config ToolConfigYAML) (interfaces.Tool, error) {
	return tf.createAgentToolWithParentConfig(config, nil)
}

// createAgentToolWithParentConfig creates a tool that wraps a remote agent with parent config inheritance
func (tf *ToolFactory) createAgentToolWithParentConfig(config ToolConfigYAML, parentConfig *AgentConfig) (interfaces.Tool, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("agent tool requires URL")
	}

	timeout := 30 * time.Second
	if config.Timeout != "" {
		if t, err := time.ParseDuration(config.Timeout); err == nil {
			timeout = t
		}
	}

	// Build agent options
	options := []Option{
		WithURL(config.URL),
		WithRemoteTimeout(timeout),
		WithName(config.Name),
		WithDescription(config.Description),
	}

	// Inherit plan approval setting from parent if available
	if parentConfig != nil && parentConfig.RequirePlanApproval != nil {
		options = append(options, WithRequirePlanApproval(*parentConfig.RequirePlanApproval))
	}

	// Create remote agent
	remoteAgent, err := NewAgent(options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create remote agent tool: %w", err)
	}

	// Need to import the tools package and use NewAgentTool
	// This is a simplified placeholder - actual implementation would need tools.NewAgentTool
	return &AgentToolWrapper{agent: remoteAgent, name: config.Name, description: config.Description}, nil
}

// createMCPTool creates an MCP tool (placeholder for future implementation)
func (tf *ToolFactory) createMCPTool(config ToolConfigYAML) (interfaces.Tool, error) {
	return nil, fmt.Errorf("MCP tool creation from YAML not implemented yet - use MCP section in agent config instead")
}

// RegisterCustomTool allows external registration of custom tools
func (tf *ToolFactory) RegisterCustomTool(name string, factory func(map[string]interface{}) (interfaces.Tool, error)) {
	tf.customFactories[name] = factory
}

// AgentToolWrapper is a simple wrapper for agent tools (placeholder implementation)
// This would normally use the actual tools.AgentTool implementation
type AgentToolWrapper struct {
	agent       *Agent
	name        string
	description string
}

// Name implements interfaces.Tool.Name
func (atw *AgentToolWrapper) Name() string {
	return atw.name
}

// Description implements interfaces.Tool.Description
func (atw *AgentToolWrapper) Description() string {
	return atw.description
}

// Parameters implements interfaces.Tool.Parameters
func (atw *AgentToolWrapper) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"input": {
			Type:        "string",
			Description: "Input to send to the agent",
			Required:    true,
		},
	}
}

// Execute implements interfaces.Tool.Execute
func (atw *AgentToolWrapper) Execute(ctx context.Context, args string) (string, error) {
	// For agent tools, Execute can be the same as Run
	return atw.agent.Run(ctx, args)
}

// Run implements interfaces.Tool.Run
func (atw *AgentToolWrapper) Run(ctx context.Context, input string) (string, error) {
	return atw.agent.Run(ctx, input)
}