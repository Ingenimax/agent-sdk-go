package eval

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func TestModelJudgeScoresResponseAndCapturesProvenance(t *testing.T) {
	reference := "Madrid"
	model := &scriptedJudgeModel{response: &interfaces.LLMResponse{
		Content: `{"score":0.85,"reason":"Correct and concise."}`,
		Model:   "judge-model-v1",
		Usage:   &interfaces.TokenUsage{InputTokens: 20, OutputTokens: 8, TotalTokens: 28},
	}}
	check := testCheck("quality", EvaluatorModelJudge, `{"rubric":"The answer must be correct and concise."}`)
	threshold := 0.8
	check.Threshold = &threshold
	output := "Madrid"
	result, err := Grade(context.Background(), Case{
		ID: "capital", Input: "What is Spain's capital?", Reference: &reference, Checks: []Check{check},
	}, Observation{
		CaseID: "capital", Status: RunStatusCompleted, Output: &output,
		Capabilities: Capabilities{Output: CoverageComplete},
	}, Evaluators{EvaluatorModelJudge: NewModelJudge(model)})
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if result.Status != CaseStatusPass || result.Metrics[0].Status != MetricStatusPass {
		t.Fatalf("result = %#v", result)
	}
	metric := result.Metrics[0]
	if metric.Score == nil || *metric.Score != 0.85 {
		t.Fatalf("score = %v", metric.Score)
	}
	if metric.Evidence["judge_model"] != "judge-model-v1" || metric.Evidence["judge_provider"] != "fixture-judge" {
		t.Fatalf("evidence = %#v", metric.Evidence)
	}
	if !strings.Contains(model.prompt, `"reference":"Madrid"`) || !strings.Contains(model.prompt, `"candidate_response":"Madrid"`) {
		t.Fatalf("prompt = %s", model.prompt)
	}
	if model.options.ResponseFormat == nil || model.options.LLMConfig == nil || model.options.LLMConfig.Temperature != 0 {
		t.Fatalf("options = %#v", model.options)
	}
}

func TestModelJudgeBelowThresholdFailsCase(t *testing.T) {
	model := &scriptedJudgeModel{response: &interfaces.LLMResponse{
		Content: `{"score":0.4,"reason":"The answer omits required details."}`,
	}}
	check := testCheck("quality", EvaluatorModelJudge, `{"rubric":"Include every required detail."}`)
	threshold := 0.7
	check.Threshold = &threshold
	output := "Partial answer"
	result, err := Grade(context.Background(), Case{
		ID: "quality", Input: "Explain it", Checks: []Check{check},
	}, Observation{
		CaseID: "quality", Status: RunStatusCompleted, Output: &output,
		Capabilities: Capabilities{Output: CoverageComplete},
	}, Evaluators{EvaluatorModelJudge: NewModelJudge(model)})
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if result.Status != CaseStatusFail || result.Metrics[0].Status != MetricStatusFail {
		t.Fatalf("result = %#v", result)
	}
}

func TestModelJudgeMalformedResponseIsMetricError(t *testing.T) {
	model := &scriptedJudgeModel{response: &interfaces.LLMResponse{Content: `not json`}}
	check := testCheck("quality", EvaluatorModelJudge, `{"rubric":"Be correct."}`)
	output := "answer"
	result, err := Grade(context.Background(), Case{
		ID: "quality", Input: "question", Checks: []Check{check},
	}, Observation{
		CaseID: "quality", Status: RunStatusCompleted, Output: &output,
		Capabilities: Capabilities{Output: CoverageComplete},
	}, Evaluators{EvaluatorModelJudge: NewModelJudge(model)})
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if result.Status != CaseStatusError || result.Metrics[0].Status != MetricStatusError {
		t.Fatalf("result = %#v", result)
	}
}

func TestModelJudgeRejectsInvalidVerdicts(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "score above range", content: `{"score":1.1,"reason":"too high"}`, want: "between 0 and 1"},
		{name: "missing reason", content: `{"score":0.5,"reason":""}`, want: "reason is required"},
		{name: "unknown field", content: `{"score":0.5,"reason":"ok","extra":true}`, want: "unknown field"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := &scriptedJudgeModel{response: &interfaces.LLMResponse{Content: test.content}}
			check := testCheck("quality", EvaluatorModelJudge, `{"rubric":"Be correct."}`)
			output := "answer"
			result, err := Grade(context.Background(), Case{
				ID: "quality", Input: "question", Checks: []Check{check},
			}, Observation{
				CaseID: "quality", Status: RunStatusCompleted, Output: &output,
				Capabilities: Capabilities{Output: CoverageComplete},
			}, Evaluators{EvaluatorModelJudge: NewModelJudge(model)})
			if err != nil {
				t.Fatalf("Grade: %v", err)
			}
			if result.Status != CaseStatusError || !strings.Contains(result.Metrics[0].Message, test.want) {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestModelJudgeValidationAndConfigurationErrors(t *testing.T) {
	judge := NewModelJudge(nil)
	if err := judge.Validate(testCheck("quality", EvaluatorModelJudge, `{"rubric":""}`)); err == nil {
		t.Fatal("empty rubric was accepted")
	}
	if err := judge.Validate(testCheck("quality", EvaluatorModelJudge, `{"rubric":"ok","unknown":true}`)); err == nil {
		t.Fatal("unknown config field was accepted")
	}

	output := "answer"
	result, err := Grade(context.Background(), Case{
		ID: "quality", Input: "question", Checks: []Check{testCheck("quality", EvaluatorModelJudge, `{"rubric":"Be correct."}`)},
	}, Observation{
		CaseID: "quality", Status: RunStatusCompleted, Output: &output,
		Capabilities: Capabilities{Output: CoverageComplete},
	}, Evaluators{EvaluatorModelJudge: judge})
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if result.Status != CaseStatusError || !strings.Contains(result.Metrics[0].Message, "not configured") {
		t.Fatalf("result = %#v", result)
	}
}

func TestModelJudgeRequestErrorIsMetricError(t *testing.T) {
	model := &scriptedJudgeModel{err: errors.New("provider unavailable")}
	check := testCheck("quality", EvaluatorModelJudge, `{"rubric":"Be correct."}`)
	output := "answer"
	result, err := Grade(context.Background(), Case{
		ID: "quality", Input: "question", Checks: []Check{check},
	}, Observation{
		CaseID: "quality", Status: RunStatusCompleted, Output: &output,
		Capabilities: Capabilities{Output: CoverageComplete},
	}, Evaluators{EvaluatorModelJudge: NewModelJudge(model)})
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if result.Status != CaseStatusError || !strings.Contains(result.Metrics[0].Message, "provider unavailable") {
		t.Fatalf("result = %#v", result)
	}
}

func TestModelJudgeDatasetRequiresRegisteredEvaluator(t *testing.T) {
	datasetJSON := []byte(`{
		"schema_version":"1",
		"id":"judge-suite",
		"cases":[{
			"id":"quality",
			"input":"answer",
			"checks":[{
				"id":"judge",
				"type":"model_judge",
				"threshold":0.8,
				"config":{"rubric":"Be correct."}
			}]
		}]
	}`)
	if _, err := ParseDataset(datasetJSON); err == nil {
		t.Fatal("model_judge dataset loaded without a registered evaluator")
	}
	evaluators := BuiltinEvaluators()
	evaluators[EvaluatorModelJudge] = NewModelJudge(nil)
	if _, err := ParseDatasetWithEvaluators(datasetJSON, evaluators); err != nil {
		t.Fatalf("ParseDatasetWithEvaluators: %v", err)
	}
}

type scriptedJudgeModel struct {
	response *interfaces.LLMResponse
	err      error
	prompt   string
	options  interfaces.GenerateOptions
}

func (m *scriptedJudgeModel) Name() string { return "fixture-judge" }

func (m *scriptedJudgeModel) GenerateDetailed(_ context.Context, prompt string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	m.prompt = prompt
	for _, option := range options {
		option(&m.options)
	}
	return m.response, m.err
}
