package gemini

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// summarize turns stream events into short strings so a whole stream can be
// compared in one assertion.
func summarize(events []interfaces.StreamEvent) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		s := string(e.Type)
		switch e.Type {
		case interfaces.StreamEventContentDelta, interfaces.StreamEventThinking:
			s += ":" + e.Content
		case interfaces.StreamEventToolUse:
			s += ":" + e.ToolCall.Name + " " + e.ToolCall.Arguments
		case interfaces.StreamEventToolResult:
			s += ":" + e.ToolCall.Name + "=" + e.Content
		}
		out = append(out, s)
	}
	return out
}

// withoutText drops content deltas whose text is in texts. Tests use it for
// text the model writes alongside tool calls: whether and where that text is
// replayed is not settled behavior, so it is left out of the comparison.
func withoutText(summary []string, texts ...string) []string {
	return slices.DeleteFunc(summary, func(s string) bool {
		text, ok := strings.CutPrefix(s, string(interfaces.StreamEventContentDelta)+":")
		return ok && slices.Contains(texts, text)
	})
}

// chunk builds one streaming chunk holding a single candidate with parts.
func chunk(parts ...map[string]any) string { return candidateJSON(parts...) }

const emptyChunk = `{"candidates":[{"finishReason":"STOP"}]}`

func TestGenerateStreamEmitsThinkingAndContent(t *testing.T) {
	api := newFakeAPI(t, streamReply(
		chunk(thoughtPart("plan")),
		chunk(textPart("Hel")),
		emptyChunk,
		chunk(textPart("lo"), map[string]any{"text": ""}),
	))
	client := api.client(t, WithModel(ModelGemini25Flash), WithThinking(true))

	ch, err := client.GenerateStream(t.Context(), "hi",
		interfaces.WithSystemMessage("sys"),
		WithTemperature(0.3),
		WithTopP(0.8),
		WithStopSequences([]string{"X"}),
		WithReasoning("minimal"),
		WithResponseFormat(interfaces.ResponseFormat{Schema: interfaces.JSONSchema{"type": "object"}}),
	)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"message_start",
		"thinking:plan",
		"content_delta:Hel",
		"content_delta:lo",
		"content_complete",
		"message_stop",
	}, summarize(collectEvents(ch)))

	reqs := api.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "/v1beta/models/"+ModelGemini25Flash+":streamGenerateContent", reqs[0].path)
	body := reqs[0].body
	assert.Equal(t, "sys", dig(body, "systemInstruction", "parts", 0, "text"))
	assert.InDelta(t, 0.3, dig(body, "generationConfig", "temperature"), 1e-6)
	assert.InDelta(t, 0.8, dig(body, "generationConfig", "topP"), 1e-6)
	assert.Equal(t, []any{"X"}, dig(body, "generationConfig", "stopSequences"))
	assert.Equal(t, "application/json", dig(body, "generationConfig", "responseMimeType"))
	assert.Equal(t, "object", dig(body, "generationConfig", "responseSchema", "type"))
	assert.Equal(t, true, dig(body, "generationConfig", "thinkingConfig", "includeThoughts"))
}

func TestGenerateStreamSendsErrorEvent(t *testing.T) {
	api := newFakeAPI(t, errReply(http.StatusInternalServerError, "server down"))
	ch, err := api.client(t).GenerateStream(t.Context(), "hi")
	require.NoError(t, err)

	events := collectEvents(ch)
	require.Len(t, events, 2)
	assert.Equal(t, interfaces.StreamEventMessageStart, events[0].Type)
	assert.Equal(t, interfaces.StreamEventError, events[1].Type)
	assert.ErrorContains(t, events[1].Error, "server down")
}

func TestGenerateWithToolsStream(t *testing.T) {
	tests := []struct {
		name      string
		replies   []reply
		options   []interfaces.GenerateOption
		want      []string
		wantCalls int
		wantErr   string
	}{
		{
			name: "model answers after tools",
			replies: []reply{
				streamReply(
					chunk(textPart("let me check")),
					chunk(callPart("lookup", map[string]any{"q": "x"}), callPart("missing", map[string]any{})),
				),
				streamReply(chunk(textPart("The answer is 42"))),
			},
			want: []string{
				"message_start",
				`tool_use:lookup {"q":"x"}`,
				"tool_use:missing {}",
				"tool_result:lookup=42",
				"tool_result:missing=Error: tool not found: missing",
				"content_delta:The answer is 42",
				"content_complete",
				"message_stop",
			},
			wantCalls: 1,
		},
		{
			name: "filtered answer without tools is replayed",
			replies: []reply{
				streamReply(chunk(textPart("direct"))),
			},
			want: []string{"message_start", "content_delta:direct", "content_complete", "message_stop"},
		},
		{
			name: "intermediate messages included on request",
			replies: []reply{
				streamReply(chunk(textPart("thinking aloud"), callPart("lookup", map[string]any{"q": "y"}))),
				streamReply(chunk(textPart("done"))),
			},
			options: []interfaces.GenerateOption{
				interfaces.WithStreamConfig(interfaces.StreamConfig{BufferSize: 4, IncludeIntermediateMessages: true}),
			},
			want: []string{
				"message_start",
				"content_delta:thinking aloud",
				`tool_use:lookup {"q":"y"}`,
				"tool_result:lookup=42",
				"content_delta:done",
				"content_complete",
				"message_stop",
			},
			wantCalls: 1,
		},
		{
			name: "empty turn after tools forces a final answer",
			replies: []reply{
				streamReply(chunk(callPart("lookup", map[string]any{"q": "z"}))),
				streamReply(emptyChunk),
				streamReply(chunk(textPart("synthesized"))),
			},
			options: []interfaces.GenerateOption{interfaces.WithMaxIterations(3)},
			want: []string{
				"message_start",
				`tool_use:lookup {"q":"z"}`,
				"tool_result:lookup=42",
				"content_delta:synthesized",
				"content_complete",
				"message_stop",
			},
			wantCalls: 1,
		},
		{
			name: "final summary disabled",
			replies: []reply{
				streamReply(chunk(callPart("lookup", map[string]any{"q": "1"}))),
			},
			options: []interfaces.GenerateOption{interfaces.WithMaxIterations(1), interfaces.WithDisableFinalSummary(true)},
			want: []string{
				"message_start",
				`tool_use:lookup {"q":"1"}`,
				"tool_result:lookup=42",
				"content_complete",
				"message_stop",
			},
			wantCalls: 1,
		},
		{
			name:    "first request fails",
			replies: []reply{errReply(http.StatusInternalServerError, "down")},
			want: []string{
				"message_start",
				"error",
			},
			wantErr: "failed to generate content stream",
		},
		{
			name: "final request fails",
			replies: []reply{
				streamReply(chunk(callPart("lookup", map[string]any{"q": "1"}))),
				errReply(http.StatusInternalServerError, "down"),
			},
			options: []interfaces.GenerateOption{interfaces.WithMaxIterations(1)},
			want: []string{
				"message_start",
				`tool_use:lookup {"q":"1"}`,
				"tool_result:lookup=42",
				"error",
			},
			wantCalls: 1,
			wantErr:   "failed to create final content",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, tt.replies...)
			lookup := scriptedTool("lookup", "42", nil)

			ch, err := api.client(t).GenerateWithToolsStream(t.Context(), "question", []interfaces.Tool{lookup}, tt.options...)
			require.NoError(t, err)

			events := collectEvents(ch)
			assert.Equal(t, tt.want, withoutText(summarize(events), "let me check"))
			if tt.wantErr != "" {
				assert.ErrorContains(t, events[len(events)-1].Error, tt.wantErr)
			}
			assert.Len(t, lookup.Calls(), tt.wantCalls)
			assert.Len(t, api.requests(), len(tt.replies))
		})
	}
}

func TestGenerateWithToolsStreamExhaustsIterations(t *testing.T) {
	api := newFakeAPI(t,
		streamReply(chunk(textPart("step one"), callPart("broken", map[string]any{"n": 1}))),
		streamReply(chunk(textPart("step two"), callPart("broken", map[string]any{"n": 2}))),
		streamReply(chunk(textPart("final"))),
	)
	broken := scriptedTool("broken", "", errors.New("boom"))

	ch, err := api.client(t).GenerateWithToolsStream(t.Context(), "question", []interfaces.Tool{broken},
		interfaces.WithSystemMessage("sys"),
		WithStopSequences([]string{"END"}),
		WithResponseFormat(interfaces.ResponseFormat{Schema: interfaces.JSONSchema{"type": "object"}}),
	)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"message_start",
		`tool_use:broken {"n":1}`,
		"tool_result:broken=Error: boom",
		`tool_use:broken {"n":2}`,
		"tool_result:broken=Error: boom",
		"content_delta:final",
		"content_complete",
		"message_stop",
	}, withoutText(summarize(collectEvents(ch)), "step one", "step two"))

	reqs := api.requests()
	require.Len(t, reqs, 3)

	// Tool turns declare the tool but do not ask for JSON output.
	for _, req := range reqs[:2] {
		assert.Equal(t, "broken", dig(req.body, "tools", 0, "functionDeclarations", 0, "name"))
		assert.Equal(t, "sys", dig(req.body, "systemInstruction", "parts", 0, "text"))
		assert.Equal(t, []any{"END"}, dig(req.body, "generationConfig", "stopSequences"))
		assert.Nil(t, dig(req.body, "generationConfig", "responseMimeType"))
	}

	// The second request replays the model's call and the tool's error.
	contents := dig(reqs[1].body, "contents").([]any)
	require.Len(t, contents, 3)
	assert.Equal(t, "broken", dig(contents[1], "parts", 0, "functionCall", "name"))
	assert.Equal(t, "Error: boom", dig(contents[2], "parts", 0, "functionResponse", "response", "result"))

	// The final request drops tools and asks for JSON.
	final := reqs[2].body
	assert.Nil(t, final["tools"])
	assert.Equal(t, "application/json", dig(final, "generationConfig", "responseMimeType"))
	contents = dig(final, "contents").([]any)
	assert.Contains(t, dig(contents[len(contents)-1], "parts", 0, "text"), "Please provide your final response")
}

func TestGenerateWithToolsStreamThinking(t *testing.T) {
	api := newFakeAPI(t, streamReply(chunk(thoughtPart("hmm"), textPart("ok"))))
	client := api.client(t, WithModel(ModelGemini25Pro), WithThinkingBudget(256))

	ch, err := client.GenerateWithToolsStream(t.Context(), "hi", nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"message_start", "thinking:hmm", "content_delta:ok", "content_complete", "message_stop"},
		summarize(collectEvents(ch)))
	assert.InDelta(t, 256, dig(api.requests()[0].body, "generationConfig", "thinkingConfig", "thinkingBudget"), 0)
}
