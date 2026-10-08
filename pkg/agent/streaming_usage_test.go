package agent

import (
	"context"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// usageStreamLLM streams a short answer and publishes token usage on the
// terminal event, the way the OpenAI provider does after #327.
type usageStreamLLM struct {
	model string
	usage *interfaces.TokenUsage
}

func (m *usageStreamLLM) Generate(context.Context, string, ...interfaces.GenerateOption) (string, error) {
	return "ok", nil
}

func (m *usageStreamLLM) GenerateWithTools(context.Context, string, []interfaces.Tool, ...interfaces.GenerateOption) (string, error) {
	return "ok", nil
}

func (m *usageStreamLLM) GenerateDetailed(context.Context, string, ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return &interfaces.LLMResponse{Content: "ok", Model: m.model, Usage: m.usage}, nil
}

func (m *usageStreamLLM) GenerateWithToolsDetailed(ctx context.Context, p string, _ []interfaces.Tool, o ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return m.GenerateDetailed(ctx, p, o...)
}

func (m *usageStreamLLM) Name() string            { return m.model }
func (m *usageStreamLLM) SupportsStreaming() bool { return true }

func (m *usageStreamLLM) GenerateStream(ctx context.Context, _ string, _ ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	eventChan := make(chan interfaces.StreamEvent, 8)
	go func() {
		defer close(eventChan)
		eventChan <- interfaces.StreamEvent{Type: interfaces.StreamEventMessageStart, Timestamp: time.Now()}
		eventChan <- interfaces.StreamEvent{Type: interfaces.StreamEventContentDelta, Content: "ok", Timestamp: time.Now()}

		stop := interfaces.StreamEvent{Type: interfaces.StreamEventMessageStop, Timestamp: time.Now()}
		if m.usage != nil {
			stop.Metadata = map[string]interface{}{
				interfaces.MetadataKeyUsage: m.usage,
				interfaces.MetadataKeyModel: m.model,
			}
		}
		eventChan <- stop
	}()
	return eventChan, nil
}

func (m *usageStreamLLM) GenerateWithToolsStream(ctx context.Context, p string, _ []interfaces.Tool, o ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	return m.GenerateStream(ctx, p, o...)
}

// TestStreamingRunSurfacesProviderTokenUsage is the agent half of #327. The
// provider aggregating usage is useless unless the tracker reads it: streaming
// LLM methods return no *interfaces.LLMResponse, so runLocalWithTracking has
// nothing to call addLLMUsage with, and the completion event reported zero.
func TestStreamingRunSurfacesProviderTokenUsage(t *testing.T) {
	llm := &usageStreamLLM{
		model: "gpt-4o",
		usage: &interfaces.TokenUsage{InputTokens: 70, OutputTokens: 41, TotalTokens: 111},
	}

	agentInstance, err := NewAgent(
		WithLLM(llm),
		WithName("usage-stream-agent"),
		WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}

	events, err := agentInstance.RunStream(context.Background(), "hello")
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}

	var complete *interfaces.AgentStreamEvent
	for event := range events {
		if event.Type == interfaces.AgentEventComplete {
			e := event
			complete = &e
		}
	}

	if complete == nil {
		t.Fatal("run produced no completion event")
	}

	usage, ok := complete.Metadata[interfaces.MetadataKeyUsage].(*interfaces.TokenUsage)
	if !ok || usage == nil {
		t.Fatalf("completion event carried no *TokenUsage under %q; metadata=%v",
			interfaces.MetadataKeyUsage, complete.Metadata)
	}
	if usage.TotalTokens != 111 {
		t.Errorf("TotalTokens = %d, want 111 as reported by the provider", usage.TotalTokens)
	}
	if usage.InputTokens != 70 || usage.OutputTokens != 41 {
		t.Errorf("usage = %+v, want input=70 output=41", *usage)
	}
}

func TestLLMUsageFromEvent(t *testing.T) {
	usage := &interfaces.TokenUsage{TotalTokens: 9}

	for _, tc := range []struct {
		name  string
		event interfaces.StreamEvent
		ok    bool
	}{
		{"no metadata", interfaces.StreamEvent{Type: interfaces.StreamEventContentDelta}, false},
		{"empty metadata", interfaces.StreamEvent{Metadata: map[string]interface{}{}}, false},
		{"unrelated metadata", interfaces.StreamEvent{Metadata: map[string]interface{}{"iteration": 1}}, false},
		{"nil usage", interfaces.StreamEvent{Metadata: map[string]interface{}{
			interfaces.MetadataKeyUsage: (*interfaces.TokenUsage)(nil)}}, false},
		// GenerateStream publishes a map under the same key; it must not be
		// mistaken for a *TokenUsage.
		{"wrong type", interfaces.StreamEvent{Metadata: map[string]interface{}{
			interfaces.MetadataKeyUsage: map[string]interface{}{"total_tokens": 9}}}, false},
		{"real usage", interfaces.StreamEvent{Metadata: map[string]interface{}{
			interfaces.MetadataKeyUsage: usage,
			interfaces.MetadataKeyModel: "gpt-4o"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, model, ok := llmUsageFromEvent(tc.event)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !tc.ok {
				return
			}
			if got != usage {
				t.Errorf("usage = %v, want the published pointer", got)
			}
			if model != "gpt-4o" {
				t.Errorf("model = %q, want gpt-4o", model)
			}
		})
	}
}
