package agent

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
)

// MockToolMemory implements both Memory and ToolMemory interfaces for testing
type MockToolMemory struct {
	messages  []interfaces.Message
	toolCalls []MockToolCall
}

type MockToolCall struct {
	toolCall interfaces.ToolCall
	result   string
}

func (m *MockToolMemory) AddMessage(ctx context.Context, message interfaces.Message) error {
	m.messages = append(m.messages, message)
	return nil
}

func (m *MockToolMemory) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return m.messages, nil
}

func (m *MockToolMemory) Clear(ctx context.Context) error {
	m.messages = nil
	m.toolCalls = nil
	return nil
}

func (m *MockToolMemory) AddToolCall(ctx context.Context, toolCall interfaces.ToolCall, result string) error {
	m.toolCalls = append(m.toolCalls, MockToolCall{toolCall: toolCall, result: result})
	return nil
}

func (m *MockToolMemory) AddAssistantMessageWithToolCalls(ctx context.Context, content string, toolCalls []interfaces.ToolCall) error {
	m.messages = append(m.messages, interfaces.Message{
		Role:      "assistant",
		Content:   content,
		ToolCalls: toolCalls,
	})
	return nil
}

// MockMemory implements only the basic Memory interface (not ToolMemory)
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

// MockLLMWithTools implements LLM interface and supports tool callbacks
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
	// Parse options to get callback
	params := &interfaces.GenerateOptions{}
	for _, opt := range options {
		if opt != nil {
			opt(params)
		}
	}

	// Simulate tool execution with callback
	if params.ToolCallback != nil && len(tools) > 0 {
		// Simulate calling the first tool
		tool := tools[0]
		toolCall := interfaces.ToolCall{
			ID:        "test-tool-call-1",
			Name:      tool.Name(),
			Arguments: `{"test": "value"}`,
		}
		
		// Simulate tool execution
		result, err := tool.Execute(ctx, `{"test": "value"}`)
		
		// Call the callback
		params.ToolCallback(ctx, toolCall, result, err)
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

func TestAgentWithToolMemory(t *testing.T) {
	// Create mock memory that supports tool memory
	mockMemory := &MockToolMemory{}
	
	// Create mock LLM
	mockLLM := &MockLLMWithTools{
		responses: []string{"I'll use the test tool"},
	}
	
	// Create mock tool
	mockTool := &MockTool{
		name:        "test_tool",
		description: "A test tool",
	}
	
	// Create agent with tool memory enabled
	agent, err := NewAgent(
		WithLLM(mockLLM),
		WithMemory(mockMemory),
		WithTools(mockTool),
		WithToolMemory(true), // Enable tool memory
		WithRequirePlanApproval(false), // Disable execution plans for direct testing
		WithName("test-agent"),
	)
	assert.NoError(t, err)
	
	// Run the agent
	response, err := agent.Run(context.Background(), "Please use the test tool")
	
	// Verify no error
	assert.NoError(t, err)
	assert.NotEmpty(t, response)
	
	// Verify that tool calls were stored in memory
	assert.Len(t, mockMemory.toolCalls, 1, "Expected one tool call to be stored in memory")
	
	storedToolCall := mockMemory.toolCalls[0]
	assert.Equal(t, "test-tool-call-1", storedToolCall.toolCall.ID)
	assert.Equal(t, "test_tool", storedToolCall.toolCall.Name)
	assert.Equal(t, "tool executed successfully", storedToolCall.result)
}

func TestAgentWithoutToolMemory(t *testing.T) {
	// Create mock memory
	mockMemory := &MockToolMemory{}
	
	// Create mock LLM
	mockLLM := &MockLLMWithTools{
		responses: []string{"I'll use the test tool"},
	}
	
	// Create mock tool
	mockTool := &MockTool{
		name:        "test_tool",
		description: "A test tool",
	}
	
	// Create agent with tool memory disabled (default)
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
	
	// Verify that tool calls were NOT stored in memory
	assert.Len(t, mockMemory.toolCalls, 0, "Expected no tool calls to be stored in memory when tool memory is disabled")
}

func TestAgentWithRegularMemoryFallback(t *testing.T) {
	// Create regular mock memory (without ToolMemory interface)
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
	
	// Create agent with tool memory enabled but regular memory
	agent, err := NewAgent(
		WithLLM(mockLLM),
		WithMemory(mockMemory),
		WithTools(mockTool),
		WithToolMemory(true), // Enable tool memory
		WithRequirePlanApproval(false), // Disable execution plans for direct testing
		WithName("test-agent"),
	)
	assert.NoError(t, err)
	
	// Run the agent
	response, err := agent.Run(context.Background(), "Please use the test tool")
	
	// Verify no error
	assert.NoError(t, err)
	assert.NotEmpty(t, response)
	
	// Verify that tool results were stored as regular messages
	messages, err := mockMemory.GetMessages(context.Background())
	assert.NoError(t, err)
	
	// Check if any message is a tool message
	foundToolMessage := false
	for _, msg := range messages {
		if msg.Role == "tool" {
			foundToolMessage = true
			assert.Equal(t, "tool executed successfully", msg.Content)
			break
		}
	}
	assert.True(t, foundToolMessage, "Expected to find a tool message in regular memory fallback")
}