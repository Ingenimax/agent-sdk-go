package guardrails

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

type mockTool struct {
	name        string
	description string
	runOutput   string
	execOutput  string
	runErr      error
	execErr     error
}

func (m *mockTool) Name() string                                             { return m.name }
func (m *mockTool) Description() string                                      { return m.description }
func (m *mockTool) Parameters() map[string]interfaces.ParameterSpec          { return nil }
func (m *mockTool) Run(ctx context.Context, input string) (string, error)    { return m.runOutput, m.runErr }
func (m *mockTool) Execute(ctx context.Context, args string) (string, error) { return m.execOutput, m.execErr }

func TestToolMiddleware_Execute(t *testing.T) {
	tool := &mockTool{
		name:       "test_tool",
		execOutput: "raw output with badword inside",
	}

	pipeline := NewPipeline([]Guardrail{NewContentFilter([]string{"badword"}, RedactAction)}, logging.New())
	middleware := NewToolMiddleware(tool, pipeline)

	result, err := middleware.Execute(context.Background(), "input with badword here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "raw output with badword inside" {
		t.Error("Execute() did not apply guardrails to output — guardrails are bypassed")
	}
	if result != "raw output with **** inside" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestToolMiddleware_Execute_BlockAction(t *testing.T) {
	tool := &mockTool{
		name:       "test_tool",
		execOutput: "clean output",
	}

	pipeline := NewPipeline([]Guardrail{NewContentFilter([]string{"blocked"}, BlockAction)}, logging.New())
	middleware := NewToolMiddleware(tool, pipeline)

	_, err := middleware.Execute(context.Background(), "this is blocked content")
	if err == nil {
		t.Error("expected error for blocked content, got nil")
	}
}
