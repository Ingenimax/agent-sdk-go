package llm

// Message represents a message in a chat conversation
type Message struct {
	Role       string // "system", "user", "assistant", "tool"
	Content    string
	ToolCallID string // ID of the tool call this message is responding to (for tool messages)
}

// GenerateParams contains parameters for text generation
type GenerateParams struct {
	Temperature      float64  // Controls randomness (0.0 to 1.0)
	TopP             float64  // Alternative to temperature for nucleus sampling
	FrequencyPenalty float64  // Penalize frequent tokens (-2.0 to 2.0)
	PresencePenalty  float64  // Penalize tokens already present (-2.0 to 2.0)
	StopSequences    []string // Stop generation at these sequences
	TopK             int      // Limit vocabulary to top K tokens
	RepeatPenalty    float64  // Penalize token repetition
	Reasoning        string   // Reasoning mode for Claude models (none, minimal, comprehensive)
}

// DefaultGenerateParams returns default generation parameters
func DefaultGenerateParams() *GenerateParams {
	return &GenerateParams{
		Temperature:      0.7,
		TopP:             1.0,
		FrequencyPenalty: 0.0,
		PresencePenalty:  0.0,
		TopK:             50,
		RepeatPenalty:    1.1,
	}
}
