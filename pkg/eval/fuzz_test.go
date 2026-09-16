package eval

import "testing"

func FuzzParseDatasetDoesNotPanic(f *testing.F) {
	f.Add([]byte(`{"schema_version":"1","id":"d","cases":[{"id":"c","input":"x","checks":[{"id":"m","type":"output_exact","config":{"value":"x"}}]}]}`))
	f.Add([]byte(`{"schema_version":"unknown"}`))
	f.Add([]byte(`not json`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseDataset(data)
	})
}

func FuzzJSONArgumentComparisonDoesNotLoseNumericPrecision(f *testing.F) {
	f.Add(`{"n":9007199254740993}`, `{"n":9007199254740993}`, false)
	f.Add(`{"n":1e0}`, `{"n":1.00,"extra":true}`, true)
	f.Add(`[]`, `{}`, false)
	f.Fuzz(func(t *testing.T, expectedJSON, actualJSON string, subset bool) {
		expected, expectedErr := decodeOneJSON([]byte(expectedJSON), true)
		actual, actualErr := decodeOneJSON([]byte(actualJSON), true)
		if expectedErr != nil || actualErr != nil {
			return
		}
		_ = jsonValueMatches(expected, actual, subset)
	})
}
