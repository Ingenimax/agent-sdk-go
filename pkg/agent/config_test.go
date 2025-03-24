package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatSystemPromptFromConfig(t *testing.T) {
	// Create an agent config
	config := AgentConfig{
		Role:      "{topic} Senior Data Researcher",
		Goal:      "Uncover cutting-edge developments in {topic}",
		Backstory: "You're a seasoned researcher with a knack for uncovering the latest developments in {topic}.",
	}

	// Create variables
	variables := map[string]string{
		"topic": "Artificial Intelligence",
	}

	// Format the system prompt
	systemPrompt := FormatSystemPromptFromConfig(config, variables)

	// Assert that the prompt was formatted correctly
	assert.Contains(t, systemPrompt, "# Role\nArtificial Intelligence Senior Data Researcher")
	assert.Contains(t, systemPrompt, "# Goal\nUncover cutting-edge developments in Artificial Intelligence")
	assert.Contains(t, systemPrompt, "# Backstory\nYou're a seasoned researcher with a knack for uncovering the latest developments in Artificial Intelligence.")
}

func TestGetAgentForTask(t *testing.T) {
	// Create task configs
	taskConfigs := TaskConfigs{
		"research_task": TaskConfig{
			Description:    "Conduct research on {topic}",
			ExpectedOutput: "A report on {topic}",
			Agent:          "researcher",
		},
		"reporting_task": TaskConfig{
			Description:    "Create a report on {topic}",
			ExpectedOutput: "A detailed report on {topic}",
			Agent:          "reporting_analyst",
			OutputFile:     "report.md",
		},
	}

	// Test getting an existing agent
	agent, err := GetAgentForTask(taskConfigs, "research_task")
	assert.NoError(t, err)
	assert.Equal(t, "researcher", agent)

	// Test getting another existing agent
	agent, err = GetAgentForTask(taskConfigs, "reporting_task")
	assert.NoError(t, err)
	assert.Equal(t, "reporting_analyst", agent)

	// Test getting a non-existent agent
	_, err = GetAgentForTask(taskConfigs, "nonexistent_task")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in configuration")
}
