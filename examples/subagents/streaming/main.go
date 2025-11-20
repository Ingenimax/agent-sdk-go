// Package main demonstrates real-time streaming from sub-agents to parent agents
// using Claude 3.5 Sonnet with extended thinking enabled.
//
// This demo shows:
//   - Parent agent delegating to a specialized math sub-agent
//   - Claude's extended thinking process streamed in real-time
//   - Tool calls (calculator) streamed as they happen
//   - Progressive content generation instead of waiting for final result
//
// Requirements:
//   - ANTHROPIC_API_KEY environment variable
//
// Usage:
//
//	export ANTHROPIC_API_KEY=your_api_key_here
//	go run streaming_demo.go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

func main() {
	// Check for API key
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("❌ Error: ANTHROPIC_API_KEY environment variable not set")
		fmt.Println("\nPlease set your Anthropic API key:")
		fmt.Println("export ANTHROPIC_API_KEY=your_api_key_here")
		fmt.Println("\nGet your API key at: https://console.anthropic.com/")
		os.Exit(1)
	}

	fmt.Println("🚀 Sub-Agent Streaming Demo with Claude 4.5 Sonnet")
	fmt.Println("=" + "================================================")
	fmt.Println()
	fmt.Println("This demo shows real-time streaming from sub-agents to parent agents.")
	fmt.Println("You'll see the sub-agent's THINKING PROCESS, content generation, and tool calls")
	fmt.Println("streaming in real-time as they happen!")
	fmt.Println("\n💡 Claude's extended thinking mode enabled - watch the reasoning unfold!")

	// Create sub-agent with calculator tool
	mathAgent, err := agent.NewAgent(
		agent.WithName("MathAgent"),
		agent.WithDescription("Specialized in mathematical calculations and numerical analysis"),
		agent.WithLLM(anthropic.NewClient(
			apiKey,
			anthropic.WithModel("claude-sonnet-4-5-20250929"), // Claude 4.5 Sonnet with extended thinking
		)),
		agent.WithTools(calculator.New()),
		agent.WithSystemPrompt(`You are a mathematical expert. When given a problem:
1. Think through the approach step by step
2. Use the calculator tool for computations
3. Explain your reasoning clearly
4. Provide the final answer

Always show your thinking process.`),
		agent.WithRequirePlanApproval(false), // Direct execution
		agent.WithStreamConfig(&interfaces.StreamConfig{
			IncludeThinking:     true,
			IncludeToolProgress: true,
			BufferSize:          100,
		}),
		agent.WithLLMConfig(interfaces.LLMConfig{
			Temperature:     1.0,  // Required for thinking mode
			EnableReasoning: true, // Enable extended thinking
			ReasoningBudget: 2048, // Token budget for thinking
		}),
	)
	if err != nil {
		fmt.Printf("❌ Failed to create math agent: %v\n", err)
		os.Exit(1)
	}

	// Create parent agent that coordinates
	parentAgent, err := agent.NewAgent(
		agent.WithName("CoordinatorAgent"),
		agent.WithDescription("Coordinates and delegates tasks to specialized agents"),
		agent.WithLLM(anthropic.NewClient(
			apiKey,
			anthropic.WithModel("claude-sonnet-4-5-20250929"),
		)),
		agent.WithAgents(mathAgent), // Register math agent as sub-agent
		agent.WithSystemPrompt(`You are a coordination agent. When you receive a task:
1. Analyze what type of task it is
2. If it's a math problem, delegate it to the MathAgent
3. Otherwise, handle it yourself

When delegating to MathAgent, clearly state that you're doing so.`),
		agent.WithRequirePlanApproval(false),
		agent.WithStreamConfig(&interfaces.StreamConfig{
			IncludeThinking:     true,
			IncludeToolProgress: true,
			BufferSize:          100,
		}),
		agent.WithLLMConfig(interfaces.LLMConfig{
			Temperature:     1.0,
			EnableReasoning: true,
			ReasoningBudget: 2048,
		}),
	)
	if err != nil {
		fmt.Printf("❌ Failed to create parent agent: %v\n", err)
		os.Exit(1)
	}

	// Example query that will trigger sub-agent
	query := "I need to calculate the compound interest on $5,000 invested at 6% annual rate for 3 years, compounded annually. Please show all the calculation steps and explain the formula being used."

	fmt.Println("📝 Query:")
	fmt.Println(query)
	fmt.Println()
	fmt.Println("🔄 Streaming Response:")
	fmt.Println(string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]) + string([]rune{'─'}[0]))

	// Run with streaming
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	eventChan, err := parentAgent.RunStream(ctx, query)
	if err != nil {
		fmt.Printf("❌ Failed to start streaming: %v\n", err)
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
			fmt.Printf("\n💭 [THINKING] %s\n", event.ThinkingStep)

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

			// Check if this is the sub-agent being called
			if event.ToolCall.Name == "MathAgent_agent" {
				fmt.Printf("\n\n🎯 [DELEGATING TO SUB-AGENT] %s\n", toolName)
				fmt.Printf("   📋 Arguments: %s\n", event.ToolCall.Arguments)
			} else {
				fmt.Printf("\n\n🔧 [TOOL CALL] %s\n", toolName)
				fmt.Printf("   📋 Arguments: %s\n", event.ToolCall.Arguments)
			}

		case interfaces.AgentEventToolResult:
			resultPreview := event.ToolCall.Result
			if len(resultPreview) > 200 {
				resultPreview = resultPreview[:200] + "..."
			}

			if event.ToolCall.Name == "MathAgent_agent" {
				fmt.Printf("   ✅ [SUB-AGENT RESULT] %s\n\n", resultPreview)
			} else {
				fmt.Printf("   ✅ [RESULT] %s\n\n", resultPreview)
			}

		case interfaces.AgentEventError:
			fmt.Printf("\n\n❌ [ERROR] %v\n", event.Error)

		case interfaces.AgentEventComplete:
			fmt.Printf("\n\n✅ [COMPLETE]\n")
		}
	}

	// Print statistics
	fmt.Println()
	fmt.Println(string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]))
	fmt.Println("📊 Streaming Statistics:")
	fmt.Println(string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]) + string([]rune{'═'}[0]))
	fmt.Printf("💭 Thinking Events: %d\n", thinkingEvents)
	fmt.Printf("📝 Content Chunks: %d\n", contentChunks)
	fmt.Printf("🔧 Tool Calls: %d\n", toolCalls)
	fmt.Printf("📏 Total Content Length: %d characters\n", len(totalContent))
	fmt.Println()

	fmt.Println("✨ Key Observations:")
	fmt.Println("   • You saw the parent agent's thinking process in real-time")
	fmt.Println("   • When it delegated to MathAgent, you saw that happen immediately")
	fmt.Println("   • The sub-agent's thinking and tool usage streamed back to you")
	fmt.Println("   • No waiting for a final result - progressive updates throughout!")
	fmt.Println()
	fmt.Println("🎯 This is the power of sub-agent streaming!")
}
