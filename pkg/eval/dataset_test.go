package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestParseDatasetResolvesDefaultsAndCanonicalizesConfig(t *testing.T) {
	first := `{
		"schema_version":"1",
		"id":"suite",
		"cases":[{
			"id":"case-1",
			"input":"hello",
			"checks":[{
				"id":"answer",
				"type":"output_contains",
				"config":{"normalization":"trim_space","value":"hello"}
			}]
		}]
	}`
	second := `{
		"id":"suite",
		"cases":[{
			"checks":[{
				"required":true,
				"threshold":1,
				"config":{"value":"hello","normalization":"trim_space"},
				"type":"output_contains",
				"id":"answer"
			}],
			"input":"hello",
			"id":"case-1"
		}],
		"schema_version":"1"
	}`

	datasetA, err := ParseDataset([]byte(first))
	if err != nil {
		t.Fatalf("ParseDataset(first): %v", err)
	}
	datasetB, err := ParseDataset([]byte(second))
	if err != nil {
		t.Fatalf("ParseDataset(second): %v", err)
	}
	check := datasetA.Cases[0].Checks[0]
	if check.Threshold == nil || *check.Threshold != 1 {
		t.Fatalf("threshold was not resolved: %#v", check.Threshold)
	}
	if check.Required == nil || !*check.Required {
		t.Fatalf("required was not resolved: %#v", check.Required)
	}
	digestA, err := DatasetDigest(datasetA)
	if err != nil {
		t.Fatalf("DatasetDigest(first): %v", err)
	}
	digestB, err := DatasetDigest(datasetB)
	if err != nil {
		t.Fatalf("DatasetDigest(second): %v", err)
	}
	if digestA != digestB {
		t.Fatalf("canonical digests differ: %s != %s", digestA, digestB)
	}
}

func TestParseDatasetRejectsAmbiguousOrInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		contains string
	}{
		{
			name:     "duplicate config key",
			json:     `{"schema_version":"1","id":"d","cases":[{"id":"c","input":"x","checks":[{"id":"m","type":"output_exact","config":{"value":"x","value":"y"}}]}]}`,
			contains: "duplicate object key",
		},
		{
			name:     "unknown config field",
			json:     `{"schema_version":"1","id":"d","cases":[{"id":"c","input":"x","checks":[{"id":"m","type":"output_exact","config":{"value":"x","surprise":true}}]}]}`,
			contains: "unknown field",
		},
		{
			name:     "unknown dataset field",
			json:     `{"schema_version":"1","id":"d","extra":1,"cases":[]}`,
			contains: "unknown field",
		},
		{
			name:     "only advisory checks",
			json:     `{"schema_version":"1","id":"d","cases":[{"id":"c","input":"x","checks":[{"id":"m","type":"output_exact","required":false,"config":{"value":"x"}}]}]}`,
			contains: "at least one required check",
		},
		{
			name:     "missing maximum",
			json:     `{"schema_version":"1","id":"d","cases":[{"id":"c","input":"x","checks":[{"id":"m","type":"max_total_tokens","config":{}}]}]}`,
			contains: "maximum is required",
		},
		{
			name:     "external schema",
			json:     `{"schema_version":"1","id":"d","cases":[{"id":"c","input":"x","checks":[{"id":"m","type":"output_json_schema","config":{"schema":{"$ref":"https://example.com/schema.json"}}}]}]}`,
			contains: "externally referenced schema",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseDataset([]byte(test.json))
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("got error %v, want one containing %q", err, test.contains)
			}
		})
	}
}

func TestCanonicalizeJSONNumber(t *testing.T) {
	tests := map[string]string{
		"1e0":    "1",
		"1.00":   "1",
		"1e-3":   "0.001",
		"-0.00":  "0",
		"1.25e2": "125",
	}
	for input, expected := range tests {
		actual, err := canonicalizeJSONNumber(input)
		if err != nil {
			t.Fatalf("canonicalizeJSONNumber(%q): %v", input, err)
		}
		if actual != expected {
			t.Fatalf("canonicalizeJSONNumber(%q) = %q, want %q", input, actual, expected)
		}
	}
	if _, err := canonicalizeJSONNumber("1e100001"); err == nil {
		t.Fatal("oversized exponent was accepted")
	}
}

func TestLoadDatasetReaderAndDocumentLimits(t *testing.T) {
	data := []byte(`{
		"schema_version":"1",
		"id":"suite",
		"cases":[{"id":"case","input":"hello","checks":[{
			"id":"answer","type":"output_exact","config":{"value":"hello"}
		}]}]
	}`)
	dataset, err := LoadDataset(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("LoadDataset: %v", err)
	}
	if dataset.ID != "suite" || len(dataset.Cases) != 1 {
		t.Fatalf("dataset = %#v", dataset)
	}
	if _, err := LoadDataset(nil); err == nil {
		t.Fatal("nil reader was accepted")
	}
	if _, err := LoadDataset(failingReader{}); err == nil {
		t.Fatal("reader failure was ignored")
	}
	if _, err := LoadDatasetWithEvaluators(bytes.NewReader(data), nil); err == nil {
		t.Fatal("nil evaluator registry was accepted")
	}
	if _, err := LoadDataset(bytes.NewReader(bytes.Repeat([]byte{' '}, int(DefaultMaxDatasetBytes)+1))); err == nil {
		t.Fatal("oversized dataset was accepted")
	}

	invalidDocuments := [][]byte{
		nil,
		{'{', 0xff, '}'},
		[]byte(`{"schema_version":"1","id":"suite","cases":[]} {}`),
	}
	for _, input := range invalidDocuments {
		if _, err := ParseDataset(input); err == nil {
			t.Fatalf("invalid dataset was accepted: %q", input)
		}
	}
}

func TestValidateDatasetRejectsInvalidStructure(t *testing.T) {
	falseValue := false
	nan := math.NaN()
	invalidUTF8 := string([]byte{0xff})
	tests := []struct {
		name       string
		mutate     func(*Dataset)
		evaluators func() Evaluators
	}{
		{"schema", func(dataset *Dataset) { dataset.SchemaVersion = "future" }, nil},
		{"dataset id", func(dataset *Dataset) { dataset.ID = "" }, nil},
		{"dataset id utf8", func(dataset *Dataset) { dataset.ID = invalidUTF8 }, nil},
		{"no cases", func(dataset *Dataset) { dataset.Cases = nil }, nil},
		{"nil evaluators", func(*Dataset) {}, func() Evaluators { return nil }},
		{"case id", func(dataset *Dataset) { dataset.Cases[0].ID = "" }, nil},
		{"case id utf8", func(dataset *Dataset) { dataset.Cases[0].ID = invalidUTF8 }, nil},
		{"duplicate case", func(dataset *Dataset) { dataset.Cases = append(dataset.Cases, dataset.Cases[0]) }, nil},
		{"input utf8", func(dataset *Dataset) { dataset.Cases[0].Input = invalidUTF8 }, nil},
		{"reference utf8", func(dataset *Dataset) { dataset.Cases[0].Reference = &invalidUTF8 }, nil},
		{"tag utf8", func(dataset *Dataset) { dataset.Cases[0].Tags = []string{invalidUTF8} }, nil},
		{"no checks", func(dataset *Dataset) { dataset.Cases[0].Checks = nil }, nil},
		{"check id", func(dataset *Dataset) { dataset.Cases[0].Checks[0].ID = "" }, nil},
		{"check utf8", func(dataset *Dataset) { dataset.Cases[0].Checks[0].Type = invalidUTF8 }, nil},
		{"duplicate check", func(dataset *Dataset) {
			dataset.Cases[0].Checks = append(dataset.Cases[0].Checks, dataset.Cases[0].Checks[0])
		}, nil},
		{"check type", func(dataset *Dataset) { dataset.Cases[0].Checks[0].Type = "" }, nil},
		{"unknown evaluator", func(dataset *Dataset) { dataset.Cases[0].Checks[0].Type = "unknown" }, nil},
		{"nil evaluator", func(*Dataset) {}, func() Evaluators {
			registry := BuiltinEvaluators()
			registry[EvaluatorOutputExact] = nil
			return registry
		}},
		{"empty config", func(dataset *Dataset) { dataset.Cases[0].Checks[0].Config = nil }, nil},
		{"null config", func(dataset *Dataset) { dataset.Cases[0].Checks[0].Config = json.RawMessage(`null`) }, nil},
		{"threshold", func(dataset *Dataset) { dataset.Cases[0].Checks[0].Threshold = &nan }, nil},
		{"no required checks", func(dataset *Dataset) { dataset.Cases[0].Checks[0].Required = &falseValue }, nil},
		{"validator panic", func(*Dataset) {}, func() Evaluators {
			return Evaluators{EvaluatorOutputExact: panickingValidationEvaluator{}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataset := reportTestDataset()
			test.mutate(&dataset)
			evaluators := BuiltinEvaluators()
			if test.evaluators != nil {
				evaluators = test.evaluators()
			}
			if err := ValidateDataset(&dataset, evaluators); err == nil {
				t.Fatal("invalid dataset was accepted")
			}
		})
	}
	if err := ValidateDataset(nil, BuiltinEvaluators()); err == nil {
		t.Fatal("nil dataset was accepted")
	}
}

func TestCanonicalizeJSONValueRecursesThroughCollections(t *testing.T) {
	value := map[string]any{
		"number": json.Number("1.2500"),
		"items":  []any{json.Number("1e-3"), true, nil},
	}
	canonical, err := canonicalizeJSONValue(value)
	if err != nil {
		t.Fatalf("canonicalizeJSONValue: %v", err)
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if string(data) != `{"items":[0.001,true,null],"number":1.25}` {
		t.Fatalf("canonical JSON = %s", data)
	}
	if _, err := canonicalizeJSONValue(json.Number("1/3")); err == nil {
		t.Fatal("non-decimal JSON number was accepted")
	}
}

type panickingValidationEvaluator struct{}

func (panickingValidationEvaluator) Name() string { return EvaluatorOutputExact }
func (panickingValidationEvaluator) Validate(Check) error {
	panic("validation failed")
}
func (panickingValidationEvaluator) Evaluate(_ context.Context, _ Evaluation) (MetricResult, error) {
	return MetricResult{}, nil
}
