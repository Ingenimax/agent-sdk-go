package eval

import (
	"context"
	"encoding/json"
	"testing"
)

func TestGradeDeterministicOutputAndSchemaChecks(t *testing.T) {
	output := `{"answer":"Example City","temperature":22}`
	evalCase := Case{
		ID:    "weather",
		Input: "weather",
		Checks: []Check{
			testCheck("contains", EvaluatorOutputContains, `{"value":"Example City"}`),
			testCheck("regex", EvaluatorOutputRegex, `{"pattern":"temperature.*22"}`),
			testCheck("schema", EvaluatorOutputJSONSchema, `{"schema":{"type":"object","required":["temperature"],"properties":{"temperature":{"type":"integer"}}}}`),
		},
	}
	observation := Observation{
		CaseID:     evalCase.ID,
		Status:     RunStatusCompleted,
		Output:     &output,
		Regradable: true,
		Capabilities: Capabilities{
			Output: CoverageComplete,
		},
	}
	result, err := Grade(context.Background(), evalCase, observation, BuiltinEvaluators())
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if result.Status != CaseStatusPass {
		t.Fatalf("status = %s, metrics = %#v", result.Status, result.Metrics)
	}
	for _, metric := range result.Metrics {
		if metric.Status != MetricStatusPass || metric.Score == nil || *metric.Score != 1 {
			t.Fatalf("metric %#v did not pass", metric)
		}
	}
}

func TestToolTrajectoryMatchesExactNumbersAndObjectSubsets(t *testing.T) {
	evalCase := Case{
		ID:    "tools",
		Input: "run",
		Checks: []Check{
			testCheck("trajectory", EvaluatorToolTrajectory, `{
				"layer":"execution",
				"mode":"exact",
				"argument_match":"subset",
				"calls":[
					{"name":"weather","arguments":{"city":"Example City","scale":1e0}},
					{"name":"format"}
				]
			}`),
			testCheck("limit", EvaluatorMaxToolCalls, `{"layer":"execution","maximum":2}`),
		},
	}
	observation := Observation{
		CaseID: evalCase.ID,
		Status: RunStatusCompleted,
		Capabilities: Capabilities{
			ToolCapture: CoverageComplete,
		},
		Trace: Trace{Spans: []ToolSpan{
			{ID: "1", Sequence: 1, AgentPath: RootAgentPath, Layer: ToolLayerExecution, Tool: "weather", Arguments: `{"city":"Example City","scale":1.00,"extra":true}`},
			{ID: "2", Sequence: 2, AgentPath: RootAgentPath, Layer: ToolLayerExecution, Tool: "format", Arguments: `{}`},
		}},
	}
	result, err := Grade(context.Background(), evalCase, observation, BuiltinEvaluators())
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if result.Status != CaseStatusPass {
		t.Fatalf("status = %s, metrics = %#v", result.Status, result.Metrics)
	}
}

func TestRequiredMissingCapabilitiesProduceCaseError(t *testing.T) {
	evalCase := Case{
		ID:    "missing",
		Input: "run",
		Checks: []Check{
			testCheck("tools", EvaluatorMaxToolCalls, `{"maximum":0}`),
			testCheck("tokens", EvaluatorMaxTotalTokens, `{"maximum":10}`),
		},
	}
	observation := Observation{
		CaseID: evalCase.ID,
		Status: RunStatusCompleted,
		Capabilities: Capabilities{
			ToolCapture: CoveragePartial,
			Usage:       CoverageUnavailable,
		},
	}
	result, err := Grade(context.Background(), evalCase, observation, BuiltinEvaluators())
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if result.Status != CaseStatusError {
		t.Fatalf("status = %s, want error", result.Status)
	}
	for _, metric := range result.Metrics {
		if metric.Status != MetricStatusUnavailable || metric.Score != nil {
			t.Fatalf("metric = %#v, want unavailable without score", metric)
		}
	}
}

func TestTokenAndLatencyBudgetsUseObservedValues(t *testing.T) {
	evalCase := Case{
		ID:    "budgets",
		Input: "run",
		Checks: []Check{
			testCheck("tokens", EvaluatorMaxTotalTokens, `{"maximum":12}`),
			testCheck("latency", EvaluatorMaxLatencyMS, `{"maximum":50}`),
		},
	}
	observation := Observation{
		CaseID:            evalCase.ID,
		Status:            RunStatusCompleted,
		ExecutionDuration: 51,
		Usage:             &TokenUsage{TotalTokens: 12},
		Capabilities:      Capabilities{Usage: CoverageComplete},
	}
	result, err := Grade(context.Background(), evalCase, observation, BuiltinEvaluators())
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if result.Status != CaseStatusFail {
		t.Fatalf("status = %s, want fail", result.Status)
	}
	if result.Metrics[0].Status != MetricStatusPass || result.Metrics[1].Status != MetricStatusFail {
		t.Fatalf("unexpected metrics: %#v", result.Metrics)
	}
}

func TestUnorderedTrajectoryUsesCompleteMatching(t *testing.T) {
	// A greedy matcher can consume the specific call with the broad expectation
	// and then fail. The augmenting matcher must find the complete assignment.
	config := trajectoryConfig{
		Mode:          "unordered",
		ArgumentMatch: "subset",
		Calls: []expectedToolCall{
			{Name: "tool", Arguments: json.RawMessage(`{}`)},
			{Name: "tool", Arguments: json.RawMessage(`{"specific":true}`)},
		},
	}
	observed := []ToolSpan{
		{Tool: "tool", Arguments: `{"specific":true}`},
		{Tool: "tool", Arguments: `{"other":true}`},
	}
	if !matchTrajectory(config, observed) {
		t.Fatal("unordered trajectory should find a complete matching")
	}
}

func TestEvaluatorPanicBecomesMetricError(t *testing.T) {
	evalCase := Case{
		ID:     "panic",
		Input:  "run",
		Checks: []Check{testCheck("panic", "panic", `{}`)},
	}
	result, err := Grade(context.Background(), evalCase, Observation{
		CaseID: evalCase.ID,
		Status: RunStatusCompleted,
	}, Evaluators{"panic": panicEvaluator{}})
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if result.Status != CaseStatusError || result.Metrics[0].Status != MetricStatusError {
		t.Fatalf("result = %#v", result)
	}
}

type panicEvaluator struct{}

func (panicEvaluator) Name() string         { return "panic" }
func (panicEvaluator) Validate(Check) error { return nil }
func (panicEvaluator) Evaluate(context.Context, Evaluation) (MetricResult, error) {
	panic("grader failed")
}

func testCheck(id, evaluator, config string) Check {
	threshold := 1.0
	required := true
	return Check{
		ID:        id,
		Type:      evaluator,
		Config:    json.RawMessage(config),
		Threshold: &threshold,
		Required:  &required,
	}
}
