package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// getTestAPIKey retrieves the API key from environment for testing
func getTestAPIKey() string {
	return os.Getenv("ANTHROPIC_API_KEY")
}

// TestStreamingIntermediateMessages tests that intermediate messages are included when the flag is set
func TestStreamingIntermediateMessages(t *testing.T) {
	// Skip if no API key is set
	apiKey := getTestAPIKey()
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping test")
	}

	client := NewClient(apiKey, WithModel("claude-3-5-haiku-20241022"))

	// Define a simple calculation tool
	calculatorTool := &testCalculatorTool{}

	testCases := []struct {
		name                        string
		includeIntermediateMessages bool
		prompt                      string
		expectedBehavior            string
	}{
		{
			name:                        "Without intermediate messages (default)",
			includeIntermediateMessages: false,
			prompt:                      "Calculate 15 + 27, then multiply that result by 2. Show your work step by step.",
			expectedBehavior:            "Should only stream final result",
		},
		{
			name:                        "With intermediate messages enabled",
			includeIntermediateMessages: true,
			prompt:                      "Calculate 15 + 27, then multiply that result by 2. Show your work step by step.",
			expectedBehavior:            "Should stream intermediate calculations",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Configure streaming with or without intermediate messages
			streamConfig := interfaces.StreamConfig{
				BufferSize:                  100,
				IncludeIntermediateMessages: tc.includeIntermediateMessages,
			}

			// Start streaming
			eventChan, err := client.GenerateWithToolsStream(
				ctx,
				tc.prompt,
				[]interfaces.Tool{calculatorTool},
				interfaces.WithStreamConfig(streamConfig),
				interfaces.WithMaxIterations(3),
			)

			if err != nil {
				t.Fatalf("Failed to start streaming: %v", err)
			}

			// Collect all content events
			var contentEvents []string
			var toolCallCount int
			hasIntermediateContent := false

			for event := range eventChan {
				switch event.Type {
				case interfaces.StreamEventContentDelta:
					contentEvents = append(contentEvents, event.Content)
					// Check if we're getting content before all tools complete
					if toolCallCount > 0 && toolCallCount < 2 {
						hasIntermediateContent = true
					}
				case interfaces.StreamEventToolUse:
					toolCallCount++
				case interfaces.StreamEventError:
					t.Errorf("Received error event: %v", event.Error)
				}
			}

			fullContent := strings.Join(contentEvents, "")
			t.Logf("Tool calls made: %d", toolCallCount)
			t.Logf("Content length: %d", len(fullContent))
			t.Logf("Has intermediate content: %v", hasIntermediateContent)

			// Verify behavior based on flag
			if tc.includeIntermediateMessages {
				if !hasIntermediateContent {
					t.Errorf("Expected intermediate content to be streamed, but it wasn't")
				}
				t.Logf("✓ Intermediate messages were streamed as expected")
			} else {
				if hasIntermediateContent {
					t.Errorf("Did not expect intermediate content to be streamed, but it was")
				}
				t.Logf("✓ Intermediate messages were filtered as expected")
			}

			// Verify we got a final response
			if len(fullContent) == 0 {
				t.Errorf("No content was streamed")
			}

			// Verify the calculation result is present
			if !strings.Contains(fullContent, "84") { // 15 + 27 = 42, 42 * 2 = 84
				t.Errorf("Expected calculation result (84) not found in content")
			}
		})
	}
}

// testCalculatorTool implements the Tool interface for testing
type testCalculatorTool struct{}

func (c *testCalculatorTool) Name() string {
	return "calculator"
}

func (c *testCalculatorTool) Description() string {
	return "Performs basic arithmetic operations"
}

func (c *testCalculatorTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"operation": {
			Type:        "string",
			Description: "The operation to perform (add, subtract, multiply, divide)",
			Required:    true,
			Enum:        []interface{}{"add", "subtract", "multiply", "divide"},
		},
		"a": {
			Type:        "number",
			Description: "First number",
			Required:    true,
		},
		"b": {
			Type:        "number",
			Description: "Second number",
			Required:    true,
		},
	}
}

func (c *testCalculatorTool) Run(ctx context.Context, input string) (string, error) {
	return c.Execute(ctx, input)
}

func (c *testCalculatorTool) Execute(ctx context.Context, args string) (string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	operation := params["operation"].(string)
	a := params["a"].(float64)
	b := params["b"].(float64)

	switch operation {
	case "add":
		return fmt.Sprintf("Result: %.2f", a+b), nil
	case "subtract":
		return fmt.Sprintf("Result: %.2f", a-b), nil
	case "multiply":
		return fmt.Sprintf("Result: %.2f", a*b), nil
	case "divide":
		if b == 0 {
			return "", fmt.Errorf("division by zero")
		}
		return fmt.Sprintf("Result: %.2f", a/b), nil
	default:
		return "", fmt.Errorf("unknown operation: %s", operation)
	}
}

// testCounterTool implements the Tool interface for testing
type testCounterTool struct {
	callCount int
}

func (c *testCounterTool) Name() string {
	return "counter"
}

func (c *testCounterTool) Description() string {
	return "Increments a counter and returns the current value"
}

func (c *testCounterTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{}
}

func (c *testCounterTool) Run(ctx context.Context, input string) (string, error) {
	return c.Execute(ctx, input)
}

func (c *testCounterTool) Execute(ctx context.Context, args string) (string, error) {
	c.callCount++
	return fmt.Sprintf("Counter is now at: %d", c.callCount), nil
}

// TestStreamingIntermediateMessagesWithMultipleIterations tests intermediate messages across multiple iterations
func TestStreamingIntermediateMessagesWithMultipleIterations(t *testing.T) {
	// Skip if no API key is set
	apiKey := getTestAPIKey()
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping test")
	}

	client := NewClient(apiKey, WithModel("claude-3-5-haiku-20241022"))

	// Define a simple counter tool that requires multiple calls
	counterTool := &testCounterTool{callCount: 0}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Enable intermediate messages
	streamConfig := interfaces.StreamConfig{
		BufferSize:                  100,
		IncludeIntermediateMessages: true,
	}

	// Reset counter
	counterTool.callCount = 0

	// Start streaming with a prompt that should trigger multiple tool calls
	eventChan, err := client.GenerateWithToolsStream(
		ctx,
		"Call the counter tool 3 times and tell me what happens each time.",
		[]interfaces.Tool{counterTool},
		interfaces.WithStreamConfig(streamConfig),
		interfaces.WithMaxIterations(4), // Allow enough iterations
	)

	if err != nil {
		t.Fatalf("Failed to start streaming: %v", err)
	}

	// Track events
	var contentSegments []string
	var toolCalls int
	contentBetweenTools := make(map[int]string)
	currentContent := ""

	for event := range eventChan {
		switch event.Type {
		case interfaces.StreamEventContentDelta:
			currentContent += event.Content
		case interfaces.StreamEventToolUse:
			if currentContent != "" && toolCalls > 0 {
				contentBetweenTools[toolCalls] = currentContent
			}
			currentContent = ""
			toolCalls++
		case interfaces.StreamEventContentComplete:
			if currentContent != "" {
				contentSegments = append(contentSegments, currentContent)
			}
		}
	}

	t.Logf("Total tool calls: %d", toolCalls)
	t.Logf("Content segments between tools: %d", len(contentBetweenTools))

	// Verify we got intermediate content between tool calls
	if len(contentBetweenTools) == 0 {
		t.Errorf("Expected intermediate content between tool calls, but got none")
	} else {
		t.Logf("✓ Received intermediate content between tool calls")
		for i, content := range contentBetweenTools {
			t.Logf("  Content after tool call %d: %d characters", i, len(content))
		}
	}

	// Verify we made the expected number of tool calls
	if toolCalls < 3 {
		t.Errorf("Expected at least 3 tool calls, got %d", toolCalls)
	}
}