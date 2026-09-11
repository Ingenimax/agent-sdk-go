package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// mockCompletionServer returns an httptest server that captures the last request
// body and replies with a terminating (end_turn) completion response.
func mockCompletionServer(t *testing.T, captured *[]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*captured = body
		resp := CompletionResponse{
			ID:         "msg_test",
			Type:       "message",
			Role:       "assistant",
			Content:    []ContentBlock{{Type: "text", Text: "done"}},
			Model:      "claude-sonnet-4-5-20250929",
			StopReason: "end_turn",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// TestGenerateWithToolsEnablesThinking verifies the tools path wires adaptive
// thinking into the request when reasoning is enabled, and omits temperature/top_p
// (rejected by adaptive-capable Claude 4.6+ models).
func TestGenerateWithToolsEnablesThinking(t *testing.T) {
	var captured []byte
	server := mockCompletionServer(t, &captured)
	defer server.Close()

	client := NewClient("test-key",
		WithModel("claude-sonnet-4-6"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	_, err := client.GenerateWithTools(context.Background(), "hi", nil,
		func(o *interfaces.GenerateOptions) {
			o.LLMConfig = &interfaces.LLMConfig{EnableReasoning: true}
		},
	)
	if err != nil {
		t.Fatalf("GenerateWithTools error: %v", err)
	}

	var sent map[string]interface{}
	if err := json.Unmarshal(captured, &sent); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	thinking, ok := sent["thinking"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected thinking object in request body, got: %s", captured)
	}
	if thinking["type"] != "adaptive" {
		t.Errorf("thinking.type = %v, want adaptive", thinking["type"])
	}
	if _, ok := thinking["budget_tokens"]; ok {
		t.Errorf("adaptive thinking must not send budget_tokens, got: %s", captured)
	}
	if _, ok := sent["temperature"]; ok {
		t.Errorf("adaptive thinking must omit temperature, got: %s", captured)
	}
	if _, ok := sent["top_p"]; ok {
		t.Errorf("adaptive thinking must omit top_p, got: %s", captured)
	}
}

// TestGenerateWithToolsNoThinkingWhenDisabled verifies the tools path omits
// thinking when reasoning is not enabled.
func TestGenerateWithToolsNoThinkingWhenDisabled(t *testing.T) {
	var captured []byte
	server := mockCompletionServer(t, &captured)
	defer server.Close()

	client := NewClient("test-key",
		WithModel("claude-sonnet-4-5-20250929"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	_, err := client.GenerateWithTools(context.Background(), "hi", nil,
		func(o *interfaces.GenerateOptions) {
			o.LLMConfig = &interfaces.LLMConfig{EnableReasoning: false}
		},
	)
	if err != nil {
		t.Fatalf("GenerateWithTools error: %v", err)
	}

	var sent map[string]interface{}
	if err := json.Unmarshal(captured, &sent); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if _, ok := sent["thinking"]; ok {
		t.Errorf("expected no thinking object when reasoning disabled, got: %s", captured)
	}
}
