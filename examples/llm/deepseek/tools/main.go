package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/deepseek"
)

// WeatherTool simulates a weather API tool
type WeatherTool struct{}

func (t *WeatherTool) Name() string {
	return "get_weather"
}

func (t *WeatherTool) Description() string {
	return "Get the current weather for a specified location"
}

func (t *WeatherTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"location": {
			Type:        "string",
			Description: "The city and state, e.g. San Francisco, CA",
			Required:    true,
		},
		"unit": {
			Type:        "string",
			Description: "Temperature unit (celsius or fahrenheit)",
			Required:    false,
			Default:     "celsius",
			Enum:        []interface{}{"celsius", "fahrenheit"},
		},
	}
}

func (t *WeatherTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *WeatherTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Location string `json:"location"`
		Unit     string `json:"unit"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	// Simulate API call
	temp := 22
	if params.Unit == "fahrenheit" {
		temp = 72
	}

	result := map[string]interface{}{
		"location":    params.Location,
		"temperature": temp,
		"unit":        params.Unit,
		"condition":   "sunny",
		"humidity":    65,
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

// CalculatorTool provides basic arithmetic operations
type CalculatorTool struct{}

func (t *CalculatorTool) Name() string {
	return "calculate"
}

func (t *CalculatorTool) Description() string {
	return "Perform basic arithmetic operations (add, subtract, multiply, divide)"
}

func (t *CalculatorTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"operation": {
			Type:        "string",
			Description: "The operation to perform",
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

func (t *CalculatorTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *CalculatorTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Operation string  `json:"operation"`
		A         float64 `json:"a"`
		B         float64 `json:"b"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	var result float64
	switch params.Operation {
	case "add":
		result = params.A + params.B
	case "subtract":
		result = params.A - params.B
	case "multiply":
		result = params.A * params.B
	case "divide":
		if params.B == 0 {
			return "", fmt.Errorf("division by zero")
		}
		result = params.A / params.B
	default:
		return "", fmt.Errorf("unknown operation: %s", params.Operation)
	}

	return fmt.Sprintf("%.2f", result), nil
}

func main() {
	// Get API key from environment
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		log.Fatal("DEEPSEEK_API_KEY environment variable is required")
	}

	// Create DeepSeek client
	client := deepseek.NewClient(
		apiKey,
		deepseek.WithModel("deepseek-chat"),
	)

	ctx := context.Background()

	// Define available tools
	tools := []interfaces.Tool{
		&WeatherTool{},
		&CalculatorTool{},
	}

	fmt.Println("=== DeepSeek Tool Calling Example ===")
	fmt.Println()

	// Example 1: Weather query
	fmt.Println("--- Example 1: Weather Query ---")
	prompt1 := "What's the weather like in San Francisco and New York? Give me the temperature in Fahrenheit."

	fmt.Printf("User: %s\n", prompt1)
	response, err := client.GenerateWithToolsDetailed(
		ctx,
		prompt1,
		tools,
		interfaces.WithTemperature(0.7),
	)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Assistant: %s\n", response.Content)
	fmt.Printf("Iterations: %v\n", response.Metadata["iterations"])
	fmt.Printf("Tokens: %d input + %d output = %d total\n\n",
		response.Usage.InputTokens,
		response.Usage.OutputTokens,
		response.Usage.TotalTokens)

	// Example 2: Calculator
	fmt.Println("--- Example 2: Calculator ---")
	prompt2 := "What is 15.5 multiplied by 8, and then add 23 to the result?"

	fmt.Printf("User: %s\n", prompt2)
	responseText, err := client.GenerateWithTools(
		ctx,
		prompt2,
		tools,
		interfaces.WithTemperature(0.1),
	)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Assistant: %s\n\n", responseText)

	// Example 3: Complex multi-tool scenario
	fmt.Println("--- Example 3: Multi-Tool Scenario ---")
	prompt3 := `I'm planning a trip to Paris. Can you:
1. Check the weather in Paris
2. Calculate how much 500 euros would be in dollars (use exchange rate 1.1)`

	fmt.Printf("User: %s\n", prompt3)

	startTime := time.Now()
	response, err = client.GenerateWithToolsDetailed(
		ctx,
		prompt3,
		tools,
		interfaces.WithTemperature(0.7),
		interfaces.WithMaxIterations(5),
	)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	duration := time.Since(startTime)

	fmt.Printf("Assistant: %s\n", response.Content)
	fmt.Printf("\nExecution Details:\n")
	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  Iterations: %v\n", response.Metadata["iterations"])
	fmt.Printf("  Token Usage: %d total (%d input + %d output)\n",
		response.Usage.TotalTokens,
		response.Usage.InputTokens,
		response.Usage.OutputTokens)

	fmt.Println("\n=== Tool Calling Demo Complete ===")
	fmt.Println("\nKey Features Demonstrated:")
	fmt.Println("  - Automatic tool selection by the model")
	fmt.Println("  - Parallel tool execution")
	fmt.Println("  - Multi-turn tool interactions")
	fmt.Println("  - Error handling in tools")
	fmt.Println("  - Token usage tracking across iterations")
}
