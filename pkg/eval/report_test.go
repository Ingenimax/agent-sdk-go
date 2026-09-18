package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportJSONExportControlsObservationRetention(t *testing.T) {
	report := reportTestFixture()
	originalOutput := *report.Cases[0].Observation.Output

	var summaryOnly bytes.Buffer
	if err := WriteJSON(&summaryOnly, report, ExportOptions{}); err != nil {
		t.Fatalf("WriteJSON(summary): %v", err)
	}
	summary, err := ReadReport(bytes.NewReader(summaryOnly.Bytes()))
	if err != nil {
		t.Fatalf("ReadReport(summary): %v", err)
	}
	observation := summary.Cases[0].Observation
	if observation.Output != nil || observation.Regradable || summary.ResolvedDataset != nil {
		t.Fatalf("summary retained regrading content: %#v", summary)
	}
	if got := observation.Trace.Spans[0]; got.Arguments != "" || got.Result != "" || got.Error != "" {
		t.Fatalf("summary retained tool content: %#v", got)
	}
	if *report.Cases[0].Observation.Output != originalOutput || !report.Cases[0].Observation.Regradable {
		t.Fatal("WriteJSON mutated the source report")
	}

	var bounded bytes.Buffer
	if err := WriteJSON(&bounded, report, ExportOptions{IncludeObservations: true, MaxContentBytes: 8}); err != nil {
		t.Fatalf("WriteJSON(bounded): %v", err)
	}
	truncated, err := ReadReport(bytes.NewReader(bounded.Bytes()))
	if err != nil {
		t.Fatalf("ReadReport(bounded): %v", err)
	}
	observation = truncated.Cases[0].Observation
	if observation.Regradable || !observation.Trace.Incomplete || observation.Trace.Reason == "" {
		t.Fatalf("truncated observation was not marked incomplete: %#v", observation)
	}
	if observation.Output == nil || len(*observation.Output) > 8 {
		t.Fatalf("bounded output = %#v", observation.Output)
	}

	var redacted bytes.Buffer
	if err := WriteJSON(&redacted, report, ExportOptions{
		IncludeObservations: true,
		Redact: func(_ string, value string) string {
			return strings.ReplaceAll(value, "secret", "[redacted]")
		},
	}); err != nil {
		t.Fatalf("WriteJSON(redacted): %v", err)
	}
	if strings.Contains(redacted.String(), "secret") {
		t.Fatalf("redacted report still contains secret content: %s", redacted.String())
	}
	redactedReport, err := ReadReport(bytes.NewReader(redacted.Bytes()))
	if err != nil {
		t.Fatalf("ReadReport(redacted): %v", err)
	}
	if redactedReport.ResolvedDataset != nil || redactedReport.Cases[0].Observation.Regradable {
		t.Fatalf("redacted report is regradable: %#v", redactedReport)
	}
}

func TestReportJSONRejectsInvalidInputs(t *testing.T) {
	if err := WriteJSON(nil, reportTestFixture(), ExportOptions{}); err == nil {
		t.Fatal("nil writer was accepted")
	}
	badVersion := reportTestFixture()
	badVersion.SchemaVersion = "future"
	if err := WriteJSON(io.Discard, badVersion, ExportOptions{}); err == nil {
		t.Fatal("unsupported schema version was accepted")
	}
	if err := WriteJSON(failingWriter{}, reportTestFixture(), ExportOptions{}); err == nil {
		t.Fatal("writer failure was ignored")
	}
	if err := WriteJSON(io.Discard, reportTestFixture(), ExportOptions{
		IncludeObservations: true,
		Redact:              func(string, string) string { panic("redactor failed") },
	}); err == nil || !strings.Contains(err.Error(), "redactor panic") {
		t.Fatalf("redactor panic error = %v", err)
	}

	if _, err := ReadReport(nil); err == nil {
		t.Fatal("nil reader was accepted")
	}
	if _, err := ReadReport(failingReader{}); err == nil {
		t.Fatal("reader failure was ignored")
	}
	invalidReports := []string{
		`{"schema_version":"1","schema_version":"1"}`,
		`{"schema_version":"1","unexpected":true}`,
		`{"schema_version":"1"} {}`,
		`{"schema_version":"future"}`,
		string([]byte{'{', 0xff, '}'}),
	}
	for _, input := range invalidReports {
		if _, err := ReadReport(strings.NewReader(input)); err == nil {
			t.Fatalf("invalid report was accepted: %q", input)
		}
	}
}

func TestReportFileAndJUnitWriters(t *testing.T) {
	report := reportTestFixture()
	report.Cases = append(report.Cases,
		CaseResult{
			CaseID:    "failed\x00case",
			Status:    CaseStatusFail,
			Metrics:   []MetricResult{{CheckID: "answer", Evaluator: EvaluatorOutputExact, Status: MetricStatusFail, Message: "wrong\x01answer"}},
			Durations: Durations{TotalMS: 250},
		},
		CaseResult{
			CaseID:  "errored",
			Status:  CaseStatusError,
			Errors:  []ErrorDetail{{Stage: "execution", Message: "provider failed"}},
			Metrics: []MetricResult{{CheckID: "usage", Evaluator: EvaluatorMaxTotalTokens, Status: MetricStatusUnavailable, Required: true, Message: "usage missing"}},
		},
		CaseResult{CaseID: "cancelled", Status: CaseStatusNotStarted},
	)

	var junit bytes.Buffer
	if err := WriteJUnit(&junit, report); err != nil {
		t.Fatalf("WriteJUnit: %v", err)
	}
	var suites junitSuites
	if err := xml.Unmarshal(junit.Bytes(), &suites); err != nil {
		t.Fatalf("parse JUnit: %v\n%s", err, junit.String())
	}
	suite := suites.Suites[0]
	if suite.Tests != 4 || suite.Failures != 1 || suite.Errors != 1 || suite.Skipped != 1 {
		t.Fatalf("JUnit counts = %#v", suite)
	}
	if strings.Contains(junit.String(), "\x00") || strings.Contains(junit.String(), "\x01") {
		t.Fatalf("JUnit contains XML-unsafe characters: %q", junit.String())
	}
	if err := WriteJUnit(nil, report); err == nil {
		t.Fatal("nil JUnit writer was accepted")
	}
	if err := WriteJUnit(failingWriter{}, report); err == nil {
		t.Fatal("JUnit writer failure was ignored")
	}

	directory := t.TempDir()
	jsonPath := filepath.Join(directory, "report.json")
	if err := WriteJSONFile(jsonPath, report, ExportOptions{IncludeObservations: true}); err != nil {
		t.Fatalf("WriteJSONFile: %v", err)
	}
	file, err := os.Open(jsonPath)
	if err != nil {
		t.Fatalf("open JSON report: %v", err)
	}
	if _, err := ReadReport(file); err != nil {
		t.Fatalf("read JSON report: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close JSON report: %v", err)
	}
	junitPath := filepath.Join(directory, "report.xml")
	if err := WriteJUnitFile(junitPath, report); err != nil {
		t.Fatalf("WriteJUnitFile: %v", err)
	}
	if data, err := os.ReadFile(junitPath); err != nil || !bytes.Contains(data, []byte("<testsuite")) {
		t.Fatalf("JUnit file = %q, %v", data, err)
	}
	if err := WriteJSONFile("", report, ExportOptions{}); err == nil {
		t.Fatal("empty output path was accepted")
	}
	if err := WriteJUnitFile(filepath.Join(directory, "missing", "report.xml"), report); err == nil {
		t.Fatal("missing output directory was accepted")
	}
}

func TestRegradeValidatesSavedObservationIdentity(t *testing.T) {
	dataset := reportTestDataset()
	if err := ValidateDataset(&dataset, BuiltinEvaluators()); err != nil {
		t.Fatalf("ValidateDataset: %v", err)
	}
	digest, err := DatasetDigest(dataset)
	if err != nil {
		t.Fatalf("DatasetDigest: %v", err)
	}
	output := "expected"
	previous := Report{
		SchemaVersion: ReportSchemaVersion,
		DatasetID:     dataset.ID,
		DatasetDigest: digest,
		Cases: []CaseResult{{
			CaseID: "case-1",
			Observation: Observation{
				CaseID:       "case-1",
				Status:       RunStatusCompleted,
				Output:       &output,
				Capabilities: Capabilities{Output: CoverageComplete},
				Regradable:   true,
			},
			Errors: []ErrorDetail{{Stage: "cleanup", Message: "close failed"}},
		}},
	}
	result, err := Regrade(context.Background(), dataset, previous, nil)
	if err != nil {
		t.Fatalf("Regrade: %v", err)
	}
	if result.Cases[0].Status != CaseStatusError || result.Summary.Errors != 1 {
		t.Fatalf("cleanup error was not preserved: %#v", result)
	}

	tests := []struct {
		name   string
		mutate func(*Report)
	}{
		{"schema", func(report *Report) { report.SchemaVersion = "future" }},
		{"dataset id", func(report *Report) { report.DatasetID = "other" }},
		{"dataset digest", func(report *Report) { report.DatasetDigest = "sha256:other" }},
		{"duplicate case", func(report *Report) { report.Cases = append(report.Cases, report.Cases[0]) }},
		{"missing case", func(report *Report) { report.Cases = nil }},
		{"case id", func(report *Report) { report.Cases[0].CaseID = "other" }},
		{"observation id", func(report *Report) { report.Cases[0].Observation.CaseID = "other" }},
		{"not regradable", func(report *Report) { report.Cases[0].Observation.Regradable = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := previous
			candidate.Cases = append([]CaseResult(nil), previous.Cases...)
			test.mutate(&candidate)
			if _, err := Regrade(context.Background(), dataset, candidate, nil); err == nil {
				t.Fatal("invalid saved report was accepted")
			}
		})
	}
}

func reportTestDataset() Dataset {
	return Dataset{
		SchemaVersion: DatasetSchemaVersion,
		ID:            "suite",
		Cases: []Case{{
			ID:    "case-1",
			Input: "prompt",
			Checks: []Check{{
				ID:     "answer",
				Type:   EvaluatorOutputExact,
				Config: json.RawMessage(`{"value":"expected"}`),
			}},
		}},
	}
}

func reportTestFixture() Report {
	dataset := reportTestDataset()
	output := "secret-output"
	errorMessage := "secret-error"
	return Report{
		SchemaVersion:   ReportSchemaVersion,
		DatasetID:       dataset.ID,
		DatasetDigest:   "sha256:test",
		ResolvedDataset: &dataset,
		Cases: []CaseResult{{
			CaseID: "case-1",
			Status: CaseStatusPass,
			Observation: Observation{
				CaseID:     "case-1",
				Status:     RunStatusCompleted,
				Output:     &output,
				Error:      &ErrorDetail{Stage: "execution", Message: errorMessage},
				Regradable: true,
				Trace: Trace{Spans: []ToolSpan{{
					Arguments: "secret-arguments",
					Result:    "secret-result",
					Error:     "secret-tool-error",
				}}},
			},
			Errors: []ErrorDetail{{Stage: "cleanup", Message: "secret-case-error"}},
			Metrics: []MetricResult{{
				CheckID: "answer",
				Status:  MetricStatusPass,
				Message: "secret-message",
				Evidence: map[string]any{
					"text":   "secret-evidence",
					"nested": []any{"secret-list", 1.0},
				},
			}},
			Durations: Durations{TotalMS: 1250},
		}},
		Summary: Summary{Total: 1, Passed: 1, PassRate: 1},
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
