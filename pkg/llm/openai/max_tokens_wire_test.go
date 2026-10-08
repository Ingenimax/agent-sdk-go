package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/openai/openai-go/v2"
)

// captureRequest serves one chat completion and returns the decoded request
// body, so a test can assert what actually went on the wire rather than what a
// struct field was set to.
func captureRequest(t *testing.T, call func(client *OpenAIClient) error) map[string]interface{} {
	t.Helper()

	var body map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if body == nil {
			_ = json.Unmarshal(raw, &body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithModel("gpt-4o"), WithBaseURL(server.URL))
	if err := call(client); err != nil {
		t.Fatalf("call: %v", err)
	}
	if body == nil {
		t.Fatal("server captured no request body")
	}
	return body
}

// TestMaxTokensReachesTheWire is the point of #347: the documented knob has to
// change the request, not just a Go struct.
func TestMaxTokensReachesTheWire(t *testing.T) {
	body := captureRequest(t, func(client *OpenAIClient) error {
		_, err := client.Generate(context.Background(), "hello", interfaces.WithMaxTokens(4096))
		return err
	})

	got, present := body["max_completion_tokens"]
	if !present {
		t.Fatalf("request carried no max_completion_tokens; body keys = %v", keysOf(body))
	}
	if int(got.(float64)) != 4096 {
		t.Errorf("max_completion_tokens = %v, want 4096", got)
	}
	// Reasoning models reject max_tokens, so the older field must stay absent.
	if _, present := body["max_tokens"]; present {
		t.Error("request also sent max_tokens; only max_completion_tokens should be used")
	}
}

// TestMaxTokensUnsetOmitsTheField keeps the default behaviour untouched for
// every caller who does not opt in.
func TestMaxTokensUnsetOmitsTheField(t *testing.T) {
	body := captureRequest(t, func(client *OpenAIClient) error {
		_, err := client.Generate(context.Background(), "hello")
		return err
	})

	if _, present := body["max_completion_tokens"]; present {
		t.Error("max_completion_tokens was sent without the caller asking for it")
	}
	if _, present := body["max_tokens"]; present {
		t.Error("max_tokens was sent without the caller asking for it")
	}
}

func TestApplyMaxTokensIgnoresUnsetAndNil(t *testing.T) {
	applyMaxTokens(nil, &interfaces.LLMConfig{MaxTokens: 10}) // must not panic

	for _, config := range []*interfaces.LLMConfig{
		nil,
		{},
		{MaxTokens: 0},
		{MaxTokens: -5},
	} {
		req := newParams()
		applyMaxTokens(req, config)
		if req.MaxCompletionTokens.Valid() {
			t.Errorf("config %+v set MaxCompletionTokens, want unset", config)
		}
	}

	req := newParams()
	applyMaxTokens(req, &interfaces.LLMConfig{MaxTokens: 256})
	if !req.MaxCompletionTokens.Valid() || req.MaxCompletionTokens.Value != 256 {
		t.Errorf("MaxCompletionTokens = %+v, want 256", req.MaxCompletionTokens)
	}
}

func newParams() *openai.ChatCompletionNewParams {
	return &openai.ChatCompletionNewParams{Model: openai.ChatModel("gpt-4o")}
}

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
