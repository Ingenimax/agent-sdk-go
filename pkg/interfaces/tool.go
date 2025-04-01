package interfaces

import "context"

// Tool represents a tool that can be used by an agent
type Tool interface {
	// Name returns the name of the tool
	Name() string

	// Description returns a description of what the tool does
	Description() string

	// Run executes the tool with the given input
	Run(ctx context.Context, input string) (string, error)

	// Parameters returns the parameters that the tool accepts
	Parameters() map[string]ParameterSpec

	// Execute executes the tool with the given arguments
	Execute(ctx context.Context, args string) (string, error)
}

// ParameterSpec defines the specification for a tool parameter
type ParameterSpec struct {
	// Type is the data type of the parameter (string, number, boolean, etc.)
	Type string

	// Description describes what the parameter is for
	Description string

	// Required indicates if the parameter is required
	Required bool

	// Default is the default value for the parameter
	Default interface{}

	// Enum is a list of possible values for the parameter
	Enum []interface{}

	// Items is the type of the items in the parameter
	Items *ParameterSpec
}

// ToolRegistry is a registry of available tools
type ToolRegistry interface {
	// Register registers a tool with the registry
	Register(tool Tool)

	// Get returns a tool by name
	Get(name string) (Tool, bool)

	// List returns all registered tools
	List() []Tool
}
