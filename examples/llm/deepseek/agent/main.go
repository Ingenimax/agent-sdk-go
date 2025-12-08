package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/deepseek"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// SearchTool simulates a web search tool
type SearchTool struct{}

func (t *SearchTool) Name() string {
	return "web_search"
}

func (t *SearchTool) Description() string {
	return "Search the web for information on a given topic"
}

func (t *SearchTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"query": {
			Type:        "string",
			Description: "The search query",
			Required:    true,
		},
	}
}

func (t *SearchTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *SearchTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Query string `json:"query"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	// Simulate search results
	results := map[string]interface{}{
		"query":   params.Query,
		"results": []string{
			"Result 1: DeepSeek-V3.2 released with 128K context window",
			"Result 2: DeepSeek reasoning models outperform GPT-4 on benchmarks",
			fmt.Sprintf("Result 3: Integration guide for %s", params.Query),
		},
	}

	resultJSON, _ := json.Marshal(results)
	return string(resultJSON), nil
}

func main() {
	// Get API key from environment
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		log.Fatal("DEEPSEEK_API_KEY environment variable is required")
	}

	ctx := context.Background()

	fmt.Println("=== DeepSeek with Memory and Tools Example ===")
	fmt.Println()

	// Create DeepSeek client
	client := deepseek.NewClient(
		apiKey,
		deepseek.WithModel("deepseek-chat"),
	)

	// Create memory for conversation persistence
	mem := memory.NewConversationBuffer()

	// Define available tools
	tools := []interfaces.Tool{
		&SearchTool{},
	}

	// Example 1: Simple tool use with memory
	fmt.Println("--- Example 1: Tool Use with Memory ---")
	prompt1 := "Search for information about DeepSeek API features"

	fmt.Printf("User: %s\n", prompt1)
	response, err := client.GenerateWithToolsDetailed(
		ctx,
		prompt1,
		tools,
		interfaces.WithMemory(mem),
		interfaces.WithSystemMessage("You are a helpful AI research assistant."),
		interfaces.WithTemperature(0.7),
	)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Assistant: %s\n", response.Content)
	fmt.Printf("Tokens Used: %d\n\n", response.Usage.TotalTokens)

	// Example 2: Follow-up question using memory
	fmt.Println("--- Example 2: Follow-up with Memory Context ---")
	prompt2 := "Based on those search results, which feature would be most useful for building a chatbot?"

	fmt.Printf("User: %s\n", prompt2)
	response, err = client.GenerateWithToolsDetailed(
		ctx,
		prompt2,
		tools,
		interfaces.WithMemory(mem),
		interfaces.WithSystemMessage("You are a helpful AI research assistant."),
		interfaces.WithTemperature(0.7),
	)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Assistant: %s\n", response.Content)
	fmt.Printf("Tokens Used: %d\n\n", response.Usage.TotalTokens)

	// Example 3: Check memory history
	fmt.Println("--- Example 3: Conversation History ---")
	messages, err := mem.GetMessages(ctx)
	if err != nil {
		log.Printf("Could not retrieve memory: %v", err)
	} else {
		fmt.Printf("Total messages in conversation history: %d\n", len(messages))
		for i, msg := range messages {
			role := msg.Role
			content := msg.Content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			fmt.Printf("  %d. [%s]: %s\n", i+1, role, content)
		}
	}

	// Example 4: Using reasoning model
	fmt.Println("\n--- Example 4: Switching to Reasoning Model ---")

	reasoningClient := deepseek.NewClient(
		apiKey,
		deepseek.WithModel("deepseek-reasoner"),
	)

	task4 := "Given the search results about DeepSeek, analyze the technical advantages of using a 128K context window for production applications. Think step-by-step."

	fmt.Printf("User: %s\n", task4)
	response, err = reasoningClient.GenerateDetailed(
		ctx,
		task4,
		interfaces.WithTemperature(0.1),
	)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Assistant (Reasoning Mode): %s\n", response.Content)
	fmt.Printf("Tokens Used: %d input + %d output\n",
		response.Usage.InputTokens,
		response.Usage.OutputTokens)

	fmt.Println("\n=== Demo Complete ===")
	fmt.Println("\nKey Features Demonstrated:")
	fmt.Println("  - DeepSeek client with tool calling")
	fmt.Println("  - Memory persistence across conversations")
	fmt.Println("  - System prompts for behavior control")
	fmt.Println("  - Model switching (chat vs reasoning)")
	fmt.Println("  - Temperature control")
	fmt.Println("  - Token usage tracking")
}
