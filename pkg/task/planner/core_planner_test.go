package planner

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLLM records the prompt and options passed to Generate. Other LLM
// methods are not used by the planner and panic if called.
type fakeLLM struct {
	interfaces.LLM
	response  string
	err       error
	prompt    string
	systemMsg string
	generateN int
}

func (f *fakeLLM) Generate(_ context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	f.generateN++
	f.prompt = prompt
	opts := &interfaces.GenerateOptions{}
	for _, o := range options {
		o(opts)
	}
	f.systemMsg = opts.SystemMessage
	return f.response, f.err
}

// fakePlanner is a configurable AIPlanner.
type fakePlanner struct {
	plan string
	err  error
}

func (f *fakePlanner) GeneratePlan(context.Context, *core.Task) (string, error) {
	return f.plan, f.err
}

func sampleTask() *core.Task {
	return &core.Task{
		ID:          "t1",
		Name:        "Migrate DB",
		Description: "move data",
		Metadata: map[string]interface{}{
			"priority":   "high",
			"deadline":   "friday",
			"complexity": "medium",
			"type":       "ops",
			"ignored":    "x",
			"priorityN":  1,
		},
	}
}

// promptContext extracts the JSON task context embedded in the prompt.
func promptContext(t *testing.T, prompt string) map[string]interface{} {
	t.Helper()
	start := strings.Index(prompt, "{")
	end := strings.LastIndex(prompt, "}")
	require.True(t, start >= 0 && end > start, prompt)
	var ctx map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(prompt[start:end+1]), &ctx))
	return ctx
}

func TestSimpleLLMPlannerGeneratePlan(t *testing.T) {
	llm := &fakeLLM{response: "the plan"}
	p := NewSimpleLLMPlanner(llm, logging.New())

	plan, err := p.GeneratePlan(context.Background(), sampleTask())
	require.NoError(t, err)
	assert.Equal(t, "the plan", plan)
	assert.Equal(t, 1, llm.generateN)
	assert.Contains(t, llm.systemMsg, "expert task planner")
	assert.Equal(t, map[string]interface{}{
		"id":          "t1",
		"name":        "Migrate DB",
		"description": "move data",
		"priority":    "high",
		"deadline":    "friday",
		"complexity":  "medium",
		"type":        "ops",
	}, promptContext(t, llm.prompt))
}

func TestSimpleLLMPlannerWithoutMetadata(t *testing.T) {
	llm := &fakeLLM{response: strings.Repeat("x", 200)}
	p := NewSimpleLLMPlannerWithSystemPrompt(llm, nil, "custom prompt")

	plan, err := p.GeneratePlan(context.Background(), &core.Task{ID: "t2", Name: "n"})
	require.NoError(t, err)
	assert.Len(t, plan, 200)
	assert.Equal(t, "custom prompt", llm.systemMsg)
	assert.Equal(t, map[string]interface{}{"id": "t2", "name": "n", "description": ""}, promptContext(t, llm.prompt))
}

func TestSimpleLLMPlannerErrors(t *testing.T) {
	_, err := NewSimpleLLMPlanner(nil, logging.New()).GeneratePlan(context.Background(), sampleTask())
	assert.EqualError(t, err, "LLM client not configured for planner")

	errBoom := errors.New("boom")
	_, err = NewSimpleLLMPlanner(&fakeLLM{err: errBoom}, logging.New()).GeneratePlan(context.Background(), sampleTask())
	assert.ErrorIs(t, err, errBoom)
	assert.ErrorContains(t, err, "failed to generate plan")
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exact", 5, "exact"},
		{"toolong", 3, "too..."},
		{"", 0, ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, truncateString(tt.in, tt.n), tt.in)
	}
}

func TestMockAIPlanner(t *testing.T) {
	plan, err := (&MockAIPlanner{}).GeneratePlan(context.Background(), &core.Task{Name: "N", Description: "D"})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(plan, "Plan for task: N\n"))
	assert.Contains(t, plan, "1. Analyze the requirements: D")
	assert.Contains(t, plan, "5. Review and finalize")
}

func TestCorePlannerCreatePlan(t *testing.T) {
	ctx := context.Background()
	task := &core.Task{ID: "t1", Name: "N", Description: "D"}
	fallback := "Plan for task: N\n\n1. Analyze the task: D\n2. Determine required steps\n" +
		"3. Execute each step in order\n4. Verify results\n5. Report completion"

	tests := []struct {
		name    string
		planner AIPlanner
		input   interface{}
		want    string
	}{
		{name: "non task input", planner: &fakePlanner{plan: "ai"}, input: "x", want: "Simple default plan"},
		{name: "ai plan", planner: &fakePlanner{plan: "ai"}, input: task, want: "ai"},
		{name: "ai error falls back", planner: &fakePlanner{err: errors.New("boom")}, input: task, want: fallback},
		{name: "nil ai planner falls back", planner: nil, input: task, want: fallback},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewCorePlannerWithAI(logging.New(), tt.planner)
			got, err := p.CreatePlan(ctx, tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewCorePlannerUsesMockPlanner(t *testing.T) {
	got, err := NewCorePlanner(logging.New()).CreatePlan(context.Background(), &core.Task{Name: "N", Description: "D"})
	require.NoError(t, err)
	want, _ := (&MockAIPlanner{}).GeneratePlan(context.Background(), &core.Task{Name: "N", Description: "D"})
	assert.Equal(t, want, got)
}

func TestAnalyzeTaskContext(t *testing.T) {
	p := &CorePlanner{logger: logging.New()}

	assert.Equal(t, map[string]interface{}{
		"name":        "Migrate DB",
		"description": "move data",
		"priority":    "high",
		"deadline":    "friday",
		"complexity":  "medium",
		"type":        "ops",
	}, p.AnalyzeTaskContext(sampleTask()))

	assert.Equal(t, map[string]interface{}{"name": "n", "description": "d"},
		p.AnalyzeTaskContext(&core.Task{Name: "n", Description: "d"}))
}

func TestSerializeTaskForAI(t *testing.T) {
	p := &CorePlanner{logger: logging.New()}

	out, err := p.SerializeTaskForAI(sampleTask())
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &decoded))
	assert.Equal(t, "t1", decoded["id"])
	assert.Equal(t, "Migrate DB", decoded["name"])
	assert.Equal(t, "move data", decoded["description"])
	assert.Equal(t, "x", decoded["metadata"].(map[string]interface{})["ignored"])
	assert.Equal(t, "high", decoded["context"].(map[string]interface{})["priority"])

	_, err = p.SerializeTaskForAI(&core.Task{Metadata: map[string]interface{}{"bad": make(chan int)}})
	assert.ErrorContains(t, err, "failed to serialize task")
}
