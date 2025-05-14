package interfaces

import (
	"context"
)

// Agent represents an agent that can perform tasks
type Agent interface {
	// Run executes the agent with the given input
	Run(ctx context.Context, input string) (string, error)

	// SetSystemPrompt sets the system prompt for the agent
	SetSystemPrompt(prompt string)

	// SetResponseFormat sets the response format for the agent
	SetResponseFormat(format interface{})

	// AddTool adds a tool to the agent
	AddTool(tool Tool)
}

// Logger represents a logging interface
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}
