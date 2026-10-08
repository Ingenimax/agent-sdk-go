package agent

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// TestConvertLLMConfigYAMLMaxTokens covers the YAML path #347 asked for:
// llm_config.max_tokens has to survive into interfaces.LLMConfig, which the
// agent then passes to the provider on every request.
func TestConvertLLMConfigYAMLMaxTokens(t *testing.T) {
	maxTokens := 4096
	temperature := 0.3

	got := convertLLMConfigYAMLToInterface(&LLMConfigYAML{
		MaxTokens:   &maxTokens,
		Temperature: &temperature,
	})
	if got == nil {
		t.Fatal("conversion returned nil")
	}
	if got.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", got.MaxTokens)
	}
	if got.Temperature != 0.3 {
		t.Errorf("Temperature = %v, want 0.3", got.Temperature)
	}
}

// TestConvertLLMConfigYAMLMaxTokensOmitted keeps "not specified" distinct from
// "set to zero", so an omitted key leaves the provider default alone.
func TestConvertLLMConfigYAMLMaxTokensOmitted(t *testing.T) {
	got := convertLLMConfigYAMLToInterface(&LLMConfigYAML{})
	if got.MaxTokens != 0 {
		t.Errorf("MaxTokens = %d, want 0 so providers keep their defaults", got.MaxTokens)
	}
}

// TestAgentPassesMaxTokensToLLM pins the end-to-end path: an agent configured
// with a max-token limit must hand it to the provider, since the provider
// plumbing is useless if the agent never forwards it.
func TestAgentPassesMaxTokensToLLM(t *testing.T) {
	llm := &maxTokensRecordingLLM{}

	agentInstance, err := NewAgent(
		WithLLM(llm),
		WithName("max-tokens-agent"),
		WithLLMConfig(interfaces.LLMConfig{MaxTokens: 1337, Temperature: 0.5}),
		WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}

	if _, err := agentInstance.Run(context.Background(), "hello"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if llm.seen == nil {
		t.Fatal("LLM received no LLMConfig")
	}
	if llm.seen.MaxTokens != 1337 {
		t.Errorf("provider saw MaxTokens = %d, want 1337", llm.seen.MaxTokens)
	}
}

type maxTokensRecordingLLM struct {
	seen *interfaces.LLMConfig
}

func (m *maxTokensRecordingLLM) record(options []interfaces.GenerateOption) {
	resolved := &interfaces.GenerateOptions{}
	for _, option := range options {
		option(resolved)
	}
	m.seen = resolved.LLMConfig
}

func (m *maxTokensRecordingLLM) Generate(_ context.Context, _ string, options ...interfaces.GenerateOption) (string, error) {
	m.record(options)
	return "ok", nil
}

func (m *maxTokensRecordingLLM) GenerateWithTools(_ context.Context, _ string, _ []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	m.record(options)
	return "ok", nil
}

func (m *maxTokensRecordingLLM) GenerateDetailed(_ context.Context, _ string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	m.record(options)
	return &interfaces.LLMResponse{Content: "ok", Model: "recording"}, nil
}

func (m *maxTokensRecordingLLM) GenerateWithToolsDetailed(_ context.Context, _ string, _ []interfaces.Tool, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	m.record(options)
	return &interfaces.LLMResponse{Content: "ok", Model: "recording"}, nil
}

func (m *maxTokensRecordingLLM) Name() string            { return "recording" }
func (m *maxTokensRecordingLLM) SupportsStreaming() bool { return false }
