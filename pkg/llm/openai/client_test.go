package openai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
	openai_client "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
)

func TestGenerate(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Expected Authorization header with test-key")
		}

		// Parse request body
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		response := openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "test response",
						Role:    "assistant",
					},
				},
			},
		}
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			t.Fatalf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create our wrapper client with a logger
	logger := logging.New()
	client := openai_client.NewClient("test-key",
		openai_client.WithModel("gpt-4"),
		openai_client.WithLogger(logger),
	)

	// Override the client to use our test server
	// We need to create a new client with the test server URL
	testClient := openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)
	client.Client = testClient
	client.ChatService = openai.NewChatService(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)

	// Test generation
	resp, err := client.Generate(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("Failed to generate: %v", err)
	}

	if resp != "test response" {
		t.Errorf("Expected response 'test response', got '%s'", resp)
	}
}

func TestChat(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Parse request body
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		response := openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "test response",
						Role:    "assistant",
					},
				},
			},
		}
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			t.Fatalf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create our wrapper client with a logger
	logger := logging.New()
	client := openai_client.NewClient("test-key",
		openai_client.WithModel("gpt-4"),
		openai_client.WithLogger(logger),
	)

	// Override the client to use our test server
	testClient := openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)
	client.Client = testClient
	client.ChatService = openai.NewChatService(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)

	// Test chat
	messages := []llm.Message{
		{
			Role:    "user",
			Content: "test message",
		},
	}

	resp, err := client.Chat(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("Failed to chat: %v", err)
	}

	if resp != "test response" {
		t.Errorf("Expected response 'test response', got '%s'", resp)
	}
}

func TestGenerateWithResponseFormat(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Parse request body
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Verify response format is present
		if reqBody["response_format"] == nil {
			t.Error("Expected response_format in request")
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		response := openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: `{"name": "test", "value": 123}`,
						Role:    "assistant",
					},
				},
			},
		}
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			t.Fatalf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create our wrapper client with a logger
	logger := logging.New()
	client := openai_client.NewClient("test-key",
		openai_client.WithModel("gpt-4"),
		openai_client.WithLogger(logger),
	)

	// Override the client to use our test server
	testClient := openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)
	client.Client = testClient
	client.ChatService = openai.NewChatService(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)

	// Test generation with response format
	resp, err := client.Generate(context.Background(), "test prompt",
		openai_client.WithResponseFormat(interfaces.ResponseFormat{
			Name: "test_format",
			Schema: interfaces.JSONSchema{
				"type": "object",
				"properties": map[string]interface{}{
					"name":  map[string]interface{}{"type": "string"},
					"value": map[string]interface{}{"type": "number"},
				},
			},
		}),
	)
	if err != nil {
		t.Fatalf("Failed to generate: %v", err)
	}

	expected := `{"name": "test", "value": 123}`
	if resp != expected {
		t.Errorf("Expected response '%s', got '%s'", expected, resp)
	}
}

func TestChatWithToolMessages(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Parse request body
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Verify that tool messages are present with tool_call_id
		messages := reqBody["messages"].([]interface{})
		foundToolMessage := false
		for _, msg := range messages {
			msgMap := msg.(map[string]interface{})
			if msgMap["role"] == "tool" {
				foundToolMessage = true
				if msgMap["tool_call_id"] != "test-tool-call-id" {
					t.Errorf("Expected tool_call_id 'test-tool-call-id', got '%s'", msgMap["tool_call_id"])
				}
				break
			}
		}
		if !foundToolMessage {
			t.Error("Expected tool message in request")
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		response := openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "test response",
						Role:    "assistant",
					},
				},
			},
		}
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			t.Fatalf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create our wrapper client with a logger
	logger := logging.New()
	client := openai_client.NewClient("test-key",
		openai_client.WithModel("gpt-4"),
		openai_client.WithLogger(logger),
	)

	// Override the client to use our test server
	testClient := openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)
	client.Client = testClient
	client.ChatService = openai.NewChatService(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)

	// Test chat with tool messages
	messages := []llm.Message{
		{
			Role:    "user",
			Content: "test message",
		},
		{
			Role:       "tool",
			Content:    "tool result",
			ToolCallID: "test-tool-call-id",
		},
	}

	resp, err := client.Chat(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("Failed to chat: %v", err)
	}

	if resp != "test response" {
		t.Errorf("Expected response 'test response', got '%s'", resp)
	}
}

func TestParallelToolExecution(t *testing.T) {
	// Create a test server that simulates parallel tool calls
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Parse request body
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Check if this is the first request (with tools) or second request (with tool results)
		messages := reqBody["messages"].([]interface{})
		hasToolResults := false
		for _, msg := range messages {
			msgMap := msg.(map[string]interface{})
			if msgMap["role"] == "tool" {
				hasToolResults = true
				break
			}
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		var response openai.ChatCompletion

		if !hasToolResults {
			// First request - return tool calls
			response = openai.ChatCompletion{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Content: "",
							Role:    "assistant",
							ToolCalls: []openai.ChatCompletionMessageToolCallUnion{
								{
									ID: "call_123",
									Function: openai.ChatCompletionMessageFunctionToolCallFunction{
										Name: "parallel_tool_use",
										Arguments: `{
											"tool_uses": [
												{
													"recipient_name": "test_tool_1",
													"parameters": {"param1": "value1"}
												},
												{
													"recipient_name": "test_tool_2",
													"parameters": {"param2": "value2"}
												}
											]
										}`,
									},
								},
							},
						},
					},
				},
			}
		} else {
			// Second request - return final response
			response = openai.ChatCompletion{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Content: "Final response after parallel tools",
							Role:    "assistant",
						},
					},
				},
			}
		}

		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			t.Fatalf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create our wrapper client with a logger
	logger := logging.New()
	client := openai_client.NewClient("test-key",
		openai_client.WithModel("gpt-4"),
		openai_client.WithLogger(logger),
	)

	// Override the client to use our test server
	testClient := openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)
	client.Client = testClient
	client.ChatService = openai.NewChatService(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)

	// Create mock tools
	mockTools := []interfaces.Tool{
		&mockTool{name: "test_tool_1", description: "Test tool 1"},
		&mockTool{name: "test_tool_2", description: "Test tool 2"},
	}

	// Test parallel tool execution
	resp, err := client.GenerateWithTools(context.Background(), "test prompt", mockTools)
	if err != nil {
		t.Fatalf("Failed to generate with tools: %v", err)
	}

	expected := "Final response after parallel tools"
	if resp != expected {
		t.Errorf("Expected response '%s', got '%s'", expected, resp)
	}
}

// mockTool implements interfaces.Tool for testing
type mockTool struct {
	name        string
	description string
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) DisplayName() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) Internal() bool {
	return false
}

func (m *mockTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"param": {
			Type:        "string",
			Description: "Test parameter",
			Required:    true,
		},
	}
}

func (m *mockTool) Execute(ctx context.Context, args string) (string, error) {
	return fmt.Sprintf("Result from %s: %s", m.name, args), nil
}

func (m *mockTool) Run(ctx context.Context, input string) (string, error) {
	return m.Execute(ctx, input)
}

func TestReasoningEffort(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request body
		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Verify reasoning_effort is present
		if reqBody["reasoning_effort"] != "low" {
			t.Errorf("Expected reasoning_effort 'low', got '%v'", reqBody["reasoning_effort"])
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "test", Role: "assistant"}},
			},
		})
	}))
	defer server.Close()

	// Create client
	client := openai_client.NewClient("test-key",
		openai_client.WithModel("gpt-5-mini"),
		openai_client.WithLogger(logging.New()),
	)
	client.ChatService = openai.NewChatService(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)

	// Test with reasoning effort
	_, err := client.Generate(context.Background(), "test",
		openai_client.WithReasoning("low"),
	)
	if err != nil {
		t.Fatalf("Failed to generate: %v", err)
	}
}
