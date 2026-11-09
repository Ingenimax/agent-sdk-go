package gemini

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
)

func TestGenerateDetailedSendsConfigAndReturnsUsage(t *testing.T) {
	api := newFakeAPI(t, reply{body: responseJSON(map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"role": "model", "parts": []any{
				thoughtPart("let me think"), textPart("Hello "), textPart("world"),
			}},
		}},
		"usageMetadata": map[string]any{"promptTokenCount": 3, "candidatesTokenCount": 5, "totalTokenCount": 8},
	})})
	client := api.client(t, WithModel(ModelGemini25Flash))

	resp, err := client.GenerateDetailed(t.Context(), "hi",
		interfaces.WithSystemMessage("be brief"),
		WithTemperature(0.5),
		WithTopP(0.9),
		WithStopSequences([]string{"END"}),
		WithReasoning("comprehensive"),
		WithResponseFormat(interfaces.ResponseFormat{
			Name: "greeting",
			Schema: interfaces.JSONSchema{
				"type":       "object",
				"properties": map[string]any{"text": map[string]any{"type": "string"}},
			},
		}),
	)
	require.NoError(t, err)

	// Thought parts are not part of the returned content.
	assert.Equal(t, "Hello world", resp.Content)
	assert.Equal(t, ModelGemini25Flash, resp.Model)
	assert.Equal(t, "gemini", resp.Metadata["provider"])
	assert.Equal(t, &interfaces.TokenUsage{InputTokens: 3, OutputTokens: 5, TotalTokens: 8}, resp.Usage)

	reqs := api.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "/v1beta/models/"+ModelGemini25Flash+":generateContent", reqs[0].path)
	body := reqs[0].body
	assert.Equal(t, "be brief", dig(body, "systemInstruction", "parts", 0, "text"))
	assert.Equal(t, "hi", dig(body, "contents", 0, "parts", 0, "text"))
	assert.Equal(t, "user", dig(body, "contents", 0, "role"))
	assert.InDelta(t, 0.5, dig(body, "generationConfig", "temperature"), 1e-6)
	assert.InDelta(t, 0.9, dig(body, "generationConfig", "topP"), 1e-6)
	assert.Equal(t, []any{"END"}, dig(body, "generationConfig", "stopSequences"))
	assert.Equal(t, "application/json", dig(body, "generationConfig", "responseMimeType"))
	assert.NotNil(t, dig(body, "generationConfig", "responseSchema", "properties", "text"))
}

func TestGenerateSkipsResponseSchemaThatDoesNotConvert(t *testing.T) {
	api := newFakeAPI(t, okReply(textPart(`{"ok":true}`)))
	client := api.client(t)

	// A numeric "type" cannot be decoded into a genai.Schema, so only the MIME type is sent.
	got, err := client.Generate(t.Context(), "hi",
		WithResponseFormat(interfaces.ResponseFormat{Schema: interfaces.JSONSchema{"type": 5}}))
	require.NoError(t, err)
	assert.Equal(t, `{"ok":true}`, got)

	body := api.requests()[0].body
	assert.Equal(t, "application/json", dig(body, "generationConfig", "responseMimeType"))
	assert.Nil(t, dig(body, "generationConfig", "responseSchema"))
}

func TestGenerateErrors(t *testing.T) {
	tests := []struct {
		name    string
		reply   reply
		wantErr string
	}{
		{"API error", errReply(http.StatusBadRequest, "bad prompt"), "failed to generate text"},
		{"no candidates", reply{body: `{"candidates":[]}`}, "no response from Gemini API"},
		{"no parts", reply{body: `{"candidates":[{"content":{"role":"model","parts":[]}}]}`}, "no response from Gemini API"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, tt.reply)
			_, err := api.client(t).Generate(t.Context(), "hi")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestGenerateRetriesFailedRequests(t *testing.T) {
	api := newFakeAPI(t,
		errReply(http.StatusInternalServerError, "try again"),
		okReply(textPart("second time lucky")),
	)
	client := api.client(t, WithRetry(retry.WithMaxAttempts(2), retry.WithInitialInterval(time.Millisecond)))

	got, err := client.Generate(t.Context(), "hi")
	require.NoError(t, err)
	assert.Equal(t, "second time lucky", got)
	assert.Len(t, api.requests(), 2)
}

func TestGenerateRetryGivesUpAfterMaxAttempts(t *testing.T) {
	api := newFakeAPI(t,
		errReply(http.StatusInternalServerError, "first"),
		errReply(http.StatusInternalServerError, "second"),
	)
	client := api.client(t, WithRetry(retry.WithMaxAttempts(2), retry.WithInitialInterval(time.Millisecond)))

	_, err := client.Generate(t.Context(), "hi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "second")
	assert.Len(t, api.requests(), 2)
}

func TestGenerateWithToolsRunsToolsAndSummarizes(t *testing.T) {
	lookup := &scriptedTool{name: "lookup", result: "42"}
	broken := &scriptedTool{name: "broken", err: errors.New("boom")}
	api := newFakeAPI(t,
		okReply(
			textPart("checking"),
			callPart("lookup", map[string]any{"q": "x"}),
			callPart("missing", map[string]any{}),
			callPart("broken", map[string]any{"q": "y"}),
		),
		okReply(callPart("lookup", map[string]any{"q": "x"})),
		okReply(textPart(" final "), textPart("answer ")),
	)
	client := api.client(t)
	mem := &mockMemory{messages: []interfaces.Message{{Role: interfaces.MessageRoleUser, Content: "question"}}}

	got, err := client.GenerateWithTools(t.Context(), "question", []interfaces.Tool{lookup, broken},
		interfaces.WithMemory(mem), interfaces.WithSystemMessage("use tools"))
	require.NoError(t, err)
	assert.Equal(t, "final  answer", got)

	assert.Equal(t, []string{`{"q":"x"}`, `{"q":"x"}`}, lookup.calls())
	assert.Equal(t, []string{`{"q":"y"}`}, broken.calls())

	reqs := api.requests()
	require.Len(t, reqs, 3)

	// The first request declares both tools and carries the system message.
	first := reqs[0].body
	assert.Equal(t, "lookup", dig(first, "tools", 0, "functionDeclarations", 0, "name"))
	assert.Equal(t, "broken", dig(first, "tools", 0, "functionDeclarations", 1, "name"))
	assert.Equal(t, "use tools", dig(first, "systemInstruction", "parts", 0, "text"))

	// The second request ends with one user turn that holds every function response.
	second := reqs[1].body
	contents := dig(second, "contents").([]any)
	last := contents[len(contents)-1]
	assert.Equal(t, "user", dig(last, "role"))
	assert.Equal(t, "42", dig(last, "parts", 0, "functionResponse", "response", "result"))
	assert.Equal(t, "tool not found: missing", dig(last, "parts", 1, "functionResponse", "response", "error"))
	assert.Equal(t, "boom", dig(last, "parts", 2, "functionResponse", "response", "error"))
	assert.Equal(t, "model", dig(contents[len(contents)-2], "role"))

	// The final request has no tools, asks for a conclusion, and carries the
	// loop warning for the repeated identical call.
	final := reqs[2].body
	assert.Nil(t, final["tools"])
	contents = dig(final, "contents").([]any)
	assert.Contains(t, dig(contents[len(contents)-1], "parts", 0, "text"), "Please provide your final response")
	repeated := dig(contents[len(contents)-2], "parts", 0, "functionResponse", "response", "result").(string)
	assert.True(t, strings.HasPrefix(repeated, "42"))
	assert.Contains(t, repeated, "[WARNING: This is call #2 to lookup")

	// Memory holds an assistant call and a tool result for each call made.
	var toolResults []string
	for _, m := range mem.messages[1:] {
		if m.Role == "tool" {
			toolResults = append(toolResults, m.Content)
		}
	}
	require.Len(t, mem.messages, 9)
	assert.Equal(t, "42", toolResults[0])
	assert.Equal(t, "Error: tool not found: missing", toolResults[1])
	assert.Equal(t, "Error: boom", toolResults[2])
	assert.Contains(t, toolResults[3], "[WARNING")
}

func TestGenerateWithToolsReturnsTextWithoutCalls(t *testing.T) {
	api := newFakeAPI(t, okReply(textPart("plain"), textPart("answer")))
	got, err := api.client(t).GenerateWithTools(t.Context(), "hi", []interfaces.Tool{&scriptedTool{name: "t"}})
	require.NoError(t, err)
	assert.Equal(t, "plain answer", got)
	assert.Len(t, api.requests(), 1)
}

func TestGenerateWithToolsDisableFinalSummary(t *testing.T) {
	api := newFakeAPI(t, okReply(textPart("partial"), callPart("t", map[string]any{"q": "1"})))
	tool := &scriptedTool{name: "t", result: "r"}

	got, err := api.client(t).GenerateWithTools(t.Context(), "hi", []interfaces.Tool{tool},
		interfaces.WithMaxIterations(1), interfaces.WithDisableFinalSummary(true))
	require.NoError(t, err)
	assert.Equal(t, "partial", got)
	assert.Len(t, api.requests(), 1)
	assert.Len(t, tool.calls(), 1)
}

func TestGenerateWithToolsSendsGenerationConfig(t *testing.T) {
	api := newFakeAPI(t,
		okReply(callPart("t", map[string]any{"q": "1"})),
		okReply(textPart(`{"a":1}`)),
	)
	tool := &scriptedTool{name: "t", result: "r"}
	setConfig := func(o *interfaces.GenerateOptions) {
		o.LLMConfig = &interfaces.LLMConfig{Temperature: 0.2, TopP: 0.4, StopSequences: []string{"STOP"}}
	}
	format := interfaces.ResponseFormat{Schema: interfaces.JSONSchema{"type": "object"}}

	got, err := api.client(t).GenerateWithTools(t.Context(), "hi", []interfaces.Tool{tool},
		setConfig, WithResponseFormat(format), interfaces.WithMaxIterations(1), nil)
	require.NoError(t, err)
	assert.Equal(t, `{"a":1}`, got)

	for i, req := range api.requests() {
		cfg := dig(req.body, "generationConfig")
		assert.InDelta(t, 0.2, dig(cfg, "temperature"), 1e-6, "request %d", i)
		assert.InDelta(t, 0.4, dig(cfg, "topP"), 1e-6, "request %d", i)
		assert.Equal(t, []any{"STOP"}, dig(cfg, "stopSequences"), "request %d", i)
		assert.Equal(t, "application/json", dig(cfg, "responseMimeType"), "request %d", i)
		assert.Equal(t, "object", dig(cfg, "responseSchema", "type"), "request %d", i)
	}
}

func TestGenerateWithToolsErrors(t *testing.T) {
	toolCall := okReply(callPart("t", map[string]any{"q": "1"}))
	tests := []struct {
		name    string
		replies []reply
		wantErr string
	}{
		{"API error", []reply{errReply(http.StatusBadRequest, "bad")}, "failed to create content"},
		{"no candidates", []reply{{body: `{"candidates":[]}`}}, "no candidates returned"},
		{"no content", []reply{{body: `{"candidates":[{"content":{"role":"model","parts":[]}}]}`}}, "no content in response"},
		{"final API error", []reply{toolCall, errReply(http.StatusBadRequest, "bad")}, "failed to create final content"},
		{"final no candidates", []reply{toolCall, {body: `{"candidates":[]}`}}, "no candidates returned in final call"},
		{"final no content", []reply{toolCall, {body: `{"candidates":[{"content":{"role":"model","parts":[]}}]}`}}, "no content in final response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, tt.replies...)
			tool := &scriptedTool{name: "t", result: "r"}
			_, err := api.client(t).GenerateWithTools(t.Context(), "hi", []interfaces.Tool{tool}, interfaces.WithMaxIterations(1))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestGenerateWithToolsStopsOnCancelledContext(t *testing.T) {
	api := newFakeAPI(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := api.client(t).GenerateWithTools(ctx, "hi", nil)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, api.requests())
}

func TestGenerateWithToolsDetailed(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		api := newFakeAPI(t, okReply(textPart("done")))
		client := api.client(t, WithModel(ModelGemini25Pro))
		resp, err := client.GenerateWithToolsDetailed(t.Context(), "hi", nil)
		require.NoError(t, err)
		assert.Equal(t, "done", resp.Content)
		assert.Equal(t, ModelGemini25Pro, resp.Model)
		assert.Nil(t, resp.Usage)
		assert.Equal(t, map[string]any{"provider": "gemini", "tools_used": true}, resp.Metadata)
	})
	t.Run("error", func(t *testing.T) {
		api := newFakeAPI(t, errReply(http.StatusBadRequest, "bad"))
		resp, err := api.client(t).GenerateWithToolsDetailed(t.Context(), "hi", nil)
		require.Error(t, err)
		assert.Nil(t, resp)
	})
}

func TestApplyMaxOutputTokens(t *testing.T) {
	clientMax := int32(100)
	tests := []struct {
		name      string
		clientMax *int32
		config    *interfaces.LLMConfig
		start     *genai.GenerationConfig
		want      *genai.GenerationConfig
	}{
		{name: "nothing set leaves config nil"},
		{name: "client default", clientMax: &clientMax, want: &genai.GenerationConfig{MaxOutputTokens: 100}},
		{
			name:      "request wins over client",
			clientMax: &clientMax,
			config:    &interfaces.LLMConfig{MaxTokens: 7},
			start:     &genai.GenerationConfig{TopK: ptr(float32(3))},
			want:      &genai.GenerationConfig{TopK: ptr(float32(3)), MaxOutputTokens: 7},
		},
		{
			name:   "clamped to int32",
			config: &interfaces.LLMConfig{MaxTokens: math.MaxInt32 + 10},
			want:   &genai.GenerationConfig{MaxOutputTokens: math.MaxInt32},
		},
		{name: "zero request keeps nil", config: &interfaces.LLMConfig{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &GeminiClient{maxOutputTokens: tt.clientMax}
			got := tt.start
			c.applyMaxOutputTokens(&got, tt.config)
			assert.Equal(t, tt.want, got)
		})
	}
}

func ptr[T any](v T) *T { return &v }
