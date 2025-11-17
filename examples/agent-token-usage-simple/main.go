package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
	// Get API key from environment variable
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Create OpenAI LLM client
	llm := openai.NewClient(
		apiKey,
		openai.WithModel("gpt-4o-mini"), // Using a cost-effective model for the example
	)

	// Create agent with token usage tracking support
	ag, err := agent.NewAgent(
		agent.WithName("TokenTrackingAgent"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt("You are a helpful assistant that provides concise answers."),
	)
	if err != nil {
		log.Fatal("Failed to create agent:", err)
	}

	ctx := context.Background()

	// Example 1: Regular execution (backward compatible - no token tracking)
	fmt.Println("=== Example 1: Regular Run (backward compatible) ===")
	simpleResponse, err := ag.Run(ctx, "What is 2+2?")
	if err != nil {
		log.Fatal("Failed to run agent:", err)
	}
	fmt.Printf("Response: %s\n\n", simpleResponse)

	// Example 2: Detailed execution with comprehensive token tracking
	fmt.Println("=== Example 2: RunDetailed (with comprehensive tracking) ===")
	detailedResponse, err := ag.RunDetailed(ctx, "Explain what machine learning is in 3 sentences.")
	if err != nil {
		log.Fatal("Failed to run agent with details:", err)
	}

	// Display the response
	fmt.Printf("Response: %s\n\n", detailedResponse.Content)

	// Display agent information
	fmt.Printf("Agent Information:\n")
	fmt.Printf("  Agent Name: %s\n", detailedResponse.AgentName)
	fmt.Printf("  Model Used: %s\n", detailedResponse.Model)

	// Display detailed token usage
	if detailedResponse.Usage != nil {
		fmt.Printf("\nToken Usage:\n")
		fmt.Printf("  Input Tokens: %d\n", detailedResponse.Usage.InputTokens)
		fmt.Printf("  Output Tokens: %d\n", detailedResponse.Usage.OutputTokens)
		fmt.Printf("  Total Tokens: %d\n", detailedResponse.Usage.TotalTokens)
		if detailedResponse.Usage.ReasoningTokens > 0 {
			fmt.Printf("  Reasoning Tokens: %d\n", detailedResponse.Usage.ReasoningTokens)
		}

		// Cost calculation for GPT-4o-mini (as of 2024)
		// Input: $0.000150 per 1K tokens, Output: $0.000600 per 1K tokens
		inputCost := float64(detailedResponse.Usage.InputTokens) * 0.000150 / 1000
		outputCost := float64(detailedResponse.Usage.OutputTokens) * 0.000600 / 1000
		totalCost := inputCost + outputCost

		fmt.Printf("\nCost Estimation (GPT-4o-mini rates):\n")
		fmt.Printf("  Input Cost: $%.6f\n", inputCost)
		fmt.Printf("  Output Cost: $%.6f\n", outputCost)
		fmt.Printf("  Total Cost: $%.6f\n", totalCost)
	}

	// Display execution analytics
	fmt.Printf("\nExecution Analytics:\n")
	fmt.Printf("  LLM Calls Made: %d\n", detailedResponse.ExecutionSummary.LLMCalls)
	fmt.Printf("  Execution Time: %dms\n", detailedResponse.ExecutionSummary.ExecutionTimeMs)
	fmt.Printf("  Tools Used: %d\n", detailedResponse.ExecutionSummary.ToolCalls)
	if len(detailedResponse.ExecutionSummary.UsedTools) > 0 {
		fmt.Printf("  Tool Names: %v\n", detailedResponse.ExecutionSummary.UsedTools)
	}
	if len(detailedResponse.ExecutionSummary.UsedSubAgents) > 0 {
		fmt.Printf("  Sub-Agents Called: %v\n", detailedResponse.ExecutionSummary.UsedSubAgents)
	}

	// Display metadata
	fmt.Printf("\nMetadata:\n")
	for key, value := range detailedResponse.Metadata {
		fmt.Printf("  %s: %v\n", key, value)
	}

	// Example 3: Multiple queries with cost tracking
	fmt.Println("\n=== Example 3: Multiple Queries with Cost Tracking ===")

	queries := []string{
		"What is the capital of France?",
		"Name three benefits of renewable energy.",
		"How does photosynthesis work?",
	}

	var totalTokens int
	var totalCost float64

	for i, query := range queries {
		fmt.Printf("\nQuery %d: %s\n", i+1, query)

		response, err := ag.RunDetailed(ctx, query)
		if err != nil {
			log.Printf("Failed to execute query %d: %v", i+1, err)
			continue
		}

		// Display brief results
		fmt.Printf("  Response: %s\n", response.Content)

		if response.Usage != nil {
			fmt.Printf("  Tokens Used: %d (Input: %d, Output: %d)\n",
				response.Usage.TotalTokens,
				response.Usage.InputTokens,
				response.Usage.OutputTokens)
			fmt.Printf("  Execution Time: %dms\n", response.ExecutionSummary.ExecutionTimeMs)

			// Calculate cost
			queryCost := (float64(response.Usage.InputTokens)*0.000150 +
						 float64(response.Usage.OutputTokens)*0.000600) / 1000
			fmt.Printf("  Query Cost: $%.6f\n", queryCost)

			// Add to totals
			totalTokens += response.Usage.TotalTokens
			totalCost += queryCost
		}
	}

	fmt.Printf("\n=== Session Summary ===\n")
	fmt.Printf("Total Tokens Across All Queries: %d\n", totalTokens)
	fmt.Printf("Total Session Cost: $%.6f\n", totalCost)

	// Example 4: Usage for monitoring and optimization
	fmt.Println("\n=== Example 4: Usage Monitoring Insights ===")

	if totalTokens > 0 {
		avgTokensPerQuery := float64(totalTokens) / float64(len(queries))
		avgCostPerQuery := totalCost / float64(len(queries))

		fmt.Printf("Average Tokens per Query: %.1f\n", avgTokensPerQuery)
		fmt.Printf("Average Cost per Query: $%.6f\n", avgCostPerQuery)

		// Provide optimization suggestions based on usage
		if avgTokensPerQuery > 1000 {
			fmt.Println("\n💡 Optimization Suggestions:")
			fmt.Println("  - Consider shorter prompts or more specific queries")
			fmt.Println("  - Use a smaller model for simple tasks")
			fmt.Println("  - Implement response caching for repeated queries")
		}

		if totalCost > 0.01 {
			fmt.Printf("\n💰 Cost Alert: Session cost ($%.6f) exceeded threshold\n", totalCost)
		}
	}

	fmt.Println("\n✅ Example completed! Check the detailed token usage above.")
	fmt.Println("🔍 Use RunDetailed() in your applications for comprehensive monitoring.")
}