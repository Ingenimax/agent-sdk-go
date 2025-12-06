// Package main demonstrates real-time streaming with DeepSeek LLM
// using the agent framework's RunStream capability.
//
// This demo shows:
//   - Agent streaming with DeepSeek LLM
//   - Real-time content generation streamed progressively
//   - Tool calls (calculator) streamed as they happen
//   - Complete streaming workflow with event processing
//
// Requirements:
//   - DEEPSEEK_API_KEY environment variable
//
// Usage:
//
//	export DEEPSEEK_API_KEY=your_api_key_here
//	go run main.go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/deepseek"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

func main() {
	// Check for API key
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		fmt.Println("Error: DEEPSEEK_API_KEY environment variable not set")
		fmt.Println("\nPlease set your DeepSeek API key:")
		fmt.Println("export DEEPSEEK_API_KEY=your_api_key_here")
		fmt.Println("\nGet your API key at: https://platform.deepseek.com/api_keys")
		os.Exit(1)
	}

	fmt.Println("DeepSeek Streaming Demo with Agent Framework")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Println("This demo shows real-time streaming with DeepSeek LLM.")
	fmt.Println("You'll see content generation and tool calls streaming in real-time!")
	fmt.Println()

	// Create agent with DeepSeek LLM and calculator tool
	mathAgent, err := agent.NewAgent(
		agent.WithName("MathAgent"),
		agent.WithDescription("Specialized in mathematical calculations"),
		agent.WithLLM(deepseek.NewClient(
			apiKey,
			deepseek.WithModel("deepseek-chat"),
		)),
		agent.WithTools(calculator.New()),
		agent.WithSystemPrompt(`You are a mathematical expert. When given a problem:
1. Think through the approach step by step
2. Use the calculator tool for computations
3. Explain your reasoning clearly
4. Provide the final answer

Always show your work.`),
		agent.WithRequirePlanApproval(false), // Direct execution
		agent.WithStreamConfig(&interfaces.StreamConfig{
			IncludeThinking:     true,
			IncludeToolProgress: true,
			BufferSize:          100,
		}),
		agent.WithLLMConfig(interfaces.LLMConfig{
			Temperature: 0.7,
		}),
	)
	if err != nil {
		fmt.Printf("Failed to create agent: %v\n", err)
		os.Exit(1)
	}

	// Example query that will trigger calculator tool
	query := "Calculate the compound interest on $10,000 invested at 5% annual rate for 3 years, compounded annually. Show me all the intermediate calculations."

	fmt.Println("Query:")
	fmt.Println(query)
	fmt.Println()
	fmt.Println("Streaming Response:")
	fmt.Println("-------------------")

	// Run with streaming
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	eventChan, err := mathAgent.RunStream(ctx, query)
	if err != nil {
		fmt.Printf("Failed to start streaming: %v\n", err)
		os.Exit(1)
	}

	// Track statistics
	var (
		thinkingEvents int
		contentChunks  int
		toolCalls      int
		totalContent   string
	)

	// Process events in real-time
	for event := range eventChan {
		switch event.Type {
		case interfaces.AgentEventThinking:
			thinkingEvents++
			fmt.Printf("\n[THINKING] %s\n", event.ThinkingStep)

		case interfaces.AgentEventContent:
			contentChunks++
			fmt.Print(event.Content)
			totalContent += event.Content

		case interfaces.AgentEventToolCall:
			toolCalls++
			toolName := event.ToolCall.Name
			if event.ToolCall.DisplayName != "" {
				toolName = event.ToolCall.DisplayName
			}

			fmt.Printf("\n\n[TOOL CALL] %s\n", toolName)
			fmt.Printf("   Arguments: %s\n", event.ToolCall.Arguments)

		case interfaces.AgentEventToolResult:
			resultPreview := event.ToolCall.Result
			if len(resultPreview) > 200 {
				resultPreview = resultPreview[:200] + "..."
			}

			fmt.Printf("   Result: %s\n\n", resultPreview)

		case interfaces.AgentEventError:
			fmt.Printf("\n\n[ERROR] %v\n", event.Error)

		case interfaces.AgentEventComplete:
			fmt.Printf("\n\n[COMPLETE]\n")
		}
	}

	// Print statistics
	fmt.Println()
	fmt.Println("===========================================")
	fmt.Println("Streaming Statistics:")
	fmt.Println("===========================================")
	fmt.Printf("Thinking Events: %d\n", thinkingEvents)
	fmt.Printf("Content Chunks: %d\n", contentChunks)
	fmt.Printf("Tool Calls: %d\n", toolCalls)
	fmt.Printf("Total Content Length: %d characters\n", len(totalContent))
	fmt.Println()

	fmt.Println("Key Observations:")
	fmt.Println("   • Content streamed progressively instead of waiting for completion")
	fmt.Println("   • Tool calls visible in real-time as they happen")
	fmt.Println("   • Natural user experience with immediate feedback")
	fmt.Println()
	fmt.Println("This is the power of streaming with DeepSeek!")
}
