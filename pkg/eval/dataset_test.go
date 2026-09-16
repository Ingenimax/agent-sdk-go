package eval

import (
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
