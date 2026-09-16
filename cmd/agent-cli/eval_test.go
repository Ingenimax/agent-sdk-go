package main

import (
	"bytes"
	"context"
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

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
