package openai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestGenerate_OmitsZeroTopPByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		if _, ok := reqBody["top_p"]; ok {
			t.Fatalf("expected top_p to be omitted when no GenerateOptions are provided")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletion{Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: "ok", Role: "assistant"}}}})
	}))
	defer server.Close()

	logger := logging.New()
	client := openai_client.NewClient("test-key",
		openai_client.WithModel("gpt-4"),
		openai_client.WithLogger(logger),
	)
	client.ChatService = openai.NewChatService(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)

	if _, err := client.Generate(context.Background(), "who are you"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
}

func TestGenerate_IncludesTopPWhenExplicitlySet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		v, ok := reqBody["top_p"].(float64)
		if !ok {
			t.Fatalf("expected top_p in request when explicitly set")
		}
		if v != 0.9 {
			t.Fatalf("expected top_p=0.9, got %v", v)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletion{Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: "ok", Role: "assistant"}}}})
	}))
	defer server.Close()

	logger := logging.New()
	client := openai_client.NewClient("test-key",
		openai_client.WithModel("gpt-4"),
		openai_client.WithLogger(logger),
	)
	client.ChatService = openai.NewChatService(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)

	if _, err := client.Generate(context.Background(), "who are you", openai_client.WithTopP(0.9)); err != nil {
		t.Fatalf("Generate failed: %v", err)
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
		_ = json.NewEncoder(w).Encode(openai.ChatCompletion{
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

func TestReasoningModelUsesResponsesAPIWithToolsAndStructuredOutput(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != "/responses" {
			t.Fatalf("expected /responses endpoint, got %s", r.URL.Path)
		}

		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		if requestCount == 1 {
			reasoning := reqBody["reasoning"].(map[string]interface{})
			if reasoning["effort"] != "low" {
				t.Fatalf("expected reasoning effort low, got %v", reasoning["effort"])
			}

			tools := reqBody["tools"].([]interface{})
			if len(tools) != 1 || tools[0].(map[string]interface{})["type"] != "function" {
				t.Fatalf("expected function tool in request, got %v", tools)
			}

			text := reqBody["text"].(map[string]interface{})
			format := text["format"].(map[string]interface{})
			if format["type"] != "json_schema" || format["name"] != "answer" {
				t.Fatalf("expected json_schema response format, got %v", format)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"resp_1",
				"object":"response",
				"created_at":0,
				"model":"gpt-5-mini",
				"status":"completed",
				"output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"test_tool_1","arguments":"{\"param\":\"x\"}","status":"completed"}],
				"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3,"output_tokens_details":{"reasoning_tokens":1}}
			}`))
			return
		}

		input := reqBody["input"].([]interface{})
		lastInput := input[len(input)-1].(map[string]interface{})
		if lastInput["type"] != "function_call_output" || lastInput["call_id"] != "call_1" {
			t.Fatalf("expected function_call_output for second request, got %v", lastInput)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_2",
			"object":"response",
			"created_at":0,
			"model":"gpt-5-mini",
			"status":"completed",
			"output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"answer\":\"ok\"}","annotations":[]}]}],
			"usage":{"input_tokens":4,"output_tokens":5,"total_tokens":9,"output_tokens_details":{"reasoning_tokens":2}}
		}`))
	}))
	defer server.Close()

	client := openai_client.NewClient("test-key",
		openai_client.WithBaseURL(server.URL),
		openai_client.WithModel("gpt-5-mini"),
		openai_client.WithResponsesAPI(true),
		openai_client.WithLogger(logging.New()),
	)

	format := interfaces.ResponseFormat{
		Type: interfaces.ResponseFormatJSON,
		Name: "answer",
		Schema: interfaces.JSONSchema{
			"type": "object",
			"properties": map[string]interface{}{
				"answer": map[string]interface{}{"type": "string"},
			},
			"required": []string{"answer"},
		},
	}

	resp, err := client.GenerateWithTools(context.Background(), "test", []interfaces.Tool{
		&mockTool{name: "test_tool_1", description: "Test tool 1"},
	}, openai_client.WithReasoning("low"), openai_client.WithResponseFormat(format))
	if err != nil {
		t.Fatalf("GenerateWithTools failed: %v", err)
	}
	if resp != `{"answer":"ok"}` {
		t.Fatalf("expected structured response, got %s", resp)
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 responses requests, got %d", requestCount)
	}
}

func TestResponsesAPIIsOptInForExistingFlows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/responses" {
			t.Fatalf("did not expect default request to use /responses")
		}
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions endpoint, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: `{"answer":"ok"}`, Role: "assistant"}}},
		})
	}))
	defer server.Close()

	client := openai_client.NewClient("test-key",
		openai_client.WithBaseURL(server.URL),
		openai_client.WithModel("gpt-5-mini"),
		openai_client.WithLogger(logging.New()),
	)

	format := interfaces.ResponseFormat{
		Type:   interfaces.ResponseFormatJSON,
		Name:   "answer",
		Schema: interfaces.JSONSchema{"type": "object"},
	}

	resp, err := client.GenerateWithTools(context.Background(), "test", []interfaces.Tool{
		&mockTool{name: "test_tool_1", description: "Test tool 1"},
	}, openai_client.WithReasoning("low"), openai_client.WithResponseFormat(format))
	if err != nil {
		t.Fatalf("GenerateWithTools failed: %v", err)
	}
	if resp != `{"answer":"ok"}` {
		t.Fatalf("expected chat completion response, got %s", resp)
	}
}

func TestFileInputsUseResponsesAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("expected /responses endpoint, got %s", r.URL.Path)
		}

		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		input := reqBody["input"].([]interface{})
		message := input[0].(map[string]interface{})
		content := message["content"].([]interface{})
		if content[0].(map[string]interface{})["type"] != "input_text" {
			t.Fatalf("expected input_text first, got %v", content[0])
		}
		fileByID := content[1].(map[string]interface{})
		if fileByID["type"] != "input_file" || fileByID["file_id"] != "file_123" {
			t.Fatalf("expected file_id input_file, got %v", fileByID)
		}
		fileByURL := content[2].(map[string]interface{})
		if fileByURL["type"] != "input_file" || fileByURL["file_url"] != "https://example.com/report.pdf" {
			t.Fatalf("expected file_url input_file, got %v", fileByURL)
		}
		fileData := content[3].(map[string]interface{})
		if fileData["type"] != "input_file" || fileData["filename"] != "notes.txt" || !strings.HasPrefix(fileData["file_data"].(string), "data:text/plain;base64,") {
			t.Fatalf("expected file_data input_file, got %v", fileData)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_1",
			"object":"response",
			"created_at":0,
			"model":"gpt-4o-mini",
			"status":"completed",
			"output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"done","annotations":[]}]}],
			"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
		}`))
	}))
	defer server.Close()

	client := openai_client.NewClient("test-key",
		openai_client.WithBaseURL(server.URL),
		openai_client.WithModel("gpt-4o-mini"),
		openai_client.WithLogger(logging.New()),
	)

	resp, err := client.Generate(context.Background(), "Summarize these files.",
		openai_client.WithFileID("file_123"),
		openai_client.WithFileURL("https://example.com/report.pdf"),
		openai_client.WithFileData("notes.txt", "text/plain", []byte("hello")),
	)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp != "done" {
		t.Fatalf("expected response, got %s", resp)
	}
}

func TestUploadUserData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files" {
			t.Fatalf("expected /files endpoint, got %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		if r.FormValue("purpose") != "user_data" {
			t.Fatalf("expected purpose user_data, got %s", r.FormValue("purpose"))
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected file form field: %v", err)
		}
		defer file.Close()
		if header.Filename != "notes.txt" {
			t.Fatalf("expected filename notes.txt, got %s", header.Filename)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"file_123","object":"file","bytes":5,"created_at":0,"filename":"notes.txt","purpose":"user_data","status":"processed"}`))
	}))
	defer server.Close()

	client := openai_client.NewClient("test-key",
		openai_client.WithBaseURL(server.URL),
		openai_client.WithLogger(logging.New()),
	)

	fileID, err := client.UploadUserData(context.Background(), strings.NewReader("hello"), "notes.txt", "text/plain")
	if err != nil {
		t.Fatalf("UploadUserData failed: %v", err)
	}
	if fileID != "file_123" {
		t.Fatalf("expected file_123, got %s", fileID)
	}
}

func TestResponsesAPIValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		client  *openai_client.OpenAIClient
		options []interfaces.GenerateOption
		wantErr string
	}{
		{
			name:   "file input requires exactly one source",
			client: openai_client.NewClient("test-key", openai_client.WithModel("gpt-4o-mini")),
			options: []interfaces.GenerateOption{
				func(options *interfaces.GenerateOptions) {
					options.FileInputs = append(options.FileInputs, interfaces.FileInput{FileID: "file_123", FileURL: "https://example.com/report.pdf"})
				},
			},
			wantErr: "exactly one of FileID, FileURL, or FileData",
		},
		{
			name:   "file data requires filename",
			client: openai_client.NewClient("test-key", openai_client.WithModel("gpt-4o-mini")),
			options: []interfaces.GenerateOption{
				openai_client.WithFileData("", "text/plain", []byte("hello")),
			},
			wantErr: "with FileData requires Filename",
		},
		{
			name:   "responses api rejects unsupported stop sequences",
			client: openai_client.NewClient("test-key", openai_client.WithResponsesAPI(true)),
			options: []interfaces.GenerateOption{
				openai_client.WithStopSequences([]string{"END"}),
			},
			wantErr: "does not support stop_sequences",
		},
		{
			name:   "responses api rejects memory",
			client: openai_client.NewClient("test-key", openai_client.WithResponsesAPI(true)),
			options: []interfaces.GenerateOption{
				interfaces.WithMemory(&mockMemory{}),
			},
			wantErr: "does not support memory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.client.Generate(context.Background(), "test", tt.options...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestStreamingValidationErrors(t *testing.T) {
	t.Run("responses api streaming is explicit error", func(t *testing.T) {
		client := openai_client.NewClient("test-key", openai_client.WithResponsesAPI(true))
		_, err := client.GenerateStream(context.Background(), "test")
		if err == nil || !strings.Contains(err.Error(), "responses api streaming is not supported") {
			t.Fatalf("expected responses streaming error, got %v", err)
		}
	})

	t.Run("file inputs are not silently ignored in streaming", func(t *testing.T) {
		client := openai_client.NewClient("test-key")
		_, err := client.GenerateStream(context.Background(), "test", openai_client.WithFileID("file_123"))
		if err == nil || !strings.Contains(err.Error(), "file inputs are not supported with streaming") {
			t.Fatalf("expected file input streaming error, got %v", err)
		}
	})
}

// mockMemory is a simple in-memory implementation for testing
type mockMemory struct {
	messages []interfaces.Message
}

func (m *mockMemory) AddMessage(ctx context.Context, message interfaces.Message) error {
	m.messages = append(m.messages, message)
	return nil
}

func (m *mockMemory) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return m.messages, nil
}

func (m *mockMemory) Clear(ctx context.Context) error {
	m.messages = nil
	return nil
}

func TestGenerateWithMemory(t *testing.T) {
	tests := []struct {
		name     string
		history  []interfaces.Message
		prompt   string
		expected int // expected number of messages in request
	}{
		{
			name:     "empty memory",
			history:  nil, // No memory provided
			prompt:   "Hello",
			expected: 1, // Just the current user message
		},
		{
			name: "conversation with system message",
			history: []interfaces.Message{
				{Role: interfaces.MessageRoleSystem, Content: "You are helpful"},
				{Role: interfaces.MessageRoleUser, Content: "Hi"},
				{Role: interfaces.MessageRoleAssistant, Content: "Hello!"},
				{Role: interfaces.MessageRoleUser, Content: "How are you?"}, // Current prompt should be in memory
			},
			prompt:   "How are you?",
			expected: 4, // system + user + assistant + current user (from memory)
		},
		{
			name: "conversation with tool call",
			history: []interfaces.Message{
				{Role: interfaces.MessageRoleUser, Content: "Check status"},
				{Role: interfaces.MessageRoleAssistant, Content: "Checking..."},
				{
					Role:       interfaces.MessageRoleTool,
					Content:    "All good",
					ToolCallID: "call_123",
					Metadata:   map[string]interface{}{"tool_name": "status_check"},
				},
				{Role: interfaces.MessageRoleUser, Content: "Thanks"}, // Current prompt should be in memory
			},
			prompt:   "Thanks",
			expected: 4, // user + assistant + tool + current user (from memory)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server that validates the request structure
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Parse request body to validate messages
				var reqBody map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
					t.Fatalf("Failed to decode request body: %v", err)
				}

				// Verify messages array
				messages, ok := reqBody["messages"].([]interface{})
				if !ok {
					t.Fatalf("Expected messages array in request")
				}

				if len(messages) != tt.expected {
					t.Errorf("Expected %d messages in request, got %d", tt.expected, len(messages))
				}

				// Verify system message comes first if present
				if len(tt.history) > 0 {
					hasSystemMessage := false
					for _, msg := range tt.history {
						if msg.Role == interfaces.MessageRoleSystem {
							hasSystemMessage = true
							break
						}
					}

					if hasSystemMessage && len(messages) > 0 {
						firstMsg := messages[0].(map[string]interface{})
						if firstMsg["role"] != "system" {
							t.Errorf("Expected first message to be system message, got: %v", firstMsg["role"])
						}
					}
				}

				// Send mock response
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
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Fatalf("Failed to encode response: %v", err)
				}
			}))
			defer server.Close()

			// Create client with test server
			client := openai_client.NewClient("test-key",
				openai_client.WithBaseURL(server.URL),
				openai_client.WithLogger(logging.New()))

			var memory interfaces.Memory
			if tt.history != nil {
				memory = &mockMemory{messages: tt.history}
			}

			// Test Generate with memory
			_, err := client.Generate(context.Background(), tt.prompt,
				interfaces.WithMemory(memory))

			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
		})
	}
}
