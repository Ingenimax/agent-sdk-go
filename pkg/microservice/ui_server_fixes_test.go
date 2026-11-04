package microservice

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/stretchr/testify/assert"
)

// MockLLMWithGetModel extends the existing MockLLM with GetModel method
type MockLLMWithGetModel struct {
	MockLLM
	model string
}

func (m *MockLLMWithGetModel) GetModel() string {
	return m.model
}

// MockLLMWithEmptyModel extends the existing MockLLM with GetModel method that returns empty string
type MockLLMWithEmptyModel struct {
	MockLLM
}

func (m *MockLLMWithEmptyModel) GetModel() string {
	return ""
}

// MockLLMWithEmptyName extends the existing MockLLM with Name method that returns empty string
type MockLLMWithEmptyName struct {
	MockLLM
}

func (m *MockLLMWithEmptyName) Name() string {
	return ""
}

// MockTestTool is a mock implementation of the Tool interface
type MockTestTool struct {
	name        string
	description string
}

func (m *MockTestTool) Name() string {
	return m.name
}

func (m *MockTestTool) Description() string {
	return m.description
}

func (m *MockTestTool) Run(ctx context.Context, input string) (string, error) {
	return "test result", nil
}

func (m *MockTestTool) Execute(ctx context.Context, input string) (string, error) {
	return "test result", nil
}

func (m *MockTestTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"input": {
			Type:        "string",
			Description: "Test input parameter",
			Required:    true,
		},
	}
}

func TestHTTPServerWithUI_getModelName(t *testing.T) {
	tests := []struct {
		name     string
		setupLLM func() interfaces.LLM
		expected string
	}{
		{
			name: "LLM with GetModel method returns model name",
			setupLLM: func() interfaces.LLM {
				return &MockLLMWithGetModel{
					MockLLM: MockLLM{response: "test", err: nil},
					model:   "gpt-4",
				}
			},
			expected: "gpt-4",
		},
		{
			name: "LLM with GetModel method returns empty string, fallback to Name",
			setupLLM: func() interfaces.LLM {
				return &MockLLMWithEmptyModel{
					MockLLM: MockLLM{response: "test", err: nil},
				}
			},
			expected: "mock-llm (model not specified)",
		},
		{
			name: "LLM without GetModel method, fallback to Name",
			setupLLM: func() interfaces.LLM {
				return &MockLLM{response: "test", err: nil}
			},
			expected: "mock-llm (model not specified)",
		},
		{
			name: "LLM without GetModel and Name returns empty",
			setupLLM: func() interfaces.LLM {
				return &MockLLMWithEmptyName{
					MockLLM: MockLLM{response: "test", err: nil},
				}
			},
			expected: "Unknown LLM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create agent with the test LLM
			var agentOptions []agent.Option
			if tt.setupLLM() != nil {
				agentOptions = append(agentOptions, agent.WithLLM(tt.setupLLM()))
			}
			agentOptions = append(agentOptions,
				agent.WithName("test-agent"),
				agent.WithDescription("test agent"),
				agent.WithSystemPrompt("test prompt"),
			)

			testAgent, err := agent.NewAgent(agentOptions...)
			assert.NoError(t, err)

			// Create UI server
			server := &HTTPServerWithUI{
				HTTPServer: HTTPServer{
					agent: testAgent,
				},
			}

			// Test getModelName
			result := server.getModelName()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHTTPServerWithUI_getModelName_NoLLM(t *testing.T) {
	// Test the case where getModelName handles nil LLM gracefully
	// Create a UI server with an agent that has no LLM
	server := &HTTPServerWithUI{
		HTTPServer: HTTPServer{
			agent: &agent.Agent{}, // Agent with no LLM
		},
	}

	// Test getModelName with nil LLM
	result := server.getModelName()
	assert.Equal(t, "No LLM configured", result)
}

func TestHTTPServerWithUI_getToolNames(t *testing.T) {
	tests := []struct {
		name       string
		setupTools func() []interfaces.Tool
		expected   []string
	}{
		{
			name: "Agent with multiple tools",
			setupTools: func() []interfaces.Tool {
				tool1 := &MockTestTool{name: "calculator", description: "A calculator tool"}
				tool2 := &MockTestTool{name: "search", description: "A search tool"}
				return []interfaces.Tool{tool1, tool2}
			},
			expected: []string{"calculator", "search"},
		},
		{
			name: "Agent with no tools",
			setupTools: func() []interfaces.Tool {
				return []interfaces.Tool{}
			},
			expected: []string{},
		},
		{
			name: "Agent with single tool",
			setupTools: func() []interfaces.Tool {
				tool := &MockTestTool{name: "weather", description: "A weather tool"}
				return []interfaces.Tool{tool}
			},
			expected: []string{"weather"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock LLM (required for agent creation)
			mockLLM := &MockLLM{response: "test", err: nil}

			// Create agent with test tools
			testAgent, err := agent.NewAgent(
				agent.WithLLM(mockLLM),
				agent.WithName("test-agent"),
				agent.WithDescription("test agent"),
				agent.WithSystemPrompt("test prompt"),
				agent.WithTools(tt.setupTools()...),
			)
			assert.NoError(t, err)

			// Create UI server
			server := &HTTPServerWithUI{
				HTTPServer: HTTPServer{
					agent: testAgent,
				},
			}

			// Test getToolNames
			result := server.getToolNames()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHTTPServerWithUI_getMemoryInfo(t *testing.T) {
	// EntryCount distinguishes three outcomes, which this test previously
	// conflated. It asserted 0 for an active ConversationBuffer on the premise
	// that "memory starts empty", but the read was in fact failing: memory
	// requires an org and conversation ID in the context, and getMemoryInfo
	// built its own context.Background() (#330). A failed read now reports -1
	// so it cannot be mistaken for an empty memory.
	scoped := func() context.Context {
		ctx := multitenancy.WithOrgID(context.Background(), "org-1")
		return memory.WithConversationID(ctx, "conv-1")
	}

	tests := []struct {
		name               string
		setupMemory        func() interfaces.Memory
		ctx                context.Context
		expectedType       string
		expectedStatus     string
		expectedEntryCount int
	}{
		{
			name:               "active memory, scoped context, empty",
			setupMemory:        func() interfaces.Memory { return memory.NewConversationBuffer() },
			ctx:                scoped(),
			expectedType:       "buffer",
			expectedStatus:     "active",
			expectedEntryCount: 0, // genuinely empty: the read succeeded
		},
		{
			name:               "active memory, unscoped context, count unknown",
			setupMemory:        func() interfaces.Memory { return memory.NewConversationBuffer() },
			ctx:                context.Background(),
			expectedType:       "buffer",
			expectedStatus:     "active",
			expectedEntryCount: -1, // the read failed; not the same as empty
		},
		{
			name:               "no memory",
			setupMemory:        func() interfaces.Memory { return nil },
			ctx:                scoped(),
			expectedType:       "none",
			expectedStatus:     "inactive",
			expectedEntryCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLLM := &MockLLM{response: "test", err: nil}

			agentOptions := []agent.Option{
				agent.WithLLM(mockLLM),
				agent.WithName("test-agent"),
				agent.WithDescription("test agent"),
				agent.WithSystemPrompt("test prompt"),
			}

			if mem := tt.setupMemory(); mem != nil {
				agentOptions = append(agentOptions, agent.WithMemory(mem))
			}

			testAgent, err := agent.NewAgent(agentOptions...)
			assert.NoError(t, err)

			server := &HTTPServerWithUI{
				HTTPServer: HTTPServer{agent: testAgent},
			}

			result := server.getMemoryInfo(tt.ctx)
			assert.Equal(t, tt.expectedType, result.Type)
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, tt.expectedEntryCount, result.EntryCount,
				"EntryCount: -1 means unknown, 0 means genuinely empty")
		})
	}
}

func TestHTTPServerWithUI_RemoteAgent(t *testing.T) {
	// Test remote agent handling - we can't easily create a real remote agent in tests
	// but we can test that remote agent detection works correctly

	// Create a local agent first
	mockLLM := &MockLLM{response: "test", err: nil}
	testAgent, err := agent.NewAgent(
		agent.WithLLM(mockLLM),
		agent.WithName("test-agent"),
		agent.WithSystemPrompt("test prompt"),
	)
	assert.NoError(t, err)

	// Create UI server
	server := &HTTPServerWithUI{
		HTTPServer: HTTPServer{
			agent: testAgent,
		},
	}

	// Test that it's recognized as a local agent
	assert.False(t, testAgent.IsRemote())

	// Test local agent methods work
	model := server.getModelName()
	assert.Equal(t, "mock-llm (model not specified)", model)

	systemPrompt := server.getSystemPrompt()
	assert.Equal(t, "test prompt", systemPrompt)

	memInfo := server.getMemoryInfo(context.Background())
	assert.Equal(t, "none", memInfo.Type)
	assert.Equal(t, "inactive", memInfo.Status)
}
