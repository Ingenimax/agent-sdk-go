package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
)

// This example demonstrates how to use streaming sub-agents.
//
// When a parent agent uses a sub-agent via AgentTool, and both support streaming,
// the sub-agent's stream events (thinking, tool calls, content) are forwarded in
// real-time to the parent agent's stream, making them visible to the end user.
//
// Key Features:
// - Real-time visibility into sub-agent thinking and execution
// - Tool calls and results from sub-agents are streamed
// - Automatic fallback to blocking execution when streaming is not available
// - Sub-agent events are tagged with metadata for identification

func main() {
	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Create LLM client
	llm := openai.NewClient(
		apiKey,
		openai.WithModel("gpt-4o"),
	)

	// Configure LLM
	llmConfig := interfaces.LLMConfig{
		Temperature: 0.7,
	}

	// Configure streaming to include thinking and tool progress
	streamConfig := &interfaces.StreamConfig{
		BufferSize:                  100,
		IncludeThinking:             true,
		IncludeToolProgress:         true,
		IncludeIntermediateMessages: true,
	}

	// Create a research specialist sub-agent
	researchAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithName("research_specialist"),
		agent.WithSystemPrompt(`You are a research specialist. Your role is to:
1. Analyze research topics thoroughly
2. Break down complex topics into key concepts
3. Provide detailed, well-structured research insights
4. Use <thinking> tags to show your reasoning process

Always think step-by-step and explain your research methodology.`),
		agent.WithLLMConfig(llmConfig),
		agent.WithStreamConfig(streamConfig),
	)
	if err != nil {
		log.Fatalf("Failed to create research agent: %v", err)
	}

	// Create a writing specialist sub-agent
	writingAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithName("writing_specialist"),
		agent.WithSystemPrompt(`You are a writing specialist. Your role is to:
1. Create clear, engaging content
2. Adapt tone and style to the audience
3. Structure information logically
4. Use <thinking> tags to show your writing process

Think carefully about word choice and flow.`),
		agent.WithLLMConfig(llmConfig),
		agent.WithStreamConfig(streamConfig),
	)
	if err != nil {
		log.Fatalf("Failed to create writing agent: %v", err)
	}

	// Create agent tools for the sub-agents
	researchTool := tools.NewAgentTool(researchAgent)

	writingTool := tools.NewAgentTool(writingAgent)

	// Update descriptions for clarity
	researchTool.SetDescription("Delegate research tasks to the research specialist. Use this for gathering information, analyzing topics, and conducting in-depth research.")
	writingTool.SetDescription("Delegate writing tasks to the writing specialist. Use this for creating articles, summaries, and well-structured content.")

	// Create the main coordinator agent
	coordinatorAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithName("coordinator"),
		agent.WithSystemPrompt(`You are a coordinator agent that manages research and writing tasks.

Available tools:
- research_specialist_agent: For research and analysis tasks
- writing_specialist_agent: For writing and content creation tasks

Your role is to:
1. Analyze the user's request
2. Break it down into research and writing subtasks
3. Delegate to the appropriate specialist agents
4. Coordinate the results into a final response

Use <thinking> tags to show your coordination strategy.`),
		agent.WithTools(researchTool, writingTool),
		agent.WithLLMConfig(llmConfig),
		agent.WithStreamConfig(streamConfig),
	)
	if err != nil {
		log.Fatalf("Failed to create coordinator agent: %v", err)
	}

	// Example task: Create an article about AI agents
	ctx := context.Background()
	userQuery := "Create a brief article about how AI agents work. Include key concepts and make it accessible to non-technical readers."

	fmt.Println(repeatString("=", 80))
	fmt.Println("STREAMING SUB-AGENT EXAMPLE")
	fmt.Println(repeatString("=", 80))
	fmt.Printf("\nUser Query: %s\n\n", userQuery)
	fmt.Println("Streaming events (showing real-time sub-agent execution):")
	fmt.Println(repeatString("-", 80))

	// Execute with streaming
	eventChan, err := coordinatorAgent.RunStream(ctx, userQuery)
	if err != nil {
		log.Fatalf("Failed to start streaming: %v", err)
	}

	// Process and display streaming events
	var currentAgent string
	var contentBuffer string

	for event := range eventChan {
		// Check if this is a sub-agent event
		if event.Metadata != nil {
			if subAgent, ok := event.Metadata["sub_agent"].(string); ok {
				if subAgent != currentAgent {
					if currentAgent != "" {
						fmt.Printf("\n[Switching to %s]\n", subAgent)
					} else {
						fmt.Printf("[%s activated]\n", subAgent)
					}
					currentAgent = subAgent
				}
			}
		}

		switch event.Type {
		case interfaces.AgentEventThinking:
			// Display thinking steps (from both parent and sub-agents)
			prefix := "[COORDINATOR THINKING]"
			if currentAgent != "" {
				prefix = fmt.Sprintf("[%s THINKING]", currentAgent)
			}
			fmt.Printf("\n%s\n%s\n", prefix, event.ThinkingStep)

		case interfaces.AgentEventToolCall:
			// Display tool calls with details
			if event.ToolCall != nil {
				agentLabel := "COORDINATOR"
				if currentAgent != "" {
					agentLabel = currentAgent
				}
				fmt.Printf("\n[%s TOOL CALL: %s]\n", agentLabel, event.ToolCall.DisplayName)
				if event.ToolCall.Arguments != "" {
					fmt.Printf("Arguments: %s\n", event.ToolCall.Arguments)
				}
			}

		case interfaces.AgentEventToolResult:
			// Display tool results
			if event.ToolCall != nil {
				agentLabel := "COORDINATOR"
				if currentAgent != "" {
					agentLabel = currentAgent
				}
				fmt.Printf("\n[%s TOOL RESULT: %s]\n", agentLabel, event.ToolCall.Name)
				if event.ToolCall.Result != "" {
					// Truncate long results for display
					result := event.ToolCall.Result
					if len(result) > 200 {
						result = result[:200] + "...[truncated]"
					}
					fmt.Printf("Result: %s\n", result)
				}
			}

		case interfaces.AgentEventContent:
			// Accumulate content (final response)
			contentBuffer += event.Content

		case interfaces.AgentEventComplete:
			// Execution complete
			fmt.Println("\n" + repeatString("=", 80))
			fmt.Println("FINAL RESPONSE")
			fmt.Println(repeatString("=", 80))
			fmt.Println(contentBuffer)

		case interfaces.AgentEventError:
			// Handle errors
			log.Printf("Error during execution: %v", event.Error)
		}
	}

	fmt.Println("\n" + repeatString("=", 80))
	fmt.Println("Streaming completed successfully!")
	fmt.Println("\nKey Observations:")
	fmt.Println("- Sub-agent thinking steps were visible in real-time")
	fmt.Println("- Tool calls from sub-agents were streamed to the parent")
	fmt.Println("- Each sub-agent's execution was clearly identified")
	fmt.Println("- The entire workflow was transparent to the end user")
	fmt.Println(repeatString("=", 80))
}

// Helper function to repeat strings
func repeatString(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}
