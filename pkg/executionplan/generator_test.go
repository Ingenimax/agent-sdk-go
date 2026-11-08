package executionplan

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

const planJSON = `Here is the plan:
{"description": "look it up", "steps": [{"toolName": "search", "description": "search the web", "input": "cats", "parameters": {"q": "cats"}}]}`

// optionsRecordingLLM records the prompt and resolved options of each call.
type optionsRecordingLLM struct {
	testutil.FakeLLM
	prompts []string
	options []*interfaces.GenerateOptions
}

func (m *optionsRecordingLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	resolved := &interfaces.GenerateOptions{}
	for _, option := range options {
		option(resolved)
	}
	m.prompts = append(m.prompts, prompt)
	m.options = append(m.options, resolved)
	return m.FakeLLM.Generate(ctx, prompt)
}

func searchTools() []interfaces.Tool {
	return []interfaces.Tool{&testutil.FakeTool{ToolName: "search", Desc: "searches the web"}}
}

func TestGenerateExecutionPlanNoTools(t *testing.T) {
	llm := &optionsRecordingLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: planJSON}}
	g := NewGenerator(llm, nil, "", true)
	if _, err := g.GenerateExecutionPlan(context.Background(), "find cats"); err == nil {
		t.Error("GenerateExecutionPlan() with no tools returned no error")
	}
	if len(llm.prompts) != 0 {
		t.Errorf("LLM was called %d times, want 0", len(llm.prompts))
	}
}

func TestGenerateExecutionPlanStatus(t *testing.T) {
	tests := []struct {
		name            string
		requireApproval bool
		want            ExecutionPlanStatus
	}{
		{"approval required", true, StatusPendingApproval},
		{"approval not required", false, StatusApproved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := &optionsRecordingLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: planJSON}}
			plan, err := NewGenerator(llm, searchTools(), "", tt.requireApproval).GenerateExecutionPlan(context.Background(), "find cats")
			if err != nil {
				t.Fatalf("GenerateExecutionPlan() error = %v", err)
			}
			if plan.Status != tt.want {
				t.Errorf("Status = %q, want %q", plan.Status, tt.want)
			}
			if plan.Description != "look it up" || len(plan.Steps) != 1 || plan.Steps[0].ToolName != "search" {
				t.Errorf("plan = %+v, want the parsed plan", plan)
			}
		})
	}
}

func TestGenerateExecutionPlanErrors(t *testing.T) {
	sentinel := errors.New("model down")
	tests := []struct {
		name     string
		response string
		llmErr   error
		wantErr  string
		wantIs   error
	}{
		{"llm error", "", sentinel, "failed to generate execution plan", sentinel},
		{"no json", "no plan here", nil, "failed to parse execution plan", nil},
		{"bad json", "{not json}", nil, "failed to parse execution plan", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := &optionsRecordingLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: tt.response, Err: tt.llmErr}}
			_, err := NewGenerator(llm, searchTools(), "", true).GenerateExecutionPlan(context.Background(), "x")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("GenerateExecutionPlan() error = %v, want it to contain %q", err, tt.wantErr)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("error = %v, want it to wrap %v", err, tt.wantIs)
			}
		})
	}
}

func TestGenerateExecutionPlanOptions(t *testing.T) {
	llm := &optionsRecordingLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: planJSON}}
	if _, err := NewGenerator(llm, searchTools(), "be careful", true).GenerateExecutionPlan(context.Background(), "find cats"); err != nil {
		t.Fatalf("GenerateExecutionPlan() error = %v", err)
	}
	if len(llm.options) != 1 {
		t.Fatalf("LLM called %d times, want 1", len(llm.options))
	}
	if got := llm.options[0].SystemMessage; got != "be careful" {
		t.Errorf("SystemMessage = %q, want the system prompt", got)
	}
	if llm.options[0].Parts != nil {
		t.Errorf("Parts = %+v, want none without media", llm.options[0].Parts)
	}
	if !strings.Contains(llm.prompts[0], "find cats") || !strings.Contains(llm.prompts[0], "search") {
		t.Errorf("prompt = %q, want the input and the tool name", llm.prompts[0])
	}
}

func TestGenerateExecutionPlanWithMedia(t *testing.T) {
	image := interfaces.NewPartFromBytes([]byte{1}, "image/png")
	pdf := interfaces.NewPartFromURI("https://example.com/a.pdf", "application/pdf")
	// Spare capacity would let an append without a clone write into the
	// caller's backing array.
	media := make([]*interfaces.Part, 2, 4)
	media[0], media[1] = image, pdf

	llm := &optionsRecordingLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: planJSON}}
	g := NewGenerator(llm, searchTools(), "", false)
	if _, err := g.GenerateExecutionPlanWithMedia(context.Background(), "describe this", media); err != nil {
		t.Fatalf("GenerateExecutionPlanWithMedia() error = %v", err)
	}

	parts := llm.options[0].Parts
	if len(parts) != 3 || parts[0] != image || parts[1] != pdf {
		t.Fatalf("Parts = %+v, want the media first then the prompt", parts)
	}
	if parts[2].Text != llm.prompts[0] {
		t.Errorf("last part = %q, want the planning prompt", parts[2].Text)
	}
	if extended := media[:3]; extended[2] != nil {
		t.Errorf("caller's media backing array was written to: %+v", extended[2])
	}
}

func TestGenerateExecutionPlanWithEmptyMedia(t *testing.T) {
	llm := &optionsRecordingLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: planJSON}}
	g := NewGenerator(llm, searchTools(), "", false)
	if _, err := g.GenerateExecutionPlanWithMedia(context.Background(), "x", []*interfaces.Part{}); err != nil {
		t.Fatalf("GenerateExecutionPlanWithMedia() error = %v", err)
	}
	if llm.options[0].Parts != nil {
		t.Errorf("Parts = %+v, want none for empty media", llm.options[0].Parts)
	}
}

func TestModifyExecutionPlan(t *testing.T) {
	original := NewExecutionPlan("old", []ExecutionStep{{ToolName: "search", Input: "dogs"}})
	original.TaskID = "task-1"
	original.Status = StatusApproved

	llm := &optionsRecordingLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: planJSON}}
	modified, err := NewGenerator(llm, searchTools(), "sys", false).ModifyExecutionPlan(context.Background(), original, "use cats")
	if err != nil {
		t.Fatalf("ModifyExecutionPlan() error = %v", err)
	}
	if modified.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want the original task ID", modified.TaskID)
	}
	if modified.Status != StatusPendingApproval {
		t.Errorf("Status = %q, want %q", modified.Status, StatusPendingApproval)
	}
	if !strings.Contains(llm.prompts[0], "use cats") {
		t.Errorf("prompt = %q, want the modifications", llm.prompts[0])
	}
	if llm.options[0].SystemMessage != "sys" {
		t.Errorf("SystemMessage = %q, want the system prompt", llm.options[0].SystemMessage)
	}
}

func TestModifyExecutionPlanErrors(t *testing.T) {
	sentinel := errors.New("model down")
	tests := []struct {
		name     string
		response string
		llmErr   error
		wantErr  string
	}{
		{"llm error", "", sentinel, "failed to generate modified execution plan"},
		{"parse error", "nope", nil, "failed to parse modified execution plan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := &optionsRecordingLLM{FakeLLM: testutil.FakeLLM{DefaultResponse: tt.response, Err: tt.llmErr}}
			plan := NewExecutionPlan("old", nil)
			_, err := NewGenerator(llm, searchTools(), "", false).ModifyExecutionPlan(context.Background(), plan, "x")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ModifyExecutionPlan() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
