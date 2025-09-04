package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
	"github.com/Ingenimax/agent-sdk-go/pkg/structuredoutput"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
	"google.golang.org/genai"
)

// ANSI color codes for terminal output
const (
	ColorReset = "\033[0m"
	ColorGray  = "\033[90m" // Dark gray for reasoning/thinking
	ColorWhite = "\033[97m" // Bright white for final response
	ColorGreen = "\033[32m" // Green for success
	ColorRed   = "\033[31m" // Red for errors
)

// TopicAnalysis struct for structured output example
type TopicAnalysis struct {
	Summary         string   `json:"summary" description:"A brief summary of the topic"`
	KeyPoints       []string `json:"key_points" description:"List of key points"`
	ComplexityScore int      `json:"complexity_score" description:"Complexity score from 1-10"`
}

// CompanyAnalysis struct for a more complex structured output example
type CompanyInfo struct {
	Name        string `json:"name" description:"Company name"`
	Founded     int    `json:"founded,omitempty" description:"Year the company was founded"`
	Industry    string `json:"industry" description:"Primary industry sector"`
	Description string `json:"description" description:"Brief company description"`
}

type ProductInfo struct {
	Name       string   `json:"name" description:"Product name"`
	Category   string   `json:"category" description:"Product category"`
	Features   []string `json:"features" description:"Key product features"`
	LaunchYear int      `json:"launch_year,omitempty" description:"Year the product was launched"`
}

type CompanyAnalysis struct {
	Company     CompanyInfo   `json:"company" description:"Company information"`
	Products    []ProductInfo `json:"products" description:"Main products or services"`
	Competitors []string      `json:"competitors,omitempty" description:"Main competitors"`
	MarketShare float64       `json:"market_share,omitempty" description:"Estimated market share percentage"`
	Revenue     float64       `json:"revenue,omitempty" description:"Annual revenue in billions USD"`
	Employees   int           `json:"employees,omitempty" description:"Number of employees"`
	StockSymbol string        `json:"stock_symbol,omitempty" description:"Stock ticker symbol"`
}

// ExampleTool implements a simple weather tool for testing
type ExampleTool struct{}

func (t *ExampleTool) Name() string {
	return "get_weather"
}

func (t *ExampleTool) Description() string {
	return "Get current weather information for a location"
}

func (t *ExampleTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"location": {
			Type:        "string",
			Description: "The location to get weather for (e.g., 'New York', 'Tokyo')",
			Required:    true,
		},
		"units": {
			Type:        "string",
			Description: "Temperature units (celsius or fahrenheit)",
			Required:    false,
			Default:     "celsius",
			Enum:        []any{"celsius", "fahrenheit"},
		},
	}
}

func (t *ExampleTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *ExampleTool) Execute(ctx context.Context, args string) (string, error) {
	// Simple mock implementation
	log.Printf("Weather tool called with args: %s", args)
	return "The weather is sunny and 22°C with light clouds.", nil
}

func main() {
	fmt.Println("🌟 Gemini API Integration Examples")
	fmt.Println("=================================")
	fmt.Println()

	// Get API key from environment
	apiKey := os.Getenv("GEMINI_API_KEY")
	vertexProjectID := os.Getenv("GEMINI_VERTEX_PROJECT_ID")
	if apiKey == "" && vertexProjectID == "" {
		log.Fatal("GEMINI_API_KEY or GEMINI_VERTEX_PROJECT_ID environment variable is required. Get your API key from https://aistudio.google.com/app/apikey")
	}
	authOption := gemini.WithAPIKey(apiKey)
	backendOption := gemini.WithBackend(genai.BackendGeminiAPI)
	if apiKey != "" {
		authOption = gemini.WithAPIKey(apiKey)
		backendOption = gemini.WithBackend(genai.BackendGeminiAPI)
	}
	if vertexProjectID != "" {
		authOption = gemini.WithProjectID(vertexProjectID)
		backendOption = gemini.WithBackend(genai.BackendVertexAI)
	}

	ctx := context.Background()

	// Create Gemini client
	client, err := gemini.NewClient(
		ctx,
		authOption, backendOption,
		gemini.WithModel(gemini.ModelGemini20Flash),
	)
	if err != nil {
		log.Fatalf("Failed to create Gemini client: %v", err)
	}

	fmt.Printf("Created Gemini client: %s\n", client.Name())
	fmt.Printf("Using model: %s\n", client.GetModel())
	fmt.Printf("Supports streaming: %t\n\n", client.SupportsStreaming())

	// Example 1: Basic text generation
	fmt.Println("=== Example 1: Basic Text Generation ===")
	response, err := client.Generate(ctx, "Write a haiku about artificial intelligence")
	if err != nil {
		log.Fatalf("Failed to generate text: %v", err)
	}
	fmt.Printf("Response: %s\n\n", response)

	// Example 2: Text generation with system message and reasoning
	fmt.Println("=== Example 2: Text Generation with Reasoning ===")
	response, err = client.Generate(ctx,
		"Explain why the sky is blue",
		gemini.WithSystemMessage("You are a science teacher explaining concepts to middle school students."),
		gemini.WithReasoning("comprehensive"),
		gemini.WithTemperature(0.3),
	)
	if err != nil {
		log.Fatalf("Failed to generate text with reasoning: %v", err)
	}
	fmt.Printf("Response with comprehensive reasoning:\n%s\n\n", formatReasoningResponse(response))

	// Example 2b: Native Thinking Tokens
	fmt.Println("=== Example 2b: Native Thinking Tokens ===")

	// Create a new client with thinking enabled using a thinking-capable model
	thinkingClient, err := gemini.NewClient(
		ctx,
		authOption, backendOption,
		gemini.WithModel(gemini.ModelGemini25Flash),
		gemini.WithThinking(true),
		gemini.WithThinkingBudget(1000), // 1K thinking tokens
	)
	if err != nil {
		log.Fatalf("Failed to create thinking client: %v", err)
	}

	response, err = thinkingClient.Generate(ctx,
		"Solve this step-by-step: What is the area of a circle with radius 7?",
	)
	if err != nil {
		log.Fatalf("Failed to generate with thinking: %v", err)
	}
	fmt.Printf("Response with native thinking: %s\n\n", response)

	// Example 3: Structured output using structuredoutput package
	fmt.Println("=== Example 3: Structured Output ===")

	// Create response format from struct using the structuredoutput package
	responseFormat := structuredoutput.NewResponseFormat(TopicAnalysis{})

	response, err = client.Generate(ctx,
		"Analyze the topic of quantum computing",
		gemini.WithResponseFormat(*responseFormat),
	)
	if err != nil {
		log.Fatalf("Failed to generate structured output: %v", err)
	}

	// Parse and display the structured response
	var analysis TopicAnalysis
	if err := json.Unmarshal([]byte(response), &analysis); err != nil {
		log.Printf("Failed to parse JSON response: %v", err)
		fmt.Printf("Raw JSON response: %s\n\n", response)
	} else {
		fmt.Printf("Structured response:\n")
		fmt.Printf("  Summary: %s\n", analysis.Summary)
		fmt.Printf("  Complexity Score: %d\n", analysis.ComplexityScore)
		fmt.Printf("  Key Points:\n")
		for i, point := range analysis.KeyPoints {
			fmt.Printf("    %d. %s\n", i+1, point)
		}
		fmt.Println()
	}

	// Example 3b: Complex structured output with nested structs
	fmt.Println("=== Example 3b: Complex Structured Output ===")

	// Create response format from complex struct
	companyFormat := structuredoutput.NewResponseFormat(CompanyAnalysis{})

	response, err = client.Generate(ctx,
		"Analyze Apple Inc. as a technology company",
		gemini.WithResponseFormat(*companyFormat),
	)
	if err != nil {
		log.Fatalf("Failed to generate complex structured output: %v", err)
	}

	// Parse and display the complex structured response
	var companyAnalysis CompanyAnalysis
	if err := json.Unmarshal([]byte(response), &companyAnalysis); err != nil {
		log.Printf("Failed to parse complex JSON response: %v", err)
		fmt.Printf("Raw JSON response: %s\n\n", response)
	} else {
		fmt.Printf("Company Analysis:\n")
		fmt.Printf("  Company: %s (%s)\n", companyAnalysis.Company.Name, companyAnalysis.Company.Industry)
		fmt.Printf("  Founded: %d\n", companyAnalysis.Company.Founded)
		fmt.Printf("  Description: %s\n", companyAnalysis.Company.Description)

		if companyAnalysis.StockSymbol != "" {
			fmt.Printf("  Stock Symbol: %s\n", companyAnalysis.StockSymbol)
		}
		if companyAnalysis.Revenue > 0 {
			fmt.Printf("  Revenue: $%.1fB\n", companyAnalysis.Revenue)
		}
		if companyAnalysis.Employees > 0 {
			fmt.Printf("  Employees: %d\n", companyAnalysis.Employees)
		}
		if companyAnalysis.MarketShare > 0 {
			fmt.Printf("  Market Share: %.1f%%\n", companyAnalysis.MarketShare)
		}

		if len(companyAnalysis.Products) > 0 {
			fmt.Printf("  Products:\n")
			for _, product := range companyAnalysis.Products {
				fmt.Printf("    - %s (%s)", product.Name, product.Category)
				if product.LaunchYear > 0 {
					fmt.Printf(" - Launched %d", product.LaunchYear)
				}
				fmt.Println()
				if len(product.Features) > 0 {
					fmt.Printf("      Features: %s\n", strings.Join(product.Features, ", "))
				}
			}
		}

		if len(companyAnalysis.Competitors) > 0 {
			fmt.Printf("  Competitors: %s\n", strings.Join(companyAnalysis.Competitors, ", "))
		}
		fmt.Println()
	}

	// Example 4: Function calling
	fmt.Println("=== Example 4: Function Calling ===")
	tools := []interfaces.Tool{
		calculator.New(),
		&ExampleTool{},
	}

	response, err = client.GenerateWithTools(ctx,
		"What's the weather like in Tokyo? Also calculate 15 * 23 for me.",
		tools,
		gemini.WithSystemMessage("You are a helpful assistant. Use the available tools to answer questions accurately."),
	)
	if err != nil {
		log.Fatalf("Failed to generate with tools: %v", err)
	}
	fmt.Printf("Response with tools: %s\n\n", response)

	// Example 5: Streaming response
	fmt.Println("=== Example 5: Streaming Response ===")
	fmt.Print("Streaming response: ")

	stream, err := client.GenerateStream(ctx,
		"Tell me an interesting story about space exploration in exactly 3 paragraphs",
		gemini.WithTemperature(0.8),
	)
	if err != nil {
		log.Fatalf("Failed to create stream: %v", err)
	}

	for event := range stream {
		switch event.Type {
		case interfaces.StreamEventContentDelta:
			fmt.Print(event.Content)
		case interfaces.StreamEventError:
			fmt.Printf("\nError: %v\n", event.Error)
			return
		case interfaces.StreamEventMessageStop:
			fmt.Println("\n[Stream completed]")
		}
	}
	fmt.Println()

	// Example 5b: Streaming with Native Thinking (if supported)
	if gemini.SupportsThinking(gemini.ModelGemini25Flash) {
		fmt.Println("=== Example 5b: Streaming with Native Thinking ===")

		// Create thinking-enabled client for streaming
		thinkingClient, err := gemini.NewClient(
			ctx,
			authOption, backendOption,
			gemini.WithModel(gemini.ModelGemini25Flash),
			gemini.WithDynamicThinking(), // Enable dynamic thinking
		)
		if err != nil {
			log.Fatalf("Failed to create thinking client: %v", err)
		}

		fmt.Print("Streaming with thinking: ")

		thinkingStream, err := thinkingClient.GenerateStream(ctx,
			"Solve this math problem step by step: If a train travels 300 km in 4 hours, then speeds up and travels 200 km in 1.5 hours, what is the average speed for the entire journey?",
		)
		if err != nil {
			log.Fatalf("Failed to create thinking stream: %v", err)
		}

		for event := range thinkingStream {
			switch event.Type {
			case interfaces.StreamEventThinking:
				fmt.Printf("%s💭 %s%s", ColorGray, event.Content, ColorReset)
			case interfaces.StreamEventContentDelta:
				fmt.Printf("%s%s%s", ColorWhite, event.Content, ColorReset)
			case interfaces.StreamEventError:
				fmt.Printf("\n%sError: %v%s\n", ColorRed, event.Error, ColorReset)
				return
			case interfaces.StreamEventMessageStop:
				fmt.Printf("\n%s[Stream with thinking completed]%s\n", ColorGreen, ColorReset)
			}
		}
		fmt.Println()
	} else {
		fmt.Println("=== Example 5b: Thinking Not Available ===")
		fmt.Printf("Native thinking tokens not available for current setup.\n\n")
	}

	// Example 6: Streaming with tools
	fmt.Println("=== Example 6: Streaming with Tools ===")
	fmt.Print("Streaming with tools: ")

	toolStream, err := client.GenerateWithToolsStream(ctx,
		"Calculate the area of a circle with radius 7, then tell me about the weather in Paris",
		tools,
		gemini.WithSystemMessage("You are a helpful assistant. Use tools when needed and provide clear explanations."),
		interfaces.WithMaxIterations(4), // Allow more iterations for multiple tool calls
	)
	if err != nil {
		log.Fatalf("Failed to create tool stream: %v", err)
	}

	for event := range toolStream {
		switch event.Type {
		case interfaces.StreamEventContentDelta:
			fmt.Print(event.Content)
		case interfaces.StreamEventToolUse:
			if event.ToolCall != nil {
				fmt.Printf("\n[Using tool: %s]", event.ToolCall.Name)
			}
		case interfaces.StreamEventToolResult:
			if event.ToolCall != nil {
				fmt.Printf("\n[Tool result: %s]", event.Content)
			}
		case interfaces.StreamEventError:
			fmt.Printf("\nError: %v\n", event.Error)
			return
		case interfaces.StreamEventMessageStop:
			fmt.Println("\n[Stream completed]")
		}
	}
	fmt.Println()

	// Example 7: Different reasoning modes
	fmt.Println("=== Example 7: Different Reasoning Modes ===")

	question := "How do you solve the equation 2x + 8 = 20?"

	// No reasoning
	fmt.Println("No reasoning mode:")
	response, err = client.Generate(ctx, question, gemini.WithReasoning("none"))
	if err != nil {
		log.Fatalf("Failed to generate with no reasoning: %v", err)
	}
	fmt.Printf("%s\n\n", response)

	// Minimal reasoning
	fmt.Println("Minimal reasoning mode:")
	response, err = client.Generate(ctx, question, gemini.WithReasoning("minimal"))
	if err != nil {
		log.Fatalf("Failed to generate with minimal reasoning: %v", err)
	}
	fmt.Printf("%s\n\n", response)

	// Comprehensive reasoning
	fmt.Println("Comprehensive reasoning mode:")
	response, err = client.Generate(ctx, question, gemini.WithReasoning("comprehensive"))
	if err != nil {
		log.Fatalf("Failed to generate with comprehensive reasoning: %v", err)
	}
	fmt.Printf("%s\n\n", response)

	// Example 8: Model capabilities demonstration
	fmt.Println("=== Example 8: Model Capabilities ===")
	models := []string{
		gemini.ModelGemini25Pro,
		gemini.ModelGemini25Flash,
		gemini.ModelGemini25FlashLite,
		gemini.ModelGemini20Flash,
		gemini.ModelGemini15Pro,
		gemini.ModelGemini15Flash,
		gemini.ModelGemini15Flash8B,
	}

	for _, model := range models {
		capabilities := gemini.GetModelCapabilities(model)
		fmt.Printf("Model: %s\n", model)
		fmt.Printf("  - Supports streaming: %t\n", capabilities.SupportsStreaming)
		fmt.Printf("  - Supports tool calling: %t\n", capabilities.SupportsToolCalling)
		fmt.Printf("  - Supports vision: %t\n", capabilities.SupportsVision)
		fmt.Printf("  - Supports audio: %t\n", capabilities.SupportsAudio)
		fmt.Printf("  - Max input tokens: %d\n", capabilities.MaxInputTokens)
		fmt.Printf("  - Max output tokens: %d\n", capabilities.MaxOutputTokens)
		fmt.Println()
	}

	fmt.Println("✅ All examples completed successfully!")
}

// formatReasoningResponse formats the response to show reasoning in gray and final response in white
func formatReasoningResponse(response string) string {
	// Look for common reasoning patterns and format them
	lines := strings.Split(response, "\n")
	var result strings.Builder

	inReasoningSection := false

	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Detect reasoning/thinking patterns
		isReasoningLine := strings.Contains(strings.ToLower(trimmedLine), "step") ||
			strings.Contains(strings.ToLower(trimmedLine), "think") ||
			strings.Contains(strings.ToLower(trimmedLine), "reasoning") ||
			strings.Contains(strings.ToLower(trimmedLine), "process") ||
			strings.Contains(strings.ToLower(trimmedLine), "let me") ||
			strings.Contains(strings.ToLower(trimmedLine), "first") ||
			strings.Contains(strings.ToLower(trimmedLine), "then") ||
			strings.Contains(strings.ToLower(trimmedLine), "next") ||
			strings.Contains(strings.ToLower(trimmedLine), "so") ||
			strings.Contains(strings.ToLower(trimmedLine), "because") ||
			strings.Contains(strings.ToLower(trimmedLine), "therefore") ||
			(strings.HasPrefix(trimmedLine, "1.") || strings.HasPrefix(trimmedLine, "2.") ||
				strings.HasPrefix(trimmedLine, "3.") || strings.HasPrefix(trimmedLine, "4."))

		// Check for final answer indicators
		isFinalAnswer := strings.Contains(strings.ToLower(trimmedLine), "summary") ||
			strings.Contains(strings.ToLower(trimmedLine), "in conclusion") ||
			strings.Contains(strings.ToLower(trimmedLine), "final") ||
			strings.Contains(strings.ToLower(trimmedLine), "answer") ||
			strings.Contains(strings.ToLower(trimmedLine), "result") ||
			(strings.Contains(strings.ToLower(trimmedLine), "does that") && strings.Contains(strings.ToLower(trimmedLine), "sense"))

		// Start reasoning section
		if !inReasoningSection && isReasoningLine {
			inReasoningSection = true
			result.WriteString(fmt.Sprintf("%s💭 REASONING PROCESS:%s\n", ColorGray, ColorReset))
			result.WriteString(fmt.Sprintf("%s%s%s\n", ColorGray, strings.Repeat("-", 40), ColorReset))
		}

		// End reasoning section and start final answer
		if inReasoningSection && isFinalAnswer {
			result.WriteString(fmt.Sprintf("%s%s%s\n", ColorGray, strings.Repeat("-", 40), ColorReset))
			result.WriteString(fmt.Sprintf("%s📝 FINAL ANSWER:%s\n", ColorGreen, ColorReset))
			inReasoningSection = false
		}

		// Format the line based on current section
		if inReasoningSection {
			result.WriteString(fmt.Sprintf("%s%s%s\n", ColorGray, line, ColorReset))
		} else {
			result.WriteString(fmt.Sprintf("%s%s%s\n", ColorWhite, line, ColorReset))
		}

		// Add extra formatting for the last line if we're still in reasoning
		if i == len(lines)-1 && inReasoningSection {
			result.WriteString(fmt.Sprintf("%s%s%s\n", ColorGray, strings.Repeat("-", 40), ColorReset))
			result.WriteString(fmt.Sprintf("%s📝 FINAL ANSWER:%s\n", ColorGreen, ColorReset))
		}
	}

	return result.String()
}
