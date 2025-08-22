package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

// ANSI color codes for terminal output
const (
	ColorReset  = "\033[0m"
	ColorGray   = "\033[90m"  // Dark gray for thinking
	ColorWhite  = "\033[97m"  // Bright white for content
	ColorGreen  = "\033[32m"  // Green for success
	ColorYellow = "\033[33m"  // Yellow for tools
	ColorRed    = "\033[31m"  // Red for errors
	ColorBlue   = "\033[34m"  // Blue for metadata
)

// This example demonstrates streaming functionality with the Agent SDK
// Supports both Anthropic and OpenAI streaming implementations
//
// Required environment variables:
// - ANTHROPIC_API_KEY or OPENAI_API_KEY (depending on which provider you want to test)
// - LLM_PROVIDER: "anthropic" or "openai" (optional, defaults to "anthropic")
//
// Example usage:
// export ANTHROPIC_API_KEY=your_anthropic_key
// export LLM_PROVIDER=anthropic
// go run main.go

func main() {
	fmt.Println("🚀 Agent SDK Streaming Examples")
	fmt.Println("===============================")
	fmt.Println()

	// Get provider choice
	provider := getEnvWithDefault("LLM_PROVIDER", "anthropic")
	fmt.Printf("Using provider: %s\n\n", provider)

	ctx := context.Background()
	
	// Add required context for memory operations
	ctx = multitenancy.WithOrgID(ctx, "streaming-examples")
	ctx = memory.WithConversationID(ctx, "streaming-demo")

	// Example 1: Basic LLM Streaming
	fmt.Println("📡 Example 1: Basic LLM Streaming")
	fmt.Println("--------------------------------")
	if err := basicLLMStreaming(ctx, provider); err != nil {
		log.Printf("Example 1 failed: %v", err)
	}
	fmt.Println()

	// Example 2: Agent Streaming
	fmt.Println("🤖 Example 2: Agent Streaming")
	fmt.Println("-----------------------------")
	if err := agentStreaming(ctx, provider); err != nil {
		log.Printf("Example 2 failed: %v", err)
	}
	fmt.Println()

	// Example 3: Streaming with Tools
	fmt.Println("🛠️  Example 3: Streaming with Tools")
	fmt.Println("----------------------------------")
	if err := streamingWithTools(ctx, provider); err != nil {
		log.Printf("Example 3 failed: %v", err)
	}
	fmt.Println()

	// Example 4: Advanced Streaming Features
	fmt.Println("⚡ Example 4: Advanced Streaming Features")
	fmt.Println("----------------------------------------")
	if err := advancedStreamingFeatures(ctx, provider); err != nil {
		log.Printf("Example 4 failed: %v", err)
	}
	fmt.Println()

	// Example 5: Real LLM Reasoning Demonstration
	fmt.Println("🧠 Example 5: Real LLM Reasoning Demonstration")
	fmt.Println("----------------------------------------------")
	if err := realReasoningDemo(ctx, provider); err != nil {
		log.Printf("Example 5 failed: %v", err)
	}
	fmt.Println()

	fmt.Println("✅ All streaming examples completed!")
}

// basicLLMStreaming demonstrates basic streaming from an LLM
func basicLLMStreaming(ctx context.Context, provider string) error {
	llm, err := createLLM(provider)
	if err != nil {
		return fmt.Errorf("failed to create LLM: %w", err)
	}

	// Check if LLM supports streaming
	if !llm.SupportsStreaming() {
		return fmt.Errorf("LLM %s does not support streaming", llm.Name())
	}

	streamingLLM := llm.(interfaces.StreamingLLM)

	// Start streaming
	fmt.Println("Starting LLM streaming...")
	eventChan, err := streamingLLM.GenerateStream(
		ctx,
		"Explain quantum computing in simple terms",
		func(opts *interfaces.GenerateOptions) {
			opts.SystemMessage = "You are a helpful science teacher. Think through your explanation step-by-step before responding. Use <thinking> tags to show your reasoning process, then provide your explanation."
		},
	)
	if err != nil {
		return fmt.Errorf("failed to start streaming: %w", err)
	}

	// Process streaming events
	fmt.Print("Response: ")
	var inThinking bool
	
	for event := range eventChan {
		switch event.Type {
		case interfaces.StreamEventContentDelta:
			content := event.Content
			
			// Check for thinking tags in content
			if strings.Contains(content, "<thinking>") {
				parts := strings.Split(content, "<thinking>")
				fmt.Printf("%s%s%s", ColorWhite, parts[0], ColorReset)
				if len(parts) > 1 {
					fmt.Printf("\n%s💭 THINKING PROCESS:%s\n", ColorGray, ColorReset)
					fmt.Printf("%s%s%s\n", ColorGray, strings.Repeat("-", 40), ColorReset)
					fmt.Printf("%s%s", ColorGray, parts[1])
					inThinking = true
				}
			} else if strings.Contains(content, "</thinking>") {
				parts := strings.Split(content, "</thinking>")
				fmt.Printf("%s%s", ColorGray, parts[0])
				fmt.Printf("\n%s%s%s\n", ColorGray, strings.Repeat("-", 40), ColorReset)
				fmt.Printf("%s📝 FINAL ANSWER:%s\n", ColorGreen, ColorReset)
				if len(parts) > 1 {
					fmt.Printf("%s%s%s", ColorWhite, parts[1], ColorReset)
				}
				inThinking = false
			} else {
				// Regular content - color based on whether we're in thinking mode
				if inThinking {
					fmt.Printf("%s%s", ColorGray, content)
				} else {
					fmt.Printf("%s%s%s", ColorWhite, content, ColorReset)
				}
			}
		case interfaces.StreamEventThinking:
			fmt.Printf("\n%s[Thinking: %s]%s\n", ColorGray, event.Content, ColorReset)
		case interfaces.StreamEventError:
			fmt.Printf("\n%s[Error: %v]%s\n", ColorRed, event.Error, ColorReset)
			return event.Error
		case interfaces.StreamEventMessageStop:
			if inThinking {
				fmt.Printf("%s\n%s%s\n", ColorReset, ColorGray, strings.Repeat("-", 40))
				fmt.Printf("%s📝 FINAL ANSWER:%s\n", ColorGreen, ColorReset)
			}
			fmt.Print("\n")
		}
	}

	fmt.Printf("%s[Stream completed]%s\n", ColorGreen, ColorReset)
	return nil
}

// agentStreaming demonstrates streaming from an agent
func agentStreaming(ctx context.Context, provider string) error {
	llm, err := createLLM(provider)
	if err != nil {
		return fmt.Errorf("failed to create LLM: %w", err)
	}

	// Create memory store
	memoryStore := memory.NewConversationBuffer()

	// Create agent with streaming-capable LLM
	agentInstance, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(memoryStore),
		agent.WithSystemPrompt("You are an expert in explaining complex topics. Use step-by-step reasoning."),
		agent.WithName("StreamingTeacher"),
	)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Check if agent supports streaming
	streamingAgent, ok := interface{}(agentInstance).(interfaces.StreamingAgent)
	if !ok {
		return fmt.Errorf("agent does not support streaming")
	}

	// Add organization context
	ctx = multitenancy.WithOrgID(ctx, "streaming-example")

	fmt.Println("Starting agent streaming...")
	eventChan, err := streamingAgent.RunStream(ctx, "How does machine learning work? Give me a step-by-step explanation.")
	if err != nil {
		return fmt.Errorf("failed to start agent streaming: %w", err)
	}

	// Process agent streaming events
	fmt.Print("Agent Response: ")
	var inThinking bool
	
	for event := range eventChan {
		switch event.Type {
		case interfaces.AgentEventContent:
			content := event.Content
			
			// Check for thinking tags in content
			if strings.Contains(content, "<thinking>") {
				parts := strings.Split(content, "<thinking>")
				fmt.Printf("%s%s%s", ColorWhite, parts[0], ColorReset)
				if len(parts) > 1 {
					fmt.Printf("\n%s💭 THINKING PROCESS:%s\n", ColorGray, ColorReset)
					fmt.Printf("%s%s%s\n", ColorGray, strings.Repeat("-", 40), ColorReset)
					fmt.Printf("%s%s", ColorGray, parts[1])
					inThinking = true
				}
			} else if strings.Contains(content, "</thinking>") {
				parts := strings.Split(content, "</thinking>")
				fmt.Printf("%s%s", ColorGray, parts[0])
				fmt.Printf("\n%s%s%s\n", ColorGray, strings.Repeat("-", 40), ColorReset)
				fmt.Printf("%s📝 FINAL ANSWER:%s\n", ColorGreen, ColorReset)
				if len(parts) > 1 {
					fmt.Printf("%s%s%s", ColorWhite, parts[1], ColorReset)
				}
				inThinking = false
			} else {
				// Regular content - color based on whether we're in thinking mode
				if inThinking {
					fmt.Printf("%s%s", ColorGray, content)
				} else {
					fmt.Printf("%s%s%s", ColorWhite, content, ColorReset)
				}
			}
		case interfaces.AgentEventThinking:
			fmt.Printf("\n%s🤔 [Thinking: %s]%s\n", ColorGray, event.ThinkingStep, ColorReset)
		case interfaces.AgentEventToolCall:
			if event.ToolCall != nil {
				fmt.Printf("\n%s🔧 [Tool Call: %s - Status: %s]%s\n", ColorYellow, event.ToolCall.Name, event.ToolCall.Status, ColorReset)
			}
		case interfaces.AgentEventToolResult:
			if event.ToolCall != nil {
				fmt.Printf("%s🔧 [Tool Result: %s]%s\n", ColorYellow, event.ToolCall.Result, ColorReset)
			}
		case interfaces.AgentEventError:
			fmt.Printf("\n%s❌ [Error: %v]%s\n", ColorRed, event.Error, ColorReset)
			return event.Error
		case interfaces.AgentEventComplete:
			if inThinking {
				fmt.Printf("%s\n%s%s\n", ColorReset, ColorGray, strings.Repeat("-", 40))
				fmt.Printf("%s📝 FINAL ANSWER:%s\n", ColorGreen, ColorReset)
			}
			fmt.Print("\n")
		}
	}

	fmt.Printf("%s[Agent streaming completed]%s\n", ColorGreen, ColorReset)
	return nil
}

// streamingWithTools demonstrates streaming with tool usage
func streamingWithTools(ctx context.Context, provider string) error {
	llm, err := createLLM(provider)
	if err != nil {
		return fmt.Errorf("failed to create LLM: %w", err)
	}

	// Create calculator tool
	calculatorTool := calculator.New()

	// Create agent with tools
	agentInstance, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithTools(calculatorTool),
		agent.WithSystemPrompt("You are a math tutor. Use the calculator tool for any calculations."),
		agent.WithName("MathTutor"),
	)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	streamingAgent, ok := interface{}(agentInstance).(interfaces.StreamingAgent)
	if !ok {
		return fmt.Errorf("agent does not support streaming")
	}

	ctx = multitenancy.WithOrgID(ctx, "streaming-example")

	fmt.Println("Starting streaming with tools...")
	eventChan, err := streamingAgent.RunStream(ctx, "Calculate the compound interest for $1000 at 5% annual rate for 10 years, compounded quarterly. Show your work step by step.")
	if err != nil {
		return fmt.Errorf("failed to start streaming: %w", err)
	}

	// Process events with tool tracking
	fmt.Print("Response: ")
	toolCount := 0
	for event := range eventChan {
		switch event.Type {
		case interfaces.AgentEventContent:
			fmt.Print(event.Content)
		case interfaces.AgentEventThinking:
			fmt.Printf("\n🤔 [Thinking: %s]\n", event.ThinkingStep)
		case interfaces.AgentEventToolCall:
			if event.ToolCall != nil {
				toolCount++
				fmt.Printf("\n🔧 [Tool Call #%d: %s]\n", toolCount, event.ToolCall.Name)
				fmt.Printf("   Arguments: %s\n", event.ToolCall.Arguments)
				fmt.Printf("   Status: %s\n", event.ToolCall.Status)
			}
		case interfaces.AgentEventToolResult:
			if event.ToolCall != nil {
				fmt.Printf("✅ [Tool Result: %s]\n", event.ToolCall.Result)
			}
		case interfaces.AgentEventError:
			fmt.Printf("\n❌ [Error: %v]\n", event.Error)
			return event.Error
		case interfaces.AgentEventComplete:
			fmt.Print("\n")
		}
	}

	fmt.Printf("[Streaming with tools completed - %d tools used]\n", toolCount)
	return nil
}

// advancedStreamingFeatures demonstrates advanced streaming features
func advancedStreamingFeatures(ctx context.Context, provider string) error {
	llm, err := createLLM(provider)
	if err != nil {
		return fmt.Errorf("failed to create LLM: %w", err)
	}

	streamingLLM := llm.(interfaces.StreamingLLM)

	// Configure custom streaming options
	streamConfig := interfaces.StreamConfig{
		BufferSize:          200, // Larger buffer for high-throughput scenarios
		IncludeThinking:     true,
		IncludeToolProgress: true,
	}

	fmt.Println("Starting advanced streaming with custom configuration...")

	var prompt string
	var options []interfaces.GenerateOption

	// Different prompts based on provider to showcase their unique features
	switch provider {
	case "openai":
		prompt = "Using o1-style reasoning, solve this step by step: If a car travels 60 km/h for 2 hours, then 80 km/h for 1.5 hours, what's the average speed?"
		options = []interfaces.GenerateOption{
			interfaces.WithStreamConfig(streamConfig),
			func(opts *interfaces.GenerateOptions) {
				opts.SystemMessage = "You are a math expert. Show your reasoning process."
			},
		}
	case "anthropic":
		prompt = "Think through this problem step by step: What are the implications of quantum entanglement for information transfer?"
		options = []interfaces.GenerateOption{
			interfaces.WithStreamConfig(streamConfig),
			func(opts *interfaces.GenerateOptions) {
				opts.SystemMessage = "You are a quantum physics expert. Think through problems methodically."
			},
		}
	}

	// Start streaming with custom config
	eventChan, err := streamingLLM.GenerateStream(ctx, prompt, options...)
	if err != nil {
		return fmt.Errorf("failed to start advanced streaming: %w", err)
	}

	// Advanced event processing with metrics
	startTime := time.Now()
	eventCount := 0
	contentLength := 0
	thinkingEvents := 0

	fmt.Print("Response: ")
	var inThinking bool
	
	for event := range eventChan {
		eventCount++

		switch event.Type {
		case interfaces.StreamEventMessageStart:
			fmt.Printf("\n%s[Stream started at %s]%s\n", ColorBlue, event.Timestamp.Format("15:04:05.000"), ColorReset)
		case interfaces.StreamEventContentDelta:
			content := event.Content
			contentLength += len(content)
			
			// Check for thinking tags in content
			if strings.Contains(content, "<thinking>") {
				parts := strings.Split(content, "<thinking>")
				fmt.Printf("%s%s%s", ColorWhite, parts[0], ColorReset)
				if len(parts) > 1 {
					fmt.Printf("\n%s💭 THINKING PROCESS:%s\n", ColorGray, ColorReset)
					fmt.Printf("%s%s%s\n", ColorGray, strings.Repeat("-", 40), ColorReset)
					fmt.Printf("%s%s", ColorGray, parts[1])
					inThinking = true
					thinkingEvents++
				}
			} else if strings.Contains(content, "</thinking>") {
				parts := strings.Split(content, "</thinking>")
				fmt.Printf("%s%s", ColorGray, parts[0])
				fmt.Printf("\n%s%s%s\n", ColorGray, strings.Repeat("-", 40), ColorReset)
				fmt.Printf("%s📝 FINAL ANSWER:%s\n", ColorGreen, ColorReset)
				if len(parts) > 1 {
					fmt.Printf("%s%s%s", ColorWhite, parts[1], ColorReset)
				}
				inThinking = false
			} else {
				// Regular content - color based on whether we're in thinking mode
				if inThinking {
					fmt.Printf("%s%s", ColorGray, content)
				} else {
					fmt.Printf("%s%s%s", ColorWhite, content, ColorReset)
				}
			}
		case interfaces.StreamEventThinking:
			thinkingEvents++
			fmt.Printf("\n%s🧠 [Thinking #%d: %s]%s\n", ColorGray, thinkingEvents, event.Content, ColorReset)
		case interfaces.StreamEventError:
			fmt.Printf("\n%s❌ [Error: %v]%s\n", ColorRed, event.Error, ColorReset)
			return event.Error
		case interfaces.StreamEventMessageStop:
			if inThinking {
				fmt.Printf("%s\n%s%s\n", ColorReset, ColorGray, strings.Repeat("-", 40))
				fmt.Printf("%s📝 FINAL ANSWER:%s\n", ColorGreen, ColorReset)
			}
			fmt.Print("\n")
			duration := time.Since(startTime)
			fmt.Printf("%s[Stream completed - Duration: %v, Events: %d, Content: %d chars, Thinking events: %d]%s\n",
				ColorGreen, duration, eventCount, contentLength, thinkingEvents, ColorReset)
		}

		// Show metadata if available
		if len(event.Metadata) > 0 {
			for key, value := range event.Metadata {
				if key == "usage" {
					fmt.Printf("%s📊 [Usage info: %v]%s\n", ColorBlue, value, ColorReset)
				}
			}
		}
	}

	fmt.Printf("%s[Advanced streaming completed]%s\n", ColorGreen, ColorReset)
	return nil
}

// Helper functions

func createLLM(provider string) (interfaces.LLM, error) {
	switch provider {
	case "anthropic":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is required")
		}
		return anthropic.NewClient(
			apiKey,
			anthropic.WithModel(anthropic.Claude37Sonnet),
		), nil

	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY environment variable is required")
		}
		return openai.NewClient(
			apiKey,
			openai.WithModel("gpt-4o"),
		), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// realReasoningDemo demonstrates real LLM reasoning tokens with colored output
func realReasoningDemo(ctx context.Context, provider string) error {
	llm, err := createLLM(provider)
	if err != nil {
		return fmt.Errorf("failed to create LLM: %w", err)
	}

	// Check if LLM supports streaming
	if !llm.SupportsStreaming() {
		return fmt.Errorf("LLM %s does not support streaming", llm.Name())
	}

	streamingLLM := llm.(interfaces.StreamingLLM)

	// Use different prompts and models based on provider capabilities
	var prompt string
	var options []interfaces.GenerateOption
	
	switch provider {
	case "anthropic":
		prompt = "Solve this step-by-step: If a car travels 120 km in 2 hours for the first part of a journey, then 200 km in 2.5 hours for the second part, what is the average speed for the entire journey?"
		options = []interfaces.GenerateOption{
			interfaces.WithReasoning(true, 20000), // Enable reasoning with 20k token budget
			func(opts *interfaces.GenerateOptions) {
				opts.SystemMessage = "You are a math tutor. Solve the problem systematically."
			},
		}
		
		// Check if the current model supports thinking tokens
		anthropicClient, _ := llm.(*anthropic.AnthropicClient)
		if anthropicClient != nil {
			modelSupportsThinking := anthropic.SupportsThinking(anthropicClient.Model)
			if modelSupportsThinking {
				fmt.Printf("%s✅ Using Anthropic native reasoning tokens (thinking) - Model: %s%s\n", ColorGreen, anthropicClient.Model, ColorReset)
			} else {
				fmt.Printf("%s⚠️  Current model (%s) does not support thinking tokens%s\n", ColorYellow, anthropicClient.Model, ColorReset)
				fmt.Printf("%sℹ️  Supported models: claude-3-7-sonnet-20250219, claude-sonnet-4-20250514, claude-opus-4-20250514%s\n", ColorBlue, ColorReset)
				fmt.Printf("%s💡 To test thinking tokens, create client with: anthropic.WithModel(anthropic.Claude37Sonnet)%s\n", ColorBlue, ColorReset)
			}
		} else {
			fmt.Printf("%sUsing Anthropic native reasoning tokens (thinking)%s\n", ColorBlue, ColorReset)
		}
	case "openai":
		prompt = "Solve this step-by-step: If a car travels 120 km in 2 hours for the first part of a journey, then 200 km in 2.5 hours for the second part, what is the average speed for the entire journey?"
		options = []interfaces.GenerateOption{
			interfaces.WithReasoning(true), // Enable reasoning detection
			func(opts *interfaces.GenerateOptions) {
				opts.SystemMessage = "You are a math tutor. Solve the problem systematically."
			},
		}
		// Check if it's an o1 model - we need to create a new client to check the model
		openaiClient, _ := llm.(*openai.OpenAIClient)
		if openaiClient != nil && strings.HasPrefix(openaiClient.Model, "o1-") {
			fmt.Printf("%sUsing OpenAI o1 model with built-in reasoning (not exposed in stream)%s\n", ColorBlue, ColorReset)
		} else {
			fmt.Printf("%sUsing OpenAI model (reasoning tokens not available for this model)%s\n", ColorBlue, ColorReset)
		}
	}

	fmt.Printf("%sPrompt: %s%s\n", ColorBlue, prompt, ColorReset)
	fmt.Println()

	eventChan, err := streamingLLM.GenerateStream(ctx, prompt, options...)
	if err != nil {
		return fmt.Errorf("failed to start reasoning demo: %w", err)
	}

	// Process streaming events with proper thinking content accumulation
	fmt.Println("Response:")
	fmt.Println("=" + strings.Repeat("=", 60))
	
	var thinkingContent strings.Builder
	var inThinkingMode bool
	var thinkingBlockCount int
	
	for event := range eventChan {
		switch event.Type {
		case interfaces.StreamEventContentDelta:
			content := event.Content
			fmt.Printf("%s%s%s", ColorWhite, content, ColorReset)
		case interfaces.StreamEventThinking:
			if !inThinkingMode {
				// Starting new thinking block
				inThinkingMode = true
				thinkingBlockCount++
				thinkingContent.Reset()
				fmt.Printf("\n%s💭 THINKING BLOCK #%d:%s\n", ColorGray, thinkingBlockCount, ColorReset)
				fmt.Printf("%s%s%s\n", ColorGray, strings.Repeat("-", 50), ColorReset)
			}
			// Accumulate thinking content
			thinkingContent.WriteString(event.Content)
			fmt.Printf("%s%s", ColorGray, event.Content)
		case interfaces.StreamEventContentComplete:
			if inThinkingMode {
				// End of thinking block
				fmt.Printf("%s\n%s%s\n", ColorReset, ColorGray, strings.Repeat("-", 50))
				inThinkingMode = false
			}
		case interfaces.StreamEventError:
			fmt.Printf("\n%s❌ [Error: %v]%s\n", ColorRed, event.Error, ColorReset)
			return event.Error
		case interfaces.StreamEventMessageStop:
			if inThinkingMode {
				// Close any open thinking block
				fmt.Printf("%s\n%s%s\n", ColorReset, ColorGray, strings.Repeat("-", 50))
			}
			fmt.Print("\n")
			if thinkingBlockCount > 0 {
				fmt.Printf("%s✅ Completed with %d reasoning blocks%s\n", ColorGreen, thinkingBlockCount, ColorReset)
			} else {
				fmt.Printf("%sℹ️  No reasoning tokens detected (may not be supported by this model)%s\n", ColorYellow, ColorReset)
			}
		}
	}

	fmt.Println()
	fmt.Printf("%s✅ Real reasoning demonstration completed!%s\n", ColorGreen, ColorReset)
	return nil
}