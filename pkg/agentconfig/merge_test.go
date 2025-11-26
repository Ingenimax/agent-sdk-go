package agentconfig

import (
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeAgentConfig_RemotePriority(t *testing.T) {
	tests := []struct {
		name     string
		remote   *agent.AgentConfig
		local    *agent.AgentConfig
		expected *agent.AgentConfig
	}{
		{
			name: "remote overrides non-empty local fields",
			remote: &agent.AgentConfig{
				Role: "Senior Software Engineer",
				Goal: "Build scalable systems",
			},
			local: &agent.AgentConfig{
				Role:      "Junior Developer",
				Goal:      "Learn basics",
				Backstory: "New to the team",
			},
			expected: &agent.AgentConfig{
				Role:      "Senior Software Engineer",
				Goal:      "Build scalable systems",
				Backstory: "New to the team", // From local since remote is empty
			},
		},
		{
			name: "local fills gaps when remote has empty fields",
			remote: &agent.AgentConfig{
				Role: "Data Analyst",
				// Goal is empty
			},
			local: &agent.AgentConfig{
				Role:      "Should be overridden",
				Goal:      "Analyze data patterns",
				Backstory: "Expert in analytics",
			},
			expected: &agent.AgentConfig{
				Role:      "Data Analyst",
				Goal:      "Analyze data patterns", // From local
				Backstory: "Expert in analytics",   // From local
			},
		},
		{
			name: "remote nil pointer fields use local values",
			remote: &agent.AgentConfig{
				Role: "DevOps Engineer",
				// MaxIterations is nil
			},
			local: &agent.AgentConfig{
				Role:          "Should be overridden",
				MaxIterations: intPtr(10),
			},
			expected: &agent.AgentConfig{
				Role:          "DevOps Engineer",
				MaxIterations: intPtr(10), // From local
			},
		},
		{
			name: "merge tools - remote tools kept, local tools appended if not present",
			remote: &agent.AgentConfig{
				Role: "Research Agent",
				Tools: []agent.ToolConfigYAML{
					{Name: "web_search", Type: "search"},
				},
			},
			local: &agent.AgentConfig{
				Role: "Should be overridden",
				Tools: []agent.ToolConfigYAML{
					{Name: "web_search", Type: "search"}, // Duplicate - should not be added
					{Name: "calculator", Type: "math"},   // New tool - should be added
				},
			},
			expected: &agent.AgentConfig{
				Role: "Research Agent",
				Tools: []agent.ToolConfigYAML{
					{Name: "web_search", Type: "search"},
					{Name: "calculator", Type: "math"}, // Added from local
				},
			},
		},
		{
			name: "deep merge LLMProvider",
			remote: &agent.AgentConfig{
				Role: "AI Assistant",
				LLMProvider: &agent.LLMProviderYAML{
					Provider: "anthropic",
					// Model is empty
				},
			},
			local: &agent.AgentConfig{
				Role: "Should be overridden",
				LLMProvider: &agent.LLMProviderYAML{
					Provider: "openai", // Should be overridden
					Model:    "gpt-4",  // Should be used
				},
			},
			expected: &agent.AgentConfig{
				Role: "AI Assistant",
				LLMProvider: &agent.LLMProviderYAML{
					Provider: "anthropic",
					Model:    "gpt-4", // From local
				},
			},
		},
		{
			name: "recursive merge SubAgents",
			remote: &agent.AgentConfig{
				Role: "Manager Agent",
				SubAgents: map[string]agent.AgentConfig{
					"worker1": {
						Role: "Worker from remote",
						Goal: "Process tasks",
					},
				},
			},
			local: &agent.AgentConfig{
				Role: "Should be overridden",
				SubAgents: map[string]agent.AgentConfig{
					"worker1": {
						Role:      "Should be overridden",
						Goal:      "Should be overridden",
						Backstory: "Experienced worker", // Should be kept
					},
					"worker2": {
						Role: "Additional worker",
						Goal: "Handle overflow",
					},
				},
			},
			expected: &agent.AgentConfig{
				Role: "Manager Agent",
				SubAgents: map[string]agent.AgentConfig{
					"worker1": {
						Role:      "Worker from remote",
						Goal:      "Process tasks",
						Backstory: "Experienced worker", // From local
					},
					"worker2": {
						Role: "Additional worker", // Added from local
						Goal: "Handle overflow",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeAgentConfig(tt.remote, tt.local, MergeStrategyRemotePriority)
			require.NotNil(t, result)

			assert.Equal(t, tt.expected.Role, result.Role)
			assert.Equal(t, tt.expected.Goal, result.Goal)
			assert.Equal(t, tt.expected.Backstory, result.Backstory)

			if tt.expected.MaxIterations != nil {
				require.NotNil(t, result.MaxIterations)
				assert.Equal(t, *tt.expected.MaxIterations, *result.MaxIterations)
			}

			if tt.expected.Tools != nil {
				require.NotNil(t, result.Tools)
				assert.Equal(t, len(tt.expected.Tools), len(result.Tools))
				for i, expectedTool := range tt.expected.Tools {
					assert.Equal(t, expectedTool.Name, result.Tools[i].Name)
					assert.Equal(t, expectedTool.Type, result.Tools[i].Type)
				}
			}

			if tt.expected.LLMProvider != nil {
				require.NotNil(t, result.LLMProvider)
				assert.Equal(t, tt.expected.LLMProvider.Provider, result.LLMProvider.Provider)
				assert.Equal(t, tt.expected.LLMProvider.Model, result.LLMProvider.Model)
			}

			if tt.expected.SubAgents != nil {
				require.NotNil(t, result.SubAgents)
				assert.Equal(t, len(tt.expected.SubAgents), len(result.SubAgents))
				for name, expectedSub := range tt.expected.SubAgents {
					resultSub, exists := result.SubAgents[name]
					require.True(t, exists, "SubAgent %s should exist", name)
					assert.Equal(t, expectedSub.Role, resultSub.Role)
					assert.Equal(t, expectedSub.Goal, resultSub.Goal)
					assert.Equal(t, expectedSub.Backstory, resultSub.Backstory)
				}
			}
		})
	}
}

func TestMergeAgentConfig_LocalPriority(t *testing.T) {
	local := &agent.AgentConfig{
		Role:      "Local Role",
		Goal:      "Local Goal",
		Backstory: "",
	}

	remote := &agent.AgentConfig{
		Role:      "Remote Role",
		Goal:      "",
		Backstory: "Remote Backstory",
	}

	result := MergeAgentConfig(local, remote, MergeStrategyLocalPriority)
	require.NotNil(t, result)

	// Local values should take priority
	assert.Equal(t, "Local Role", result.Role)
	assert.Equal(t, "Local Goal", result.Goal)
	// Remote value used when local is empty
	assert.Equal(t, "Remote Backstory", result.Backstory)
}

func TestMergeAgentConfig_NilHandling(t *testing.T) {
	config := &agent.AgentConfig{
		Role: "Test Role",
	}

	t.Run("nil remote returns local", func(t *testing.T) {
		result := MergeAgentConfig(nil, config, MergeStrategyRemotePriority)
		assert.Equal(t, config, result)
	})

	t.Run("nil local returns remote", func(t *testing.T) {
		result := MergeAgentConfig(config, nil, MergeStrategyRemotePriority)
		assert.Equal(t, config, result)
	})

	t.Run("both nil returns nil", func(t *testing.T) {
		result := MergeAgentConfig(nil, nil, MergeStrategyRemotePriority)
		assert.Nil(t, result)
	})
}

func TestMergeAgentConfig_ConfigSourceMetadata(t *testing.T) {
	remote := &agent.AgentConfig{
		Role: "Remote Role",
		ConfigSource: &agent.ConfigSourceMetadata{
			Type:   "remote",
			Source: "config-server:8080",
			Variables: map[string]string{
				"API_KEY": "remote-key",
			},
		},
	}

	local := &agent.AgentConfig{
		Role: "Local Role",
		ConfigSource: &agent.ConfigSourceMetadata{
			Type:   "local",
			Source: "/path/to/local.yaml",
			Variables: map[string]string{
				"DB_HOST": "localhost",
			},
		},
	}

	result := MergeAgentConfig(remote, local, MergeStrategyRemotePriority)
	require.NotNil(t, result)
	require.NotNil(t, result.ConfigSource)

	// Should be marked as merged
	assert.Equal(t, "merged", result.ConfigSource.Type)
	assert.Contains(t, result.ConfigSource.Source, "merged")
	assert.Contains(t, result.ConfigSource.Source, "config-server:8080")
	assert.Contains(t, result.ConfigSource.Source, "/path/to/local.yaml")

	// Variables should be merged
	assert.Equal(t, "remote-key", result.ConfigSource.Variables["API_KEY"])
	assert.Equal(t, "localhost", result.ConfigSource.Variables["DB_HOST"])
}

func intPtr(i int) *int {
	return &i
}
