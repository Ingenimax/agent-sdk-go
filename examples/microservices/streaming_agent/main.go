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
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/microservice"
	"github.com/Ingenimax/agent-sdk-go/pkg/structuredoutput"
)

// ProgrammingConcept represents a structured explanation of a programming concept
// This matches Anthropic Claude's flattened structure preference
type ProgrammingConcept struct {
	Concept             string               `json:"concept" description:"The main concept being explained"`
	Definition          string               `json:"definition" description:"Clear definition of the concept"`
	Analogy             string               `json:"analogy,omitempty" description:"An analogy to help understand the concept"`
	KeyComponents       []KeyComponent       `json:"key_components" description:"Key components or requirements"`
	CodeExample         CodeExample          `json:"code_example" description:"A practical code example"`
	Benefits            []string             `json:"benefits" description:"List of benefits"`
	Considerations      []string             `json:"considerations,omitempty" description:"Important considerations"`
	CommonApps          []string             `json:"common_applications,omitempty" description:"Common applications"`
	AlternativeApproach *AlternativeApproach `json:"alternative_approach,omitempty" description:"Alternative implementation approach"`
}

type KeyComponent struct {
	Component   string `json:"component" description:"Name of the component"`
	Description string `json:"description" description:"Description of what this component does"`
	Importance  string `json:"importance,omitempty" description:"Why this component is important"`
}

type CodeExample struct {
	Language    string   `json:"language" description:"Programming language used"`
	Problem     string   `json:"problem" description:"The problem being solved"`
	Code        string   `json:"code" description:"Code implementation"`
	Explanation string   `json:"explanation" description:"Explanation of how the code works"`
	StepByStep  []string `json:"step_by_step,omitempty" description:"Step-by-step execution breakdown"`
}

type AlternativeApproach struct {
	Description string `json:"description" description:"Description of the alternative approach"`
	Code        string `json:"code" description:"Alternative code implementation"`
}

// ANSI color codes for terminal output
const (
	ColorReset  = "\033[0m"
	ColorGray   = "\033[90m" // Dark gray for thinking
	ColorWhite  = "\033[97m" // Bright white for content
	ColorGreen  = "\033[32m" // Green for success
	ColorYellow = "\033[33m" // Yellow for tools
	ColorRed    = "\033[31m" // Red for errors
	ColorBlue   = "\033[34m" // Blue for logs
	ColorBold   = "\033[1m"  // Bold text
)

// This example demonstrates streaming with structured output using AgentMicroservice
// It shows how to:
// 1. Create a streaming agent with thinking support and structured JSON output
// 2. Define Go structs for response schema using struct tags
// 3. Use structuredoutput.NewResponseFormat() to auto-generate JSON schema
// 4. Wrap the agent in a microservice
// 5. Use event handlers for streaming with custom output formatting
// 6. Parse and beautifully display structured JSON responses
//
// Required environment variables:
// - ANTHROPIC_API_KEY or OPENAI_API_KEY (depending on provider)
// - LLM_PROVIDER: "anthropic" or "openai" (optional, defaults to "anthropic")

func main() {
	fmt.Println("Streaming Agent with Structured Output Example")
	fmt.Println("==============================================")
	fmt.Println()

	// Get provider and API key
	provider := getEnvWithDefault("LLM_PROVIDER", "anthropic")
	fmt.Printf("Using provider: %s\n", provider)

	// Create LLM client
	llm, err := createLLMClient(provider)
	if err != nil {
		log.Fatalf("Failed to create LLM client: %v", err)
	}

	// Step 1: Create streaming agent with thinking support and structured output
	fmt.Println("\n1. Creating streaming agent with thinking support and structured output...")

	// Create response format from Go struct using the utility
	responseFormat := structuredoutput.NewResponseFormat(ProgrammingConcept{})

	streamingAgent, err := agent.NewAgent(
		agent.WithName("StructuredStreamingAgent"),
		agent.WithDescription("A streaming agent that provides structured responses with thinking process"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt(`You are a helpful AI assistant that provides comprehensive, well-structured educational content about programming concepts.

Instructions:
- Always respond with valid JSON matching the exact schema provided
- Use simple strings for fields like "definition" - avoid nested objects
- Include practical code examples with detailed explanations
- Provide analogies as a separate string field
- List key components as an array with component/description/importance fields
- Include benefits and considerations as simple string arrays
- Be comprehensive but use a flattened JSON structure for better parsing`),
		agent.WithLLMConfig(interfaces.LLMConfig{
			Temperature:     0.7,
			EnableReasoning: true, // Enable thinking for supported models
			ReasoningBudget: 2048,
		}),
		agent.WithResponseFormat(*responseFormat),
	)
	if err != nil {
		log.Fatalf("Failed to create streaming agent: %v", err)
	}
	fmt.Printf("Created streaming agent: %s\n", streamingAgent.GetName())

	// Step 2: Create and start microservice
	fmt.Println("\n2. Creating microservice wrapper...")
	service, err := microservice.CreateMicroservice(streamingAgent, microservice.Config{
		Port: 0, // Auto-assign port
	})
	if err != nil {
		log.Fatalf("Failed to create microservice: %v", err)
	}

	// Start the microservice
	if err := service.Start(); err != nil {
		log.Fatalf("Failed to start microservice: %v", err)
	}

	// Wait for service to be ready
	fmt.Printf("Starting microservice on port %d...\n", service.GetPort())
	if err := service.WaitForReady(10 * time.Second); err != nil {
		log.Fatalf("Microservice failed to become ready: %v", err)
	}
	fmt.Printf("Microservice ready on port %d\n", service.GetPort())

	// Step 3: Set up event handlers and structured output streaming
	fmt.Println("\n3. Setting up event handlers and structured output streaming...")

	query := "Explain the concept of recursion in programming with a simple example. Provide a structured response with definition, key components, a practical code example, and benefits."
	fmt.Printf("Query: %s\n", query)
	fmt.Println("Expected output: Structured JSON with concept explanation")
	fmt.Println(strings.Repeat("-", 80))

	if err := streamWithHandlers(service, query); err != nil {
		fmt.Printf("%sError: %v%s\n", ColorRed, err, ColorReset)
	}

	// Cleanup
	fmt.Println("\nCleaning up...")
	if err := service.Stop(); err != nil {
		log.Printf("Warning: failed to stop microservice: %v", err)
	}
	fmt.Println("Microservice stopped")
	fmt.Println("\nStreaming example completed successfully!")
}

// streamWithHandlers demonstrates using event handlers with structured output formatting
func streamWithHandlers(service *microservice.AgentMicroservice, input string) error {
	ctx := context.Background()

	fmt.Printf("%s[LOG]%s Setting up event handlers\n", ColorBlue, ColorReset)

	inThinkingMode := false
	var completeResponse strings.Builder

	// Register event handlers with logging and structured output formatting
	service.
		OnThinking(func(thinking string) {
			if !inThinkingMode {
				fmt.Printf("\n%s[LOG]%s Entering thinking mode\n", ColorBlue, ColorReset)
				fmt.Printf("%s<thinking>%s\n", ColorGray, ColorReset)
				inThinkingMode = true
			}
			fmt.Printf("%s%s%s", ColorGray, thinking, ColorReset)
		}).
		OnContent(func(content string) {
			if inThinkingMode {
				fmt.Printf("\n%s</thinking>%s\n", ColorGray, ColorReset)
				fmt.Printf("%s[LOG]%s Exiting thinking mode, displaying content\n", ColorBlue, ColorReset)
				inThinkingMode = false
			}
			// Accumulate the complete response for structured formatting
			completeResponse.WriteString(content)
			// Show raw streaming content
			fmt.Printf("%s%s%s", ColorWhite, content, ColorReset)
		}).
		OnToolCall(func(toolCall *interfaces.ToolCallEvent) {
			fmt.Printf("\n%s[LOG]%s Tool call initiated: %s\n", ColorBlue, ColorReset, toolCall.Name)
			fmt.Printf("%sTool Call: %s%s%s\n", ColorYellow, ColorBold, toolCall.Name, ColorReset)
			if toolCall.Arguments != "" {
				fmt.Printf("%s   Arguments: %s%s\n", ColorYellow, toolCall.Arguments, ColorReset)
			}
			fmt.Printf("%s   Status: %s%s\n", ColorYellow, toolCall.Status, ColorReset)
		}).
		OnToolResult(func(toolCall *interfaces.ToolCallEvent) {
			fmt.Printf("%s[LOG]%s Tool execution completed: %s\n", ColorBlue, ColorReset, toolCall.Name)
			fmt.Printf("%sTool Result: %s%s\n", ColorGreen, toolCall.Result, ColorReset)
		}).
		OnError(func(err error) {
			fmt.Printf("\n%s[LOG]%s Error occurred during streaming\n", ColorBlue, ColorReset)
			fmt.Printf("\n%sError: %v%s\n", ColorRed, err, ColorReset)
		}).
		OnComplete(func() {
			if inThinkingMode {
				fmt.Printf("\n%s</thinking>%s\n", ColorGray, ColorReset)
			}
			fmt.Printf("\n%s[LOG]%s Stream completed successfully\n", ColorBlue, ColorReset)

			// Parse and display the structured response
			responseText := completeResponse.String()
			if responseText != "" {

				fmt.Printf("\n%s=== STRUCTURED OUTPUT ===%s\n", ColorBold, ColorReset)
				displayStructuredResponse(responseText)
			} else {
				fmt.Printf("%s[WARNING]%s No response content accumulated\n", ColorRed, ColorReset)
			}

			fmt.Printf("%sStream completed%s\n", ColorGreen, ColorReset)
		})

	fmt.Printf("%s[LOG]%s Starting stream execution\n", ColorBlue, ColorReset)
	return service.Stream(ctx, input)
}

// displayStructuredResponse parses and beautifully displays the structured JSON response
func displayStructuredResponse(responseText string) {
	// Clean the response text - remove markdown code blocks if present
	cleanedResponse := strings.TrimSpace(responseText)

	// Remove markdown code block markers if they exist
	cleanedResponse = strings.TrimPrefix(cleanedResponse, "```json")
	cleanedResponse = strings.TrimPrefix(cleanedResponse, "```")
	cleanedResponse = strings.TrimSuffix(cleanedResponse, "```")

	cleanedResponse = strings.TrimSpace(cleanedResponse)

	// Find JSON object boundaries
	start := strings.Index(cleanedResponse, "{")
	end := strings.LastIndex(cleanedResponse, "}")

	if start == -1 || end == -1 || start >= end {
		fmt.Printf("%sError: Could not find valid JSON in response%s\n", ColorRed, ColorReset)
		fmt.Printf("Response text: %s\n", responseText)
		return
	}

	jsonText := cleanedResponse[start : end+1]

	var concept ProgrammingConcept
	if err := json.Unmarshal([]byte(jsonText), &concept); err != nil {
		fmt.Printf("%sError parsing JSON: %v%s\n", ColorRed, err, ColorReset)
		fmt.Printf("Attempted to parse: %s\n", jsonText)
		return
	}

	fmt.Printf("\n%s📚 Concept:%s %s\n", ColorBold, ColorReset, concept.Concept)

	fmt.Printf("\n%s📖 Definition:%s\n", ColorBold, ColorReset)
	fmt.Printf("  %s\n", concept.Definition)

	if concept.Analogy != "" {
		fmt.Printf("\n%s💡 Analogy:%s\n", ColorBold, ColorReset)
		fmt.Printf("  %s\n", concept.Analogy)
	}

	if len(concept.KeyComponents) > 0 {
		fmt.Printf("\n%s🔧 Key Components:%s\n", ColorBold, ColorReset)
		for _, component := range concept.KeyComponents {
			fmt.Printf("  • %s%s%s: %s\n", ColorYellow, component.Component, ColorReset, component.Description)
			if component.Importance != "" {
				fmt.Printf("    %sImportance:%s %s\n", ColorBold, ColorReset, component.Importance)
			}
		}
	}

	fmt.Printf("\n%s💻 Code Example:%s %s\n", ColorBold, ColorReset, concept.CodeExample.Problem)
	fmt.Printf("```%s\n%s\n```\n", concept.CodeExample.Language, concept.CodeExample.Code)

	if concept.CodeExample.Explanation != "" {
		fmt.Printf("\n%sExplanation:%s\n", ColorBold, ColorReset)
		fmt.Printf("  %s\n", concept.CodeExample.Explanation)
	}

	if len(concept.CodeExample.StepByStep) > 0 {
		fmt.Printf("\n%sStep-by-Step Execution:%s\n", ColorBold, ColorReset)
		for i, step := range concept.CodeExample.StepByStep {
			fmt.Printf("  %d. %s\n", i+1, step)
		}
	}

	if len(concept.Benefits) > 0 {
		fmt.Printf("\n%s✅ Benefits:%s\n", ColorBold, ColorReset)
		for _, benefit := range concept.Benefits {
			fmt.Printf("  • %s\n", benefit)
		}
	}

	if len(concept.Considerations) > 0 {
		fmt.Printf("\n%s⚠️  Considerations:%s\n", ColorBold, ColorReset)
		for _, consideration := range concept.Considerations {
			fmt.Printf("  • %s\n", consideration)
		}
	}

	if len(concept.CommonApps) > 0 {
		fmt.Printf("\n%s🚀 Common Applications:%s\n", ColorBold, ColorReset)
		for _, app := range concept.CommonApps {
			fmt.Printf("  • %s\n", app)
		}
	}

	if concept.AlternativeApproach != nil {
		fmt.Printf("\n%s🔄 Alternative Approach:%s %s\n", ColorBold, ColorReset, concept.AlternativeApproach.Description)
		fmt.Printf("```%s\n%s\n```\n", concept.CodeExample.Language, concept.AlternativeApproach.Code)
	}
}

// createLLMClient creates the appropriate LLM client based on provider
func createLLMClient(provider string) (interfaces.LLM, error) {
	switch provider {
	case "anthropic":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is required for Anthropic provider")
		}
		return anthropic.NewClient(
			apiKey,
			anthropic.WithModel(anthropic.ClaudeSonnet4),
		), nil

	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY environment variable is required for OpenAI provider")
		}
		// Note: o4-mini is a reasoning model but individual reasoning tokens
		// are not yet streamed in the Go SDK v2.1.1. Only final responses are streamed.
		return openai.NewClient(
			apiKey,
			openai.WithModel("o4-mini"),
		), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s (supported: anthropic, openai)", provider)
	}
}

// getEnvWithDefault gets environment variable with default value
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
