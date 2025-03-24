package agent

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
)

// MockLLM is a mock implementation of interfaces.LLM for testing
type MockLLM struct{}

func (m *MockLLM) Name() string {
	return "MockLLM"
}

func (m *MockLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	// Return a mock YAML response
	return `
agent:
  role: Test Agent
  goal: Help users with testing
  backstory: You were created to help with testing the auto-configuration feature.

tasks:
  test_task_1:
    description: Run a test suite and report results
    expected_output: A summary of test results with pass/fail status
  
  test_task_2:
    description: Generate test cases for a new feature
    expected_output: A list of test cases in a structured format
    output_file: test_cases.md
`, nil
}

func (m *MockLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	return m.Generate(ctx, prompt, options...)
}

func TestGenerateConfigFromSystemPrompt(t *testing.T) {
	// Create mock LLM
	mockLLM := &MockLLM{}

	// Test system prompt
	systemPrompt := "You are a test agent responsible for helping with software testing."

	// Generate configurations
	agentConfig, taskConfigs, err := GenerateConfigFromSystemPrompt(context.Background(), mockLLM, systemPrompt)

	// Assert no error
	assert.NoError(t, err)

	// Assert agent config
	assert.Equal(t, "Test Agent", agentConfig.Role)
	assert.Equal(t, "Help users with testing", agentConfig.Goal)
	assert.Equal(t, "You were created to help with testing the auto-configuration feature.", agentConfig.Backstory)

	// Assert task configs
	assert.Equal(t, 2, len(taskConfigs))

	// The order may vary due to map iteration, so we need to find the tasks by description
	for _, task := range taskConfigs {
		if task.Description == "Run a test suite and report results" {
			assert.Equal(t, "A summary of test results with pass/fail status", task.ExpectedOutput)
			assert.Empty(t, task.OutputFile)
		} else if task.Description == "Generate test cases for a new feature" {
			assert.Equal(t, "A list of test cases in a structured format", task.ExpectedOutput)
			assert.Equal(t, "test_cases.md", task.OutputFile)
		} else {
			t.Errorf("Unexpected task description: %s", task.Description)
		}
	}
}

func TestNewAgentWithAutoConfig(t *testing.T) {
	// Create mock LLM
	mockLLM := &MockLLM{}

	// Create agent with auto-config
	agent, err := NewAgentWithAutoConfig(
		context.Background(),
		WithLLM(mockLLM),
		WithSystemPrompt("You are a test agent responsible for helping with software testing."),
	)

	// Assert no error
	assert.NoError(t, err)

	// Assert agent has generated configs
	assert.NotNil(t, agent.GetGeneratedAgentConfig())
	assert.NotEmpty(t, agent.GetGeneratedTaskConfigs())

	// Assert agent config
	agentConfig := agent.GetGeneratedAgentConfig()
	assert.Equal(t, "Test Agent", agentConfig.Role)
	assert.Equal(t, "Help users with testing", agentConfig.Goal)

	// Assert task configs
	taskConfigs := agent.GetGeneratedTaskConfigs()
	assert.Equal(t, 2, len(taskConfigs))

	// Task names should be auto_task_1 and auto_task_2
	var found1, found2 bool
	for name, task := range taskConfigs {
		if name == "auto_task_1" || name == "auto_task_2" {
			// Task agent should be set to the agent name
			assert.Equal(t, "Auto-Configured Agent", task.Agent)

			if task.Description == "Run a test suite and report results" {
				found1 = true
			} else if task.Description == "Generate test cases for a new feature" {
				found2 = true
			}
		}
	}

	assert.True(t, found1, "Auto task 1 not found")
	assert.True(t, found2, "Auto task 2 not found")
}
