package interfaces

import "testing"

// TestWithMaxTokens covers the option's contract, including the "unset" case
// that lets callers forward a config value without special-casing zero.
func TestWithMaxTokens(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input int
		want  int
	}{
		{"positive", 4096, 4096},
		{"zero leaves provider default", 0, 0},
		{"negative is ignored", -1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := &GenerateOptions{}
			WithMaxTokens(tc.input)(options)

			if tc.want == 0 {
				// Must not even allocate an LLMConfig, or it would clobber a
				// config supplied by another option ordered after it.
				if options.LLMConfig != nil && options.LLMConfig.MaxTokens != 0 {
					t.Fatalf("MaxTokens = %d, want unset", options.LLMConfig.MaxTokens)
				}
				return
			}
			if options.LLMConfig == nil {
				t.Fatal("LLMConfig was not allocated")
			}
			if options.LLMConfig.MaxTokens != tc.want {
				t.Errorf("MaxTokens = %d, want %d", options.LLMConfig.MaxTokens, tc.want)
			}
		})
	}
}

// TestWithMaxTokensPreservesOtherFields guards against the option replacing a
// config that earlier options already populated.
func TestWithMaxTokensPreservesOtherFields(t *testing.T) {
	options := &GenerateOptions{}
	WithTemperature(0.3)(options)
	WithMaxTokens(1234)(options)
	WithTopP(0.9)(options)

	if options.LLMConfig.Temperature != 0.3 {
		t.Errorf("Temperature = %v, want 0.3", options.LLMConfig.Temperature)
	}
	if options.LLMConfig.TopP != 0.9 {
		t.Errorf("TopP = %v, want 0.9", options.LLMConfig.TopP)
	}
	if options.LLMConfig.MaxTokens != 1234 {
		t.Errorf("MaxTokens = %d, want 1234", options.LLMConfig.MaxTokens)
	}
}
