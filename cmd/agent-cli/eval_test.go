package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agenteval "github.com/Ingenimax/agent-sdk-go/pkg/eval"
)

func TestEvalCommandRegradesSavedObservationsWithoutProvider(t *testing.T) {
	datasetJSON := []byte(`{
		"schema_version":"1",
		"id":"cli-suite",
		"cases":[{
			"id":"hello",
			"input":"say hello",
			"checks":[{"id":"answer","type":"output_exact","config":{"value":"hello"}}]
		}]
	}`)
	dataset, err := agenteval.ParseDataset(datasetJSON)
	if err != nil {
		t.Fatalf("ParseDataset: %v", err)
	}
	digest, err := agenteval.DatasetDigest(dataset)
	if err != nil {
		t.Fatalf("DatasetDigest: %v", err)
	}
	output := "hello"
	previous := agenteval.Report{
		SchemaVersion: agenteval.ReportSchemaVersion,
		DatasetID:     dataset.ID,
		DatasetDigest: digest,
		Cases: []agenteval.CaseResult{{
			CaseID: "hello",
			Observation: agenteval.Observation{
				CaseID:     "hello",
				Status:     agenteval.RunStatusCompleted,
				Output:     &output,
				Regradable: true,
				Capabilities: agenteval.Capabilities{
					Output: agenteval.CoverageComplete,
				},
			},
		}},
	}

	directory := t.TempDir()
	datasetPath := filepath.Join(directory, "dataset.json")
	observationsPath := filepath.Join(directory, "observations.json")
	if err := os.WriteFile(datasetPath, datasetJSON, 0600); err != nil {
		t.Fatalf("write dataset: %v", err)
	}
	if err := agenteval.WriteJSONFile(observationsPath, previous, agenteval.ExportOptions{IncludeObservations: true}); err != nil {
		t.Fatalf("write observations: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runEvalCommand([]string{
		"--dataset", datasetPath,
		"--observations", observationsPath,
		"--include-observations",
	}, &stdout, &stderr)
	if exitCode != evalExitPass {
		t.Fatalf("exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "1 passed, 0 failed, 0 errors") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	report, err := agenteval.ReadReport(bytes.NewReader(stdout.Bytes()))
	if err != nil {
		t.Fatalf("ReadReport(stdout): %v\n%s", err, stdout.String())
	}
	if report.Cases[0].Status != agenteval.CaseStatusPass || report.Summary.Passed != 1 {
		t.Fatalf("report = %#v", report)
	}
	if _, err := agenteval.Regrade(context.Background(), dataset, report, nil); err != nil {
		t.Fatalf("CLI full output cannot be regraded: %v", err)
	}
}

func TestEvalCommandRequiresDataset(t *testing.T) {
	var stderr bytes.Buffer
	if code := runEvalCommand(nil, ioDiscard{}, &stderr); code != evalExitError {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stderr.String(), "--dataset is required") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestEvalFlagsRequireCompleteJudgeConfiguration(t *testing.T) {
	var stderr bytes.Buffer
	if _, err := parseEvalFlags([]string{
		"--dataset", "dataset.json",
		"--judge-provider", "openai",
	}, &stderr); err == nil {
		t.Fatal("incomplete judge configuration was accepted")
	}
	if !strings.Contains(stderr.String(), "--judge-provider and --judge-model") {
		t.Fatalf("stderr = %q", stderr.String())
	}

	options, err := parseEvalFlags([]string{
		"--dataset", "dataset.json",
		"--judge-provider", "openai",
		"--judge-model", "openrouter/free",
	}, ioDiscard{})
	if err != nil {
		t.Fatalf("parseEvalFlags: %v", err)
	}
	if options.judgeProvider != "openai" || options.judgeModel != "openrouter/free" {
		t.Fatalf("options = %#v", options)
	}
}

func TestEvalFlagsValidateAllExecutionLimits(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unexpected argument", []string{"--dataset", "dataset.json", "extra"}},
		{"format", []string{"--dataset", "dataset.json", "--format", "yaml"}},
		{"concurrency", []string{"--dataset", "dataset.json", "--concurrency", "0"}},
		{"spans", []string{"--dataset", "dataset.json", "--max-spans", "0"}},
		{"content", []string{"--dataset", "dataset.json", "--max-content-bytes", "0"}},
		{"case timeout", []string{"--dataset", "dataset.json", "--timeout", "-1s"}},
		{"grading timeout", []string{"--dataset", "dataset.json", "--grading-timeout", "-1s"}},
		{"cleanup timeout", []string{"--dataset", "dataset.json", "--cleanup-timeout", "-1s"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if _, err := parseEvalFlags(test.args, &stderr); err == nil {
				t.Fatalf("invalid flags were accepted: %v", test.args)
			}
			if stderr.Len() == 0 {
				t.Fatal("invalid flags produced no diagnostic")
			}
		})
	}

	options, err := parseEvalFlags([]string{
		"--dataset", "dataset.json",
		"--config", "config.json",
		"--observations", "observations.json",
		"--format", "junit",
		"--output", "report.xml",
		"--include-observations",
		"--concurrency", "3",
		"--timeout", "2s",
		"--grading-timeout", "4s",
		"--cleanup-timeout", "5s",
		"--max-spans", "12",
		"--max-content-bytes", "2048",
	}, ioDiscard{})
	if err != nil {
		t.Fatalf("parseEvalFlags(valid): %v", err)
	}
	if options.format != "junit" || options.concurrency != 3 || !options.includeObservations || options.maxSpans != 12 {
		t.Fatalf("options = %#v", options)
	}
	if code := runEvalCommand([]string{"--help"}, nil, nil); code != evalExitPass {
		t.Fatalf("help exit = %d", code)
	}
}

func TestEvalCommandReportsFailedChecksAndWritesJUnit(t *testing.T) {
	datasetPath, observationsPath := writeCLIRegradeFixture(t, "wrong")
	outputPath := filepath.Join(t.TempDir(), "report.xml")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runEvalCommand([]string{
		"--dataset", datasetPath,
		"--observations", observationsPath,
		"--format", "junit",
		"--output", outputPath,
	}, &stdout, &stderr)
	if exitCode != evalExitFail {
		t.Fatalf("exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "0 passed, 1 failed") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read JUnit: %v", err)
	}
	if !bytes.Contains(data, []byte(`<failure`)) {
		t.Fatalf("JUnit does not contain failure: %s", data)
	}
}

func TestEvalCommandSurfacesInputAndOutputErrors(t *testing.T) {
	var stderr bytes.Buffer
	if code := runEvalCommand([]string{"--dataset", filepath.Join(t.TempDir(), "missing.json")}, nil, &stderr); code != evalExitError {
		t.Fatalf("missing dataset exit = %d", code)
	}
	if !strings.Contains(stderr.String(), "open dataset") {
		t.Fatalf("stderr = %q", stderr.String())
	}

	directory := t.TempDir()
	invalidDataset := filepath.Join(directory, "invalid.json")
	if err := os.WriteFile(invalidDataset, []byte(`{"schema_version":"1"}`), 0600); err != nil {
		t.Fatalf("write invalid dataset: %v", err)
	}
	stderr.Reset()
	if code := runEvalCommand([]string{"--dataset", invalidDataset}, nil, &stderr); code != evalExitError {
		t.Fatalf("invalid dataset exit = %d", code)
	}

	datasetPath, observationsPath := writeCLIRegradeFixture(t, "hello")
	stderr.Reset()
	if code := runEvalCommand([]string{
		"--dataset", datasetPath,
		"--observations", filepath.Join(directory, "missing-observations.json"),
	}, nil, &stderr); code != evalExitError {
		t.Fatalf("missing observations exit = %d", code)
	}
	if !strings.Contains(stderr.String(), "open observations") {
		t.Fatalf("stderr = %q", stderr.String())
	}

	stderr.Reset()
	if code := runEvalCommand([]string{
		"--dataset", datasetPath,
		"--observations", observationsPath,
		"--output", filepath.Join(directory, "missing", "report.json"),
	}, nil, &stderr); code != evalExitError {
		t.Fatalf("bad output exit = %d", code)
	}
	if !strings.Contains(stderr.String(), "create temporary report") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestLoadEvalConfigDefaultsOverridesAndErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	if _, _, err := loadEvalConfig(missing); err == nil {
		t.Fatal("missing explicit config was accepted")
	}
	invalid := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{`), 0600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	if _, _, err := loadEvalConfig(invalid); err == nil {
		t.Fatal("invalid config was accepted")
	}

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"provider":"openai","system_prompt":"test"}`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LLM_PROVIDER", "ollama")
	config, canonical, err := loadEvalConfig(configPath)
	if err != nil {
		t.Fatalf("loadEvalConfig: %v", err)
	}
	if config.Provider != "ollama" || config.Model != "llama3.2:latest" {
		t.Fatalf("config = %#v", config)
	}
	var decoded CLIConfig
	if err := json.Unmarshal(canonical, &decoded); err != nil || decoded.Provider != "ollama" {
		t.Fatalf("canonical config = %s, %v", canonical, err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LLM_PROVIDER", "vllm")
	defaults, _, err := loadEvalConfig("")
	if err != nil {
		t.Fatalf("load default config: %v", err)
	}
	if defaults.Provider != "vllm" || defaults.Model == "" || defaults.ConversationID != "cli-session" {
		t.Fatalf("defaults = %#v", defaults)
	}
}

func TestCreateEvalLLMConfiguration(t *testing.T) {
	for _, key := range []string{
		"OPENAI_API_KEY", "OPENAI_BASE_URL", "ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL",
		"GOOGLE_CLOUD_PROJECT", "OLLAMA_BASE_URL", "VLLM_BASE_URL",
		"AGENT_EVAL_JUDGE_API_KEY", "AGENT_EVAL_JUDGE_BASE_URL", "AGENT_EVAL_JUDGE_PROJECT_ID",
	} {
		t.Setenv(key, "")
	}
	ctx := context.Background()
	if _, err := createEvalLLM(ctx, nil); err == nil {
		t.Fatal("nil config was accepted")
	}
	for _, config := range []*CLIConfig{
		{Provider: "openai", Model: "test"},
		{Provider: "anthropic", Model: "test"},
		{Provider: "vertex", Model: "test"},
		{Provider: "unknown", Model: "test"},
	} {
		if _, err := createEvalLLM(ctx, config); err == nil {
			t.Fatalf("invalid provider configuration was accepted: %#v", config)
		}
	}

	t.Setenv("OPENAI_API_KEY", "fallback-key")
	t.Setenv("OPENAI_BASE_URL", "https://fallback.invalid/v1")
	for _, config := range []*CLIConfig{
		{Provider: "openai", Model: "test"},
		{Provider: "ollama", Model: "test"},
		{Provider: "vllm", Model: "test"},
	} {
		model, err := createEvalLLM(ctx, config)
		if err != nil || model == nil {
			t.Fatalf("createEvalLLM(%s) = %#v, %v", config.Provider, model, err)
		}
	}
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	if model, err := createEvalLLM(ctx, &CLIConfig{Provider: "anthropic", Model: "test"}); err != nil || model == nil {
		t.Fatalf("create anthropic model = %#v, %v", model, err)
	}

	if value := evalEnvironmentValue("AGENT_EVAL_JUDGE_API_KEY", "OPENAI_API_KEY"); value != "fallback-key" {
		t.Fatalf("fallback environment value = %q", value)
	}
	t.Setenv("AGENT_EVAL_JUDGE_API_KEY", "judge-key")
	if value := evalEnvironmentValue("AGENT_EVAL_JUDGE_API_KEY", "OPENAI_API_KEY"); value != "judge-key" {
		t.Fatalf("override environment value = %q", value)
	}
	if model, err := createEvalJudgeLLM(ctx, "openai", "judge"); err != nil || model == nil {
		t.Fatalf("create judge model = %#v, %v", model, err)
	}

	evaluators, err := createEvalEvaluators(ctx, evalCommandOptions{})
	if err != nil || evaluators[agenteval.EvaluatorModelJudge] != nil {
		t.Fatalf("builtin evaluators = %#v, %v", evaluators, err)
	}
	evaluators, err = createEvalEvaluators(ctx, evalCommandOptions{judgeProvider: "openai", judgeModel: "judge"})
	if err != nil || evaluators[agenteval.EvaluatorModelJudge] == nil {
		t.Fatalf("judge evaluators = %#v, %v", evaluators, err)
	}

	config := &CLIConfig{
		Provider:       "ollama",
		Model:          "test",
		SystemPrompt:   "test",
		MaxIterations:  1,
		OrgID:          "org",
		ConversationID: "conversation",
	}
	if subject, err := createEvalAgent(ctx, config, agenteval.NewRecorder(agenteval.RecorderOptions{})); err != nil || subject == nil {
		t.Fatalf("createEvalAgent = %#v, %v", subject, err)
	}
	if revision := evaluationBuildRevision(); revision == "" {
		t.Fatal("evaluation build revision is empty")
	}
}

func writeCLIRegradeFixture(t *testing.T, actual string) (string, string) {
	t.Helper()
	datasetJSON := []byte(`{
		"schema_version":"1",
		"id":"cli-suite",
		"cases":[{"id":"hello","input":"say hello","checks":[{
			"id":"answer","type":"output_exact","config":{"value":"hello"}
		}]}]
	}`)
	dataset, err := agenteval.ParseDataset(datasetJSON)
	if err != nil {
		t.Fatalf("ParseDataset: %v", err)
	}
	digest, err := agenteval.DatasetDigest(dataset)
	if err != nil {
		t.Fatalf("DatasetDigest: %v", err)
	}
	report := agenteval.Report{
		SchemaVersion: agenteval.ReportSchemaVersion,
		DatasetID:     dataset.ID,
		DatasetDigest: digest,
		Cases: []agenteval.CaseResult{{
			CaseID: "hello",
			Observation: agenteval.Observation{
				CaseID:       "hello",
				Status:       agenteval.RunStatusCompleted,
				Output:       &actual,
				Regradable:   true,
				Capabilities: agenteval.Capabilities{Output: agenteval.CoverageComplete},
			},
		}},
	}
	directory := t.TempDir()
	datasetPath := filepath.Join(directory, "dataset.json")
	observationsPath := filepath.Join(directory, "observations.json")
	if err := os.WriteFile(datasetPath, datasetJSON, 0600); err != nil {
		t.Fatalf("write dataset: %v", err)
	}
	if err := agenteval.WriteJSONFile(observationsPath, report, agenteval.ExportOptions{IncludeObservations: true}); err != nil {
		t.Fatalf("write observations: %v", err)
	}
	return datasetPath, observationsPath
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
