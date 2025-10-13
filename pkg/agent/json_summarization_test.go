package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/structuredoutput"
)

func TestIsStructuredJSONResponse(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{"valid simple object", `{"key": "value"}`, true},
		{"valid with whitespace", `  {"key": "value"}  `, true},
		{"valid nested", `{"a":{"b":{"c":"d"}}}`, true},
		{"valid with arrays", `{"items":["a","b","c"],"count":3}`, true},
		{"valid with numbers", `{"id":123,"price":45.67,"active":true}`, true},
		{"valid empty object", `{}`, true},
		{"valid with newlines", "{\n  \"key\": \"value\"\n}", true},
		{"invalid plain text", "This is just text", false},
		{"invalid partial start", `{"incomplete`, false},
		{"invalid partial end", `incomplete"}`, false},
		{"invalid array", `["not", "an", "object"]`, false},
		{"invalid empty string", "", false},
		{"invalid whitespace", "   ", false},
		{"invalid number", "123", false},
		{"invalid boolean", "true", false},
		{"invalid string", `"just a string"`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStructuredJSONResponse(tt.content); got != tt.expected {
				t.Errorf("isStructuredJSONResponse() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConvertToHumanReadable(t *testing.T) {
	tests := []struct {
		name           string
		json           string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:           "agent response with reasoning",
			json:           `{"reasoning":["User needs help"],"status":"active","priority":"high"}`,
			wantContains:   []string{"[AI:", "reasoning: User needs help", "status: active", "priority: high"},
			wantNotContain: []string{"{", "}", `["User needs help"]`},
		},
		{
			name:           "custom fields",
			json:           `{"thoughts":["Analyzing"],"confidence":0.9,"action":"respond"}`,
			wantContains:   []string{"[AI:", "thoughts: Analyzing", "confidence: 0.9", "action: respond"},
			wantNotContain: []string{"{", "}", `["Analyzing"]`},
		},
		{
			name:           "mixed data types",
			json:           `{"message":"Hello","count":42,"enabled":true,"score":95.5}`,
			wantContains:   []string{"[AI:", "message: Hello"},
			wantNotContain: []string{"{", "}", `"message"`},
		},
		{
			name:           "empty arrays skipped",
			json:           `{"empty_array":[],"valid_field":"content","another_empty":[]}`,
			wantContains:   []string{"[AI:", "valid_field: content"},
			wantNotContain: []string{"empty_array", "another_empty"},
		},
		{
			name:           "null and empty values skipped",
			json:           `{"null_field":null,"empty_string":"","valid":"content"}`,
			wantContains:   []string{"[AI:", "valid: content"},
			wantNotContain: []string{"null_field", "empty_string"},
		},
		{
			name:           "invalid json returns fallback",
			json:           `{invalid json}`,
			wantContains:   []string{"[Generated structured response]"},
			wantNotContain: []string{"{invalid json}"},
		},
		{
			name:           "empty object returns fallback",
			json:           `{}`,
			wantContains:   []string{"[Generated structured response]"},
			wantNotContain: []string{},
		},
		{
			name:           "large object limited to 3 fields",
			json:           `{"field1":"value1","field2":"value2","field3":"value3","field4":"value4","field5":"value5"}`,
			wantContains:   []string{"[AI:"},
			wantNotContain: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToHumanReadable(tt.json)

			if got == "" {
				t.Error("convertToHumanReadable() returned empty string")
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("convertToHumanReadable() = %q, want to contain %q", got, want)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if strings.Contains(got, notWant) {
					t.Errorf("convertToHumanReadable() = %q, want to NOT contain %q", got, notWant)
				}
			}

			// Verify field limit for large JSON
			if tt.name == "large object limited to 3 fields" {
				commaCount := strings.Count(got, ",")
				if commaCount > 2 {
					t.Errorf("convertToHumanReadable() should limit to 3 fields (2 commas max), got %d commas", commaCount)
				}
			}
		})
	}
}

type mockLLMForIntegration struct {
	callCount int
}

func (m *mockLLMForIntegration) Name() string            { return "mock-llm" }
func (m *mockLLMForIntegration) SupportsStreaming() bool { return false }

func (m *mockLLMForIntegration) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	m.callCount++

	if m.callCount == 1 {
		return `{"task":"analyze","status":"progress","details":"Processing"}`, nil
	}

	// Should see summary in prompt, not raw JSON
	if strings.Contains(prompt, `"task":"analyze"`) {
		// This would be the bug - concatenated response
		return `{"task":"analyze","status":"progress"}{"task":"respond","status":"complete"}`, nil
	}

	// With fix, should see summary format
	return `{"task":"respond","status":"complete","details":"Finished"}`, nil
}

func (m *mockLLMForIntegration) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	return m.Generate(ctx, prompt, options...)
}

func TestJSONConcatenationPrevention(t *testing.T) {
	mockLLM := &mockLLMForIntegration{}
	conversationMemory := memory.NewConversationBuffer()

	type TestResponse struct {
		Task    string `json:"task"`
		Status  string `json:"status"`
		Details string `json:"details"`
	}
	responseFormat := structuredoutput.NewResponseFormat(TestResponse{})

	agent, err := NewAgent(
		WithLLM(mockLLM),
		WithMemory(conversationMemory),
		WithResponseFormat(*responseFormat),
		WithSystemPrompt("You are a helpful assistant."),
		WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "test-org")
	ctx = memory.WithConversationID(ctx, "test-conversation")

	// First call
	response1, err := agent.Run(ctx, "Analyze this request")
	if err != nil {
		t.Fatalf("agent.Run() error = %v", err)
	}

	var firstResp TestResponse
	if err := json.Unmarshal([]byte(response1), &firstResp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Second call
	response2, err := agent.Run(ctx, "Continue processing")
	if err != nil {
		t.Fatalf("agent.Run() error = %v", err)
	}

	// Verify response is single JSON object, not concatenated
	var secondResp TestResponse
	if err := json.Unmarshal([]byte(response2), &secondResp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	jsonCount := strings.Count(response2, `"task":`)
	if jsonCount != 1 {
		t.Errorf("Expected single JSON object, got %d objects in: %s", jsonCount, response2)
	}

	// Verify conversation history contains the actual responses
	history, err := conversationMemory.GetMessages(ctx)
	if err != nil {
		t.Fatalf("GetMessages() error = %v", err)
	}

	// Check that we have the expected number of messages
	if len(history) < 4 {
		t.Errorf("Expected at least 4 messages in history, got %d", len(history))
	}

	// Verify that assistant messages contain the expected responses
	assistantMessages := 0
	for _, msg := range history {
		if msg.Role == interfaces.MessageRoleAssistant {
			assistantMessages++
			// The first response should contain "analyze" task
			if assistantMessages == 1 && !strings.Contains(msg.Content, "analyze") {
				t.Errorf("First assistant message should contain 'analyze', got: %s", msg.Content)
			}
			// The second response should contain "respond" task
			if assistantMessages == 2 && !strings.Contains(msg.Content, "respond") {
				t.Errorf("Second assistant message should contain 'respond', got: %s", msg.Content)
			}
		}
	}

	if assistantMessages != 2 {
		t.Errorf("Expected 2 assistant messages, got %d", assistantMessages)
	}
}
