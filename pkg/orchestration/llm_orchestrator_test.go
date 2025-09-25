package orchestration

import (
	"testing"
)

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "JSON in markdown code block",
			input: "```json\n{\n  \"action\": \"create_new_task\",\n  \"reasoning\": [\n    \"User requested 'deploy eks' which is a clear deployment request\"\n  ]\n}\n```",
			expected: "{\n  \"action\": \"create_new_task\",\n  \"reasoning\": [\n    \"User requested 'deploy eks' which is a clear deployment request\"\n  ]\n}",
		},
		{
			name: "JSON in generic code block",
			input: "```\n{\"test\": \"value\"}\n```",
			expected: `{"test": "value"}`,
		},
		{
			name: "JSON in code block with json identifier",
			input: "```json\n{\"test\": \"value\"}\n```",
			expected: `{"test": "value"}`,
		},
		{
			name: "Plain JSON without code blocks",
			input: `{"test": "value"}`,
			expected: `{"test": "value"}`,
		},
		{
			name: "JSON with extra text before and after",
			input: `Here is the response: {"test": "value"} and that's it.`,
			expected: `{"test": "value"}`,
		},
		{
			name: "Empty string",
			input: "",
			expected: "",
		},
		{
			name: "No JSON found",
			input: "This is just text without JSON",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.input)
			if result != tt.expected {
				t.Errorf("extractJSON() = %q, expected %q", result, tt.expected)
			}
		})
	}
}