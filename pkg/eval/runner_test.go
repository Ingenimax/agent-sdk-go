package eval

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

type subjectFunc func(context.Context, string) (*interfaces.AgentResponse, error)

func (f subjectFunc) RunDetailed(ctx context.Context, input string) (*interfaces.AgentResponse, error) {
	return f(ctx, input)
}

func TestRunnerUsesFreshTargetsAndPreservesDatasetOrder(t *testing.T) {
	dataset := Dataset{
		SchemaVersion: DatasetSchemaVersion,
		ID:            "ordering",
		Cases: []Case{
			runnerCase("slow", "slow"),
			runnerCase("fast", "fast"),
			runnerCase("medium", "medium"),
		},
	}
	var built atomic.Int32
	var closed atomic.Int32
	runner := Runner{
		Options: RunnerOptions{Concurrency: 3, AttemptID: sequentialAttemptIDs()},
		Factory: func(_ context.Context, evalCase Case, _ *Recorder) (Target, error) {
			built.Add(1)
			delay := map[string]time.Duration{
				"slow":   20 * time.Millisecond,
				"fast":   1 * time.Millisecond,
				"medium": 10 * time.Millisecond,
			}[evalCase.ID]
			return Target{
				Subject: subjectFunc(func(ctx context.Context, input string) (*interfaces.AgentResponse, error) {
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						return nil, ctx.Err()
					}
					return &interfaces.AgentResponse{
						Content:   input,
						AgentName: evalCase.ID,
						Model:     "fake-model",
						Usage:     &interfaces.TokenUsage{TotalTokens: 5},
					}, nil
				}),
				Close: func(context.Context) error {
					closed.Add(1)
					return nil
				},
				Capabilities: Capabilities{Usage: CoverageComplete},
			}, nil
		},
	}

	report, err := runner.Run(context.Background(), dataset)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if built.Load() != 3 || closed.Load() != 3 {
		t.Fatalf("built=%d closed=%d", built.Load(), closed.Load())
	}
	for i, id := range []string{"slow", "fast", "medium"} {
		if report.Cases[i].CaseID != id || report.Cases[i].Status != CaseStatusPass {
			t.Fatalf("case[%d] = %#v", i, report.Cases[i])
		}
	}
	if report.Summary.Passed != 3 || report.Summary.PassRate != 1 {
		t.Fatalf("summary = %#v", report.Summary)
	}
}

func TestRunnerRecordsCooperativeCaseTimeout(t *testing.T) {
	dataset := Dataset{
		SchemaVersion: DatasetSchemaVersion,
		ID:            "timeout",
		Cases:         []Case{runnerCase("blocked", "unused")},
	}
	runner := Runner{
		Options: RunnerOptions{CaseTimeout: 5 * time.Millisecond},
		Factory: func(context.Context, Case, *Recorder) (Target, error) {
			return Target{Subject: subjectFunc(func(ctx context.Context, _ string) (*interfaces.AgentResponse, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			})}, nil
		},
	}
	report, err := runner.Run(context.Background(), dataset)
	if err != nil {
		t.Fatalf("Run returned suite error: %v", err)
	}
	result := report.Cases[0]
	if result.Status != CaseStatusError || result.Observation.Status != RunStatusCancelled {
		t.Fatalf("result = %#v", result)
	}
	if result.Observation.Error == nil || !strings.Contains(result.Observation.Error.Message, "deadline") {
		t.Fatalf("execution error = %#v", result.Observation.Error)
	}
}

func TestRunnerCancellationPreservesPartialReport(t *testing.T) {
	dataset := Dataset{
		SchemaVersion: DatasetSchemaVersion,
		ID:            "cancelled",
		Cases: []Case{
			runnerCase("started", "unused"),
			runnerCase("queued", "unused"),
		},
	}
	started := make(chan struct{}, 1)
	runner := Runner{
		Factory: func(context.Context, Case, *Recorder) (Target, error) {
			return Target{Subject: subjectFunc(func(ctx context.Context, _ string) (*interfaces.AgentResponse, error) {
				started <- struct{}{}
				<-ctx.Done()
				return nil, ctx.Err()
			})}, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	report, err := runner.Run(ctx, dataset)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context canceled", err)
	}
	if report.Cases[0].Observation.Status != RunStatusCancelled {
		t.Fatalf("started case = %#v", report.Cases[0])
	}
	if report.Cases[1].Status != CaseStatusNotStarted {
		t.Fatalf("queued case = %#v", report.Cases[1])
	}
	if report.Summary.Errors != 1 || report.Summary.NotStarted != 1 {
		t.Fatalf("summary = %#v", report.Summary)
	}
}

func TestRunnerDoesNotGradeTruncatedOutputAsComplete(t *testing.T) {
	dataset := Dataset{
		SchemaVersion: DatasetSchemaVersion,
		ID:            "large-output",
		Cases: []Case{{
			ID:    "large",
			Input: "run",
			Checks: []Check{
				testCheck("answer", EvaluatorOutputContains, `{"value":"end"}`),
			},
		}},
	}
	runner := Runner{
		Options: RunnerOptions{Recorder: RecorderOptions{MaxContentBytes: 5}},
		Factory: func(context.Context, Case, *Recorder) (Target, error) {
			return Target{Subject: subjectFunc(func(context.Context, string) (*interfaces.AgentResponse, error) {
				return &interfaces.AgentResponse{Content: "aaaaa-end"}, nil
			})}, nil
		},
	}
	report, err := runner.Run(context.Background(), dataset)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	result := report.Cases[0]
	if result.Status != CaseStatusError || result.Metrics[0].Status != MetricStatusUnavailable {
		t.Fatalf("result = %#v", result)
	}
	if result.Observation.Output == nil || *result.Observation.Output != "aaaaa" {
		t.Fatalf("output = %#v", result.Observation.Output)
	}
	if result.Observation.Capabilities.Output != CoveragePartial || result.Observation.Regradable {
		t.Fatalf("observation = %#v", result.Observation)
	}
}

func TestRunnerCleansUpPartiallyConstructedTarget(t *testing.T) {
	dataset := Dataset{
		SchemaVersion: DatasetSchemaVersion,
		ID:            "setup-error",
		Cases:         []Case{runnerCase("case", "unused")},
	}
	var closed atomic.Bool
	runner := Runner{
		Factory: func(context.Context, Case, *Recorder) (Target, error) {
			return Target{Close: func(context.Context) error {
				closed.Store(true)
				return nil
			}}, errors.New("setup failed")
		},
	}
	report, err := runner.Run(context.Background(), dataset)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !closed.Load() {
		t.Fatal("partially constructed target was not closed")
	}
	if report.Cases[0].Status != CaseStatusError {
		t.Fatalf("case status = %s", report.Cases[0].Status)
	}
}

func TestRegradePreservesCleanupFailure(t *testing.T) {
	dataset := Dataset{
		SchemaVersion: DatasetSchemaVersion,
		ID:            "cleanup",
		Cases:         []Case{runnerCase("case", "answer")},
	}
	runner := Runner{
		Factory: func(context.Context, Case, *Recorder) (Target, error) {
			return Target{
				Subject: subjectFunc(func(context.Context, string) (*interfaces.AgentResponse, error) {
					return &interfaces.AgentResponse{Content: "answer"}, nil
				}),
				Close: func(context.Context) error { return errors.New("close failed") },
			}, nil
		},
	}
	report, err := runner.Run(context.Background(), dataset)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Cases[0].Status != CaseStatusError {
		t.Fatalf("case = %#v", report.Cases[0])
	}
	regraded, err := Regrade(context.Background(), dataset, report, nil)
	if err != nil {
		t.Fatalf("Regrade: %v", err)
	}
	if regraded.Cases[0].Status != CaseStatusError || len(regraded.Cases[0].Errors) == 0 || regraded.Cases[0].Errors[0].Stage != "cleanup" {
		t.Fatalf("regraded case = %#v", regraded.Cases[0])
	}
}

func TestJSONAndJUnitReportsSupportFullRegrading(t *testing.T) {
	dataset := Dataset{
		SchemaVersion: DatasetSchemaVersion,
		ID:            "reports",
		Cases:         []Case{runnerCase("case", "answer")},
	}
	runner := Runner{
		Factory: func(context.Context, Case, *Recorder) (Target, error) {
			return Target{Subject: subjectFunc(func(context.Context, string) (*interfaces.AgentResponse, error) {
				return &interfaces.AgentResponse{
					Content: "answer",
					Usage:   &interfaces.TokenUsage{TotalTokens: 3},
				}, nil
			})}, nil
		},
	}
	report, err := runner.Run(context.Background(), dataset)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var metricsOnly bytes.Buffer
	if err := WriteJSON(&metricsOnly, report, ExportOptions{}); err != nil {
		t.Fatalf("WriteJSON(metrics): %v", err)
	}
	stripped, err := ReadReport(bytes.NewReader(metricsOnly.Bytes()))
	if err != nil {
		t.Fatalf("ReadReport(metrics): %v", err)
	}
	if stripped.Cases[0].Observation.Regradable || stripped.Cases[0].Observation.Output != nil {
		t.Fatalf("metrics-only report retained observations: %#v", stripped.Cases[0].Observation)
	}
	if _, err := Regrade(context.Background(), dataset, stripped, nil); err == nil {
		t.Fatal("Regrade accepted a metrics-only report")
	}

	var full bytes.Buffer
	if err := WriteJSON(&full, report, ExportOptions{IncludeObservations: true}); err != nil {
		t.Fatalf("WriteJSON(full): %v", err)
	}
	if strings.Contains(full.String(), `"TotalTokens"`) {
		t.Fatalf("report leaked interfaces.TokenUsage field names: %s", full.String())
	}
	if !strings.Contains(full.String(), `"total_tokens": 3`) {
		t.Fatalf("report is missing stable token fields: %s", full.String())
	}
	loaded, err := ReadReport(bytes.NewReader(full.Bytes()))
	if err != nil {
		t.Fatalf("ReadReport(full): %v", err)
	}
	regraded, err := Regrade(context.Background(), dataset, loaded, nil)
	if err != nil {
		t.Fatalf("Regrade: %v", err)
	}
	if regraded.Cases[0].Status != CaseStatusPass {
		t.Fatalf("regraded case = %#v", regraded.Cases[0])
	}

	var redacted bytes.Buffer
	if err := WriteJSON(&redacted, report, ExportOptions{
		IncludeObservations: true,
		Redact: func(_, _ string) string {
			return "[redacted]"
		},
	}); err != nil {
		t.Fatalf("WriteJSON(redacted): %v", err)
	}
	redactedReport, err := ReadReport(bytes.NewReader(redacted.Bytes()))
	if err != nil {
		t.Fatalf("ReadReport(redacted): %v", err)
	}
	if redactedReport.ResolvedDataset != nil || redactedReport.Cases[0].Observation.Regradable {
		t.Fatalf("redacted report retained regrading data: %#v", redactedReport.Cases[0].Observation)
	}
	if got := redactedReport.Cases[0].Observation.Output; got == nil || *got != "[redacted]" {
		t.Fatalf("redacted output = %#v", got)
	}
	if err := WriteJSON(&bytes.Buffer{}, report, ExportOptions{
		IncludeObservations: true,
		Redact: func(_, _ string) string {
			panic("redactor failed")
		},
	}); err == nil || !strings.Contains(err.Error(), "redactor panic") {
		t.Fatalf("redactor panic error = %v", err)
	}

	var junit bytes.Buffer
	if err := WriteJUnit(&junit, report); err != nil {
		t.Fatalf("WriteJUnit: %v", err)
	}
	if !strings.Contains(junit.String(), `<testsuite name="reports" tests="1" failures="0" errors="0" skipped="0"`) {
		t.Fatalf("unexpected JUnit output:\n%s", junit.String())
	}
}

func TestJUnitDistinguishesFailuresErrorsAndSkippedCases(t *testing.T) {
	report := Report{
		SchemaVersion: ReportSchemaVersion,
		DatasetID:     "junit",
		Cases: []CaseResult{
			{
				CaseID: "failed",
				Status: CaseStatusFail,
				Metrics: []MetricResult{{
					CheckID: "answer", Evaluator: EvaluatorOutputExact,
					Status: MetricStatusFail, Message: "mismatch",
				}},
			},
			{
				CaseID: "errored",
				Status: CaseStatusError,
				Errors: []ErrorDetail{{Stage: "execution", Message: "failed"}},
			},
			{CaseID: "skipped", Status: CaseStatusNotStarted},
		},
	}
	var output bytes.Buffer
	if err := WriteJUnit(&output, report); err != nil {
		t.Fatalf("WriteJUnit: %v", err)
	}
	xml := output.String()
	for _, expected := range []string{
		`tests="3" failures="1" errors="1" skipped="1"`,
		`<failure message="evaluation expectations failed">`,
		`<error message="evaluation could not complete">`,
		`<skipped message="case was not started"></skipped>`,
	} {
		if !strings.Contains(xml, expected) {
			t.Fatalf("JUnit output missing %q:\n%s", expected, xml)
		}
	}
}

func runnerCase(id, output string) Case {
	return Case{
		ID:    id,
		Input: output,
		Checks: []Check{
			testCheck("answer", EvaluatorOutputExact, `{"value":"`+output+`"}`),
		},
	}
}

func sequentialAttemptIDs() func() string {
	var next atomic.Int32
	return func() string { return "attempt-" + string(rune('0'+next.Add(1))) }
}
