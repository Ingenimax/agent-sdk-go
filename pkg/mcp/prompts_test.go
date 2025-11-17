package mcp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Create mock server specifically for prompt tests
type mockPromptServer struct {
	mock.Mock
	name string
}

func (m *mockPromptServer) Initialize(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockPromptServer) ListTools(ctx context.Context) ([]interfaces.MCPTool, error) {
	args := m.Called(ctx)
	return args.Get(0).([]interfaces.MCPTool), args.Error(1)
}

func (m *mockPromptServer) CallTool(ctx context.Context, name string, toolArgs interface{}) (*interfaces.MCPToolResponse, error) {
	args := m.Called(ctx, name, toolArgs)
	return args.Get(0).(*interfaces.MCPToolResponse), args.Error(1)
}

func (m *mockPromptServer) ListResources(ctx context.Context) ([]interfaces.MCPResource, error) {
	args := m.Called(ctx)
	return args.Get(0).([]interfaces.MCPResource), args.Error(1)
}

func (m *mockPromptServer) GetResource(ctx context.Context, uri string) (*interfaces.MCPResourceContent, error) {
	args := m.Called(ctx, uri)
	return args.Get(0).(*interfaces.MCPResourceContent), args.Error(1)
}

func (m *mockPromptServer) WatchResource(ctx context.Context, uri string) (<-chan interfaces.MCPResourceUpdate, error) {
	args := m.Called(ctx, uri)
	return args.Get(0).(<-chan interfaces.MCPResourceUpdate), args.Error(1)
}

func (m *mockPromptServer) ListPrompts(ctx context.Context) ([]interfaces.MCPPrompt, error) {
	args := m.Called(ctx)
	if prompts := args.Get(0); prompts != nil {
		return prompts.([]interfaces.MCPPrompt), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockPromptServer) GetPrompt(ctx context.Context, name string, variables map[string]interface{}) (*interfaces.MCPPromptResult, error) {
	args := m.Called(ctx, name, variables)
	if result := args.Get(0); result != nil {
		return result.(*interfaces.MCPPromptResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockPromptServer) CreateMessage(ctx context.Context, request *interfaces.MCPSamplingRequest) (*interfaces.MCPSamplingResponse, error) {
	args := m.Called(ctx, request)
	return args.Get(0).(*interfaces.MCPSamplingResponse), args.Error(1)
}

func (m *mockPromptServer) GetServerInfo() (*interfaces.MCPServerInfo, error) {
	args := m.Called()
	if info := args.Get(0); info != nil {
		return info.(*interfaces.MCPServerInfo), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockPromptServer) GetCapabilities() (*interfaces.MCPServerCapabilities, error) {
	args := m.Called()
	if caps := args.Get(0); caps != nil {
		return caps.(*interfaces.MCPServerCapabilities), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockPromptServer) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestNewPromptManager(t *testing.T) {
	server1 := &mockPromptServer{name: "server1"}
	server2 := &mockPromptServer{name: "server2"}
	servers := []interfaces.MCPServer{server1, server2}

	manager := NewPromptManager(servers)

	assert.NotNil(t, manager)
	assert.Len(t, manager.servers, 2)
	assert.NotNil(t, manager.logger)
	assert.Equal(t, servers, manager.servers)
}

func TestPromptManager_ListAllPrompts(t *testing.T) {
	ctx := context.Background()

	prompts1 := []interfaces.MCPPrompt{
		{Name: "prompt1", Description: "First prompt"},
		{Name: "prompt2", Description: "Second prompt"},
	}

	prompts2 := []interfaces.MCPPrompt{
		{Name: "prompt3", Description: "Third prompt"},
	}

	t.Run("successful listing from all servers", func(t *testing.T) {
		server1 := &mockPromptServer{}
		server2 := &mockPromptServer{}
		servers := []interfaces.MCPServer{server1, server2}

		server1.On("ListPrompts", ctx).Return(prompts1, nil)
		server2.On("ListPrompts", ctx).Return(prompts2, nil)

		manager := NewPromptManager(servers)
		result, err := manager.ListAllPrompts(ctx)

		assert.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, prompts1, result["server-0"])
		assert.Equal(t, prompts2, result["server-1"])

		server1.AssertExpectations(t)
		server2.AssertExpectations(t)
	})

	t.Run("one server fails, continues with others", func(t *testing.T) {
		server1 := &mockPromptServer{}
		server2 := &mockPromptServer{}
		servers := []interfaces.MCPServer{server1, server2}

		server1.On("ListPrompts", ctx).Return(nil, errors.New("connection failed"))
		server2.On("ListPrompts", ctx).Return(prompts2, nil)

		manager := NewPromptManager(servers)
		result, err := manager.ListAllPrompts(ctx)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, prompts2, result["server-1"])
		assert.NotContains(t, result, "server-0")

		server1.AssertExpectations(t)
		server2.AssertExpectations(t)
	})

	t.Run("empty servers list", func(t *testing.T) {
		manager := NewPromptManager([]interfaces.MCPServer{})
		result, err := manager.ListAllPrompts(ctx)

		assert.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestPromptManager_FindPrompts(t *testing.T) {
	ctx := context.Background()

	prompts := []interfaces.MCPPrompt{
		{
			Name:        "code_review",
			Description: "Review code for quality",
			Metadata:    map[string]string{"category": "development"},
		},
		{
			Name:        "write_docs",
			Description: "Generate documentation",
			Metadata:    map[string]string{"category": "documentation"},
		},
		{
			Name:        "debug_help",
			Description: "Help with debugging code",
			Metadata:    map[string]string{"type": "development"},
		},
	}

	tests := []struct {
		name            string
		pattern         string
		expectedMatches int
		expectedNames   []string
	}{
		{
			name:            "search by name",
			pattern:         "code",
			expectedMatches: 2, // code_review and debug code
			expectedNames:   []string{"code_review", "debug_help"},
		},
		{
			name:            "search by description",
			pattern:         "documentation",
			expectedMatches: 1,
			expectedNames:   []string{"write_docs"},
		},
		{
			name:            "search by metadata value",
			pattern:         "development",
			expectedMatches: 2,
			expectedNames:   []string{"code_review", "debug_help"},
		},
		{
			name:            "case insensitive search",
			pattern:         "CODE",
			expectedMatches: 2,
			expectedNames:   []string{"code_review", "debug_help"},
		},
		{
			name:            "no matches",
			pattern:         "nonexistent",
			expectedMatches: 0,
			expectedNames:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &mockPromptServer{}
			server.On("ListPrompts", ctx).Return(prompts, nil)

			manager := NewPromptManager([]interfaces.MCPServer{server})
			matches, err := manager.FindPrompts(ctx, tt.pattern)

			assert.NoError(t, err)
			assert.Len(t, matches, tt.expectedMatches)

			matchedNames := make([]string, len(matches))
			for i, match := range matches {
				matchedNames[i] = match.Prompt.Name
			}
			assert.ElementsMatch(t, tt.expectedNames, matchedNames)

			server.AssertExpectations(t)
		})
	}
}

func TestPromptManager_GetPrompt(t *testing.T) {
	ctx := context.Background()
	promptName := "test_prompt"
	variables := map[string]interface{}{"var1": "value1"}

	prompts := []interfaces.MCPPrompt{
		{Name: promptName, Description: "Test prompt"},
		{Name: "other_prompt", Description: "Other prompt"},
	}

	expectedResult := &interfaces.MCPPromptResult{
		Prompt: "Generated prompt content",
		// Note: Temporarily commented out due to interface inconsistency between
		// prompt messages (string) and sampling messages (MCPContent struct)
		// Messages: []interfaces.MCPMessage{{Role: "user", Content: "Hello"}},
	}

	t.Run("successful prompt retrieval", func(t *testing.T) {
		server1 := &mockPromptServer{}
		server2 := &mockPromptServer{}
		servers := []interfaces.MCPServer{server1, server2}

		server1.On("ListPrompts", ctx).Return([]interfaces.MCPPrompt{}, nil)
		server2.On("ListPrompts", ctx).Return(prompts, nil)
		server2.On("GetPrompt", ctx, promptName, variables).Return(expectedResult, nil)

		manager := NewPromptManager(servers)
		result, err := manager.GetPrompt(ctx, promptName, variables)

		assert.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, server2, result.Server)
		assert.Equal(t, promptName, result.Prompt.Name)
		assert.Equal(t, *expectedResult, result.Result)

		server1.AssertExpectations(t)
		server2.AssertExpectations(t)
	})

	t.Run("prompt not found on any server", func(t *testing.T) {
		server := &mockPromptServer{}
		server.On("ListPrompts", ctx).Return([]interfaces.MCPPrompt{}, nil)

		manager := NewPromptManager([]interfaces.MCPServer{server})
		result, err := manager.GetPrompt(ctx, "nonexistent", variables)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt not found on any server: nonexistent")
		assert.Nil(t, result)

		server.AssertExpectations(t)
	})

	t.Run("server has prompt but GetPrompt fails", func(t *testing.T) {
		server1 := &mockPromptServer{}
		server2 := &mockPromptServer{}
		servers := []interfaces.MCPServer{server1, server2}

		// First server has prompt but GetPrompt fails
		server1.On("ListPrompts", ctx).Return(prompts, nil)
		server1.On("GetPrompt", ctx, promptName, variables).Return(nil, errors.New("execution error"))

		// Second server has prompt and succeeds
		server2.On("ListPrompts", ctx).Return(prompts, nil)
		server2.On("GetPrompt", ctx, promptName, variables).Return(expectedResult, nil)

		manager := NewPromptManager(servers)
		result, err := manager.GetPrompt(ctx, promptName, variables)

		assert.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, server2, result.Server)

		server1.AssertExpectations(t)
		server2.AssertExpectations(t)
	})
}

func TestPromptManager_ExecutePromptTemplate(t *testing.T) {
	ctx := context.Background()
	promptName := "test_template"
	variables := map[string]interface{}{"name": "John"}

	prompts := []interfaces.MCPPrompt{
		{Name: promptName, Description: "Test template"},
	}

	t.Run("single prompt string", func(t *testing.T) {
		server := &mockPromptServer{}
		server.On("ListPrompts", ctx).Return(prompts, nil)
		server.On("GetPrompt", ctx, promptName, variables).Return(&interfaces.MCPPromptResult{
			Prompt: "Hello, John!",
		}, nil)

		manager := NewPromptManager([]interfaces.MCPServer{server})
		content, err := manager.ExecutePromptTemplate(ctx, promptName, variables)

		assert.NoError(t, err)
		assert.Equal(t, "Hello, John!", content)

		server.AssertExpectations(t)
	})

	t.Run("multiple messages", func(t *testing.T) {
		server := &mockPromptServer{}
		server.On("ListPrompts", ctx).Return(prompts, nil)
		server.On("GetPrompt", ctx, promptName, variables).Return(&interfaces.MCPPromptResult{
			Messages: []interfaces.MCPPromptMessage{
				{Role: "system", Content: "You are a helpful assistant"},
				{Role: "user", Content: "Hello, John!"},
			},
		}, nil)

		manager := NewPromptManager([]interfaces.MCPServer{server})
		content, err := manager.ExecutePromptTemplate(ctx, promptName, variables)

		assert.NoError(t, err)
		expected := "system: You are a helpful assistant\nuser: Hello, John!"
		assert.Equal(t, expected, content)

		server.AssertExpectations(t)
	})

	t.Run("messages without roles", func(t *testing.T) {
		server := &mockPromptServer{}
		server.On("ListPrompts", ctx).Return(prompts, nil)
		server.On("GetPrompt", ctx, promptName, variables).Return(&interfaces.MCPPromptResult{
			Messages: []interfaces.MCPPromptMessage{
				{Content: "First message"},
				{Content: "Second message"},
			},
		}, nil)

		manager := NewPromptManager([]interfaces.MCPServer{server})
		content, err := manager.ExecutePromptTemplate(ctx, promptName, variables)

		assert.NoError(t, err)
		expected := "First message\nSecond message"
		assert.Equal(t, expected, content)

		server.AssertExpectations(t)
	})

	t.Run("no content returned", func(t *testing.T) {
		server := &mockPromptServer{}
		server.On("ListPrompts", ctx).Return(prompts, nil)
		server.On("GetPrompt", ctx, promptName, variables).Return(&interfaces.MCPPromptResult{}, nil)

		manager := NewPromptManager([]interfaces.MCPServer{server})
		content, err := manager.ExecutePromptTemplate(ctx, promptName, variables)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "returned no content")
		assert.Empty(t, content)

		server.AssertExpectations(t)
	})

	t.Run("prompt not found", func(t *testing.T) {
		server := &mockPromptServer{}
		server.On("ListPrompts", ctx).Return([]interfaces.MCPPrompt{}, nil)

		manager := NewPromptManager([]interfaces.MCPServer{server})
		content, err := manager.ExecutePromptTemplate(ctx, "nonexistent", variables)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt not found")
		assert.Empty(t, content)

		server.AssertExpectations(t)
	})
}

func TestPromptManager_GetPromptsByCategory(t *testing.T) {
	ctx := context.Background()

	prompts := []interfaces.MCPPrompt{
		{
			Name:     "prompt1",
			Metadata: map[string]string{"category": "development"},
		},
		{
			Name:     "prompt2",
			Metadata: map[string]string{"category": "documentation"},
		},
		{
			Name:     "prompt3",
			Metadata: map[string]string{"type": "development"}, // Alternative key
		},
		{
			Name:     "prompt4",
			Metadata: map[string]string{"group": "DEVELOPMENT"}, // Case insensitive
		},
		{
			Name:     "prompt5",
			Metadata: map[string]string{"other": "value"}, // No category
		},
		{
			Name:     "prompt6",
			Metadata: nil, // No metadata
		},
	}

	server := &mockPromptServer{}
	server.On("ListPrompts", ctx).Return(prompts, nil)

	manager := NewPromptManager([]interfaces.MCPServer{server})
	matches, err := manager.GetPromptsByCategory(ctx, "development")

	assert.NoError(t, err)
	assert.Len(t, matches, 3) // prompt1, prompt3, prompt4

	matchedNames := make([]string, len(matches))
	for i, match := range matches {
		matchedNames[i] = match.Prompt.Name
	}
	assert.ElementsMatch(t, []string{"prompt1", "prompt3", "prompt4"}, matchedNames)

	server.AssertExpectations(t)
}

func TestPromptManager_ValidatePromptVariables(t *testing.T) {
	manager := NewPromptManager(nil)

	tests := []struct {
		name          string
		prompt        interfaces.MCPPrompt
		variables     map[string]interface{}
		expectError   bool
		expectedError string
	}{
		{
			name: "all required variables provided",
			prompt: interfaces.MCPPrompt{
				Arguments: []interfaces.MCPPromptArgument{
					{Name: "name", Required: true},
					{Name: "optional", Required: false},
				},
			},
			variables: map[string]interface{}{
				"name": "John",
			},
			expectError: false,
		},
		{
			name: "missing required variable",
			prompt: interfaces.MCPPrompt{
				Arguments: []interfaces.MCPPromptArgument{
					{Name: "required1", Required: true},
					{Name: "required2", Required: true},
					{Name: "optional", Required: false},
				},
			},
			variables: map[string]interface{}{
				"required1": "value1",
				"optional":  "optional_value",
			},
			expectError:   true,
			expectedError: "missing required variables: required2",
		},
		{
			name: "multiple missing required variables",
			prompt: interfaces.MCPPrompt{
				Arguments: []interfaces.MCPPromptArgument{
					{Name: "req1", Required: true},
					{Name: "req2", Required: true},
					{Name: "req3", Required: true},
				},
			},
			variables:     map[string]interface{}{},
			expectError:   true,
			expectedError: "missing required variables: req1, req2, req3",
		},
		{
			name: "no required variables",
			prompt: interfaces.MCPPrompt{
				Arguments: []interfaces.MCPPromptArgument{
					{Name: "optional1", Required: false},
					{Name: "optional2", Required: false},
				},
			},
			variables:   map[string]interface{}{},
			expectError: false,
		},
		{
			name:        "prompt with no arguments",
			prompt:      interfaces.MCPPrompt{Arguments: []interfaces.MCPPromptArgument{}},
			variables:   map[string]interface{}{"extra": "value"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidatePromptVariables(tt.prompt, tt.variables)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPromptManager_BuildVariablesFromTemplate(t *testing.T) {
	manager := NewPromptManager(nil)

	t.Run("valid template parsing", func(t *testing.T) {
		template := "Hello {{.Name}}, you have {{.Count}} messages"
		data := struct {
			Name  string
			Count int
		}{
			Name:  "John",
			Count: 5,
		}

		variables, err := manager.BuildVariablesFromTemplate(template, data)

		assert.NoError(t, err)
		assert.NotNil(t, variables)
		// Currently returns empty map - implementation is incomplete
		assert.Equal(t, map[string]interface{}{}, variables)
	})

	t.Run("invalid template", func(t *testing.T) {
		template := "Hello {{.Name"
		data := map[string]interface{}{}

		variables, err := manager.BuildVariablesFromTemplate(template, data)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse template")
		assert.Nil(t, variables)
	})

	t.Run("template execution error", func(t *testing.T) {
		template := "Hello {{.InvalidField}}"
		data := map[string]interface{}{}

		variables, err := manager.BuildVariablesFromTemplate(template, data)

		// Go's template doesn't error on missing fields by default, it just outputs <no value>
		// So this test case actually succeeds and returns empty map
		assert.NoError(t, err)
		assert.NotNil(t, variables)
		assert.Equal(t, map[string]interface{}{}, variables)
	})
}

func TestPromptManager_matchesPattern(t *testing.T) {
	manager := NewPromptManager(nil)

	prompt := interfaces.MCPPrompt{
		Name:        "CodeReview",
		Description: "Review code for Quality",
		Metadata: map[string]string{
			"category": "Development",
			"type":     "Analysis",
		},
	}

	tests := []struct {
		pattern  string
		expected bool
	}{
		{"code", true},        // Name match (case insensitive)
		{"CODE", true},        // Name match (case insensitive)
		{"review", true},      // Name match
		{"quality", true},     // Description match
		{"QUALITY", true},     // Description match (case insensitive)
		{"development", true}, // Metadata key match
		{"analysis", true},    // Metadata value match
		{"nonexistent", false},
		{"", true}, // Empty pattern matches everything due to Contains
	}

	for _, tt := range tests {
		t.Run("pattern_"+tt.pattern, func(t *testing.T) {
			result := manager.matchesPattern(prompt, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPromptManager_matchesCategory(t *testing.T) {
	manager := NewPromptManager(nil)

	tests := []struct {
		name     string
		prompt   interfaces.MCPPrompt
		category string
		expected bool
	}{
		{
			name: "exact category match",
			prompt: interfaces.MCPPrompt{
				Metadata: map[string]string{"category": "development"},
			},
			category: "development",
			expected: true,
		},
		{
			name: "case insensitive category match",
			prompt: interfaces.MCPPrompt{
				Metadata: map[string]string{"category": "DEVELOPMENT"},
			},
			category: "development",
			expected: true,
		},
		{
			name: "alternative key 'type'",
			prompt: interfaces.MCPPrompt{
				Metadata: map[string]string{"type": "development"},
			},
			category: "development",
			expected: true,
		},
		{
			name: "alternative key 'group'",
			prompt: interfaces.MCPPrompt{
				Metadata: map[string]string{"group": "development"},
			},
			category: "development",
			expected: true,
		},
		{
			name: "no metadata",
			prompt: interfaces.MCPPrompt{
				Metadata: nil,
			},
			category: "development",
			expected: false,
		},
		{
			name: "no matching category",
			prompt: interfaces.MCPPrompt{
				Metadata: map[string]string{"other": "value"},
			},
			category: "development",
			expected: false,
		},
		{
			name: "empty category value",
			prompt: interfaces.MCPPrompt{
				Metadata: map[string]string{"category": ""},
			},
			category: "development",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.matchesCategory(tt.prompt, tt.category)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPromptParameterInfo(t *testing.T) {
	tests := []struct {
		name     string
		prompt   interfaces.MCPPrompt
		expected string
	}{
		{
			name: "no parameters",
			prompt: interfaces.MCPPrompt{
				Arguments: []interfaces.MCPPromptArgument{},
			},
			expected: "No parameters required",
		},
		{
			name: "single required parameter",
			prompt: interfaces.MCPPrompt{
				Arguments: []interfaces.MCPPromptArgument{
					{
						Name:        "name",
						Type:        "string",
						Required:    true,
						Description: "The user's name",
					},
				},
			},
			expected: "Parameters:\nname (string) *required* - The user's name",
		},
		{
			name: "multiple parameters with defaults",
			prompt: interfaces.MCPPrompt{
				Arguments: []interfaces.MCPPromptArgument{
					{
						Name:     "name",
						Type:     "string",
						Required: true,
					},
					{
						Name:        "count",
						Type:        "integer",
						Required:    false,
						Description: "Number of items",
						Default:     10,
					},
				},
			},
			expected: "Parameters:\nname (string) *required*\ncount (integer) - Number of items (default: 10)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPromptParameterInfo(tt.prompt)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSuggestPromptVariables(t *testing.T) {
	prompt := interfaces.MCPPrompt{
		Arguments: []interfaces.MCPPromptArgument{
			{Name: "name", Default: nil},
			{Name: "project", Default: "myproject"},
			{Name: "language", Default: nil},
			{Name: "format", Default: nil},
			{Name: "custom", Default: nil},
		},
	}

	context := map[string]interface{}{
		"user":    "john",
		"project": "myapp",
		"custom":  "context_value",
	}

	suggested := SuggestPromptVariables(prompt, context)

	// Should use default when available
	assert.Equal(t, "myproject", suggested["project"])

	// Should use context value for custom
	assert.Equal(t, "context_value", suggested["custom"])

	// Should suggest common values
	assert.Equal(t, "go", suggested["language"])
	assert.Equal(t, "markdown", suggested["format"])

	// Should use context for name (mapped from user)
	assert.Equal(t, "john", suggested["name"])
}

// Benchmark tests
func BenchmarkPromptManager_FindPrompts(b *testing.B) {
	ctx := context.Background()
	prompts := make([]interfaces.MCPPrompt, 100)
	for i := 0; i < 100; i++ {
		prompts[i] = interfaces.MCPPrompt{
			Name:        fmt.Sprintf("prompt_%d", i),
			Description: fmt.Sprintf("Description for prompt %d", i),
			Metadata:    map[string]string{"category": "test"},
		}
	}

	server := &mockPromptServer{}
	server.On("ListPrompts", ctx).Return(prompts, nil)

	manager := NewPromptManager([]interfaces.MCPServer{server})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.FindPrompts(ctx, "prompt")
	}
}

func BenchmarkPromptManager_matchesPattern(b *testing.B) {
	manager := NewPromptManager(nil)
	prompt := interfaces.MCPPrompt{
		Name:        "TestPrompt",
		Description: "A test prompt for benchmarking",
		Metadata: map[string]string{
			"category": "test",
			"type":     "benchmark",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.matchesPattern(prompt, "test")
	}
}

func BenchmarkGetPromptParameterInfo(b *testing.B) {
	prompt := interfaces.MCPPrompt{
		Arguments: []interfaces.MCPPromptArgument{
			{Name: "name", Type: "string", Required: true, Description: "User name"},
			{Name: "count", Type: "int", Required: false, Default: 10},
			{Name: "format", Type: "string", Required: false, Description: "Output format"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetPromptParameterInfo(prompt)
	}
}
