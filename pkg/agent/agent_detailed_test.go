package agent

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
)

type MockLLMForDetailed struct {
	responses []string
	callCount int
}

func (m *MockLLMForDetailed) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	if m.callCount < len(m.responses) {
		response := m.responses[m.callCount]
		m.callCount++
		return response, nil
	}
	return "mock response", nil
}

func (m *MockLLMForDetailed) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	return m.Generate(ctx, prompt, options...)
}

func (m *MockLLMForDetailed) GenerateDetailed(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	content, err := m.Generate(ctx, prompt, options...)
	if err != nil {
		return nil, err
	}

	return &interfaces.LLMResponse{
		Content:    content,
		Model:      "mock-model",
		StopReason: "complete",
		Usage: &interfaces.TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
		Metadata: map[string]interface{}{
			"provider": "mock",
		},
	}, nil
}

func (m *MockLLMForDetailed) GenerateWithToolsDetailed(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return m.GenerateDetailed(ctx, prompt, options...)
}

func (m *MockLLMForDetailed) Name() string {
	return "mock-detailed-llm"
}

func (m *MockLLMForDetailed) SupportsStreaming() bool {
	return false
}

func TestAgentRunDetailed(t *testing.T) {
	// Create mock LLM
	mockLLM := &MockLLMForDetailed{
		responses: []string{"Hello, this is a test response"},
	}

	// Create agent
	agent, err := NewAgent(
		WithLLM(mockLLM),
		WithName("test-agent"),
		WithRequirePlanApproval(false), // Disable execution plans for direct testing
	)
	assert.NoError(t, err)

	// Test RunDetailed
	response, err := agent.RunDetailed(context.Background(), "Hello")

	// Verify response
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "Hello, this is a test response", response.Content)
	assert.Equal(t, "test-agent", response.AgentName)
	assert.Equal(t, "mock-model", response.Model)

	// Verify token usage
	assert.NotNil(t, response.Usage)
	assert.Equal(t, 100, response.Usage.InputTokens)
	assert.Equal(t, 50, response.Usage.OutputTokens)
	assert.Equal(t, 150, response.Usage.TotalTokens)

	// Verify execution summary
	assert.Equal(t, 1, response.ExecutionSummary.LLMCalls)
	assert.True(t, response.ExecutionSummary.ExecutionTimeMs >= 0)

	// Verify metadata
	assert.NotNil(t, response.Metadata)
	assert.Equal(t, "test-agent", response.Metadata["agent_name"])
}

func TestAgentRunBackwardCompatibility(t *testing.T) {
	// Create mock LLM
	mockLLM := &MockLLMForDetailed{
		responses: []string{"Simple response"},
	}

	// Create agent
	agent, err := NewAgent(
		WithLLM(mockLLM),
		WithName("test-agent"),
		WithRequirePlanApproval(false),
	)
	assert.NoError(t, err)

	// Test regular Run method (backward compatibility)
	response, err := agent.Run(context.Background(), "Hello")

	// Verify response
	assert.NoError(t, err)
	assert.Equal(t, "Simple response", response)

	// Ensure no detailed tracking overhead occurred
	// This is tested implicitly by the fact that the mock LLM's basic Generate method was called
}

func TestAgentRunDetailedWithTools(t *testing.T) {
	// Create mock LLM
	mockLLM := &MockLLMForDetailed{
		responses: []string{"Used tool successfully"},
	}

	// Create mock tool
	mockTool := &MockTool{
		name:        "test_tool",
		description: "A test tool",
	}

	// Create agent with tools
	agent, err := NewAgent(
		WithLLM(mockLLM),
		WithName("tool-agent"),
		WithTools(mockTool),
		WithRequirePlanApproval(false),
	)
	assert.NoError(t, err)

	// Test RunDetailed with tools
	response, err := agent.RunDetailed(context.Background(), "Use the tool")

	// Verify response
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "Used tool successfully", response.Content)

	// Verify tool usage tracking
	assert.Equal(t, 1, response.ExecutionSummary.ToolCalls)
	assert.Contains(t, response.ExecutionSummary.UsedTools, "test_tool")

	// Verify token usage
	assert.NotNil(t, response.Usage)
	assert.Equal(t, 100, response.Usage.InputTokens)
	assert.Equal(t, 50, response.Usage.OutputTokens)
	assert.Equal(t, 150, response.Usage.TotalTokens)
}

func TestUsageTrackerAggregation(t *testing.T) {
	// Test the usage tracker directly
	tracker := newUsageTracker(true)

	// Add multiple LLM usages
	usage1 := &interfaces.TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
	}
	tracker.addLLMUsage(usage1, "model1")

	usage2 := &interfaces.TokenUsage{
		InputTokens:  200,
		OutputTokens: 75,
		TotalTokens:  275,
	}
	tracker.addLLMUsage(usage2, "model2")

	// Add tool calls
	tracker.addToolCall("tool1")
	tracker.addToolCall("tool2")
	tracker.addToolCall("tool1") // Duplicate should not be added again

	// Set execution time
	tracker.setExecutionTime(1500)

	// Get results
	totalUsage, execSummary, primaryModel := tracker.getResults()

	// Verify aggregated usage
	assert.Equal(t, 300, totalUsage.InputTokens)   // 100 + 200
	assert.Equal(t, 125, totalUsage.OutputTokens)  // 50 + 75
	assert.Equal(t, 425, totalUsage.TotalTokens)   // 150 + 275

	// Verify execution summary
	assert.Equal(t, 2, execSummary.LLMCalls)
	assert.Equal(t, 2, execSummary.ToolCalls)
	assert.Equal(t, int64(1500), execSummary.ExecutionTimeMs)
	assert.Len(t, execSummary.UsedTools, 2)
	assert.Contains(t, execSummary.UsedTools, "tool1")
	assert.Contains(t, execSummary.UsedTools, "tool2")

	// Verify primary model is the first one
	assert.Equal(t, "model1", primaryModel)
}

func TestUsageTrackerDisabled(t *testing.T) {
	// Test the usage tracker with detailed=false
	tracker := newUsageTracker(false)

	// Add usage
	usage := &interfaces.TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
	}
	tracker.addLLMUsage(usage, "model1")
	tracker.addToolCall("tool1")
	tracker.setExecutionTime(1000)

	// Get results - should be nil/empty when disabled
	totalUsage, execSummary, primaryModel := tracker.getResults()
	assert.Nil(t, totalUsage)
	assert.Nil(t, execSummary)
	assert.Empty(t, primaryModel)
}

func TestAgentRunDetailedMemoryIntegration(t *testing.T) {
	// Create mock memory
	mockMemory := &MockMemory{}

	// Create mock LLM
	mockLLM := &MockLLMForDetailed{
		responses: []string{"Response with memory"},
	}

	// Create agent with memory
	agent, err := NewAgent(
		WithLLM(mockLLM),
		WithMemory(mockMemory),
		WithName("memory-agent"),
		WithRequirePlanApproval(false),
	)
	assert.NoError(t, err)

	// Test RunDetailed
	response, err := agent.RunDetailed(context.Background(), "Test with memory")

	// Verify response
	assert.NoError(t, err)
	assert.Equal(t, "Response with memory", response.Content)

	// Verify memory was used
	messages, err := mockMemory.GetMessages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, messages, 2) // User message + Assistant response

	// Verify first message (user)
	assert.Equal(t, interfaces.MessageRoleUser, messages[0].Role)
	assert.Equal(t, "Test with memory", messages[0].Content)

	// Verify second message (assistant)
	assert.Equal(t, interfaces.MessageRoleAssistant, messages[1].Role)
	assert.Equal(t, "Response with memory", messages[1].Content)
}

func TestUsageTrackerContextManagement(t *testing.T) {
	ctx := context.Background()
	tracker := newUsageTracker(true)

	// Test context with tracker
	ctx = withUsageTracker(ctx, tracker)
	retrievedTracker := getUsageTracker(ctx)

	assert.NotNil(t, retrievedTracker)
	assert.Equal(t, tracker, retrievedTracker)

	// Test context without tracker
	emptyCtx := context.Background()
	nilTracker := getUsageTracker(emptyCtx)
	assert.Nil(t, nilTracker)
}