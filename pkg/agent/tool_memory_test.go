package agent

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
)

// MockMemory implements the Memory interface for testing
type MockMemory struct {
	messages []interfaces.Message
}

func (m *MockMemory) AddMessage(ctx context.Context, message interfaces.Message) error {
	m.messages = append(m.messages, message)
	return nil
}

func (m *MockMemory) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return m.messages, nil
}

func (m *MockMemory) Clear(ctx context.Context) error {
	m.messages = nil
	return nil
}

// MockLLMWithTools implements LLM interface and stores tool calls in memory
type MockLLMWithTools struct {
	responses []string
	callCount int
}

func (m *MockLLMWithTools) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	if m.callCount < len(m.responses) {
		response := m.responses[m.callCount]
		m.callCount++
		return response, nil
	}
	return "mock response", nil
}

func (m *MockLLMWithTools) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	// Parse options to get memory
	params := &interfaces.GenerateOptions{}
	for _, opt := range options {
		if opt != nil {
			opt(params)
		}
	}

	// Simulate tool execution with memory storage
	if params.Memory != nil && len(tools) > 0 {
		// Simulate calling the first tool
		tool := tools[0]
		toolCall := interfaces.ToolCall{
			ID:        "test-tool-call-1",
			Name:      tool.Name(),
			Arguments: `{"test": "value"}`,
		}
		
		// Store assistant message with tool call
		_ = params.Memory.AddMessage(ctx, interfaces.Message{
			Role:      "assistant",
			Content:   "",
			ToolCalls: []interfaces.ToolCall{toolCall},
		})
		
		// Simulate tool execution
		result, err := tool.Execute(ctx, `{"test": "value"}`)
		
		// Store tool result
		if err != nil {
			_ = params.Memory.AddMessage(ctx, interfaces.Message{
				Role:       "tool",
				Content:    "Error: " + err.Error(),
				ToolCallID: toolCall.ID,
			})
		} else {
			_ = params.Memory.AddMessage(ctx, interfaces.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: toolCall.ID,
			})
		}
	}

	if m.callCount < len(m.responses) {
		response := m.responses[m.callCount]
		m.callCount++
		return response, nil
	}
	return "mock response after tool use", nil
}

func (m *MockLLMWithTools) Name() string {
	return "mock-llm-with-tools"
}

// MockTool for testing
type MockTool struct {
	name        string
	description string
}

func (m *MockTool) Name() string {
	return m.name
}

func (m *MockTool) Description() string {
	return m.description
}

func (m *MockTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"test": {
			Type:        "string",
			Description: "Test parameter",
			Required:    true,
		},
	}
}

func (m *MockTool) Run(ctx context.Context, input string) (string, error) {
	return "tool executed successfully", nil
}

func (m *MockTool) Execute(ctx context.Context, input string) (string, error) {
	return "tool executed successfully", nil
}

func TestAgentWithToolsStoresInMemory(t *testing.T) {
	// Create mock memory
	mockMemory := &MockMemory{}
	
	// Create mock LLM
	mockLLM := &MockLLMWithTools{
		responses: []string{"I'll use the test tool"},
	}
	
	// Create mock tool
	mockTool := &MockTool{
		name:        "test_tool",
		description: "A test tool",
	}
	
	// Create agent
	agent, err := NewAgent(
		WithLLM(mockLLM),
		WithMemory(mockMemory),
		WithTools(mockTool),
		WithRequirePlanApproval(false), // Disable execution plans for direct testing
		WithName("test-agent"),
	)
	assert.NoError(t, err)
	
	// Run the agent
	response, err := agent.Run(context.Background(), "Please use the test tool")
	
	// Verify no error
	assert.NoError(t, err)
	assert.NotEmpty(t, response)
	
	// Verify that tool calls and results were stored in memory
	messages, err := mockMemory.GetMessages(context.Background())
	assert.NoError(t, err)
	assert.True(t, len(messages) >= 3, "Expected at least 3 messages: user, assistant with tool call, tool result")
	
	// Check for assistant message with tool call
	foundAssistantWithToolCall := false
	foundToolResult := false
	
	for _, msg := range messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			foundAssistantWithToolCall = true
			assert.Equal(t, "test-tool-call-1", msg.ToolCalls[0].ID)
			assert.Equal(t, "test_tool", msg.ToolCalls[0].Name)
		}
		if msg.Role == "tool" {
			foundToolResult = true
			assert.Equal(t, "tool executed successfully", msg.Content)
			assert.Equal(t, "test-tool-call-1", msg.ToolCallID)
		}
	}
	
	assert.True(t, foundAssistantWithToolCall, "Expected to find assistant message with tool call")
	assert.True(t, foundToolResult, "Expected to find tool result message")
}

func TestAgentWithoutMemoryDoesNotCrash(t *testing.T) {
	// Create mock LLM
	mockLLM := &MockLLMWithTools{
		responses: []string{"I'll use the test tool"},
	}
	
	// Create mock tool
	mockTool := &MockTool{
		name:        "test_tool",
		description: "A test tool",
	}
	
	// Create agent without memory
	agent, err := NewAgent(
		WithLLM(mockLLM),
		// No memory provided
		WithTools(mockTool),
		WithRequirePlanApproval(false), // Disable execution plans for direct testing
		WithName("test-agent"),
	)
	assert.NoError(t, err)
	
	// Run the agent
	response, err := agent.Run(context.Background(), "Please use the test tool")
	
	// Verify no error (should work without memory)
	assert.NoError(t, err)
	assert.NotEmpty(t, response)
}