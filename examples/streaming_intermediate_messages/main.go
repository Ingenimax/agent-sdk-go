package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("Please set ANTHROPIC_API_KEY environment variable")
	}

	// Create Anthropic client
	llmClient := anthropic.NewClient(
		apiKey,
		anthropic.WithModel("claude-3-5-haiku-20241022"),
	)

	// Create calculation tool for demonstration
	calculatorTool := &CalculatorTool{}

	// Demonstrate the difference between with and without intermediate messages
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("DEMONSTRATION: Intermediate Messages in Streaming")
	fmt.Println(strings.Repeat("=", 80))

	// Test prompt that requires multiple tool calls
	testPrompt := "Calculate the following step by step:\n1. Add 15 and 27\n2. Multiply the result by 3\n3. Divide that by 2\nExplain each step as you go."

	// Run without intermediate messages (default behavior)
	fmt.Println("\n📋 SCENARIO 1: Without Intermediate Messages (Default)")
	fmt.Println(strings.Repeat("-", 60))
	runStreamingDemo(llmClient, calculatorTool, testPrompt, false)

	// Wait a moment between demos
	time.Sleep(2 * time.Second)

	// Run with intermediate messages enabled
	fmt.Println("\n📋 SCENARIO 2: With Intermediate Messages Enabled")
	fmt.Println(strings.Repeat("-", 60))
	runStreamingDemo(llmClient, calculatorTool, testPrompt, true)
}

// CalculatorTool implements the Tool interface for basic arithmetic
type CalculatorTool struct{}

func (c *CalculatorTool) Name() string {
	return "calculator"
}

func (c *CalculatorTool) Description() string {
	return "Performs basic arithmetic operations"
}

func (c *CalculatorTool) Parameters() map[string]interfaces.ParameterSpec {
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

func (c *CalculatorTool) Run(ctx context.Context, input string) (string, error) {
	return c.Execute(ctx, input)
}

func (c *CalculatorTool) Execute(ctx context.Context, args string) (string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	operation, ok := params["operation"].(string)
	if !ok {
		return "", fmt.Errorf("operation must be a string")
	}

	a, ok := params["a"].(float64)
	if !ok {
		return "", fmt.Errorf("parameter 'a' must be a number")
	}

	b, ok := params["b"].(float64)
	if !ok {
		return "", fmt.Errorf("parameter 'b' must be a number")
	}

	log.Printf("🔧 Calculator: %s(%.2f, %.2f)", operation, a, b)

	switch operation {
	case "add":
		return fmt.Sprintf("%.2f", a+b), nil
	case "subtract":
		return fmt.Sprintf("%.2f", a-b), nil
	case "multiply":
		return fmt.Sprintf("%.2f", a*b), nil
	case "divide":
		if b == 0 {
			return "", fmt.Errorf("division by zero")
		}
		return fmt.Sprintf("%.2f", a/b), nil
	default:
		return "", fmt.Errorf("unknown operation: %s", operation)
	}
}

func runStreamingDemo(llmClient interfaces.LLM, tool interfaces.Tool, prompt string, includeIntermediate bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Configure streaming
	streamConfig := &interfaces.StreamConfig{
		BufferSize:                  100,
		IncludeToolProgress:         true,
		IncludeIntermediateMessages: includeIntermediate,
	}

	if includeIntermediate {
		fmt.Println("✅ Intermediate messages ENABLED - You'll see the LLM's thinking between tool calls")
	} else {
		fmt.Println("❌ Intermediate messages DISABLED - You'll only see the final result")
	}
	fmt.Println()

	// Create agent with streaming configuration
	streamingAgent, err := agent.NewAgent(
		agent.WithLLM(llmClient),
		agent.WithTools(tool),
		agent.WithMaxIterations(4),
		agent.WithStreamConfig(streamConfig),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Start streaming
	eventChan, err := streamingAgent.RunStream(ctx, prompt)
	if err != nil {
		log.Fatalf("Failed to start streaming: %v", err)
	}

	// Process streaming events
	fmt.Println("🚀 Starting stream...")
	fmt.Println()

	var fullContent strings.Builder
	toolCallCount := 0
	contentDuringTools := false

	for event := range eventChan {
		switch event.Type {
		case interfaces.AgentEventContent:
			// Track if we're getting content while tools are still being called
			if toolCallCount > 0 && toolCallCount < 3 {
				contentDuringTools = true
			}
			fmt.Print(event.Content)
			fullContent.WriteString(event.Content)

		case interfaces.AgentEventToolCall:
			if event.ToolCall != nil {
				switch event.ToolCall.Status {
				case "starting":
					toolCallCount++
					fmt.Printf("\n🔧 [Tool Call #%d] %s", toolCallCount, event.ToolCall.Name)
					if event.ToolCall.Arguments != "" {
						fmt.Printf(" with args: %s", event.ToolCall.Arguments)
					}
					fmt.Println()
				case "completed":
					fmt.Printf("✅ [Tool Result] %s\n\n", event.ToolCall.Result)
				}
			}

		case interfaces.AgentEventError:
			fmt.Printf("\n❌ Error: %v\n", event.Error)

		case interfaces.AgentEventComplete:
			fmt.Println("\n\n✨ Stream completed")
		}
	}

	// Summary
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("📊 Summary:\n")
	fmt.Printf("   - Tool calls made: %d\n", toolCallCount)
	fmt.Printf("   - Total content length: %d characters\n", fullContent.Len())
	fmt.Printf("   - Content during tool iterations: %v\n", contentDuringTools)

	if includeIntermediate && contentDuringTools {
		fmt.Println("   ✅ Intermediate messages were successfully streamed!")
	} else if !includeIntermediate && !contentDuringTools {
		fmt.Println("   ✅ Intermediate messages were correctly filtered!")
	}
}